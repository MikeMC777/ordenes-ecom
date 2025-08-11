$env:USER_SERVICE_ADDR="localhost:50051"
$env:POSTGRES_DSN="postgres://user:pass@localhost:5432/ordenesdb?sslmode=disable"
go run .\cmd\user-service\main.go