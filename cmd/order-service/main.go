// @title          Order Service API
// @version        1.0
// @description    REST API for order lifecycle (create, query).
// @BasePath       /

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	_ "github.com/MikeMC777/ordenes-ecom/docs-order"
	"github.com/MikeMC777/ordenes-ecom/internal/config"
	ord "github.com/MikeMC777/ordenes-ecom/internal/order"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type HTTPError struct {
	Error string `json:"error"`
}

// createOrderHandler godoc
// @Summary      Create order
// @Description  Validates user, checks stock, decrements inventory, and stores order & items.
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        body  body      order.CreateOrderRequest  true  "user_id & items"
// @Success      201   {object}  map[string]interface{}
// @Failure      400   {object}  HTTPError
// @Failure      409   {object}  HTTPError
// @Failure      500   {object}  HTTPError
// @Router       /orders [post]
func createOrderHandler(repo ord.Repository, ext *ord.Ext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			UserID string `json:"user_id"`
			Items  []struct {
				ProductID string `json:"product_id"`
				Quantity  int    `json:"quantity"`
			} `json:"items"`
		}
		if err := c.BindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{"invalid json"})
			return
		}
		if in.UserID == "" || len(in.Items) == 0 {
			c.JSON(http.StatusBadRequest, HTTPError{"user_id & items required"})
			return
		}

		// validate user (gRPC)
		ok, err := ext.ValidateUser(c.Request.Context(), in.UserID)
		if err != nil || !ok {
			c.JSON(http.StatusBadRequest, HTTPError{"invalid user"})
			return
		}

		// calculate total, freeze price, and adjust stock (automatic)
		total := decimal.Zero
		type decRec struct {
			ProductID string
			Qty       int
		}
		var toRollback []decRec
		priceByProduct := make(map[string]string, len(in.Items)) // freeze unit price by product_id

		for _, it := range in.Items {
			if it.ProductID == "" || it.Quantity <= 0 {
				c.JSON(http.StatusBadRequest, HTTPError{"invalid item"})
				return
			}

			// 1) Bring product (price/current stock)
			p, err := ext.FetchProduct(c.Request.Context(), it.ProductID)
			if err != nil {
				c.JSON(http.StatusBadRequest, HTTPError{"product not found"})
				return
			}

			// 2) Freeze price and accumulate total
			priceDec, err := decimal.NewFromString(p.Price)
			if err != nil {
				c.JSON(http.StatusInternalServerError, HTTPError{"invalid product price"})
				return
			}
			line := priceDec.Mul(decimal.NewFromInt(int64(it.Quantity)))
			total = total.Add(line)
			priceByProduct[it.ProductID] = priceDec.StringFixed(2)

			// 3) Automatically adjust stock with PUT /products/{id} (negative delta)
			if err := ext.AdjustStock(c.Request.Context(), it.ProductID, -it.Quantity); err != nil {
				// rollback of what has already been deducted
				for i := len(toRollback) - 1; i >= 0; i-- {
					_ = ext.AdjustStock(c.Request.Context(), toRollback[i].ProductID, +toRollback[i].Qty)
				}
				if err.Error() == "insufficient stock" {
					c.JSON(http.StatusConflict, HTTPError{"insufficient stock"})
					return
				}
				c.JSON(http.StatusBadRequest, HTTPError{"product not found"})
				return
			}
			toRollback = append(toRollback, decRec{ProductID: it.ProductID, Qty: it.Quantity})
		}

		// The order + items (unit price “frozen”) persists.
		var items []ord.Item
		for _, it := range in.Items {
			items = append(items, ord.Item{
				ID:        uuid.NewString(),
				OrderID:   "", // set below
				ProductID: it.ProductID,
				Quantity:  it.Quantity,
				Price:     priceByProduct[it.ProductID], // <- we keep the price frozen
			})
		}
		o := &ord.Order{
			ID:     uuid.NewString(),
			UserID: in.UserID,
			Status: "pending",
			Total:  total.StringFixed(2),
		}
		for i := range items {
			items[i].OrderID = o.ID
		}

		if err := repo.Create(c.Request.Context(), o, items); err != nil {
			// rollback stock if persistence fails
			for i := len(toRollback) - 1; i >= 0; i-- {
				_ = ext.AdjustStock(c.Request.Context(), toRollback[i].ProductID, +toRollback[i].Qty)
			}
			c.JSON(http.StatusInternalServerError, HTTPError{"create order error"})
			return
		}

		outOrder, outItems, _ := repo.GetByID(c.Request.Context(), o.ID)
		c.JSON(http.StatusCreated, gin.H{"order": outOrder, "items": outItems})
	}
}

