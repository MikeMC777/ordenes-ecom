package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	prod "github.com/MikeMC777/ordenes-ecom/internal/product"
)

//
// ===== STUB REPO EN MEMORIA (implementa product.Repository) =====
//

type stubRepo struct {
	items     map[string]*prod.Product
	lastQuery prod.Query
}

func newStubRepo() *stubRepo {
	return &stubRepo{items: make(map[string]*prod.Product)}
}

func (s *stubRepo) List(ctx context.Context, q prod.Query) ([]prod.Product, error) {
	s.lastQuery = q
	out := make([]prod.Product, 0, len(s.items))
	for _, v := range s.items {
		// filtro mínimo por nombre/descr cuando Q viene con search
		if q.Q != "" {
			if !containsFold(v.Name, q.Q) && !containsFold(v.Description, q.Q) {
				continue
			}
		}
		out = append(out, *v)
	}
	// paginación simple
	start := q.Offset
	if start > len(out) {
		return []prod.Product{}, nil
	}
	end := start + q.Limit
	if end > len(out) || q.Limit <= 0 {
		end = len(out)
	}
	return out[start:end], nil
}

func (s *stubRepo) GetByID(ctx context.Context, id string) (*prod.Product, error) {
	p, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *p
	return &cp, nil
}

func (s *stubRepo) Create(ctx context.Context, p *prod.Product) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Name == "" || p.Price == "" || p.Stock < 0 {
		return fmt.Errorf("invalid")
	}
	cp := *p
	cp.CreatedAt = time.Now().UTC()
	cp.UpdatedAt = cp.CreatedAt
	s.items[p.ID] = &cp
	return nil
}

// Nota: como tu handler no distingue "stock omitido" (usa int, no *int), este stub
// siempre pisa el stock con el valor recibido (incluido 0).
func (s *stubRepo) Update(ctx context.Context, p *prod.Product, updatePrice bool) error {
	cur, ok := s.items[p.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if p.Name != "" {
		cur.Name = p.Name
	}
	if p.Description != "" {
		cur.Description = p.Description
	}
	if updatePrice && p.Price != "" {
		cur.Price = p.Price
	}
	if p.Stock < 0 {
		return fmt.Errorf("invalid stock")
	}
	cur.Stock = p.Stock
	cur.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *stubRepo) Delete(ctx context.Context, id string) (bool, error) {
	if _, ok := s.items[id]; !ok {
		return false, nil
	}
	delete(s.items, id)
	return true, nil
}

func containsFold(s, sub string) bool {
	return bytes.Contains(bytes.ToLower([]byte(s)), bytes.ToLower([]byte(sub)))
}

//
// ===== ROUTER de pruebas que usa TUS handlers del main =====
//

func newRouter(repo prod.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Igual que tu main:
	r.GET("/products", listOnlyHandler(repo))
	r.GET("/products/search", searchHandler(repo))
	r.GET("/products/:id", getProductHandler(repo))
	r.POST("/products", createProductHandler(repo))
	r.PUT("/products/:id", updateProductHandler(repo))
	r.DELETE("/products/:id", deleteProductHandler(repo))
	return r
}

//
// ===== TESTS =====
//

// /products → paginación SOLAMENTE (no debe mandar Q al repo)
func TestListProducts_PaginationOnly_NoSearch(t *testing.T) {
	repo := newStubRepo()
	for i := 1; i <= 3; i++ {
		_ = repo.Create(context.Background(), &prod.Product{
			ID:          fmt.Sprintf("%d", i),
			Name:        fmt.Sprintf("Prod %d", i),
			Description: "desc",
			Price:       "10.00",
			Stock:       5,
		})
	}
	r := newRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/products?limit=2&offset=1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Items  []prod.Product `json:"items"`
		Limit  int            `json:"limit"`
		Offset int            `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("len=%d, esperado=2", len(got.Items))
	}
	if repo.lastQuery.Q != "" {
		t.Fatalf("listOnlyHandler no debe aplicar búsqueda; Q=%q", repo.lastQuery.Q)
	}
}

// /products/search → exige q (≥2); devuelve filtrado + paginado
func TestSearchProducts_RequiresQAndFilters(t *testing.T) {
	repo := newStubRepo()
	_ = repo.Create(context.Background(), &prod.Product{ID: "a", Name: "Mouse Pro", Description: "inalámbrico", Price: "99.90", Stock: 5})
	_ = repo.Create(context.Background(), &prod.Product{ID: "b", Name: "Teclado", Description: "mecánico", Price: "149.90", Stock: 3})
	r := newRouter(repo)

	// falta q ⇒ 400
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/products/search?limit=10&offset=0", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400 por q faltante, got %d", w.Code)
		}
	}

	// q demasiado corta ⇒ 400
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/products/search?q=m&limit=10&offset=0", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400 por q corta, got %d", w.Code)
		}
	}

	// q válida ⇒ 200 + 1 resultado (Mouse Pro)
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/products/search?q=mo&limit=10&offset=0", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var got struct {
			Q      string         `json:"q"`
			Items  []prod.Product `json:"items"`
			Limit  int            `json:"limit"`
			Offset int            `json:"offset"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got.Q == "" || len(got.Items) != 1 || got.Items[0].ID != "a" {
			t.Fatalf("resultado inesperado: q=%q items=%+v", got.Q, got.Items)
		}
		if repo.lastQuery.Q == "" {
			t.Fatalf("debió enviarse Q al repo en search")
		}
	}
}

