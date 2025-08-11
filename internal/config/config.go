package config

import "os"

type Config struct {
    UserSvcAddr       string
    ProductSvcBaseURL string
    PostgresDSN       string
}

func getenv(k, def string) string {
    if v := os.Getenv(k); v != "" {
        return v
    }
    return def
}

func Load() Config {
    return Config{
        UserSvcAddr:       getenv("USER_SERVICE_ADDR", "localhost:50051"),
        ProductSvcBaseURL: getenv("PRODUCT_SERVICE_BASEURL", "http://localhost:8081"),
        PostgresDSN:       getenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/ordenesdb?sslmode=disable"),
    }
}
