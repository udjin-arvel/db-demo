package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func WaitHealthy(ctx context.Context, pg, my *sql.DB) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if pg.PingContext(ctx) == nil && my.PingContext(ctx) == nil {
				return nil
			}
		}
	}
}

func LoadPostgres(ctx context.Context, db *sql.DB, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	if _, err := db.ExecContext(ctx, PostgresDrop+PostgresCreate); err != nil {
		return fmt.Errorf("pg schema: %w", err)
	}
	if err := insertUsersPG(ctx, db, users); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = conn.Raw(func(driverConn interface{}) error {
		pgc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("postgres copy: expected *pgx.Conn, got %T", driverConn)
		}
		src := &pgEventsSource{nEvents: nEvents, batch: batch, nUsers: nUsers}
		for {
			n, has := src.nextBatchSize()
			if !has {
				return nil
			}
			src.buf = src.buf[:n]
			GenEventsBatch(src.buf, src.nextStartID, nUsers, rng)
			count, err := pgc.CopyFrom(ctx, pgx.Identifier{"events"}, []string{"id", "user_id", "event_type", "created_at", "value", "payload"}, src)
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
	})
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, IndexPostgres); err != nil {
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

func insertUsersPG(ctx context.Context, db *sql.DB, users []UserRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO users (id, region, segment) VALUES ($1,$2,$3)`)
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

func LoadMySQL(ctx context.Context, db *sql.DB, users []UserRow, nEvents, batch, nUsers int64, rng *rand.Rand) error {
	if _, err := db.ExecContext(ctx, MySQLDrop+MySQLCreate); err != nil {
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
	if _, err := db.ExecContext(ctx, IndexMySQL); err != nil {
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
	if _, err := db.ExecContext(ctx, SQLiteDrop+SQLiteCreate); err != nil {
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
	if _, err := db.ExecContext(ctx, IndexSQLite); err != nil {
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