// /products/:id
func TestGetProduct_OK_And_NotFound(t *testing.T) {
	repo := newStubRepo()
	_ = repo.Create(context.Background(), &prod.Product{ID: "x", Name: "Headset", Price: "149.90", Stock: 7})
	r := newRouter(repo)

	// OK
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/products/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}

	// 404
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/products/nope", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("esperaba 404, got %d body=%s", w.Code, w.Body.String())
		}
	}
}

// POST /products
func TestCreateProduct_Valid_And_Invalid(t *testing.T) {
	repo := newStubRepo()
	r := newRouter(repo)

	// válido
	valid := `{"name":"Starter Kit","description":"Básico","price":"49.90","stock":10}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/products", bytes.NewBufferString(valid))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}

	// inválido: falta name/price
	invalid := `{"description":"x","stock":1}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/products", bytes.NewBufferString(invalid))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400, got %d body=%s", w.Code, w.Body.String())
		}
	}

	// inválido: stock negativo
	neg := `{"name":"Bad","price":"1.00","stock":-1}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/products", bytes.NewBufferString(neg))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400 por stock negativo, got %d body=%s", w.Code, w.Body.String())
		}
	}
}

// PUT /products/:id (parcial). En tu handler: si no envías price, NO se modifica.
func TestUpdateProduct_Partial_WithAndWithoutPrice(t *testing.T) {
	repo := newStubRepo()
	_ = repo.Create(context.Background(), &prod.Product{ID: "p", Name: "Mouse", Price: "10.00", Stock: 5})
	r := newRouter(repo)

	// sin price (no cambia el price); aquí enviamos stock explícito (tu handler no distingue omitido)
	up1 := `{"name":"Mouse 2","stock":4}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/products/p", bytes.NewBufferString(up1))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		got, _ := repo.GetByID(context.Background(), "p")
		if got.Name != "Mouse 2" || got.Price != "10.00" || got.Stock != 4 {
			t.Fatalf("update sin price no respetado: %+v", got)
		}
	}

	// con price (sí cambia price)
	up2 := `{"price":"12.50","stock":4}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/products/p", bytes.NewBufferString(up2))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		got, _ := repo.GetByID(context.Background(), "p")
		if got.Price != "12.50" {
			t.Fatalf("update con price no aplicado: %+v", got)
		}
	}

	// inválido: stock negativo ⇒ 400
	upBad := `{"stock":-3}`
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/products/p", bytes.NewBufferString(upBad))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400 por stock negativo, got %d body=%s", w.Code, w.Body.String())
		}
	}
}

// DELETE /products/:id
func TestDeleteProduct_OK_And_NotFound(t *testing.T) {
	repo := newStubRepo()
	_ = repo.Create(context.Background(), &prod.Product{ID: "del", Name: "X", Price: "1.00", Stock: 1})
	r := newRouter(repo)

	// OK
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/products/del", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}

	// 404
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/products/nope", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("esperaba 404, got %d body=%s", w.Code, w.Body.String())
		}
	}
}
