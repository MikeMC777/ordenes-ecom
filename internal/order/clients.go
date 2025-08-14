package order

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userpb "github.com/MikeMC777/ordenes-ecom/internal/userpb"
)

type ProductDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       string `json:"price"`
	Stock       int    `json:"stock"`
}

type Ext struct {
	HTTP           *http.Client
	User           userpb.UserServiceClient
	ProductBaseURL string
}

func NewExt(userAddr, productBaseURL string) (*Ext, error) {
	// Non-blocking gRPC connection (RPC will use WaitForReady)
	conn, err := grpc.Dial(userAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Ext{
		HTTP:           &http.Client{Timeout: 5 * time.Second},
		User:           userpb.NewUserServiceClient(conn),
		ProductBaseURL: productBaseURL,
	}, nil
}

func (e *Ext) FetchProduct(ctx context.Context, id string) (*ProductDTO, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/products/%s", e.ProductBaseURL, id), nil)
	res, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product not found")
	}
	var p ProductDTO
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (e *Ext) ValidateUser(ctx context.Context, id string) (bool, error) {
	out, err := e.User.ValidateUser(ctx, &userpb.ValidateUserRequest{Id: id})
	if err != nil {
		return false, err
	}
	return out.GetOk(), nil
}

// Adjust stock by adding delta (delta can be negative)
// Use PUT /products/{id} with { “stock”: newValue }
func (e *Ext) AdjustStock(ctx context.Context, productID string, delta int) error {
	p, err := e.FetchProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("product not found")
	}
	newStock := p.Stock + delta
	if newStock < 0 {
		return fmt.Errorf("insufficient stock")
	}
	body, _ := json.Marshal(map[string]int{"stock": newStock})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/products/%s", e.ProductBaseURL, productID),
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("product not found")
	case http.StatusBadRequest:
		return fmt.Errorf("invalid stock")
	default:
		return fmt.Errorf("update stock error: %s", res.Status)
	}
}
