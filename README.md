# DB Demo — PostgreSQL, MySQL, SQLite

Один UI, одинаковые данные (5M событий, 50K пользователей), пять фиксированных запросов. Сравнение client-server и embedded СУБД на уровне латентности и «объясняющих» метрик (планы / буферы / Handler_read).

## Быстрый старт

1. Запустите Docker Desktop (нужен работающий демон).
2. В корне репозитория:

```bash
docker compose up -d --build
docker compose run --rm backend /app/seed --events 5000000 --users 50000 --batch-size 10000
```

Эквивалент с Go на хосте (после запуска только контейнеров БД или при локальных инстансах):

```bash
# Windows PowerShell
./demo.ps1 seed --events 5000000 --users 50000 --batch-size 10000

# Unix
chmod +x demo.sh && ./demo.sh seed --events 5000000 --users 50000 --batch-size 10000
```

3. Откройте [http://localhost:3000](http://localhost:3000). API бэкенда: [http://localhost:8080](http://localhost:8080).

Бэкенд ожидает переменные `PG_DSN`, `MYSQL_DSN`, `SQLITE_PATH` (см. [`docker-compose.yml`](docker-compose.yml)). SQLite хранится в [`data/sqlite.db`](data/) на хосте.

**Без Docker:** поднимите PostgreSQL и MySQL локально с теми же учётными данными, что в compose, затем:

```bash
cd backend
go run ./cmd/seed --events 5000000 --users 50000 --batch-size 10000
go run ./cmd/server
```

В другом терминале, для UI с прокси к API:

```bash
cd client
set VITE_API_URL=http://localhost:8080
npm run dev
```

### Порты

Если `5432` или `3306` заняты (например, OSPanel), измените в `docker-compose.yml` маппинг портов и задайте `PG_DSN` / `MYSQL_DSN` соответственно, а также используйте те же DSN при `go run ./cmd/seed` с хоста.

## Результаты (p50 latency, мс)

Заполните таблицу **после** `seed` на своей машине: выполните каждый запрос Q1–Q5 в UI (или `POST /api/run` для каждой пары db/query_id). Подставьте числа из колонки «Latency p50, ms».

| Запрос | PostgreSQL | MySQL | SQLite |
|--------|------------|-------|--------|
| Q1 agg_by_type | *замерить* | *замерить* | *замерить* |
| Q2 filter_by_time | *замерить* | *замерить* | *замерить* |
| Q3 top_users | *замерить* | *замерить* | *замерить* |
| Q4 join_regions | *замерить* | *замерить* | *замерить* |
| Q5 point_lookup | *замерить* | *замерить* | *замерить* |

**Окружение замера (укажите своё):** CPU / RAM / ОС

## Почему так выходит

- **Q5 (точечный поиск по PK):** у SQLite нет сетевого round-trip до отдельного сервера — обычно меньшая латентность, чем у PostgreSQL/MySQL при том же запросе с клиента.
- **Q4 (JOIN + фильтр по segment):** PostgreSQL и MySQL строят разные планы (использование индекса по `users.segment`, вложенные циклы, хеш и т.д.) — метрики и время могут заметно расходиться.
- **Q1–Q3 на 5M строк в одном файле SQLite:** один writer, конкуренция за диск и отсутствие отдельного серверного буферного кеша часто делают полные сканы и агрегации медленнее, чем у настроенных PG/MySQL на том же железе.

## Ограничения

Это **демонстрация архитектур**, не промышленный бенчмарк. SQLite не позиционирован как замена PostgreSQL/MySQL для многопользовательских нагрузок. Для SQLite в ответе API метрика строится из `EXPLAIN QUERY PLAN` (логический план), а не из жёстких счётчиков физического I/O.

## Скринкаст

Запись сценария 20–30 с: `docker compose up` → открыть UI → выбрать Q4 → «Выполнить». Положите ролик в [`docs/demo.gif`](docs/demo.gif) (сейчас — минимальная заглушка; замените на реальный GIF со своей машины).

## Структура

- [`backend/`](backend/) — Go + chi, `GET /api/status`, `GET /api/queries`, `POST /api/run`, CLI `cmd/seed`
- [`client/`](client/) — React + Vite + TanStack Start (готовый UI)
- [`docker-compose.yml`](docker-compose.yml) — Postgres, MySQL, backend, client
