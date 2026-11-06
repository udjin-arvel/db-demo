package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/dbdemo/backend/internal/config"
	dbpkg "github.com/dbdemo/backend/internal/db"
	"github.com/dbdemo/backend/internal/seed"
	"github.com/jackc/pgx/v5"
)

func main() {
	var events, users, batch int64
	flag.Int64Var(&events, "events", 3_000_000, "number of events")
	flag.Int64Var(&users, "users", 50_000, "number of users")
	flag.Int64Var(&batch, "batch-size", 10_000, "batch size for bulk insert")
	flag.Parse()

	pgDSN, mysqlDSN, sqlitePath, chDSN, _ := config.FromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	pgConn, err := pgx.Connect(ctx, pgDSN)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pgConn.Close(ctx)

	my, err := dbpkg.OpenMySQL(mysqlDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer my.Close()

	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	sl, err := dbpkg.OpenSQLite(sqlitePath)
	if err != nil {
		log.Fatalf("sqlite: %v", err)
	}
	defer sl.Close()

	chDB, err := dbpkg.OpenClickHouse(chDSN)
	if err != nil {
		log.Fatalf("clickhouse: %v", err)
	}
	defer chDB.Close()

	log.Println("waiting for postgres, mysql, and clickhouse...")
	if err := seed.WaitHealthy(ctx, pgConn, my, chDB); err != nil {
		log.Fatalf("health: %v", err)
	}

	userRows := seed.GenerateUsers(users)

	log.Println("loading postgres...")
	rng := rand.New(rand.NewSource(42))
	if err := seed.LoadPostgres(ctx, pgConn, userRows, events, batch, users, rng); err != nil {
		log.Fatalf("postgres load: %v", err)
	}

	log.Println("loading mysql...")
	rng = rand.New(rand.NewSource(42))
	if err := seed.LoadMySQL(ctx, my, userRows, events, batch, users, rng); err != nil {
		log.Fatalf("mysql load: %v", err)
	}

	log.Println("loading sqlite...")
	rng = rand.New(rand.NewSource(42))
	if err := seed.LoadSQLite(ctx, sl, userRows, events, batch, users, rng); err != nil {
		log.Fatalf("sqlite load: %v", err)
	}

	log.Println("loading clickhouse...")
	rng = rand.New(rand.NewSource(42))
	if err := seed.LoadClickHouse(ctx, chDSN, userRows, events, batch, users, rng); err != nil {
		log.Fatalf("clickhouse load: %v", err)
	}

	pe, pu, err := seed.CountsPostgres(ctx, pgConn)
	if err != nil {
		log.Printf("postgres count: %v", err)
	} else {
		log.Printf("postgres: events=%d users=%d", pe, pu)
	}
	for name, db := range map[string]*dbpkg.Pool{
		"mysql":      {DB: my},
		"sqlite":     {DB: sl},
		"clickhouse": {DB: chDB},
	} {
		ev, us, err := seed.Counts(ctx, db.DB)
		if err != nil {
			log.Printf("%s count: %v", name, err)
			continue
		}
		log.Printf("%s: events=%d users=%d", name, ev, us)
	}
	fmt.Println("seed done")
}
