package db

import (
	"context"
	"database/sql"
	"fmt"
)

type TableStatus struct {
	Events int    `json:"events"`
	Users  int    `json:"users"`
	SizeB  int64  `json:"size_bytes"`
	Err    string `json:"error,omitempty"`
}

func StatusPostgres(ctx context.Context, db *sql.DB) TableStatus {
	var st TableStatus
	ev, err := count(ctx, db, "events")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	us, err := count(ctx, db, "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	sz, err := pgTableBytes(ctx, db, "events", "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Events, st.Users, st.SizeB = ev, us, sz
	return st
}

func StatusMySQL(ctx context.Context, db *sql.DB) TableStatus {
	var st TableStatus
	ev, err := count(ctx, db, "events")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	us, err := count(ctx, db, "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	sz, err := mysqlTableBytes(ctx, db, "demo", "events", "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Events, st.Users, st.SizeB = ev, us, sz
	return st
}

func StatusSQLite(ctx context.Context, db *sql.DB) TableStatus {
	var st TableStatus
	ev, err := count(ctx, db, "events")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	us, err := count(ctx, db, "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	sz, err := sqlitePageBytes(ctx, db)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Events, st.Users, st.SizeB = ev, us, sz
	return st
}

func StatusClickHouse(ctx context.Context, db *sql.DB) TableStatus {
	var st TableStatus
	ev, err := count(ctx, db, "events")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	us, err := count(ctx, db, "users")
	if err != nil {
		st.Err = err.Error()
		return st
	}
	sz, err := chTableBytes(ctx, db)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Events, st.Users, st.SizeB = ev, us, sz
	return st
}

func chTableBytes(ctx context.Context, db *sql.DB) (int64, error) {
	var b sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(sum(bytes_on_disk), 0)
		FROM system.parts
		WHERE database = currentDatabase() AND table IN ('events', 'users') AND active
	`).Scan(&b)
	if err != nil {
		return 0, err
	}
	if b.Valid {
		return b.Int64, nil
	}
	return 0, nil
}

func count(ctx context.Context, db *sql.DB, table string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&n)
	return n, err
}

func pgTableBytes(ctx context.Context, db *sql.DB, tables ...string) (int64, error) {
	var total int64
	for _, t := range tables {
		var b sql.NullInt64
		rel := "public." + t
		err := db.QueryRowContext(ctx, `SELECT COALESCE(pg_total_relation_size($1::regclass), 0)`, rel).Scan(&b)
		if err != nil {
			return 0, err
		}
		if b.Valid {
			total += b.Int64
		}
	}
	return total, nil
}

func mysqlTableBytes(ctx context.Context, db *sql.DB, schema string, tables ...string) (int64, error) {
	var total int64
	for _, t := range tables {
		var dl, il sql.NullInt64
		err := db.QueryRowContext(ctx,
			`SELECT COALESCE(DATA_LENGTH,0), COALESCE(INDEX_LENGTH,0) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
			schema, t).Scan(&dl, &il)
		if err != nil {
			return 0, err
		}
		total += dl.Int64 + il.Int64
	}
	return total, nil
}

func sqlitePageBytes(ctx context.Context, db *sql.DB) (int64, error) {
	var pc, ps int64
	if err := db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pc); err != nil {
		return 0, err
	}
	if err := db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&ps); err != nil {
		return 0, err
	}
	return pc * ps, nil
}
