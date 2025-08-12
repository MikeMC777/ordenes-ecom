// @title          Product Service API
// @version        1.0
// @description    REST API for product management (listing, search, CRUD).
// @BasePath       /

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/MikeMC777/ordenes-ecom/docs"
	"github.com/MikeMC777/ordenes-ecom/internal/config"
	"github.com/MikeMC777/ordenes-ecom/internal/product"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// listOnlyHandler godoc
// @Summary      List products (pagination only)
// @Description  Returns a paginated list ordered by creation date. No search filter applied.
// @Tags         products
// @Param        limit   query     int     false  "Limit (1-100)"  minimum(1) maximum(100) default(20)
// @Param        offset  query     int     false  "Offset (>=0)"   minimum(0) default(0)
// @Success      200     {object}  product.ListResponse
// @Failure      500     {object}  product.HTTPError
// @Router       /products [get]
func listOnlyHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if offset < 0 {
			offset = 0
		}

		// Empty search force: pagination only
		items, err := repo.List(c.Request.Context(), product.Query{Q: "", Limit: limit, Offset: offset})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "list error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"limit": limit, "offset": offset, "items": items})
	}
}

// searchHandler godoc
// @Summary      Search products (pagination + query)
// @Description  Returns a paginated list filtered by 'q' on name/description (ILIKE).
// @Tags         products
// @Param        q       query     string  true   "Search text (min 2 chars)"
// @Param        limit   query     int     false  "Limit (1-100)"  minimum(1) maximum(100) default(20)
// @Param        offset  query     int     false  "Offset (>=0)"   minimum(0) default(0)
// @Success      200     {object}  product.ListResponse
// @Failure      400     {object}  product.HTTPError
// @Failure      500     {object}  product.HTTPError
// @Router       /products/search [get]
func searchHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		if len(q) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "q is required (min 2 chars)"})
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if offset < 0 {
			offset = 0
		}

		items, err := repo.List(c.Request.Context(), product.Query{Q: q, Limit: limit, Offset: offset})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "search error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"q": q, "limit": limit, "offset": offset, "items": items})
	}
}

// getProduct godoc
// @Summary      Get product by ID
// @Tags         products
// @Param        id   path      string  true  "Product ID (UUID)"
// @Success      200  {object}  product.Product
// @Failure      404  {object}  product.HTTPError
// @Router       /products/{id} [get]
func getProductHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		p, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, p)
	}
}

// createProduct godoc
// @Summary      Create product
// @Tags         products
// @Accept       json
// @Produce      json
// @Param        body  body      product.CreateProductRequest  true  "name (req), price (req), description, stock"
// @Success      201   {object}  product.Product
// @Failure      400   {object}  product.HTTPError
// @Failure      500   {object}  product.HTTPError
// @Router       /products [post]
func createProductHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in product.CreateProductRequest
		// Bind JSON and validate
		if err := c.BindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		if in.Name == "" || in.Price == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name and price are required"})
			return
		}
		if in.Stock < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "stock must be >= 0"})
			return
		}
		p := &product.Product{
			ID:          uuid.NewString(),
			Name:        in.Name,
			Description: in.Description,
			Price:       in.Price,
			Stock:       in.Stock,
		}
		if err := repo.Create(c.Request.Context(), p); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create error"})
			return
		}
		// return the created one
		out, _ := repo.GetByID(c.Request.Context(), p.ID)
		c.JSON(http.StatusCreated, out)
	}
}

// updateProduct godoc
// @Summary      Update product (partial)
// @Description  If 'price' is not provided, it is not modified. Empty fields do not change.
// @Tags         products
// @Accept       json
// @Produce      json
// @Param        id    path      string                         true  "Product ID (UUID)"
// @Param        body  body      product.UpdateProductRequest   true  "name, description, price, stock"
// @Success      200   {object}  product.Product
// @Failure      400   {object}  product.HTTPError
// @Failure      404   {object}  product.HTTPError
// @Failure      500   {object}  product.HTTPError
// @Router       /products/{id} [put]
func updateProductHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var in product.UpdateProductRequest
		if err := c.BindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		updatePrice := in.Price != ""
		p := &product.Product{
			ID:          id,
			Name:        in.Name,
			Description: in.Description,
			Price:       in.Price,
			Stock:       in.Stock,
		}
		if err := repo.Update(c.Request.Context(), p, updatePrice); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update error"})
			return
		}
		out, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// deleteProduct godoc
// @Summary      Delete product by ID
// @Description  Deletes a product by its ID (UUID).
// @Tags         products
// @Param        id   path      string  true  "Product ID (UUID)"
// @Success      204  "No Content"
// @Failure      404  {object}  product.HTTPError
// @Failure      500  {object}  product.HTTPError
// @Router       /products/{id} [delete]
func deleteProductHandler(repo product.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		ok, err := repo.Delete(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "delete error"})
			return
		}
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func main() {
	cfg := config.Load()

	// DB
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

	repo := product.NewPGRepo(pool)

	// Gin
	r := gin.New()
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.Use(gin.Logger(), gin.Recovery())

	// Health
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// List
	r.GET("/products", listOnlyHandler(repo))

	// Search
	r.GET("/products/search", searchHandler(repo))

	// Get product by ID
	r.GET("/products/:id", getProductHandler(repo))

	// Create
	r.POST("/products", createProductHandler(repo))

	//Update
	r.PUT("/products/:id", updateProductHandler(repo))

	// Delete
	r.DELETE("/products/:id", deleteProductHandler(repo))

	// Server + Graceful shutdown
	srv := &http.Server{
		Addr:         cfg.ProductSvcAddr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("[http] product-service listening on %s", cfg.ProductSvcAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] error: %v", err)
		}
	}()

	// Signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("[http] shutting down...")
	ctxShutdown, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctxShutdown)
}
