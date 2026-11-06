import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  DBS, type DbId, type RunResult, getQueries, getStatus, runQuery, fmtBytes, fmtNum, demoMode,
} from "@/lib/api";

export const Route = createFileRoute("/")({
  head: () => ({
    meta: [
      { title: "DB Demo — PostgreSQL, MySQL, SQLite vs ClickHouse" },
      { name: "description", content: "Columnar OLAP ClickHouse против row-store СУБД на одинаковых данных и запросах." },
      { property: "og:title", content: "DB Demo — PostgreSQL, MySQL, SQLite vs ClickHouse" },
      { property: "og:description", content: "Columnar OLAP ClickHouse против row-store СУБД на одинаковых данных и запросах." },
      { property: "og:type", content: "website" },
      { name: "twitter:card", content: "summary_large_image" },
    ],
  }),
  component: Index,
});

type Results = Partial<Record<DbId, RunResult | "pending">>;

function Index() {
  const status = useQuery({ queryKey: ["status"], queryFn: getStatus });
  const queries = useQuery({ queryKey: ["queries"], queryFn: getQueries });
  const [selected, setSelected] = useState<Record<DbId, boolean>>({ postgres: true, mysql: true, sqlite: true, clickhouse: true });
  const [qid, setQid] = useState("q1");
  const [results, setResults] = useState<Results>({});
  const [running, setRunning] = useState(false);

  const current = queries.data?.find((q) => q.id === qid);

  async function run() {
    const dbs = DBS.filter((d) => selected[d.id]).map((d) => d.id);
    if (!dbs.length) return;
    setRunning(true);
    setResults(Object.fromEntries(dbs.map((d) => [d, "pending"])));
    // Sequential per backend run (обычно PG → MySQL → SQLite → ClickHouse), как в ТЗ
    for (const db of dbs) {
      let r: RunResult;
      try { r = await runQuery(db, qid); }
      catch (e) { r = { latency_ms: 0, metric: "—", rows_preview: [], status: "error", error: String(e) }; }
      setResults((p) => ({ ...p, [db]: r }));
    }
    setRunning(false);
  }

  const done = DBS.map((d) => ({ d, r: results[d.id] })).filter(
    (x): x is { d: (typeof DBS)[number]; r: RunResult } => !!x.r && x.r !== "pending",
  );
  const maxLat = Math.max(1, ...done.map((x) => x.r.latency_ms));
  const ch = done.find((x) => x.d.id === "clickhouse" && x.r.status === "ok")?.r.latency_ms;
  const chRatio = (id: DbId, r: RunResult) => {
    if (id === "clickhouse" || !ch || r.status !== "ok" || !r.latency_ms) return null;
    const k = r.latency_ms / ch;
    return <span className={`ml-2 font-mono text-xs ${k >= 1 ? "text-ch" : "text-primary"}`}>{k >= 1 ? `CH ×${k.toFixed(k < 10 ? 1 : 0)} быстрее` : `CH ×${(1 / k).toFixed(1)} медленнее`}</span>;
  };
  const best = done.filter((x) => x.r.status === "ok").sort((a, b) => a.r.latency_ms - b.r.latency_ms)[0]?.d.id;

  return (
    <div className="min-h-screen bg-background font-sans text-foreground scanlines">
      <header className="bg-header border-b border-border">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-full border-2 border-primary font-mono text-sm text-primary shadow-glow">DB</div>
            <span className="text-2xl font-light tracking-[0.2em]">DB<span className="text-primary glow-text">·</span>DEMO</span>
          </div>
          <span className={`rounded-full border px-4 py-1.5 font-mono text-xs ${demoMode ? "border-warning text-warning" : "border-primary text-primary"}`}>
            {demoMode ? "demo-данные · бэкенд недоступен" : "● backend online"}
          </span>
        </div>
      </header>

      <main className="mx-auto max-w-6xl space-y-12 px-6 py-14">
        <section className="max-w-3xl">
          <h1 className="text-4xl font-light leading-tight md:text-5xl">
            <span className="text-ch">ClickHouse</span> против PostgreSQL, MySQL и SQLite<br />на <span className="text-primary glow-text">одинаковых данных</span>
          </h1>
          <p className="mt-5 text-muted-foreground">
            5 000 000 событий, 50 000 пользователей, 5 фиксированных запросов. Columnar OLAP (ClickHouse) против row-store — он должен выиграть на агрегациях Q1–Q4 и проиграть на point lookup Q5.
            <span className="caret ml-1 text-primary">▌</span>
          </p>
        </section>

        <section>
          <Title n="01">Состояние БД</Title>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {DBS.map((d) => {
              const s = status.data?.[d.id];
              return (
                <div key={d.id} className={`relative rounded-xl border bg-card p-5 transition ${d.id === "clickhouse" ? "border-2 border-ch shadow-[0_0_24px_-6px_var(--ch)]" : "border-border hover:border-primary hover:shadow-glow"}`}>
                  {d.id === "clickhouse" && <span className="absolute -top-3 left-4 rounded-full bg-ch px-2.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-widest text-ch-foreground">эталон</span>}
                  <div className="flex items-baseline justify-between">
                    <h3 className={`text-lg font-semibold ${d.id === "clickhouse" ? "text-ch" : ""}`}>{d.name}</h3>
                    <span className="font-mono text-xs text-muted-foreground">{d.kind}</span>
                  </div>
                  {status.isLoading ? <p className="mt-4 font-mono text-sm text-muted-foreground">loading…</p>
                    : s?.error ? <p className="mt-4 font-mono text-sm text-destructive">{s.error}</p>
                    : (
                      <dl className="mt-4 space-y-1.5 font-mono text-sm">
                        <Row k="events" v={fmtNum(s?.events ?? 0)} />
                        <Row k="users" v={fmtNum(s?.users ?? 0)} />
                        <Row k="на диске" v={fmtBytes(s?.size_bytes ?? 0)} />
                      </dl>
                    )}
                </div>
              );
            })}
          </div>
        </section>

        <section>
          <Title n="02">Запуск</Title>
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="flex flex-wrap items-end gap-6">
              <div>
                <div className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">СУБД</div>
                <div className="flex flex-wrap gap-2">
                  {DBS.map((d) => (
                    <button key={d.id} type="button" onClick={() => setSelected((p) => ({ ...p, [d.id]: !p[d.id] }))}
                      className={`rounded-full border px-4 py-2 text-sm transition ${d.id === "clickhouse" ? (selected[d.id] ? "border-ch bg-ch text-ch-foreground font-semibold" : "border-ch/50 text-ch hover:border-ch") : selected[d.id] ? "border-primary bg-accent text-accent-foreground" : "border-border text-muted-foreground hover:border-primary"}`}>
                      {selected[d.id] ? "● " : "○ "}{d.name}
                    </button>
                  ))}
                </div>
              </div>
              <div className="min-w-64 flex-1">
                <div className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">Запрос</div>
                <select value={qid} onChange={(e) => setQid(e.target.value)}
                  className="w-full rounded-full border border-input bg-background px-4 py-2.5 text-sm outline-none focus:border-primary">
                  {queries.data?.map((q) => (
                    <option key={q.id} value={q.id}>{q.id.toUpperCase()} · {q.name} — {q.description}</option>
                  ))}
                </select>
              </div>
              <button type="button" onClick={run} disabled={running || !Object.values(selected).some(Boolean)}
                className="rounded-full bg-primary px-8 py-3 font-semibold text-primary-foreground shadow-glow transition hover:brightness-110 disabled:opacity-50">
                {running ? "Выполняется…" : "Выполнить →"}
              </button>
            </div>
            {current && (
              <pre className="mt-5 overflow-x-auto rounded-lg border border-border bg-background p-4 font-mono text-sm text-primary">
                <span className="text-muted-foreground">$ </span>{current.sql_preview}
              </pre>
            )}
          </div>
        </section>

        <section>
          <Title n="03">Результаты</Title>
          <div className="overflow-x-auto rounded-xl border border-border bg-card">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-border text-xs uppercase tracking-widest text-muted-foreground">
                <tr><th className="p-4">СУБД</th><th className="p-4">Latency p50, ms</th><th className="p-4">Метрика</th><th className="p-4">Статус</th></tr>
              </thead>
              <tbody className="font-mono">
                {DBS.filter((d) => results[d.id]).length === 0 && (
                  <tr><td colSpan={4} className="p-8 text-center text-muted-foreground">Выберите запрос и нажмите «Выполнить»</td></tr>
                )}
                {DBS.map((d) => {
                  const r = results[d.id];
                  if (!r) return null;
                  if (r === "pending") return (
                    <tr key={d.id} className="border-b border-border last:border-0">
                      <td className="p-4 font-sans font-semibold">{d.name}</td>
                      <td colSpan={3} className="p-4 text-muted-foreground">ожидание<span className="caret">_</span></td>
                    </tr>
                  );
                  return (
                    <tr key={d.id} className={`border-b border-border last:border-0 ${d.id === "clickhouse" ? "bg-ch/10 font-bold" : ""}`}>
                      <td className={`p-4 font-sans font-semibold ${d.id === "clickhouse" ? "border-l-4 border-ch text-ch" : ""}`}>{d.name}{chRatio(d.id, r)}{best === d.id && <span className="ml-2 text-primary">★</span>}</td>
                      <td className="p-4">
                        <div className="flex items-center gap-3">
                          <span className={best === d.id ? "text-primary glow-text" : ""}>{fmtLat(r.latency_ms)}</span>
                          <div className="h-1.5 w-28 overflow-hidden rounded-full bg-muted">
                            <div className={`h-full ${d.id === "clickhouse" ? "bg-ch" : "bg-primary"}`} style={{ width: `${(r.latency_ms / maxLat) * 100}%` }} />
                          </div>
                        </div>
                      </td>
                      <td className={`p-4 ${d.id === "clickhouse" ? "text-ch" : "text-muted-foreground"}`}>{r.metric || "—"}</td>
                      <td className="p-4"><StatusBadge s={r.status} err={r.error} /></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>

        {done.some((x) => x.r.rows_preview?.length) && (
          <section>
            <Title n="04">Пример данных</Title>
            <div className="grid gap-4 md:grid-cols-2">
              {done.map(({ d, r }) => (
                <div key={d.id} className={`overflow-hidden rounded-xl border bg-card ${d.id === "clickhouse" ? "border-ch" : "border-border"}`}>
                  <div className={`border-b px-4 py-3 font-semibold ${d.id === "clickhouse" ? "border-ch text-ch" : "border-border"}`}>{d.name}</div>
                  <Preview rows={r.rows_preview.slice(0, 10)} />
                </div>
              ))}
            </div>
          </section>
        )}
      </main>

      <footer className="border-t border-border py-6 text-center font-mono text-xs text-muted-foreground">
        demo, не бенчмарк · Go + chi + database/sql + clickhouse-go/v2
      </footer>
    </div>
  );
}

const fmtLat = (ms: number) => (ms < 10 ? ms.toFixed(2) : fmtNum(Math.round(ms)));

function Title({ n, children }: { n: string; children: React.ReactNode }) {
  return (
    <h2 className="mb-5 flex items-center gap-3 text-xl font-light">
      <span className="font-mono text-sm text-primary">{n}</span>{children}
      <span className="h-px flex-1 bg-border" />
    </h2>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return <div className="flex justify-between"><dt className="text-muted-foreground">{k}</dt><dd>{v}</dd></div>;
}

function StatusBadge({ s, err }: { s: string; err?: string | undefined }) {
  const cls = s === "ok" ? "border-primary text-primary" : s === "timeout" ? "border-warning text-warning" : "border-destructive text-destructive";
  return <span title={err} className={`rounded-full border px-3 py-1 text-xs uppercase ${cls}`}>{s}</span>;
}

function Preview({ rows }: { rows: Record<string, unknown>[] }) {
  if (!rows.length) return <p className="p-4 font-mono text-sm text-muted-foreground">нет строк</p>;
  const cols = Object.keys(rows[0] ?? {});
  return (
    <div className="overflow-x-auto">
      <table className="w-full font-mono text-xs">
        <thead className="text-muted-foreground"><tr>{cols.map((c) => <th key={c} className="px-3 py-2 text-left font-normal">{c}</th>)}</tr></thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-t border-border">
              {cols.map((c) => <td key={c} className="whitespace-nowrap px-3 py-1.5">{String(r[c] ?? "")}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
