package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/handler"
	"govershop-api/internal/middleware"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/pakasir"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Println("🚀 Starting Govershop API Server...")
	log.Printf("📍 Environment: %s", cfg.Env)

	// Connect to database
	db, err := config.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer config.CloseDB()

	// Initialize services
	digiflazzSvc := digiflazz.NewService(cfg)
	pakasirSvc := pakasir.NewService(cfg)

	// Initialize repositories
	productRepo := repository.NewProductRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	webhookRepo := repository.NewWebhookLogRepository(db)
	syncLogRepo := repository.NewSyncLogRepository(db)

	// Initialize handlers
	productHandler := handler.NewProductHandler(productRepo)
	orderHandler := handler.NewOrderHandler(orderRepo, paymentRepo, productRepo, digiflazzSvc, pakasirSvc)
	webhookHandler := handler.NewWebhookHandler(cfg, orderRepo, paymentRepo, webhookRepo, digiflazzSvc)
	adminHandler := handler.NewAdminHandler(cfg, digiflazzSvc, productRepo, orderRepo, syncLogRepo)
	validationHandler := handler.NewValidationHandler(cfg, productRepo, digiflazzSvc)

	// Setup router
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Swagger Documentation
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "docs/index.html")
	})
	mux.HandleFunc("GET /docs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "docs/index.html")
	})
	mux.HandleFunc("GET /docs/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		http.ServeFile(w, r, "docs/swagger.yaml")
	})

	// ==========================================
	// PUBLIC ROUTES (No auth required)
	// ==========================================

	// Product endpoints
	mux.HandleFunc("GET /api/v1/products", productHandler.GetProducts)
	mux.HandleFunc("GET /api/v1/products/categories", productHandler.GetCategories)
	mux.HandleFunc("GET /api/v1/products/brands", productHandler.GetBrands)
	mux.HandleFunc("GET /api/v1/products/{sku}", productHandler.GetProductBySKU)

	// Validation endpoints
	mux.HandleFunc("POST /api/v1/validate-account", validationHandler.ValidateAccount)
	mux.HandleFunc("POST /api/v1/calculate-price", validationHandler.CalculatePrice)

	// Order endpoints
	mux.HandleFunc("POST /api/v1/orders", orderHandler.CreateOrder)
	mux.HandleFunc("GET /api/v1/orders/{id}", orderHandler.GetOrder)
	mux.HandleFunc("POST /api/v1/orders/{id}/pay", orderHandler.InitiatePayment)
	mux.HandleFunc("POST /api/v1/orders/{id}/cancel", orderHandler.CancelOrder)
	mux.HandleFunc("GET /api/v1/orders/{id}/status", orderHandler.GetOrderStatus)

	// Payment methods
	mux.HandleFunc("GET /api/v1/payment-methods", orderHandler.GetPaymentMethods)

	// ==========================================
	// WEBHOOK ROUTES
	// ==========================================
	mux.HandleFunc("POST /api/v1/webhook/pakasir", webhookHandler.HandlePakasirWebhook)
	mux.HandleFunc("POST /api/v1/webhook/digiflazz", webhookHandler.HandleDigiflazzWebhook)

	// ==========================================
	// ADMIN ROUTES (TODO: Add auth middleware)
	// ==========================================
	mux.HandleFunc("GET /api/v1/admin/balance", adminHandler.GetBalance)
	mux.HandleFunc("GET /api/v1/admin/dashboard", adminHandler.GetDashboard)
	mux.HandleFunc("GET /api/v1/admin/orders", adminHandler.GetOrders)
	mux.HandleFunc("POST /api/v1/admin/sync/products", adminHandler.SyncProducts)

	// Admin Product CRUD
	mux.HandleFunc("GET /api/v1/admin/products", adminHandler.GetAdminProducts)
	mux.HandleFunc("GET /api/v1/admin/products/tags", adminHandler.GetAllTags)
	mux.HandleFunc("GET /api/v1/admin/products/best-sellers", adminHandler.GetBestSellers)
	mux.HandleFunc("GET /api/v1/admin/products/{sku}", adminHandler.GetAdminProduct)
	mux.HandleFunc("PUT /api/v1/admin/products/{sku}", adminHandler.UpdateAdminProduct)
	mux.HandleFunc("POST /api/v1/admin/products/{sku}/tags", adminHandler.AddProductTag)
	mux.HandleFunc("DELETE /api/v1/admin/products/{sku}/tags/{tag}", adminHandler.RemoveProductTag)

	// Apply middleware to API routes
	var apiHandler http.Handler = mux
	apiHandler = middleware.ContentTypeJSON(apiHandler)
	apiHandler = middleware.Logger(apiHandler)
	apiHandler = middleware.CORS(apiHandler)
	apiHandler = middleware.Recoverer(apiHandler)

	// Create main handler that routes docs separately
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve docs without JSON middleware
		if r.URL.Path == "/docs" || r.URL.Path == "/docs/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, "docs/index.html")
			return
		}
		if r.URL.Path == "/docs/swagger.yaml" {
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			http.ServeFile(w, r, "docs/swagger.yaml")
			return
		}
		// All other routes go through middleware chain
		apiHandler.ServeHTTP(w, r)
	})

	// Create server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mainHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("✅ Server running on http://localhost:%s", cfg.Port)
		log.Println("📋 API Documentation:")
		log.Println("   GET  /health                      - Health check")
		log.Println("   GET  /api/v1/products             - List products")
		log.Println("   GET  /api/v1/products/categories  - List categories")
		log.Println("   GET  /api/v1/products/brands      - List brands")
		log.Println("   POST /api/v1/orders               - Create order")
		log.Println("   GET  /api/v1/orders/{id}          - Get order")
		log.Println("   POST /api/v1/orders/{id}/pay      - Initiate payment")
		log.Println("   POST /api/v1/orders/{id}/cancel   - Cancel order")
		log.Println("   GET  /api/v1/admin/balance        - Check Digiflazz balance")
		log.Println("   POST /api/v1/admin/sync/products  - Sync products")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("⏳ Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("👋 Server exited")
}
