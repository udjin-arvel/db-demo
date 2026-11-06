package seed

// Use separate Exec calls per statement: MySQL driver disallows multiple statements
// in one query unless multiStatements=true in the DSN.

var PostgresDropStmts = []string{
	`DROP TABLE IF EXISTS events CASCADE`,
	`DROP TABLE IF EXISTS users CASCADE`,
}

var PostgresCreateStmts = []string{
	`CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
)`,
	`CREATE TABLE events (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  event_type VARCHAR(20) NOT NULL,
  created_at TIMESTAMP NOT NULL,
  value NUMERIC(12,2) NOT NULL,
  payload TEXT
)`,
}

var MySQLSchemaStmts = []string{
	`SET FOREIGN_KEY_CHECKS=0`,
	`DROP TABLE IF EXISTS events`,
	`DROP TABLE IF EXISTS users`,
	`SET FOREIGN_KEY_CHECKS=1`,
	`CREATE TABLE users (
  id INT PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
) ENGINE=InnoDB`,
	`CREATE TABLE events (
  id INT PRIMARY KEY,
  user_id INT NOT NULL,
  event_type VARCHAR(20) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  value DECIMAL(12,2) NOT NULL,
  payload TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB`,
}

var SQLiteDropStmts = []string{
	`DROP TABLE IF EXISTS events`,
	`DROP TABLE IF EXISTS users`,
}

var SQLiteCreateStmts = []string{
	`CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
)`,
	`CREATE TABLE events (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  event_type VARCHAR(20) NOT NULL,
  created_at TEXT NOT NULL,
  value NUMERIC(12,2) NOT NULL,
  payload TEXT
)`,
}

var IndexPostgresStmts = []string{
	`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_users_segment ON users(segment)`,
}

var IndexMySQLStmts = []string{
	`CREATE INDEX idx_events_created_at ON events(created_at)`,
	`CREATE INDEX idx_events_user_id ON events(user_id)`,
	`CREATE INDEX idx_users_segment ON users(segment)`,
}

var IndexSQLiteStmts = []string{
	`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_users_segment ON users(segment)`,
}

var ClickHouseDropStmts = []string{
	`DROP TABLE IF EXISTS events`,
	`DROP TABLE IF EXISTS users`,
}

var ClickHouseCreateStmts = []string{
	`CREATE TABLE users (
  id UInt64,
  region LowCardinality(String),
  segment LowCardinality(String)
) ENGINE = MergeTree ORDER BY id`,
	`CREATE TABLE events (
  id UInt64,
  user_id UInt64,
  event_type LowCardinality(String),
  created_at DateTime,
  value Decimal(12, 2),
  payload String
) ENGINE = MergeTree ORDER BY (event_type, created_at)`,
}
