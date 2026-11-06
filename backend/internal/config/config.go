package config

import (
	"os"
)

// FromEnv loads DSNs and paths. Defaults match docker-compose published ports.
func FromEnv() (pgDSN, mysqlDSN, sqlitePath, chDSN, listen string) {
	pgDSN = os.Getenv("PG_DSN")
	if pgDSN == "" {
		pgDSN = "postgres://demo:demo@127.0.0.1:5432/demo?sslmode=disable"
	}
	mysqlDSN = os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		mysqlDSN = "demo:demo@tcp(127.0.0.1:3306)/demo?parseTime=true&loc=UTC"
	}
	sqlitePath = os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = "./data/sqlite.db"
	}
	chDSN = os.Getenv("CLICKHOUSE_DSN")
	if chDSN == "" {
		chDSN = "clickhouse://default@127.0.0.1:9000/demo"
	}
	listen = os.Getenv("LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	return
}