// getOrderHandler godoc
// @Summary      Get order by ID
// @Tags         orders
// @Param        id   path      string  true  "Order ID (UUID)"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  HTTPError
// @Router       /orders/{id} [get]
func getOrderHandler(repo ord.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		o, items, err := repo.GetByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, HTTPError{"not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"order": o, "items": items})
	}
}

// listOrdersByUserHandler godoc
// @Summary      List orders by user
// @Tags         orders
// @Param        user_id  path   string  true   "User ID (UUID)"
// @Param        limit    query  int     false  "Limit (1-100)"  minimum(1) maximum(100) default(20)
// @Param        offset   query  int     false  "Offset (>=0)"   minimum(0) default(0)
// @Success      200      {object}  map[string]interface{}
// @Failure      500      {object}  HTTPError
// @Router       /orders/user/{user_id} [get]
func listOrdersByUserHandler(repo ord.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}
		list, err := repo.ListByUser(c.Request.Context(), c.Param("user_id"), limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, HTTPError{"list error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": list, "limit": limit, "offset": offset})
	}
}

// updateOrderStatusHandler godoc
// @Summary      Update order status
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        id    path   string              true  "Order ID (UUID)"
// @Param        body  body   map[string]string   true  "status: pending|paid|canceled"
// @Success      200   {object}  map[string]interface{}
// @Failure      400   {object}  HTTPError
// @Failure      404   {object}  HTTPError
// @Failure      500   {object}  HTTPError
// @Router       /orders/{id}/status [put]
func updateOrderStatusHandler(repo ord.Repository, ext *ord.Ext) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var in struct {
			Status string `json:"status"`
		}
		if err := c.BindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{"invalid json"})
			return
		}

		// normalize and validate
		newStatus := strings.ToLower(strings.TrimSpace(in.Status))
		allowed := map[string]bool{"pending": true, "paid": true, "canceled": true}
		if !allowed[newStatus] {
			c.JSON(http.StatusBadRequest, HTTPError{"invalid status"})
			return
		}

		// current status + items
		o, items, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, HTTPError{"not found"})
			return
		}
		if o.Status == newStatus {
			// nothing to change
			c.JSON(http.StatusOK, gin.H{"order": o, "items": items})
			return
		}

		// rollback stock only if we go from pending to canceled
		if o.Status == "pending" && newStatus == "canceled" {
			for _, it := range items {
				// best-effort: if any setting fails, we continue
				_ = ext.AdjustStock(c.Request.Context(), it.ProductID, +it.Quantity)
			}
		}

		// update status in DB
		if err := repo.UpdateStatus(c.Request.Context(), id, newStatus); err != nil {
			if err == ord.ErrNotFound {
				c.JSON(http.StatusNotFound, HTTPError{"not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, HTTPError{"update status error"})
			return
		}

		// returns the updated order
		o2, items2, _ := repo.GetByID(c.Request.Context(), id)
		c.JSON(http.StatusOK, gin.H{"order": o2, "items": items2})
	}
}

// getOrderItemsHandler godoc
// @Summary      Order items
// @Tags         orders
// @Param        id   path   string  true  "Order ID (UUID)"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  HTTPError
// @Router       /orders/{id}/items [get]
func getOrderItemsHandler(repo ord.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// validate order existence
		if o, _, err := repo.GetByID(c.Request.Context(), c.Param("id")); err != nil || o == nil {
			c.JSON(http.StatusNotFound, HTTPError{"not found"})
			return
		}
		items, err := repo.GetItems(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, HTTPError{"items error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	ext, err := ord.NewExt(cfg.UserSvcAddr, cfg.ProductSvcBaseURL)
	if err != nil {
		log.Fatalf("ext clients: %v", err)
	}

	repo := ord.NewPGRepo(pool)

	// Gin
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// POST /orders  — create an order by verifying user and stock
	// Create
	r.POST("/orders", createOrderHandler(repo, ext))

	// Get order by ID
	r.GET("/orders/:id", getOrderHandler(repo))

	// List orders by user
	r.GET("/orders/user/:user_id", listOrdersByUserHandler(repo))

	// Update order status
	r.PUT("/orders/:id/status", updateOrderStatusHandler(repo, ext))

	//Get order items
	r.GET("/orders/:id/items", getOrderItemsHandler(repo))

	srv := &http.Server{Addr: cfg.ProductSvcBaseURL /* placeholder to reuse config? set your ORDER_SERVICE_ADDR */, Handler: r, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}

	go func() {
		addr := ":8082" // or cfg.OrderSvcAddr
		srv.Addr = addr
		log.Printf("[http] order-service listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctxSh, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctxSh)
}
