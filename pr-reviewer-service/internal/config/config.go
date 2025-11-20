package config

import "os"

type Config struct {
	Addr string
	DSN  string
}

func Load() Config {
	return Config{
		Addr: getEnv("HTTP_ADDR", ":8080"),
		DSN:  getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
