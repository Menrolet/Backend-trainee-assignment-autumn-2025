BINARY := pr-reviewer-service
HTTP_ADDR ?= :8080
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/pr_reviewer?sslmode=disable

.PHONY: build run test docker-build compose-up compose-down

build:
	@mkdir -p bin
	go build -o bin/$(BINARY) ./cmd/server

run:
	HTTP_ADDR=$(HTTP_ADDR) DATABASE_URL=$(DATABASE_URL) go run ./cmd/server

test:
	go test ./...

docker-build:
	docker build -t $(BINARY):local .

compose-up:
	docker compose up --build

compose-down:
	docker compose down -v
