package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	UserSvcAddr string
	PostgresDSN string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	_ = godotenv.Load() // carga .env si existe
	cfg := Config{
		UserSvcAddr: getenv("USER_SERVICE_ADDR", "localhost:50051"),
		PostgresDSN: getenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/ordenesdb?sslmode=disable"),
	}
	log.Printf("[config] USER_SERVICE_ADDR=%s", cfg.UserSvcAddr)
	return cfg
}
