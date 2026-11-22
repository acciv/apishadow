# apishadow

Зеркалирует API-запросы на вторую версию сервиса и сравнивает ответы.

Основной сервис отвечает клиенту как обычно. Копия запроса уходит на shadow-апстрим в фоне. Оба ответа пишутся в JSONL. Python-анализатор сравнивает их и показывает, где версии расходятся.

## Зачем

Проверять новую версию API на реальном трафике, не отдавая клиенту её ответы.

## Устройство

- **Go** — HTTP-прокси: зеркалирование, таймауты, счётчики, запись захвата.
- **Rust** — нормализация записей: убирает нестабильные заголовки, считает отпечаток.
- **Python** — сравнение ответов: статусы, заголовки, JSON-поля, время ответа.
- **ClickHouse** — схема и экспорт отчёта в NDJSON.
- Docker, docker-compose, GitHub Actions.

## Запуск

```bash
make test
make demo
```

Демо поднимает два тестовых API, прокси, делает три запроса и печатает отчёт.

Вручную:

```bash
cargo build --release --manifest-path rust/Cargo.toml
cd go && go run ./cmd/proxy -config ../config/apishadow.conf
```

Docker:

```bash
docker compose up --build
curl http://127.0.0.1:8080/v1/users
```

## Конфиг

`config/apishadow.conf`:

```ini
listen=:8080
primary=http://127.0.0.1:9101
shadow=http://127.0.0.1:9102
capture_file=capture.jsonl
mirror_ratio=1.0
timeout_ms=3000
```

`mirror_ratio=0.1` зеркалирует примерно 10% запросов.

## Эндпоинты прокси

- `GET /healthz` — статус и версия capture-ядра.
- `GET /stats` — счётчики запросов и ошибок.
- всё остальное проксируется на primary и опционально на shadow.

## Формат записи

Одна строка JSONL на ответ:

```json
{
  "id": "1730000000000000000-1",
  "role": "primary",
  "method": "GET",
  "path": "/v1/users",
  "status": 200,
  "headers": {"content-type": "application/json"},
  "body": "{\"version\":\"1.0\"}",
  "duration_ms": 4.2
}
```

Rust убирает `Date`, `Set-Cookie`, `X-Request-Id` и похожие заголовки перед записью.

## Анализ

```bash
PYTHONPATH=python python3 -m apishadow \
  --primary primary.jsonl --shadow shadow.jsonl \
  --out report.json --fail-on major
```

Что сравнивается:

- код ответа;
- стабильные заголовки;
- тело: JSON-поля рекурсивно, остальное как строку;
- время ответа, если отличается больше чем в 2 раза.

Поля `timestamp`, `request_id`, `trace_id` и похожие игнорируются. Список можно расширить через `--ignore`.

Вердикты: `match`, `minor_diff`, `major_diff`. Коды возврата: `0` — норма, `1` — различия выше порога, `2` — ошибка файла.

## Экспорт в ClickHouse

```bash
PYTHONPATH=python python3 -m apishadow.clickhouse \
  --report report.json --out diffs.ndjson
```

Схема таблицы: `docs/schema.sql`.

## Тесты

```bash
make test
```

Rust: 4 теста. Go: тесты прокси и конфига. Python: 5 тестов сравнения.

Репозиторий: https://github.com/acciv/apishadow

Сделал acciv.
