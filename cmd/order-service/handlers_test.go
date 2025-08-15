package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	ord "github.com/MikeMC777/ordenes-ecom/internal/order"
	userpb "github.com/MikeMC777/ordenes-ecom/internal/userpb"
	"google.golang.org/grpc"
)

//
// ---------- STUBS & FAKES ----------
//

// stubRepo implements the ord.Repository interface in memory.
type stubRepo struct {
	lastOrder *ord.Order
	lastItems []ord.Item
}

func (s *stubRepo) Create(ctx context.Context, o *ord.Order, items []ord.Item) error {
	// save to memory
	cp := *o
	s.lastOrder = &cp
	s.lastItems = append([]ord.Item(nil), items...)
	return nil
}

func (s *stubRepo) GetByID(ctx context.Context, id string) (*ord.Order, []ord.Item, error) {
	if s.lastOrder == nil || s.lastOrder.ID != id {
		return nil, nil, fmt.Errorf("not found")
	}
	return s.lastOrder, s.lastItems, nil
}

func (s *stubRepo) GetItems(ctx context.Context, orderID string) ([]ord.Item, error) {
	if s.lastOrder == nil || s.lastOrder.ID != orderID {
		return nil, fmt.Errorf("not found")
	}
	return s.lastItems, nil
}

func (s *stubRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]ord.Order, error) {
	if s.lastOrder != nil && s.lastOrder.UserID == userID {
		return []ord.Order{*s.lastOrder}, nil
	}
	return []ord.Order{}, nil
}

func (s *stubRepo) UpdateStatus(ctx context.Context, id, status string) error {
	if s.lastOrder == nil || s.lastOrder.ID != id {
		return fmt.Errorf("not found")
	}
	s.lastOrder.Status = status
	return nil
}

// fakeUserClient implements userpb.UserServiceClient, but only uses ValidateUser.
type fakeUserClient struct {
	ok bool
}

func (f *fakeUserClient) ValidateUser(ctx context.Context, in *userpb.ValidateUserRequest, opts ...grpc.CallOption) (*userpb.ValidateUserResponse, error) {
	// Returns valid according to configuration
	return &userpb.ValidateUserResponse{Ok: f.ok}, nil
}

// The other methods are not used by the handler, but the interface requires them.
// Minimum implementations for compilation:
func (f *fakeUserClient) CreateUser(context.Context, *userpb.CreateUserRequest, ...grpc.CallOption) (*userpb.UserResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserClient) GetUser(context.Context, *userpb.GetUserRequest, ...grpc.CallOption) (*userpb.UserResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserClient) UpdateUser(context.Context, *userpb.UpdateUserRequest, ...grpc.CallOption) (*userpb.UserResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserClient) DeleteUser(context.Context, *userpb.DeleteUserRequest, ...grpc.CallOption) (*userpb.DeleteUserResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeUserClient) AuthenticateUser(context.Context, *userpb.AuthRequest, ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// productFake sirve GET /products/:id y PUT /products/:id manteniendo stock en memoria.
type productState struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price string `json:"price"`
	Stock int    `json:"stock"`
}

