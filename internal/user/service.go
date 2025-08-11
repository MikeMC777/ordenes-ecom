package user

import (
	"context"

	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/MikeMC777/ordenes-ecom/internal/userpb"
)

type Service struct {
	pb.UnimplementedUserServiceServer
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateUser
func (s *Service) CreateUser(ctx context.Context, in *pb.CreateUserRequest) (*pb.UserResponse, error) {
	if in.GetUsername() == "" || in.GetEmail() == "" || in.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "username, email and password are required")
	}
	hash, err := HashPassword(in.GetPassword())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash error: %v", err)
	}
	u := &User{
		ID:           uuid.NewString(),
		Username:     in.GetUsername(),
		Email:        in.GetEmail(),
		PasswordHash: hash,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		if err == ErrAlreadyExist {
			return nil, status.Error(codes.AlreadyExists, "user exists (username/email)")
		}
		return nil, status.Errorf(codes.Internal, "create error: %v", err)
	}
	return &pb.UserResponse{User: &pb.User{
		Id: u.ID, Username: u.Username, Email: u.Email, CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}}, nil
}

// GetUser
func (s *Service) GetUser(ctx context.Context, in *pb.GetUserRequest) (*pb.UserResponse, error) {
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	u, err := s.repo.GetByID(ctx, in.GetId())
	if err != nil {
		if err == ErrNotFound {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "get error: %v", err)
	}
	return &pb.UserResponse{User: &pb.User{
		Id: u.ID, Username: u.Username, Email: u.Email, CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}}, nil
}

// UpdateUser
func (s *Service) UpdateUser(ctx context.Context, in *pb.UpdateUserRequest) (*pb.UserResponse, error) {
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	updatePassword := false
	var newHash string
	if in.GetPassword() != "" {
		h, err := HashPassword(in.GetPassword())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "hash error: %v", err)
		}
		newHash = h
		updatePassword = true
	}

	u := &User{
		ID:           in.GetId(),
		Username:     in.GetUsername(), // vacío => no cambia
		Email:        in.GetEmail(),    // vacío => no cambia
		PasswordHash: newHash,          // vacío => no cambia
	}
	if err := s.repo.Update(ctx, u, updatePassword); err != nil {
		return nil, status.Errorf(codes.Internal, "update error: %v", err)
	}

	// Devolver el estado actual
	out, err := s.repo.GetByID(ctx, in.GetId())
	if err != nil {
		if err == ErrNotFound {
			return nil, status.Error(codes.NotFound, "user not found after update")
		}
		return nil, status.Errorf(codes.Internal, "refetch error: %v", err)
	}
	return &pb.UserResponse{User: &pb.User{
		Id: out.ID, Username: out.Username, Email: out.Email,
		CreatedAt: out.CreatedAt.Format(time.RFC3339),
	}}, nil
}

// DeleteUser
func (s *Service) DeleteUser(ctx context.Context, in *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ok, err := s.repo.Delete(ctx, in.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete error: %v", err)
	}
	if !ok {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return &pb.DeleteUserResponse{Deleted: true}, nil
}

// AuthenticateUser (renombrado)
func (s *Service) AuthenticateUser(ctx context.Context, in *pb.AuthRequest) (*pb.AuthResponse, error) {
	if in.GetEmail() == "" || in.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}
	u, err := s.repo.GetByEmail(ctx, in.GetEmail())
	if err != nil {
		if err == ErrNotFound {
			return &pb.AuthResponse{Ok: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "auth error: %v", err)
	}
	ok := CheckPassword(u.PasswordHash, in.GetPassword())
	return &pb.AuthResponse{UserId: u.ID, Ok: ok}, nil
}

// ValidateUser (existe por ID)
func (s *Service) ValidateUser(ctx context.Context, in *pb.ValidateUserRequest) (*pb.ValidateUserResponse, error) {
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	_, err := s.repo.GetByID(ctx, in.GetId())
	if err != nil {
		if err == ErrNotFound {
			return &pb.ValidateUserResponse{Ok: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "validate error: %v", err)
	}
	return &pb.ValidateUserResponse{Ok: true}, nil
}

// Helper para crear repo desde pool (por si quieres inyectar afuera)
func NewRepoFromPool(pool *pgxpool.Pool) Repository { return NewPGRepo(pool) }
