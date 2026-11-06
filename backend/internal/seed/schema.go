package seed

const (
	PostgresDrop = `
DROP TABLE IF EXISTS events CASCADE;
DROP TABLE IF EXISTS users CASCADE;
`
	PostgresCreate = `
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
);
CREATE TABLE events (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  event_type VARCHAR(20) NOT NULL,
  created_at TIMESTAMP NOT NULL,
  value NUMERIC(12,2) NOT NULL,
  payload TEXT
);
`
	MySQLDrop = `
SET FOREIGN_KEY_CHECKS=0;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS users;
SET FOREIGN_KEY_CHECKS=1;
`
	MySQLCreate = `
CREATE TABLE users (
  id INT PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
) ENGINE=InnoDB;
CREATE TABLE events (
  id INT PRIMARY KEY,
  user_id INT NOT NULL,
  event_type VARCHAR(20) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  value DECIMAL(12,2) NOT NULL,
  payload TEXT,
  FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB;
`
	SQLiteDrop = `
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS users;
`
	SQLiteCreate = `
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  region VARCHAR(20) NOT NULL,
  segment VARCHAR(20) NOT NULL
);
CREATE TABLE events (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  event_type VARCHAR(20) NOT NULL,
  created_at TEXT NOT NULL,
  value NUMERIC(12,2) NOT NULL,
  payload TEXT
);
`
)

const IndexPostgres = `
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);
CREATE INDEX IF NOT EXISTS idx_users_segment ON users(segment);
`
const IndexMySQL = `
CREATE INDEX idx_events_created_at ON events(created_at);
CREATE INDEX idx_events_user_id ON events(user_id);
CREATE INDEX idx_users_segment ON users(segment);
`
const IndexSQLite = `
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);
CREATE INDEX IF NOT EXISTS idx_users_segment ON users(segment);
`
