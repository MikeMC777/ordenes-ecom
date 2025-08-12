package product

import "time"

type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// We store price as a string to avoid rounding errors (NUMERIC in Postgres)
	Price     string    `json:"price"`
	Stock     int       `json:"stock"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HTTPError represents a standard error in JSON.
// swagger:model
type HTTPError struct {
	// Error message
	// example: not found
	Error string `json:"error"`
}

// ListResponse represents the paginated response of products.
// swagger:model
type ListResponse struct {
	// search query applied
	Q string `json:"q,omitempty"` // <-- omitempty
	// limit applied
	Limit int `json:"limit"`
	// offset applied
	Offset int `json:"offset"`
	// total items found
	Items []Product `json:"items"`
}

// CreateProductRequest payload of creation.
// swagger:model CreateProductRequest
type CreateProductRequest struct {
	Name        string `json:"name"        example:"Mecanical Keyboard"`
	Description string `json:"description" example:"RGB 60%"`
	Price       string `json:"price"       example:"199.90"`
	Stock       int    `json:"stock"       example:"10"`
}

// UpdateProductRequest payload of partial update.
// swagger:model UpdateProductRequest
type UpdateProductRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       string `json:"price"`
	Stock       int    `json:"stock"`
}
