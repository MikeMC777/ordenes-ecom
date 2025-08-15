# syntax=docker/dockerfile:1
FROM golang:1.21 AS build
WORKDIR /app

# Bring modules
COPY go.mod go.sum ./
RUN go mod download

# Copy code
COPY . .

# Package to compile (passed with --build-arg PKG=...)
ARG PKG=./cmd/product-service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app $PKG

# Minimalist final image
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/app /app/app
ENTRYPOINT ["/app/app"]
