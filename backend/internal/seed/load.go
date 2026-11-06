package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func execPgxStmts(ctx context.Context, conn *pgx.Conn, stmts []string) error {
	for _, q := range stmts {
		if _, err := conn.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func execSQLStmts(ctx context.Context, db *sql.DB, stmts []string) error {
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func WaitHealthy(ctx context.Context, pg *pgx.Conn, my, ch *sql.DB) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if pg.Ping(ctx) == nil && my.PingContext(ctx) == nil && ch.PingContext(ctx) == nil {
				return nil
			}
		}
	}
}

// LoadPostgres uses a native pgx connection so CopyFrom runs on *pgx.Conn (not database/sql).
func LoadPostgres(ctx context.Context, conn *pgx.Conn, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	if err := execPgxStmts(ctx, conn, PostgresDropStmts); err != nil {
		return fmt.Errorf("pg schema: %w", err)
	}
	if err := execPgxStmts(ctx, conn, PostgresCreateStmts); err != nil {
		return fmt.Errorf("pg schema: %w", err)
	}
	if err := insertUsersPG(ctx, conn, users); err != nil {
		return err
	}
	src := &pgEventsSource{nEvents: nEvents, batch: batch, nUsers: nUsers}
	for {
		n, has := src.nextBatchSize()
		if !has {
			break
		}
		src.buf = src.buf[:n]
		GenEventsBatch(src.buf, src.nextStartID, nUsers, rng)
		count, err := conn.CopyFrom(ctx, pgx.Identifier{"events"}, []string{"id", "user_id", "event_type", "created_at", "value", "payload"}, src)
		if err != nil {
			return fmt.Errorf("copy at id %d: %w", src.nextStartID, err)
		}
		if int64(count) != n {
			return fmt.Errorf("copy: expected %d rows, got %d", n, count)
		}
		src.done += n
		src.nextStartID += n
		if src.done%500000 == 0 || src.done == nEvents {
			log.Printf("postgres events: %d / %d", src.done, nEvents)
		}
	}
	if err := execPgxStmts(ctx, conn, IndexPostgresStmts); err != nil {
		return fmt.Errorf("pg index: %w", err)
	}
	return nil
}

type pgEventsSource struct {
	nEvents, batch, nUsers int64
	buf                    []EventRow
	done                   int64
	nextStartID            int64
	rowIdx                 int
}

func (s *pgEventsSource) nextBatchSize() (int64, bool) {
	if s.done >= s.nEvents {
		return 0, false
	}
	n := s.batch
	if n > s.nEvents-s.done {
		n = s.nEvents - s.done
	}
	if s.nextStartID == 0 {
		s.nextStartID = 1
	}
	if cap(s.buf) < int(n) {
		s.buf = make([]EventRow, n)
	}
	s.buf = s.buf[:n]
	s.rowIdx = 0
	return n, true
}

func (s *pgEventsSource) Next() bool {
	if s.rowIdx >= len(s.buf) {
		return false
	}
	s.rowIdx++
	return true
}

func (s *pgEventsSource) Values() ([]interface{}, error) {
	ev := s.buf[s.rowIdx-1]
	return []interface{}{ev.ID, ev.UserID, ev.EventType, ev.CreatedAt, ev.Value, ev.Payload}, nil
}

func (s *pgEventsSource) Err() error { return nil }

