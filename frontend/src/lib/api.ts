// Thin client for the Go backend described in the spec (/api/status, /api/queries, /api/run).
// Base URL comes from VITE_API_URL; when the backend is unreachable we fall back to demo data.

export type DbId = "postgres" | "mysql" | "sqlite" | "clickhouse";
export const DBS: { id: DbId; name: string; kind: string }[] = [
  { id: "postgres", name: "PostgreSQL", kind: "client-server" },
  { id: "mysql", name: "MySQL", kind: "client-server" },
  { id: "sqlite", name: "SQLite", kind: "embedded" },
  { id: "clickhouse", name: "ClickHouse", kind: "columnar OLAP" },
];

export type DbStatus = { events: number; users: number; size_bytes: number; error?: string };
export type StatusResponse = Partial<Record<DbId, DbStatus>>;
export type QueryInfo = { id: string; name: string; description: string; sql_preview: string };
export type RunResult = {
  latency_ms: number;
  metric: string;
  rows_preview: Record<string, unknown>[];
  status: "ok" | "timeout" | "error" | string;
  error?: string;
};

const BASE = (import.meta.env["VITE_API_URL"] as string | undefined)?.replace(/\/$/, "") ?? "";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), 35_000);
  try {
    const r = await fetch(`${BASE}${path}`, {
      ...init,
      signal: ctrl.signal,
      headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const ct = r.headers.get("content-type") ?? "";
    if (!ct.includes("json")) throw new Error("not json");
    return (await r.json()) as T;
  } finally {
    clearTimeout(t);
  }
}

// ---------- demo fallback ----------
const DEMO_QUERIES: QueryInfo[] = [
  { id: "q1", name: "agg_by_type", description: "Полный скан + агрегация", sql_preview: "SELECT event_type, COUNT(*), SUM(value) FROM events GROUP BY event_type" },
  { id: "q2", name: "filter_by_time", description: "Индекс по времени + агрегация", sql_preview: "SELECT user_id, SUM(value) FROM events WHERE created_at BETWEEN ? AND ? GROUP BY user_id ORDER BY SUM(value) DESC LIMIT 20" },
  { id: "q3", name: "top_users", description: "Сортировка + limit", sql_preview: "SELECT user_id, COUNT(*) AS cnt FROM events GROUP BY user_id ORDER BY cnt DESC LIMIT 10" },
  { id: "q4", name: "join_regions", description: "JOIN с большой таблицей", sql_preview: "SELECT u.region, SUM(e.value) FROM events e JOIN users u ON e.user_id = u.id WHERE u.segment = 'premium' GROUP BY u.region" },
  { id: "q5", name: "point_lookup", description: "Контрольный: SQLite без сети", sql_preview: "SELECT * FROM events WHERE id = ?" },
];
const DEMO_STATUS: StatusResponse = {
  postgres: { events: 3_000_000, users: 50_000, size_bytes: 612_000_000 },
  mysql: { events: 3_000_000, users: 50_000, size_bytes: 708_000_000 },
  sqlite: { events: 3_000_000, users: 50_000, size_bytes: 545_000_000 },
  clickhouse: { events: 3_000_000, users: 50_000, size_bytes: 118_000_000 },
};
const DEMO_LAT: Record<string, [number, number, number, number]> = {
  q1: [1240, 980, 2100, 38], q2: [85, 120, 210, 14], q3: [1630, 1410, 2890, 52], q4: [720, 1350, 1980, 96], q5: [0.9, 1.1, 0.05, 24],
};
const DEMO_METRIC: Record<string, [string, string, string, string]> = {
  q1: ["shared read=45k, hit=2k", "Handler_read_rnd_next=5.0M", "SCAN events", "read_rows=3.0M, read_bytes=36 MB"],
  q2: ["shared read=3.1k, hit=410", "Handler_read_next=412k", "SEARCH events USING INDEX idx_events_created_at", "read_rows=246k, read_bytes=3.9 MB"],
  q3: ["shared read=44k, hit=3k", "Handler_read_next=5.0M", "SCAN events USING INDEX idx_events_user_id", "read_rows=3.0M, read_bytes=12 MB"],
  q4: ["shared read=46k, hit=1.2k", "Handler_read_key=16.7k", "SCAN e; SEARCH u USING INTEGER PRIMARY KEY", "read_rows=3.05M, read_bytes=37 MB"],
  q5: ["shared hit=4", "Handler_read_key=1", "SEARCH events USING INTEGER PRIMARY KEY (rowid=?)", "read_rows=3.0M, read_bytes=24 MB"],
};
const DEMO_ROWS: Record<string, Record<string, unknown>[]> = {
  q1: ["click", "view", "purchase", "signup", "logout"].map((t, i) => ({ event_type: t, count: 1_000_000 + i * 731, sum: (50_123_456.78 + i * 9_876).toFixed(2) })),
  q2: Array.from({ length: 8 }, (_, i) => ({ user_id: 41_000 - i * 977, sum: (4_210.5 - i * 101).toFixed(2) })),
  q3: Array.from({ length: 8 }, (_, i) => ({ user_id: 1_200 + i * 313, cnt: 152 - i * 2 })),
  q4: ["eu", "us", "asia", "latam", "africa"].map((r, i) => ({ region: r, sum: (16_400_000 - i * 120_000).toFixed(2) })),
  q5: [{ id: 2_718_281, user_id: 31_415, event_type: "view", created_at: "2024-03-14T09:26:53Z", value: "42.00", payload: "lorem ipsum…" }],
};
const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

export let demoMode = false;

export async function getStatus(): Promise<StatusResponse> {
  try { return await req<StatusResponse>("/api/status"); } catch { demoMode = true; return DEMO_STATUS; }
}
export async function getQueries(): Promise<QueryInfo[]> {
  try { return await req<QueryInfo[]>("/api/queries"); } catch { demoMode = true; return DEMO_QUERIES; }
}
export async function runQuery(db: DbId, query_id: string): Promise<RunResult> {
  if (!demoMode) {
    try {
      return await req<RunResult>("/api/run", { method: "POST", body: JSON.stringify({ db, query_id }) });
    } catch (e) {
      if (!(e instanceof Error) || !/HTTP|not json|fetch/i.test(e.message)) {
        return { latency_ms: 30000, metric: "—", rows_preview: [], status: "timeout" };
      }
      demoMode = true;
    }
  }
  const i = DBS.findIndex((d) => d.id === db);
  const lat = DEMO_LAT[query_id]?.[i] ?? 0;
  await delay(Math.min(lat / 3, 900) + 150);
  return { latency_ms: lat, metric: DEMO_METRIC[query_id]?.[i] ?? "—", rows_preview: DEMO_ROWS[query_id] ?? [], status: "ok" };
}

export function fmtBytes(b: number) {
  if (!b) return "—";
  const u = ["B", "KB", "MB", "GB"]; let i = 0;
  while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
  return `${b.toFixed(i ? 1 : 0)} ${u[i]}`;
}
export const fmtNum = (n: number) => (n ?? 0).toLocaleString("ru-RU");
