package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/dbdemo/backend/internal/api"
	"github.com/dbdemo/backend/internal/config"
	"github.com/dbdemo/backend/internal/db"
)

func main() {
	pgDSN, mysqlDSN, sqlitePath, chDSN, listen := config.FromEnv()

	pg, err := db.OpenPostgres(pgDSN)
	if err != nil {
		log.Fatalf("postgres open: %v", err)
	}
	defer pg.Close()

	my, err := db.OpenMySQL(mysqlDSN)
	if err != nil {
		log.Fatalf("mysql open: %v", err)
	}
	defer my.Close()

	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
		log.Fatalf("mkdir sqlite dir: %v", err)
	}
	sl, err := db.OpenSQLite(sqlitePath)
	if err != nil {
		log.Fatalf("sqlite open: %v", err)
	}
	defer sl.Close()

	ch, err := db.OpenClickHouse(chDSN)
	if err != nil {
		log.Fatalf("clickhouse open: %v", err)
	}
	defer ch.Close()

	h := &api.Handler{
		PG:         &db.Pool{DB: pg},
		MySQL:      &db.Pool{DB: my},
		SQLite:     &db.Pool{DB: sl},
		ClickHouse: &db.Pool{DB: ch},
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:5173", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/api/status", h.Status)
	r.Get("/api/queries", h.Queries)
	r.Post("/api/run", h.Run)

	srv := &http.Server{Addr: listen, Handler: r}

	go func() {
		log.Printf("listening %s", listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
