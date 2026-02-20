package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Embed timezone data for Windows compatibility

	"embed"
	"govershop-api/internal/config"
	"govershop-api/internal/handler"
	"govershop-api/internal/middleware"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/email"
	"govershop-api/internal/service/pakasir"
	"govershop-api/internal/service/qrispw"
)

//go:embed docs/*
var docsFS embed.FS

func main() {
	// Set timezone to WIB (Asia/Jakarta) using FixedZone for consistency
	loc := time.FixedZone("Asia/Jakarta", 7*3600)
	time.Local = loc
	log.Printf("🕐 Timezone set to %s (WIB)", loc.String())

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

	// Run auto-migrations
	config.RunMigrations(db)

	// Initialize services
	digiflazzSvc := digiflazz.NewService(cfg)
	pakasirSvc := pakasir.NewService(cfg)
	qrispwSvc := qrispw.NewService(cfg)
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
	orderHandler := handler.NewOrderHandler(cfg, orderRepo, paymentRepo, productRepo, digiflazzSvc, pakasirSvc, qrispwSvc, emailSvc)
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

	// Initialize rate limiters (4-tier strategy)
	strictRL := middleware.NewRateLimiter(5, time.Minute)    // Auth endpoints: 5 req/min
	moderateRL := middleware.NewRateLimiter(20, time.Minute) // Financial endpoints: 20 req/min
	standardRL := middleware.NewRateLimiter(60, time.Minute) // General endpoints: 60 req/min

	log.Println("🛡️  Rate limiters initialized:")
	log.Println("   🔴 Strict:   5 req/min  (login, forgot/reset password)")
	log.Println("   🟠 Moderate: 20 req/min (orders, payments, validation)")
	log.Println("   🟡 Standard: 60 req/min (general API endpoints)")
	log.Println("   🟢 Relaxed:  unlimited  (webhooks, health)")

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

	// Product endpoints (Standard: 60 req/min)
	mux.HandleFunc("GET /api/v1/products", standardRL.Limit(productHandler.GetProducts))
	mux.HandleFunc("GET /api/v1/products/filters", standardRL.Limit(productHandler.GetFilters))
	mux.HandleFunc("GET /api/v1/products/categories", standardRL.Limit(productHandler.GetCategories))
	mux.HandleFunc("GET /api/v1/products/brands", standardRL.Limit(productHandler.GetBrands))
	mux.HandleFunc("GET /api/v1/products/{sku}", standardRL.Limit(productHandler.GetProductBySKU))

	// Content endpoints (Standard: 60 req/min)
	mux.HandleFunc("GET /api/v1/content/carousel", standardRL.Limit(contentHandler.GetCarousel))
	mux.HandleFunc("GET /api/v1/content/brands", standardRL.Limit(contentHandler.GetBrandImages))
	mux.HandleFunc("GET /api/v1/content/popup", standardRL.Limit(contentHandler.GetPopup))
	mux.HandleFunc("GET /api/v1/brands/{brand}", standardRL.Limit(contentHandler.GetPublicBrandSetting))

	// Validation endpoints (Moderate: 20 req/min)
	mux.HandleFunc("POST /api/v1/validate-account", moderateRL.Limit(validationHandler.ValidateAccount))
	mux.HandleFunc("POST /api/v1/calculate-price", moderateRL.Limit(validationHandler.CalculatePrice))

	// Order endpoints (Moderate for writes, Standard for reads)
	mux.HandleFunc("POST /api/v1/orders", moderateRL.Limit(orderHandler.CreateOrder))
	mux.HandleFunc("GET /api/v1/orders/{id}", standardRL.Limit(orderHandler.GetOrder))
	mux.HandleFunc("POST /api/v1/orders/{id}/pay", moderateRL.Limit(orderHandler.InitiatePayment))
	mux.HandleFunc("POST /api/v1/orders/{id}/cancel", moderateRL.Limit(orderHandler.CancelOrder))
	mux.HandleFunc("GET /api/v1/orders/{id}/status", standardRL.Limit(orderHandler.GetOrderStatus))
	mux.HandleFunc("GET /api/v1/orders/track", standardRL.Limit(orderHandler.TrackOrders))

	// Payment methods (Standard: 60 req/min)
	mux.HandleFunc("GET /api/v1/payment-methods", standardRL.Limit(orderHandler.GetPaymentMethods))

	// Admin Auth (Strict: 5 req/min)
	mux.HandleFunc("POST /api/v1/admin/login", strictRL.Limit(adminHandler.Login))

	// Member Auth (Strict: 5 req/min)
	mux.HandleFunc("POST /api/v1/member/login", strictRL.Limit(memberHandler.Login))
	mux.HandleFunc("POST /api/v1/member/forgot-password", strictRL.Limit(memberHandler.ForgotPassword))
	mux.HandleFunc("POST /api/v1/member/reset-password", strictRL.Limit(memberHandler.ResetPassword))

	// ==========================================
	// WEBHOOK ROUTES
	// ==========================================
	mux.HandleFunc("POST /api/v1/webhook/pakasir", webhookHandler.HandlePakasirWebhook)
	mux.HandleFunc("POST /api/v1/webhook/qrispw", webhookHandler.HandleQrisPWWebhook)
	mux.HandleFunc("POST /api/v1/webhook/digiflazz", webhookHandler.HandleDigiflazzWebhook)

	// ==========================================
	// ADMIN ROUTES (Protected with Auth Middleware)
	// ==========================================
	mux.HandleFunc("GET /api/v1/admin/balance", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetBalance)))
	mux.HandleFunc("GET /api/v1/admin/dashboard", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetDashboard)))
	mux.HandleFunc("GET /api/v1/admin/orders", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetOrders)))
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/check-status", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.CheckOrderStatus)))
	mux.HandleFunc("POST /api/v1/admin/sync/products", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.SyncProducts)))
	mux.HandleFunc("GET /api/v1/admin/logs/sync", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetSyncLogs)))
	mux.HandleFunc("GET /api/v1/admin/logs/webhook", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetWebhookLogs)))

	// Admin Product CRUD
	mux.HandleFunc("GET /api/v1/admin/products", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetAdminProducts)))
	mux.HandleFunc("GET /api/v1/admin/products/filters", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetProductFilters)))
	mux.HandleFunc("GET /api/v1/admin/products/tags", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetAllTags)))
	mux.HandleFunc("GET /api/v1/admin/products/best-sellers", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetBestSellers)))
	mux.HandleFunc("GET /api/v1/admin/products/{sku}", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.GetAdminProduct)))
	mux.HandleFunc("PUT /api/v1/admin/products/{sku}", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.UpdateAdminProduct)))
	mux.HandleFunc("PUT /api/v1/admin/products/{sku}/image", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.UpdateProductImage)))
	mux.HandleFunc("DELETE /api/v1/admin/products/{sku}/image", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.DeleteProductImage)))
	mux.HandleFunc("POST /api/v1/admin/products/{sku}/tags", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.AddProductTag)))
	mux.HandleFunc("DELETE /api/v1/admin/products/{sku}/tags/{tag}", standardRL.Limit(authMiddleware.AdminAuth(adminHandler.RemoveProductTag)))

	// Admin Content CRUD
	mux.HandleFunc("GET /api/v1/admin/content", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.GetAllContent)))
	mux.HandleFunc("GET /api/v1/admin/content/{id}", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.GetContentByID)))
	mux.HandleFunc("POST /api/v1/admin/content", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.CreateContent)))
	mux.HandleFunc("PUT /api/v1/admin/content/{id}", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.UpdateContent)))
	mux.HandleFunc("DELETE /api/v1/admin/content/{id}", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.DeleteContent)))

	// Admin Brand Settings
	mux.HandleFunc("GET /api/v1/admin/brands", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.GetBrandSettings)))
	mux.HandleFunc("PUT /api/v1/admin/brands/{brand}", standardRL.Limit(authMiddleware.AdminAuth(contentHandler.UpdateBrandSetting)))

	// Admin TOTP / 2FA Security
	mux.HandleFunc("GET /api/v1/admin/totp/status", standardRL.Limit(authMiddleware.AdminAuth(totpHandler.GetTOTPStatus)))
	mux.HandleFunc("POST /api/v1/admin/totp/setup", standardRL.Limit(authMiddleware.AdminAuth(totpHandler.SetupTOTP)))
	mux.HandleFunc("POST /api/v1/admin/totp/enable", standardRL.Limit(authMiddleware.AdminAuth(totpHandler.EnableTOTP)))
	mux.HandleFunc("POST /api/v1/admin/totp/disable", standardRL.Limit(authMiddleware.AdminAuth(totpHandler.DisableTOTP)))

	// Admin Manual Topup (requires TOTP verification)
	mux.HandleFunc("POST /api/v1/admin/orders/{id}/manual-topup", moderateRL.Limit(authMiddleware.AdminAuth(totpHandler.ManualTopup)))

	// Admin Custom Topup (for cash/gift - requires password + TOTP)
	mux.HandleFunc("POST /api/v1/admin/topup/custom", moderateRL.Limit(authMiddleware.AdminAuth(totpHandler.CustomTopup)))

	// Admin Member Management
	mux.HandleFunc("GET /api/v1/admin/members", standardRL.Limit(authMiddleware.AdminAuth(memberHandler.GetMembers)))
	mux.HandleFunc("POST /api/v1/admin/members", standardRL.Limit(authMiddleware.AdminAuth(memberHandler.CreateMember)))
	mux.HandleFunc("GET /api/v1/admin/members/{id}", standardRL.Limit(authMiddleware.AdminAuth(memberHandler.GetMember)))
	mux.HandleFunc("PUT /api/v1/admin/members/{id}", standardRL.Limit(authMiddleware.AdminAuth(memberHandler.UpdateMember)))
	mux.HandleFunc("DELETE /api/v1/admin/members/{id}", standardRL.Limit(authMiddleware.AdminAuth(memberHandler.DeleteMember)))
	mux.HandleFunc("POST /api/v1/admin/members/{id}/topup", moderateRL.Limit(authMiddleware.AdminAuth(memberHandler.TopupMember)))

	// ==========================================
	// MEMBER ROUTES (Protected with Member Auth Middleware)
	// ==========================================
	mux.HandleFunc("GET /api/v1/member/dashboard", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetDashboard)))
	mux.HandleFunc("GET /api/v1/member/profile", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetProfile)))
	mux.HandleFunc("PUT /api/v1/member/profile", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.UpdateProfile)))
	mux.HandleFunc("GET /api/v1/member/deposits", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetDeposits)))
	mux.HandleFunc("GET /api/v1/member/products", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetProducts)))
	mux.HandleFunc("GET /api/v1/member/products/{sku}", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetProductBySku)))
	mux.HandleFunc("GET /api/v1/member/orders", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetOrders)))
	mux.HandleFunc("GET /api/v1/member/orders/{id}", standardRL.Limit(authMiddleware.MemberAuth(memberHandler.GetOrderByID)))
	mux.HandleFunc("POST /api/v1/member/orders", moderateRL.Limit(authMiddleware.MemberAuth(memberHandler.CreateOrder)))
	mux.HandleFunc("PUT /api/v1/member/password", strictRL.Limit(authMiddleware.MemberAuth(memberHandler.ChangePassword)))

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
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
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
