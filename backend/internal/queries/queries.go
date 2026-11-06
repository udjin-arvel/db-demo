package queries

// Info matches GET /api/queries and client demo data.
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SQLPreview  string `json:"sql_preview"`
}

var List = []Info{
	{ID: "q1", Name: "agg_by_type", Description: "Полный скан + агрегация", SQLPreview: "SELECT event_type, COUNT(*), SUM(value) FROM events GROUP BY event_type"},
	{ID: "q2", Name: "filter_by_time", Description: "Индекс по времени + агрегация", SQLPreview: "SELECT user_id, SUM(value) FROM events WHERE created_at BETWEEN ? AND ? GROUP BY user_id ORDER BY SUM(value) DESC LIMIT 20"},
	{ID: "q3", Name: "top_users", Description: "Сортировка + limit", SQLPreview: "SELECT user_id, COUNT(*) AS cnt FROM events GROUP BY user_id ORDER BY cnt DESC LIMIT 10"},
	{ID: "q4", Name: "join_regions", Description: "JOIN с большой таблицей", SQLPreview: "SELECT u.region, SUM(e.value) FROM events e JOIN users u ON e.user_id = u.id WHERE u.segment = 'premium' GROUP BY u.region"},
	{ID: "q5", Name: "point_lookup", Description: "Контрольный: SQLite без сети", SQLPreview: "SELECT * FROM events WHERE id = ?"},
}

// Fixed params (same for all DBs); Q2 window and Q5 point lookup id.
var (
	Q2Start = "2024-06-01 00:00:00"
	Q2End   = "2024-07-01 00:00:00"
	Q5ID    = int64(2_500_000)
)

// Dialect-specific SQL and arguments for whitelist execution.

type Dialect int

const (
	Postgres Dialect = iota
	MySQL
	SQLite
	ClickHouse
)

func SQLAndArgs(qid string, d Dialect) (sql string, args []any) {
	switch qid {
	case "q1":
		switch d {
		case ClickHouse:
			sql = `SELECT event_type, count() AS count, sum(value) AS sum FROM events GROUP BY event_type`
		default:
			sql = `SELECT event_type, COUNT(*) AS count, SUM(value) AS sum FROM events GROUP BY event_type`
		}
		return sql, nil
	case "q2":
		switch d {
		case Postgres:
			sql = `SELECT user_id, SUM(value) AS sum FROM events WHERE created_at BETWEEN $1::timestamp AND $2::timestamp GROUP BY user_id ORDER BY SUM(value) DESC LIMIT 20`
			args = []any{Q2Start, Q2End}
		case ClickHouse:
			sql = `SELECT user_id, sum(value) AS sum FROM events WHERE created_at BETWEEN toDateTime(?) AND toDateTime(?) GROUP BY user_id ORDER BY sum(value) DESC LIMIT 20`
			args = []any{Q2Start, Q2End}
		default:
			sql = `SELECT user_id, SUM(value) AS sum FROM events WHERE created_at BETWEEN ? AND ? GROUP BY user_id ORDER BY SUM(value) DESC LIMIT 20`
			args = []any{Q2Start, Q2End}
		}
		return sql, args
	case "q3":
		sql = `SELECT user_id, COUNT(*) AS cnt FROM events GROUP BY user_id ORDER BY cnt DESC LIMIT 10`
		return sql, nil
	case "q4":
		sql = `SELECT u.region, SUM(e.value) AS sum FROM events e JOIN users u ON e.user_id = u.id WHERE u.segment = 'premium' GROUP BY u.region`
		return sql, nil
	case "q5":
		switch d {
		case Postgres:
			sql = `SELECT id, user_id, event_type, created_at, value, payload FROM events WHERE id = $1`
			args = []any{Q5ID}
		default:
			sql = `SELECT id, user_id, event_type, created_at, value, payload FROM events WHERE id = ?`
			args = []any{Q5ID}
		}
		return sql, args
	default:
		return "", nil
	}
}

// ExplainMetricSQL returns SQL whose result describes the "metric" for this engine (separate from timed runs).
func ExplainMetricSQL(qid string, d Dialect) (sql string, args []any) {
	qs, a := SQLAndArgs(qid, d)
	if qs == "" {
		return "", nil
	}
	switch d {
	case Postgres:
		return `EXPLAIN (ANALYZE, BUFFERS) ` + qs, a
	case MySQL:
		return `EXPLAIN ` + qs, a
	case SQLite:
		return `EXPLAIN QUERY PLAN ` + qs, a
	case ClickHouse:
		return "", nil
	default:
		return "", nil
	}
}
