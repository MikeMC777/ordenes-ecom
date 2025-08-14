package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/MikeMC777/ordenes-ecom/internal/config"
	userSvc "github.com/MikeMC777/ordenes-ecom/internal/user"
	pb "github.com/MikeMC777/ordenes-ecom/internal/userpb"
)

func main() {
	cfg := config.Load()

	// Connection to Postgres
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("db pool error: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db ping error: %v", err)
	}
	defer pool.Close()
	log.Println("[db] connected")

	// gRPC
	lis, err := net.Listen("tcp", cfg.UserSvcAddr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}

	server := grpc.NewServer()
	repo := userSvc.NewRepoFromPool(pool)
	service := userSvc.NewService(repo)

	pb.RegisterUserServiceServer(server, service)

	// gRPC health and reflexion
	hs := health.NewServer()
	healthpb.RegisterHealthServer(server, hs)
	reflection.Register(server)

	// Gracefully Shutdown
	go func() {
		log.Printf("[grpc] listening on %s", cfg.UserSvcAddr)
		if err := server.Serve(lis); err != nil {
			log.Printf("[grpc] serve error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("[grpc] shutting down...")
	server.GracefulStop()
}
