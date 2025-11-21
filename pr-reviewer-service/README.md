# PR Reviewer Assignment Service

Микросервис назначает ревьюверов на Pull Request внутри команды, управляет командами/пользователями и поддерживает переназначение ревьюверов (до мержа PR).

## Требования
- Go 1.21+
- PostgreSQL 15+ 
- Docker + Docker Compose 
- make 

## Быстрый старт (локально)
1. Создай БД и примени миграцию:
   ```bash
   export DATABASE_URL="postgres://postgres:postgres@localhost:5432/pr_reviewer?sslmode=disable"
   psql "$DATABASE_URL" -f migrations/001_init.sql
   ```
2. Запусти сервис:
   ```bash
   go run ./cmd/server
   ```
   По умолчанию слушает `:8080` (переменная `HTTP_ADDR`), DSN берётся из `DATABASE_URL`.
3. Проверка: `curl http://localhost:8080/health`.

## Запуск через Docker Compose
```bash
docker compose up --build
```
- Поднимет PostgreSQL с начальными таблицами (миграция монтируется в `docker-entrypoint-initdb.d` и применяется на чистом volume).
- Сервис слушает `localhost:8080`, использует DSN `postgres://postgres:postgres@db:5432/pr_reviewer?sslmode=disable`.
Остановить и очистить volume: `docker compose down -v`.

## Makefile
- `make build` — собрать бинарник в `bin/`.
- `make run` — запустить с текущими `HTTP_ADDR`/`DATABASE_URL`.
- `make test` — `go test ./...`.
- `make docker-build` — собрать docker-образ `pr-reviewer-service:local`.
  - `make compose-up` / `make compose-down` — поднять/остановить docker-compose с очисткой volume.

## API
- Спецификация: `openapi.yaml`.
- Пример ручек (локально):
  ```bash
  curl -X POST http://localhost:8080/team/add \
    -H "Content-Type: application/json" \
    -d '{"team_name":"backend","members":[{"user_id":"u1","username":"Alice","is_active":true},{"user_id":"u2","username":"Bob","is_active":true}]}'

  curl "http://localhost:8080/pullRequest/create" \
    -X POST -H "Content-Type: application/json" \
    -d '{"pull_request_id":"pr1","pull_request_name":"feat","author_id":"u1"}'

  curl "http://localhost:8080/users/getReview?user_id=u2"

  # Массово деактивировать команду и попытаться переназначить ревьюверов в открытых PR
  curl -X POST http://localhost:8080/team/deactivate \
    -H "Content-Type: application/json" \
    -d '{"team_name":"backend"}'

  # Простая статистика назначений ревьюверов
  curl http://localhost:8080/stats/reviewerAssignments
  ```

## Нагрузочное тестирование (локально)
- Подготовка данных (автор/команда):
  ```bash
  curl -X POST http://localhost:8080/team/add \
    -H "Content-Type: application/json" \
    -d '{"team_name":"backend","members":[{"user_id":"u1","username":"Alice","is_active":true},{"user_id":"u2","username":"Bob","is_active":true},{"user_id":"u3","username":"Carol","is_active":true}]}'
  ```
- `/health` (hey, 1000 req, 10 потоков): p95 ~0.8–1.1 ms, 0 ошибок.
  ```bash
  hey -n 1000 -c 10 http://localhost:8080/health
  ```
- `/pullRequest/create` (vegeta, 200 req, 20 rps, уникальные ID): p95 ~15.8 ms, 100% 201.
  ```bash
  mkdir -p /tmp/prbodies
  for i in $(seq 0 199); do
    printf '{"pull_request_id":"prZ%s","pull_request_name":"feat%s","author_id":"u1"}\n' "$i" "$i" > /tmp/prbodies/body$i.json
  done
  > /tmp/targets.txt
  for i in $(seq 0 199); do
    echo "POST http://localhost:8080/pullRequest/create" >> /tmp/targets.txt
    echo "Content-Type: application/json" >> /tmp/targets.txt
    echo "@/tmp/prbodies/body$i.json" >> /tmp/targets.txt
    echo "" >> /tmp/targets.txt
  done
  vegeta attack -rate=20 -duration=10s -targets=/tmp/targets.txt -output=/tmp/vegeta.bin
  vegeta report /tmp/vegeta.bin
  ```

## Допущения и заметки
- Назначаются до двух активных ревьюверов из команды автора (автор исключён).
- После MERGED переназначение запрещено — реассайн сработает только пока статус OPEN.
- При отсутствии кандидатов назначается доступное количество (0/1).
- Тесты не написаны (проект собирается через `go test ./...`).
- Если после первого `docker compose up` нужно переиграть миграции, удаляйте volume: `docker compose down -v`.
