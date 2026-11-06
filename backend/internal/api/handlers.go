package api

import (
	"encoding/json"
	"net/http"

	"github.com/dbdemo/backend/internal/db"
	"github.com/dbdemo/backend/internal/queries"
)

// Handler holds DB pools for API methods.
type Handler struct {
	PG          *db.Pool
	MySQL       *db.Pool
	SQLite      *db.Pool
	ClickHouse  *db.Pool
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pg := db.StatusPostgres(ctx, h.PG.DB)
	my := db.StatusMySQL(ctx, h.MySQL.DB)
	sl := db.StatusSQLite(ctx, h.SQLite.DB)
	ch := db.StatusClickHouse(ctx, h.ClickHouse.DB)
	resp := map[string]any{
		"postgres":   statusJSON(pg),
		"mysql":      statusJSON(my),
		"sqlite":     statusJSON(sl),
		"clickhouse": statusJSON(ch),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func statusJSON(s db.TableStatus) any {
	if s.Err != "" {
		return map[string]string{"error": s.Err}
	}
	return map[string]any{
		"events":     s.Events,
		"users":      s.Users,
		"size_bytes": s.SizeB,
	}
}

func (h *Handler) Queries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(queries.List)
}

type runBody struct {
	DB      string `json:"db"`
	QueryID string `json:"query_id"`
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	var body runBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	var d queries.Dialect
	var pool *db.Pool
	switch body.DB {
	case "postgres":
		d, pool = queries.Postgres, h.PG
	case "mysql":
		d, pool = queries.MySQL, h.MySQL
	case "sqlite":
		d, pool = queries.SQLite, h.SQLite
	case "clickhouse":
		d, pool = queries.ClickHouse, h.ClickHouse
	default:
		http.Error(w, `{"error":"invalid db"}`, http.StatusBadRequest)
		return
	}
	if body.QueryID == "" {
		http.Error(w, `{"error":"missing query_id"}`, http.StatusBadRequest)
		return
	}
	valid := false
	for _, q := range queries.List {
		if q.ID == body.QueryID {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, `{"error":"invalid query_id"}`, http.StatusBadRequest)
		return
	}
	res := db.RunQuery(r.Context(), pool.DB, d, body.QueryID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}
