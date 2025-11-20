# syntax=docker/dockerfile:1

FROM golang:1.21 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /bin/pr-reviewer-service ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app

COPY --from=builder /bin/pr-reviewer-service /app/pr-reviewer-service

ENV HTTP_ADDR=:8080
ENV DATABASE_URL=postgres://postgres:postgres@db:5432/pr_reviewer?sslmode=disable

EXPOSE 8080

ENTRYPOINT ["/app/pr-reviewer-service"]