func newProductServer(t *testing.T, initial productState) (*httptest.Server, *productState) {
	t.Helper()
	state := &productState{
		ID:    initial.ID,
		Name:  ifEmpty(initial.Name, "TestProd"),
		Price: ifEmpty(initial.Price, "10.00"),
		Stock: initial.Stock,
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/products/", func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)
		if id != state.ID {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(state)
		case http.MethodPut:
			var body struct {
				Stock *int `json:"stock"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Stock == nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			if *body.Stock < 0 {
				http.Error(w, `{"error":"stock must be non-negative"}`, http.StatusBadRequest)
				return
			}
			state.Stock = *body.Stock
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(state)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	return srv, state
}

func ifEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

//
// ---------- TESTS ----------
//

func TestCreateOrder_HappyPath(t *testing.T) {
	t.Parallel()

	// Fake product con stock suficiente
	prodID := uuid.NewString()
	psrv, pstate := newProductServer(t, productState{
		ID:    prodID,
		Price: "15.00",
		Stock: 5,
	})
	defer psrv.Close()

	// Ext real apuntando al fake Product + fake User válido
	ext := &ord.Ext{
		HTTP:           &http.Client{Timeout: 2 * time.Second},
		User:           &fakeUserClient{ok: true},
		ProductBaseURL: strings.TrimRight(psrv.URL, "/"),
	}

	repo := &stubRepo{}

	// Router con el handler real
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/orders", createOrderHandler(repo, ext))

	// Body: 2 unidades => descuenta stock
	body := fmt.Sprintf(`{"user_id":%q,"items":[{"product_id":%q,"quantity":2}]}`, uuid.NewString(), prodID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	// Se debe haber persistido una orden
	if repo.lastOrder == nil || len(repo.lastItems) != 1 {
		t.Fatalf("no se persistió la orden/items")
	}
	// Stock debe haber bajado a 3
	if pstate.Stock != 3 {
		t.Fatalf("stock esperado=3, real=%d", pstate.Stock)
	}
}

func TestCreateOrder_InsufficientStock(t *testing.T) {
	t.Parallel()

	// Fake product con stock insuficiente (1) y pedimos 2
	prodID := uuid.NewString()
	psrv, _ := newProductServer(t, productState{
		ID:    prodID,
		Price: "10.00",
		Stock: 1,
	})
	defer psrv.Close()

	ext := &ord.Ext{
		HTTP:           &http.Client{Timeout: 2 * time.Second},
		User:           &fakeUserClient{ok: true},
		ProductBaseURL: strings.TrimRight(psrv.URL, "/"),
	}
	repo := &stubRepo{}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/orders", createOrderHandler(repo, ext))

	body := fmt.Sprintf(`{"user_id":%q,"items":[{"product_id":%q,"quantity":2}]}`, uuid.NewString(), prodID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		// Ideal: 409 (insufficient stock). Si tu Ext no valida antes del PUT, podría terminar como 400.
		t.Fatalf("status=%d body=%s (esperaba 409 o 400 según implementación de AdjustStock)", w.Code, w.Body.String())
	}
}

// ===== GET /orders/:id (not found) =====
func TestGetOrder_NotFound(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{} // vacío
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/orders/:id", getOrderHandler(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders/"+uuid.NewString(), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s (esperaba 404)", w.Code, w.Body.String())
	}
}

// ===== GET /orders/:id/items =====
func TestGetOrderItems_OK(t *testing.T) {
	t.Parallel()

	oid := uuid.NewString()
	repo := &stubRepo{
		lastOrder: &ord.Order{ID: oid, UserID: uuid.NewString(), Status: "pending", Total: "20.00"},
		lastItems: []ord.Item{{
			ID:        uuid.NewString(),
			OrderID:   oid,
			ProductID: uuid.NewString(),
			Quantity:  2,
			Price:     "10.00",
		}},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/orders/:id/items", getOrderItemsHandler(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders/"+oid+"/items", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s (esperaba 200)", w.Code, w.Body.String())
	}

	// 1) intenta []ord.Item
	var arr []ord.Item
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err == nil {
		if len(arr) != 1 {
			t.Fatalf("items len=%d, esperaba 1 (formato array)", len(arr))
		}
		return
	}
	// 2) intenta {"items":[...]}
	var wrap struct {
		Items []ord.Item `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if len(wrap.Items) != 1 {
		t.Fatalf("items len=%d, esperaba 1 (formato objeto)", len(wrap.Items))
	}
}

// ===== GET /orders/user/:user_id =====
func TestListOrdersByUser_OK(t *testing.T) {
	t.Parallel()

	uid := uuid.NewString()
	repo := &stubRepo{
		lastOrder: &ord.Order{ID: uuid.NewString(), UserID: uid, Status: "pending", Total: "50.00"},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/orders/user/:user_id", listOrdersByUserHandler(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders/user/"+uid+"?limit=10&offset=0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s (esperaba 200)", w.Code, w.Body.String())
	}

	// 1) intenta []ord.Order
	var arr []ord.Order
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err == nil {
		if len(arr) != 1 {
			t.Fatalf("len=%d, esperaba 1 (formato array). body=%s", len(arr), w.Body.String())
		}
		return
	}

	// 2) intenta {"orders":[...]}
	var wrapOrders struct {
		Orders []ord.Order `json:"orders"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrapOrders); err == nil && len(wrapOrders.Orders) > 0 {
		if len(wrapOrders.Orders) != 1 {
			t.Fatalf("len=%d, esperaba 1 (formato objeto: orders). body=%s", len(wrapOrders.Orders), w.Body.String())
		}
		return
	}

	// 3) intenta {"items":[...]}  (muchos handlers devuelven items)
	var wrapItems struct {
		Items []ord.Order `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrapItems); err == nil && len(wrapItems.Items) > 0 {
		if len(wrapItems.Items) != 1 {
			t.Fatalf("len=%d, esperaba 1 (formato objeto: items). body=%s", len(wrapItems.Items), w.Body.String())
		}
		return
	}

	t.Fatalf("respuesta no coincide con formatos esperados. body=%s", w.Body.String())
}

// ===== PUT /orders/:id/status → canceled (restock) =====
func TestUpdateOrderStatus_PendingToCanceled_Restocks(t *testing.T) {
	t.Parallel()

	prodID := uuid.NewString()
	// stock inicial 3; orden tiene qty=2 → tras cancel debe subir a 5
	psrv, pstate := newProductServer(t, productState{
		ID:    prodID,
		Price: "10.00",
		Stock: 3,
	})
	defer psrv.Close()

	oid := uuid.NewString()
	repo := &stubRepo{
		lastOrder: &ord.Order{ID: oid, UserID: uuid.NewString(), Status: "pending", Total: "20.00"},
		lastItems: []ord.Item{{
			ID:        uuid.NewString(),
			OrderID:   oid,
			ProductID: prodID,
			Quantity:  2,
			Price:     "10.00",
		}},
	}

	ext := &ord.Ext{
		HTTP:           &http.Client{Timeout: 2 * time.Second},
		User:           &fakeUserClient{ok: true},
		ProductBaseURL: strings.TrimRight(psrv.URL, "/"),
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/orders/:id/status", updateOrderStatusHandler(repo, ext))

	body := `{"status":"canceled"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/orders/"+oid+"/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s (esperaba 200)", w.Code, w.Body.String())
	}
	if pstate.Stock != 5 {
		t.Fatalf("restock falló: stock=%d, esperado=5", pstate.Stock)
	}
	if repo.lastOrder.Status != "canceled" {
		t.Fatalf("estado final=%s, esperado=canceled", repo.lastOrder.Status)
	}
}

// ===== PUT /orders/:id/status → shipped (sin restock) =====
func TestUpdateOrderStatus_PendingToShipped_NoRestock(t *testing.T) {
	t.Parallel()

	prodID := uuid.NewString()
	psrv, pstate := newProductServer(t, productState{
		ID:    prodID,
		Price: "10.00",
		Stock: 3,
	})
	defer psrv.Close()

	oid := uuid.NewString()
	repo := &stubRepo{
		lastOrder: &ord.Order{ID: oid, UserID: uuid.NewString(), Status: "pending", Total: "20.00"},
		lastItems: []ord.Item{{
			ID:        uuid.NewString(),
			OrderID:   oid,
			ProductID: prodID,
			Quantity:  2,
			Price:     "10.00",
		}},
	}

	ext := &ord.Ext{
		HTTP:           &http.Client{Timeout: 2 * time.Second},
		User:           &fakeUserClient{ok: true},
		ProductBaseURL: strings.TrimRight(psrv.URL, "/"),
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/orders/:id/status", updateOrderStatusHandler(repo, ext))

	body := `{"status":"paid"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/orders/"+oid+"/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s (esperaba 200)", w.Code, w.Body.String())
	}
	if pstate.Stock != 3 { // no cambia
		t.Fatalf("stock cambió y no debía: stock=%d", pstate.Stock)
	}
	if repo.lastOrder.Status != "paid" {
		t.Fatalf("estado final=%s, esperado=paid", repo.lastOrder.Status)
	}
}

// ===== PUT /orders/:id/status → estado inválido =====
func TestUpdateOrderStatus_InvalidStatus(t *testing.T) {
	t.Parallel()

	oid := uuid.NewString()
	repo := &stubRepo{
		lastOrder: &ord.Order{ID: oid, UserID: uuid.NewString(), Status: "pending", Total: "20.00"},
	}
	ext := &ord.Ext{} // no se usa en esta ruta para validar estado

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/orders/:id/status", updateOrderStatusHandler(repo, ext))

	body := `{"status":"wtf"}` // inválido
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/orders/"+oid+"/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s (esperaba 400)", w.Code, w.Body.String())
	}
}

func init() {
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	log.SetOutput(io.Discard)
}
