package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/dbdemo/backend/internal/queries"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const stmtTimeout = 30 * time.Second
const runs = 3

type RunResult struct {
	LatencyMS   float64        `json:"latency_ms"`
	Metric      string         `json:"metric"`
	RowsPreview []RowMap       `json:"rows_preview"`
	Status      string         `json:"status"`
	Error       string         `json:"error,omitempty"`
}

// RowMap is JSON object for preview rows (numbers decode as numbers in TS).
type RowMap map[string]interface{}

// RunQuery executes whitelist query with 3 timed runs (median ms), separate metric query.
func RunQuery(ctx context.Context, db *sql.DB, d queries.Dialect, qid string) RunResult {
	sqlStr, args := queries.SQLAndArgs(qid, d)
	if sqlStr == "" {
		return RunResult{Status: "error", Error: "unknown query_id", Metric: "—", RowsPreview: []RowMap{}}
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return RunResult{Status: "error", Error: err.Error(), Metric: "—", RowsPreview: []RowMap{}}
	}
	defer conn.Close()

	var preview []RowMap
	var latencies []float64
	type chTimed struct {
		ms   float64
		qtid string
	}
	var chRuns []chTimed
	for i := 0; i < runs; i++ {
		var pv []RowMap
		takePreview := i == 0
		runQID := ""
		if d == queries.ClickHouse {
			runQID = uuid.NewString()
		}
		ms, pv, err := timeRun(ctx, conn, sqlStr, args, takePreview, runQID)
		if takePreview {
			preview = pv
		}
		if err != nil {
			if isTimeout(err) {
				if preview == nil {
					preview = []RowMap{}
				}
				return RunResult{LatencyMS: 30000, Metric: "—", RowsPreview: preview, Status: "timeout"}
			}
			return failResult(err)
		}
		if d == queries.ClickHouse {
			chRuns = append(chRuns, chTimed{ms: ms, qtid: runQID})
		} else {
			latencies = append(latencies, ms)
		}
	}
	if preview == nil {
		preview = []RowMap{}
	}
	var median float64
	metric := "—"
	switch d {
	case queries.Postgres:
		sort.Float64s(latencies)
		median = latencies[len(latencies)/2]
		metric = postgresMetric(ctx, conn, qid)
	case queries.MySQL:
		sort.Float64s(latencies)
		median = latencies[len(latencies)/2]
		metric = mysqlMetric(ctx, conn, qid, d)
	case queries.SQLite:
		sort.Float64s(latencies)
		median = latencies[len(latencies)/2]
		metric = sqliteMetric(ctx, conn, qid, d)
	case queries.ClickHouse:
		sort.Slice(chRuns, func(i, j int) bool { return chRuns[i].ms < chRuns[j].ms })
		mid := chRuns[len(chRuns)/2]
		median = mid.ms
		metric = clickhouseMetric(ctx, conn, mid.qtid)
	}

	return RunResult{
		LatencyMS:   median,
		Metric:      metric,
		RowsPreview: preview,
		Status:      "ok",
	}
}

func failResult(err error) RunResult {
	return RunResult{LatencyMS: 0, Metric: "—", RowsPreview: []RowMap{}, Status: "error", Error: err.Error()}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context deadline exceeded") || strings.Contains(s, "canceling statement")
}

func timeRun(ctx context.Context, conn *sql.Conn, sqlStr string, args []any, collectPreview bool, chQueryID string) (float64, []RowMap, error) {
	cctx, cancel := context.WithTimeout(ctx, stmtTimeout)
	defer cancel()
	if chQueryID != "" {
		cctx = clickhouse.Context(cctx, clickhouse.WithQueryID(chQueryID))
	}
	t0 := time.Now()
	rows, err := conn.QueryContext(cctx, sqlStr, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, nil, err
	}
	var preview []RowMap
	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return 0, preview, err
		}
		if collectPreview && len(preview) < 10 {
			m := make(RowMap, len(cols))
			for i, c := range cols {
				m[c] = cellJSON(raw[i])
			}
			preview = append(preview, m)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, preview, err
	}
	ms := float64(time.Since(t0).Milliseconds())
	if ms < 0.01 {
		ms = float64(time.Since(t0).Microseconds()) / 1000
	}
	return ms, preview, nil
}

func cellJSON(v interface{}) interface{} {
	switch x := v.(type) {
	case nil:
		return nil
	case decimal.Decimal:
		f, exact := x.Float64()
		if exact {
			return f
		}
		s := x.String()
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	case []byte:
		s := string(x)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case float32:
		return float64(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return int64(x)
		}
		return x
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case uint64:
		if x <= math.MaxInt64 {
			return int64(x)
		}
		return x
	case uint32:
		return int64(x)
	case uint16:
		return int64(x)
	case uint8:
		return int64(x)
	default:
		return x
	}
}

