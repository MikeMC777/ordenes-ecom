package order

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"strings"

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
		ProductBaseURL: strings.TrimRight(productBaseURL, "/"),
	}, nil
}

func (e *Ext) FetchProduct(ctx context.Context, id string) (*ProductDTO, error) {
	url := e.ProductBaseURL + "/products/" + id
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: http error: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 256))
		return nil, fmt.Errorf("fetch %s: status=%d body=%q", url, res.StatusCode, string(b))
	}
	var p ProductDTO
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
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
		return fmt.Errorf("adjust fetch: %w", err)
	}
	newStock := p.Stock + delta
	if newStock < 0 {
		return fmt.Errorf("insufficient stock")
	}
	body, _ := json.Marshal(map[string]int{"stock": newStock})
	url := e.ProductBaseURL + "/products/" + productID
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := e.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("adjust %s: http error: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 256))
		switch res.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("product not found (status=404 %s)", url)
		case http.StatusBadRequest:
			return fmt.Errorf("invalid stock body=%q (%s)", string(b), url)
		default:
			return fmt.Errorf("update stock error: status=%d body=%q url=%s", res.StatusCode, string(b), url)
		}
	}
	return nil
}
