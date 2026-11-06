package db

import (
	"database/sql"
)

// Pool wraps *sql.DB with optional label for logs.
type Pool struct {
	DB *sql.DB
}