func clickhouseMetric(ctx context.Context, conn *sql.Conn, queryID string) string {
	if queryID == "" {
		return "—"
	}
	fctx, cancel := context.WithTimeout(ctx, stmtTimeout)
	defer cancel()
	if _, err := conn.ExecContext(fctx, `SYSTEM FLUSH LOGS`); err != nil {
		return "—"
	}
	qctx, qcancel := context.WithTimeout(ctx, stmtTimeout)
	defer qcancel()
	var readRows, readBytes uint64
	err := conn.QueryRowContext(qctx, `
		SELECT read_rows, read_bytes
		FROM system.query_log
		WHERE type = 'QueryFinish' AND query_id = ?
		ORDER BY event_time DESC
		LIMIT 1
	`, queryID).Scan(&readRows, &readBytes)
	if err != nil {
		return "—"
	}
	return fmt.Sprintf("read_rows=%s, read_bytes=%s", formatHumanUInt64(readRows), formatBytesHuman(readBytes))
}

func formatHumanUInt64(n uint64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	var suf string
	v := float64(n)
	switch {
	case n >= 1_000_000_000:
		suf = "G"
		v /= 1e9
	case n >= 1_000_000:
		suf = "M"
		v /= 1e6
	case n >= 1_000:
		suf = "k"
		v /= 1e3
	default:
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1f%s", v, suf)
}

func formatBytesHuman(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for n2 := n / unit; n2 >= unit && exp < 4; n2 /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n) / float64(div)
	return fmt.Sprintf("%.1f %s", v, units[exp])
}

func postgresMetric(ctx context.Context, conn *sql.Conn, qid string) string {
	exsql, args := queries.ExplainMetricSQL(qid, queries.Postgres)
	if exsql == "" {
		return "—"
	}
	cctx, cancel := context.WithTimeout(ctx, stmtTimeout)
	defer cancel()
	rows, err := conn.QueryContext(cctx, exsql, args...)
	if err != nil {
		return "—"
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var planLine sql.NullString
		if err := rows.Scan(&planLine); err != nil {
			return "—"
		}
		if planLine.Valid {
			lines = append(lines, planLine.String)
		}
	}
	text := strings.Join(lines, "\n")
	return parsePGBuffers(text)
}

func parsePGBuffers(explain string) string {
	var sharedRead, sharedHit int
	for _, line := range strings.Split(explain, "\n") {
		if idx := strings.Index(line, "shared read="); idx >= 0 {
			fmt.Sscanf(line[idx:], "shared read=%d", &sharedRead)
		}
		if idx := strings.Index(line, "shared hit="); idx >= 0 {
			fmt.Sscanf(line[idx:], "shared hit=%d", &sharedHit)
		}
	}
	if sharedRead == 0 && sharedHit == 0 {
		if len(explain) > 200 {
			return strings.TrimSpace(explain[:200]) + "…"
		}
		return strings.TrimSpace(explain)
	}
	return fmt.Sprintf("shared read=%d, hit=%d", sharedRead, sharedHit)
}

func mysqlMetric(ctx context.Context, conn *sql.Conn, qid string, d queries.Dialect) string {
	before, err := mysqlHandlerSnapshot(ctx, conn)
	if err != nil {
		return "—"
	}
	sqlStr, args := queries.SQLAndArgs(qid, d)
	cctx, cancel := context.WithTimeout(ctx, stmtTimeout)
	defer cancel()
	rows, err := conn.QueryContext(cctx, sqlStr, args...)
	if err != nil {
		return "—"
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "—"
	}
	after, err := mysqlHandlerSnapshot(ctx, conn)
	if err != nil {
		return "—"
	}
	var parts []string
	for k, av := range after {
		bv := before[k]
		delta := av - bv
		if delta != 0 {
			parts = append(parts, fmt.Sprintf("%s=%s", k, formatBig(delta)))
		}
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

func mysqlHandlerSnapshot(ctx context.Context, conn *sql.Conn) (map[string]int64, error) {
	rows, err := conn.QueryContext(ctx, `SHOW SESSION STATUS LIKE 'Handler_read_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int64)
	for rows.Next() {
		var name string
		var val string
		if err := rows.Scan(&name, &val); err != nil {
			return nil, err
		}
		var n int64
		fmt.Sscanf(val, "%d", &n)
		m[name] = n
	}
	return m, rows.Err()
}

func formatBig(n int64) string {
	sign := ""
	u := uint64(n)
	if n < 0 {
		sign = "-"
		u = uint64(-n)
	}
	s := fmt.Sprintf("%d", u)
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.WriteString(sign)
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(",")
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func sqliteMetric(ctx context.Context, conn *sql.Conn, qid string, d queries.Dialect) string {
	exsql, args := queries.ExplainMetricSQL(qid, d)
	if exsql == "" {
		return "—"
	}
	cctx, cancel := context.WithTimeout(ctx, stmtTimeout)
	defer cancel()
	rows, err := conn.QueryContext(cctx, exsql, args...)
	if err != nil {
		return "—"
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		raw := make([]interface{}, 4)
		ptrs := []interface{}{&raw[0], &raw[1], &raw[2], &raw[3]}
		if err := rows.Scan(ptrs...); err != nil {
			return "—"
		}
		line := fmt.Sprintf("%v|%v|%v", raw[1], raw[2], raw[3])
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "; ")
}

