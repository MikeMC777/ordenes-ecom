package order

// CreateOrderItem payload de ítem.
// swagger:model CreateOrderItem
type CreateOrderItem struct {
	ProductID string `json:"product_id" example:"4e7d4e5c-5cb9-4a3f-9f21-7e1a4f9f2b2a"`
	Quantity  int    `json:"quantity"  example:"2"`
}

// CreateOrderRequest payload de creación de orden.
// swagger:model CreateOrderRequest
type CreateOrderRequest struct {
	UserID string            `json:"user_id" example:"b2f5ff47-2b1e-4f22-8a96-5f3c1f2f2e7b"`
	Items  []CreateOrderItem `json:"items"`
}
