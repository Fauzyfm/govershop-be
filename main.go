package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"embed"
	"govershop-api/internal/config"
	"govershop-api/internal/handler"
	"govershop-api/internal/middleware"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/email"
	"govershop-api/internal/service/pakasir"
)

//go:embed docs/*
var docsFS embed.FS

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
	emailSvc := email.NewService(cfg)

	// Initialize repositories
	productRepo := repository.NewProductRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	webhookRepo := repository.NewWebhookLogRepository(db)
	syncLogRepo := repository.NewSyncLogRepository(db)
	contentRepo := repository.NewContentRepository(db)
	adminSecurityRepo := repository.NewAdminSecurityRepository(db)
	userRepo := repository.NewUserRepository(db)

	// Initialize handlers
	productHandler := handler.NewProductHandler(productRepo)
	orderHandler := handler.NewOrderHandler(orderRepo, paymentRepo, productRepo, digiflazzSvc, pakasirSvc)
	webhookHandler := handler.NewWebhookHandler(cfg, orderRepo, paymentRepo, webhookRepo, userRepo, digiflazzSvc)
	adminHandler := handler.NewAdminHandler(cfg, digiflazzSvc, productRepo, orderRepo, syncLogRepo, paymentRepo, pakasirSvc, webhookRepo, userRepo)

	// Start background jobs
	adminHandler.StartSyncJob(context.Background())

	validationHandler := handler.NewValidationHandler(cfg, productRepo, orderRepo, digiflazzSvc)
	contentHandler := handler.NewContentHandler(contentRepo)
	totpHandler := handler.NewTOTPHandler(cfg, adminSecurityRepo, orderRepo, paymentRepo, digiflazzSvc)
	memberHandler := handler.NewMemberHandler(cfg, userRepo, productRepo, orderRepo, digiflazzSvc, emailSvc)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(cfg)

	// Setup router
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Swagger Documentation
	// read from embedded FS
	indexHTML, _ := docsFS.ReadFile("docs/index.html")
	swaggerYAML, _ := docsFS.ReadFile("docs/swagger.yaml")

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("GET /docs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("GET /docs/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write(swaggerYAML)
	})

	// ==========================================
	// PUBLIC ROUTES (No auth required)
	// ==========================================

	// Product endpoints
	mux.HandleFunc("GET /api/v1/products", productHandler.GetProducts)
	mux.HandleFunc("GET /api/v1/products/filters", productHandler.GetFilters)
	mux.HandleFunc("GET /api/v1/products/categories", productHandler.GetCategories)
	mux.HandleFunc("GET /api/v1/products/brands", productHandler.GetBrands)
	mux.HandleFunc("GET /api/v1/products/{sku}", productHandler.GetProductBySKU)

	// Content endpoints (Public)
	mux.HandleFunc("GET /api/v1/content/carousel", contentHandler.GetCarousel)
	mux.HandleFunc("GET /api/v1/content/brands", contentHandler.GetBrandImages)
	mux.HandleFunc("GET /api/v1/content/popup", contentHandler.GetPopup)
	mux.HandleFunc("GET /api/v1/brands/{brand}", contentHandler.GetPublicBrandSetting)

	// Validation endpoints
	mux.HandleFunc("POST /api/v1/validate-account", validationHandler.ValidateAccount)
	mux.HandleFunc("POST /api/v1/calculate-price", validationHandler.CalculatePrice)

	// Order endpoints
	mux.HandleFunc("POST /api/v1/orders", orderHandler.CreateOrder)
	mux.HandleFunc("GET /api/v1/orders/{id}", orderHandler.GetOrder)
	mux.HandleFunc("POST /api/v1/orders/{id}/pay", orderHandler.InitiatePayment)
	mux.HandleFunc("POST /api/v1/orders/{id}/cancel", orderHandler.CancelOrder)
	mux.HandleFunc("GET /api/v1/orders/{id}/status", orderHandler.GetOrderStatus)
	mux.HandleFunc("GET /api/v1/orders/track", orderHandler.TrackOrders)

	// Payment methods
	mux.HandleFunc("GET /api/v1/payment-methods", orderHandler.GetPaymentMethods)

	// Admin Auth (Public)
	mux.HandleFunc("POST /api/v1/admin/login", adminHandler.Login)

	// Member Auth (Public)
	mux.HandleFunc("POST /api/v1/member/login", memberHandler.Login)
	mux.HandleFunc("POST /api/v1/member/forgot-password", memberHandler.ForgotPassword)
	mux.HandleFunc("POST /api/v1/member/reset-password", memberHandler.ResetPassword)

	// ==========================================
	// WEBHOOK ROUTES
	// ==========================================
	mux.HandleFunc("POST /api/v1/webhook/pakasir", webhookHandler.HandlePakasirWebhook)
	mux.HandleFunc("POST /api/v1/webhook/digiflazz", webhookHandler.HandleDigiflazzWebhook)

	// ==========================================
	// ADMIN ROUTES (Protected with Auth Middleware)
	// ==========================================
	mux.HandleFunc("GET /api/v1/admin/balance", authMiddleware.AdminAuth(adminHandler.GetBalance))
	mux.HandleFunc("GET /api/v1/admin/dashboard", authMiddleware.AdminAuth(adminHandler.GetDashboard))
	mux.HandleFunc("GET /api/v1/admin/orders", authMiddleware.AdminAuth(adminHandler.GetOrders))
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/check-status", authMiddleware.AdminAuth(adminHandler.CheckOrderStatus))
	mux.HandleFunc("POST /api/v1/admin/sync/products", authMiddleware.AdminAuth(adminHandler.SyncProducts))
	mux.HandleFunc("GET /api/v1/admin/logs/sync", authMiddleware.AdminAuth(adminHandler.GetSyncLogs))
	mux.HandleFunc("GET /api/v1/admin/logs/webhook", authMiddleware.AdminAuth(adminHandler.GetWebhookLogs))

	// Admin Product CRUD
	mux.HandleFunc("GET /api/v1/admin/products", authMiddleware.AdminAuth(adminHandler.GetAdminProducts))
	mux.HandleFunc("GET /api/v1/admin/products/filters", authMiddleware.AdminAuth(adminHandler.GetProductFilters))
	mux.HandleFunc("GET /api/v1/admin/products/tags", authMiddleware.AdminAuth(adminHandler.GetAllTags))
	mux.HandleFunc("GET /api/v1/admin/products/best-sellers", authMiddleware.AdminAuth(adminHandler.GetBestSellers))
	mux.HandleFunc("GET /api/v1/admin/products/{sku}", authMiddleware.AdminAuth(adminHandler.GetAdminProduct))
	mux.HandleFunc("PUT /api/v1/admin/products/{sku}", authMiddleware.AdminAuth(adminHandler.UpdateAdminProduct))
	mux.HandleFunc("PUT /api/v1/admin/products/{sku}/image", authMiddleware.AdminAuth(adminHandler.UpdateProductImage))
	mux.HandleFunc("DELETE /api/v1/admin/products/{sku}/image", authMiddleware.AdminAuth(adminHandler.DeleteProductImage))
	mux.HandleFunc("POST /api/v1/admin/products/{sku}/tags", authMiddleware.AdminAuth(adminHandler.AddProductTag))
	mux.HandleFunc("DELETE /api/v1/admin/products/{sku}/tags/{tag}", authMiddleware.AdminAuth(adminHandler.RemoveProductTag))

	// Admin Content CRUD
	mux.HandleFunc("GET /api/v1/admin/content", authMiddleware.AdminAuth(contentHandler.GetAllContent))
	mux.HandleFunc("GET /api/v1/admin/content/{id}", authMiddleware.AdminAuth(contentHandler.GetContentByID))
	mux.HandleFunc("POST /api/v1/admin/content", authMiddleware.AdminAuth(contentHandler.CreateContent))
	mux.HandleFunc("PUT /api/v1/admin/content/{id}", authMiddleware.AdminAuth(contentHandler.UpdateContent))
	mux.HandleFunc("DELETE /api/v1/admin/content/{id}", authMiddleware.AdminAuth(contentHandler.DeleteContent))

	// Admin Brand Settings
	mux.HandleFunc("GET /api/v1/admin/brands", authMiddleware.AdminAuth(contentHandler.GetBrandSettings))
	mux.HandleFunc("PUT /api/v1/admin/brands/{brand}", authMiddleware.AdminAuth(contentHandler.UpdateBrandSetting))

	// Admin TOTP / 2FA Security
	mux.HandleFunc("GET /api/v1/admin/totp/status", authMiddleware.AdminAuth(totpHandler.GetTOTPStatus))
	mux.HandleFunc("POST /api/v1/admin/totp/setup", authMiddleware.AdminAuth(totpHandler.SetupTOTP))
	mux.HandleFunc("POST /api/v1/admin/totp/enable", authMiddleware.AdminAuth(totpHandler.EnableTOTP))
	mux.HandleFunc("POST /api/v1/admin/totp/disable", authMiddleware.AdminAuth(totpHandler.DisableTOTP))

	// Admin Manual Topup (requires TOTP verification)
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/manual-topup", authMiddleware.AdminAuth(totpHandler.ManualTopup))

	// Admin Custom Topup (for cash/gift - requires password + TOTP)
	mux.HandleFunc("POST /api/v1/admin/topup/custom", authMiddleware.AdminAuth(totpHandler.CustomTopup))

	// Admin Member Management
	mux.HandleFunc("GET /api/v1/admin/members", authMiddleware.AdminAuth(memberHandler.GetMembers))
	mux.HandleFunc("POST /api/v1/admin/members", authMiddleware.AdminAuth(memberHandler.CreateMember))
	mux.HandleFunc("GET /api/v1/admin/members/{id}", authMiddleware.AdminAuth(memberHandler.GetMember))
	mux.HandleFunc("PUT /api/v1/admin/members/{id}", authMiddleware.AdminAuth(memberHandler.UpdateMember))
	mux.HandleFunc("DELETE /api/v1/admin/members/{id}", authMiddleware.AdminAuth(memberHandler.DeleteMember))
	mux.HandleFunc("POST /api/v1/admin/members/{id}/topup", authMiddleware.AdminAuth(memberHandler.TopupMember))

	// ==========================================
	// MEMBER ROUTES (Protected with Member Auth Middleware)
	// ==========================================
	mux.HandleFunc("GET /api/v1/member/dashboard", authMiddleware.MemberAuth(memberHandler.GetDashboard))
	mux.HandleFunc("GET /api/v1/member/profile", authMiddleware.MemberAuth(memberHandler.GetProfile))
	mux.HandleFunc("PUT /api/v1/member/profile", authMiddleware.MemberAuth(memberHandler.UpdateProfile))
	mux.HandleFunc("GET /api/v1/member/deposits", authMiddleware.MemberAuth(memberHandler.GetDeposits))
	mux.HandleFunc("GET /api/v1/member/products", authMiddleware.MemberAuth(memberHandler.GetProducts))
	mux.HandleFunc("GET /api/v1/member/products/{sku}", authMiddleware.MemberAuth(memberHandler.GetProductBySku))
	mux.HandleFunc("GET /api/v1/member/orders", authMiddleware.MemberAuth(memberHandler.GetOrders))
	mux.HandleFunc("GET /api/v1/member/orders/{id}", authMiddleware.MemberAuth(memberHandler.GetOrderByID))
	mux.HandleFunc("POST /api/v1/member/orders", authMiddleware.MemberAuth(memberHandler.CreateOrder))
	mux.HandleFunc("PUT /api/v1/member/password", authMiddleware.MemberAuth(memberHandler.ChangePassword))

	// Apply middleware to API routes
	var apiHandler http.Handler = mux
	apiHandler = middleware.ContentTypeJSON(apiHandler)
	apiHandler = middleware.Logger(apiHandler)
	apiHandler = middleware.CORS(apiHandler, cfg.FrontendURL)
	apiHandler = middleware.Recoverer(apiHandler)

	// Create main handler that routes docs separately
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve docs without JSON middleware
		if r.URL.Path == "/docs" || r.URL.Path == "/docs/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexHTML)
			return
		}
		if r.URL.Path == "/docs/swagger.yaml" {
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.Write(swaggerYAML)
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
