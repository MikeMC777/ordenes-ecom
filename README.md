# ecommerce-orders

1. Microservices-based Go backend for managing users, products, and orders in e-commerce.
2. REST API and gRPC in Go for an online ordering system: users, product catalog, and order processing.
3. Go microservices with PostgreSQL and Docker to orchestrate authentication, catalog, and order flow.

Services for a sample e-commerce site:

- **user-service** (gRPC - users)
- **product-service** (HTTP/REST - products)
- **order-service** (HTTP/REST - orders, integrates with user and product)

## Tech Stack

- Go 1.21+
- PostgreSQL 16
- gRPC (user-service)
- Gin (product/order)
- Migrations: `pressly/goose`
- HTTP Docs: Swagger (swaggo)
- gRPC Docs: `protoc-gen-doc`
- Docker Compose for local environment

## Structure (summary)

cmd/
product-service/
order-service/
user-service/ # gRPC
internal/
product/ # repository/product entities (PG)
order/ # repository/order entities (PG) + ext client
user/ # user logic (gRPC server)
userpb/ # generated gRPC stubs (Go)
docs/ # Swagger for product-service
docs-order/ # Swagger for order-service
docs-grpc/ # Generated gRPC docs (UserService.md)
db/migrations/ # SQL migrations
scripts/
seed.ps1 # seeds in PowerShell
OrdenesEcom.postman_collection.json
docker-compose.yml

## Requirements

- Go 1.21+ (local)
- Docker Desktop (for Compose)
- `protoc` (if you are going to regenerate gRPC stubs or docs)
- (Optional) `goose` (local) and `swag` CLI

**Project setup steps** (Docker)

## 1. Environment variables (example)

Create a `.env` file in the root directory with the environment variables

> Note: `ORDER` consumes `USER` via gRPC and `PRODUCT` via HTTP. Locally **without Docker**, change `PRODUCT_SERVICE_BASEURL` to `http://localhost:8081` and `USER_SERVICE_ADDR` to `localhost:50051`.

## 2. Bring everything up with Docker Compose (including migrations)

`docker compose up -d --build`

## 3. Seeds (test data)

Make sure product-service lists products first:

`curl “http://localhost:8081/products?limit=5&offset=0”`

Script scripts/seed.ps1:

`pwsh -File .\scripts\seed.ps1`

This creates a user in user-service (gRPC via grpcurl) and also an order against order-service using existing products.


## Testing

`go test ./... -count=1`

## Generate documentation

Swagger (HTTP)

Product-service:

`go install github.com/swaggo/swag/cmd/swag@v1.16.3`
`swag init -g cmd/product-service/main.go -o docs`

Order-service:

`swag init -g cmd/order-service/main.go -o docs-order`


Served at:

`http://localhost:8081/docs/index.html`

`http://localhost:8082/docs/index.html`

gRPC (UserService)

Generate Markdown with protoc-gen-doc:

go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest

PROTO_DIR=proto
PROTO_FILE=proto/user.proto

``mkdir -p docs-grpc
protoc -I “$PROTO_DIR” \
  --doc_out=docs-grpc \
  --doc_opt=markdown,UserService.md \
  “$PROTO_FILE”``

## Endpoints (summary)

Product-service (HTTP)

- GET /products — pagination only.
- GET /products/search?q=... — search + pagination (q ≥ 2).
- GET /products/{id}
- POST /products
- PUT /products/{id}
- DELETE /products/{id}

Order-service (HTTP)

- POST /orders
- GET /orders/{id}
- GET /orders/user/{user_id}
- PUT /orders/{id}/status
- GET /orders/{id}/items

User-service (gRPC)

CreateUser, GetUser, UpdateUser, DeleteUser
AuthenticateUser, ValidateUser

## Troubleshooting

- order-service returns “product not found”: check that PRODUCT_SERVICE_BASEURL points to http://product:8081 in Docker and to http://localhost:8081 locally.
- Migrator in Compose fails: check docker compose logs -f migrator and validate that db is healthy and that the db/migrations paths exist.
- Swagger only shows some endpoints: run swag init pointing to the correct -g; check @Router, @Param annotations, etc.

## Postman Collection

Saved in `scripts/OrdenesEcom.postman_collection.json`