func insertUsersPG(ctx context.Context, conn *pgx.Conn, users []UserRow) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, region, segment) VALUES ($1,$2,$3)`, u.ID, u.Region, u.Segment); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}
	return tx.Commit(ctx)
}

func LoadMySQL(ctx context.Context, db *sql.DB, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	if err := execSQLStmts(ctx, db, MySQLSchemaStmts); err != nil {
		return fmt.Errorf("mysql schema: %w", err)
	}
	if err := insertUsersMySQL(ctx, db, users); err != nil {
		return err
	}
	const maxPlaceholders = 60000
	const cols = 6
	maxRows := int64(maxPlaceholders / cols)
	if batch > maxRows {
		batch = maxRows
	}
	buf := make([]EventRow, batch)
	var sb strings.Builder
	args := make([]interface{}, 0, batch*cols)
	var done int64
	for done < nEvents {
		n := batch
		if n > nEvents-done {
			n = nEvents - done
		}
		GenEventsBatch(buf[:n], done+1, nUsers, rng)
		sb.Reset()
		sb.WriteString(`INSERT INTO events (id, user_id, event_type, created_at, value, payload) VALUES `)
		args = args[:0]
		for i := 0; i < int(n); i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?,?,?,?)")
			ev := buf[i]
			args = append(args, ev.ID, ev.UserID, ev.EventType, ev.CreatedAt, ev.Value, ev.Payload)
		}
		if _, err := db.ExecContext(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("mysql insert batch at %d: %w", done, err)
		}
		done += n
		if done%500000 == 0 || done == nEvents {
			log.Printf("mysql events: %d / %d", done, nEvents)
		}
	}
	if err := execSQLStmts(ctx, db, IndexMySQLStmts); err != nil {
		return fmt.Errorf("mysql index: %w", err)
	}
	return nil
}

func insertUsersMySQL(ctx context.Context, db *sql.DB, users []UserRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO users (id, region, segment) VALUES (?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, u := range users {
		if _, err := stmt.ExecContext(ctx, u.ID, u.Region, u.Segment); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	return tx.Commit()
}

func LoadSQLite(ctx context.Context, db *sql.DB, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	if err := execSQLStmts(ctx, db, SQLiteDropStmts); err != nil {
		return fmt.Errorf("sqlite schema: %w", err)
	}
	if err := execSQLStmts(ctx, db, SQLiteCreateStmts); err != nil {
		return fmt.Errorf("sqlite schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA synchronous=OFF`); err != nil {
		return err
	}
	if err := insertUsersSQLite(ctx, db, users); err != nil {
		return err
	}
	buf := make([]EventRow, batch)
	var done int64
	for done < nEvents {
		n := batch
		if n > nEvents-done {
			n = nEvents - done
		}
		GenEventsBatch(buf[:n], done+1, nUsers, rng)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO events (id, user_id, event_type, created_at, value, payload) VALUES (?,?,?,?,?,?)`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		for i := 0; i < int(n); i++ {
			ev := buf[i]
			ts := ev.CreatedAt.UTC().Format("2006-01-02 15:04:05.000000")
			if _, err := stmt.ExecContext(ctx, ev.ID, ev.UserID, ev.EventType, ts, ev.Value, ev.Payload); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return err
			}
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
		done += n
		if done%500000 == 0 || done == nEvents {
			log.Printf("sqlite events: %d / %d", done, nEvents)
		}
	}
	if _, err := db.ExecContext(ctx, `PRAGMA synchronous=NORMAL`); err != nil {
		return err
	}
	if err := execSQLStmts(ctx, db, IndexSQLiteStmts); err != nil {
		return fmt.Errorf("sqlite index: %w", err)
	}
	if _, err := db.ExecContext(ctx, `ANALYZE`); err != nil {
		log.Printf("sqlite analyze: %v", err)
	}
	return nil
}

func insertUsersSQLite(ctx context.Context, db *sql.DB, users []UserRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO users (id, region, segment) VALUES (?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, u := range users {
		if _, err := stmt.ExecContext(ctx, u.ID, u.Region, u.Segment); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	return tx.Commit()
}

func Counts(ctx context.Context, db *sql.DB) (ev, us int, err error) {
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&ev)
	if err != nil {
		return
	}
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&us)
	return
}

// LoadClickHouse creates tables and loads data via the native clickhouse-go batch API.
func LoadClickHouse(ctx context.Context, dsn string, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	opt, err := ch.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("clickhouse dsn: %w", err)
	}
	conn, err := ch.Open(opt)
	if err != nil {
		return fmt.Errorf("clickhouse open: %w", err)
	}
	defer conn.Close()

	for _, q := range ClickHouseDropStmts {
		if err := conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("clickhouse schema: %w", err)
		}
	}
	for _, q := range ClickHouseCreateStmts {
		if err := conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("clickhouse schema: %w", err)
		}
	}

	ub, err := conn.PrepareBatch(ctx, "INSERT INTO users")
	if err != nil {
		return fmt.Errorf("clickhouse users batch: %w", err)
	}
	for _, u := range users {
		if err := ub.Append(u.ID, u.Region, u.Segment); err != nil {
			return fmt.Errorf("clickhouse users append: %w", err)
		}
	}
	if err := ub.Send(); err != nil {
		return fmt.Errorf("clickhouse users send: %w", err)
	}

	buf := make([]EventRow, batch)
	var done int64
	for done < nEvents {
		n := batch
		if n > nEvents-done {
			n = nEvents - done
		}
		GenEventsBatch(buf[:n], done+1, nUsers, rng)

		eb, err := conn.PrepareBatch(ctx, "INSERT INTO events")
		if err != nil {
			return fmt.Errorf("clickhouse events batch at %d: %w", done, err)
		}
		for i := 0; i < int(n); i++ {
			ev := buf[i]
			val, err := decimal.NewFromString(ev.Value)
			if err != nil {
				return fmt.Errorf("clickhouse decimal: %w", err)
			}
			if err := eb.Append(ev.ID, ev.UserID, ev.EventType, ev.CreatedAt, val, ev.Payload); err != nil {
				return fmt.Errorf("clickhouse events append: %w", err)
			}
		}
		if err := eb.Send(); err != nil {
			return fmt.Errorf("clickhouse events send at %d: %w", done, err)
		}
		done += n
		if done%500000 == 0 || done == nEvents {
			log.Printf("clickhouse events: %d / %d", done, nEvents)
		}
	}
	return nil
}

// CountsPostgres reads row counts via pgx.
func CountsPostgres(ctx context.Context, conn *pgx.Conn) (ev, us int, err error) {
	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM events`).Scan(&ev)
	if err != nil {
		return
	}
	err = conn.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&us)
	return
}
