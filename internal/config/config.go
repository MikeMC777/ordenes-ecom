package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	UserSvcAddr       string
	ProductSvcAddr    string
	ProductSvcBaseURL string
	OrderSvcAddr      string
	PostgresDSN       string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	_ = godotenv.Load() // load .env if it exists
	cfg := Config{
		UserSvcAddr:       getenv("USER_SERVICE_ADDR", "localhost:50051"),
		ProductSvcAddr:    getenv("PRODUCT_SERVICE_ADDR", ":8081"),
		ProductSvcBaseURL: getenv("PRODUCT_SERVICE_BASEURL", "http://product:8081"),
		OrderSvcAddr:      getenv("ORDER_SERVICE_ADDR", ":8082"),
		PostgresDSN:       getenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/ordenesdb?sslmode=disable"),
	}
	log.Printf("[config] USER_SERVICE_ADDR=%s", cfg.UserSvcAddr)
	log.Printf("[config] PRODUCT_SERVICE_ADDR=%s", cfg.ProductSvcAddr)
	log.Printf("[config] ORDER_SERVICE_ADDR=%s", cfg.OrderSvcAddr)
	return cfg
}
