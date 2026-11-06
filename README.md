# DB Demo — PostgreSQL, MySQL, SQLite и ClickHouse

Один UI, одинаковые данные (3M событий, 50K пользователей), пять фиксированных запросов. Row-store (PostgreSQL, MySQL, SQLite) сравниваются с columnar OLAP **ClickHouse** по латентности и «объясняющим» метрикам (буферы / Handler_read / план / `read_rows` + `read_bytes` в ClickHouse).

Для ClickHouse в Docker рекомендуется выделить **не меньше ~4 GB RAM** в Docker Desktop (Windows/WSL2), иначе сервер может работать нестабильно.

## Быстрый старт

1. Запустите Docker Desktop (нужен работающий демон).
2. В корне репозитория:

```bash
docker compose up -d --build
docker compose run --rm backend /app/seed --events 3000000 --users 50000 --batch-size 10000
```

Эквивалент с Go на хосте (после запуска только контейнеров БД или при локальных инстансах):

```bash
# Windows PowerShell
./demo.ps1 seed --events 3000000 --users 50000 --batch-size 10000

# Unix
chmod +x demo.sh && ./demo.sh seed --events 3000000 --users 50000 --batch-size 10000
```

3. Откройте [http://localhost:3000](http://localhost:3000). API бэкенда: [http://localhost:8080](http://localhost:8080).

Бэкенд ожидает переменные `PG_DSN`, `MYSQL_DSN`, `SQLITE_PATH`, `CLICKHOUSE_DSN` (см. [`docker-compose.yml`](docker-compose.yml)). SQLite хранится в [`data/sqlite.db`](data/) на хосте.

**Без Docker:** поднимите PostgreSQL, MySQL и ClickHouse локально с теми же учётными данными, что в compose, затем:

```bash
cd backend
go run ./cmd/seed --events 3000000 --users 50000 --batch-size 10000
go run ./cmd/server
```

В другом терминале, для UI с прокси к API:

```bash
cd frontend
set VITE_API_URL=http://localhost:8080
npm run dev
```

### Порты

Если `5432`, `3306`, `8123` или `9000` заняты, измените в `docker-compose.yml` маппинг портов и задайте `PG_DSN`, `MYSQL_DSN`, `CLICKHOUSE_DSN` соответственно, а также используйте те же DSN при `go run ./cmd/seed` с хоста.

## Результаты (p50 latency, мс)

Заполните таблицу **после** `seed` на своей машине: выполните каждый запрос Q1–Q5 в UI (или `POST /api/run` для каждой пары db/query_id). Подставьте числа из колонки «Latency p50, ms».

| Запрос | PostgreSQL | MySQL | SQLite | ClickHouse |
|--------|------------|-------|--------|------------|
| Q1 agg_by_type | *замерить* | *замерить* | *замерить* | *замерить* |
| Q2 filter_by_time | *замерить* | *замерить* | *замерить* | *замерить* |
| Q3 top_users | *замерить* | *замерить* | *замерить* | *замерить* |
| Q4 join_regions | *замерить* | *замерить* | *замерить* | *замерить* |
| Q5 point_lookup | *замерить* | *замерить* | *замерить* | *замерить* |

**Окружение замера (укажите своё):** CPU / RAM / ОС

## Почему так выходит

- **Q1–Q4:** у ClickHouse columnar-хранение и MergeTree (`ORDER BY (event_type, created_at)`) обычно дают сильно меньший объём читаемых данных — это видно в метрике `read_bytes` из `system.query_log`.
- **Q5 (точечный поиск по id):** у row-store с B-tree по PK поиск по одному ключу часто быстрее; у ClickHouse без ключа сортировки по `id` запрос ведёт себя как тяжёлый scan по гранулам.
- **Q5 и SQLite:** встроенный движок без сетевого round-trip часто даёт самую низкую латентность на point lookup среди четырёх СУБД.
- **Q4:** PostgreSQL и MySQL строят разные планы JOIN — время и метрики могут заметно расходиться.

## Ограничения

Это **демонстрация архитектур**, не промышленный бенчмарк. Для SQLite метрика строится из `EXPLAIN QUERY PLAN`; для ClickHouse — из `read_rows` / `read_bytes` в `system.query_log` (честное сравнение разных типов счётчиков).

## Скринкаст

Запись сценария 20–30 с: `docker compose up` → открыть UI → выбрать Q4 → «Выполнить». Положите ролик в [`docs/demo.gif`](docs/demo.gif).

## Структура

- [`backend/`](backend/) — Go + chi, `GET /api/status`, `GET /api/queries`, `POST /api/run`, CLI `cmd/seed`, драйвер `clickhouse-go` (batch insert в seed)
- [`frontend/`](frontend/) — React + Vite + TanStack Start (UI)
- [`clickhouse/init/`](clickhouse/init/) — init SQL для Docker (права на `system.query_log` / `system.parts`)
- [`docker-compose.yml`](docker-compose.yml) — Postgres, MySQL, ClickHouse, backend, frontend
