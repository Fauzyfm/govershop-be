package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/pakasir"

	"time"

	"github.com/golang-jwt/jwt/v5"
)

// LoginRequest handles admin login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles POST /api/v1/admin/login
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Validate credentials from config (env)
	if req.Username != h.config.AdminUsername || req.Password != h.config.AdminPassword {
		http.Error(w, "Username atau password salah", http.StatusUnauthorized)
		return
	}

	// Generate JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": req.Username,
		"exp": time.Now().Add(time.Hour * 24).Unix(), // 24 hours
	})

	tokenString, err := token.SignedString([]byte(h.config.JWTSecret))
	if err != nil {
		InternalError(w, "Gagal generate token")
		return
	}

	Success(w, "Login berhasil", map[string]string{
		"token": tokenString,
	})
}

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	config       *config.Config
	digiflazzSvc *digiflazz.Service
	productRepo  *repository.ProductRepository
	orderRepo    *repository.OrderRepository
	syncLogRepo  *repository.SyncLogRepository
	paymentRepo  *repository.PaymentRepository
	pakasirSvc   *pakasir.Service
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(
	cfg *config.Config,
	digiflazzSvc *digiflazz.Service,
	productRepo *repository.ProductRepository,
	orderRepo *repository.OrderRepository,
	syncLogRepo *repository.SyncLogRepository,
	paymentRepo *repository.PaymentRepository,
	pakasirSvc *pakasir.Service,
) *AdminHandler {
	return &AdminHandler{
		config:       cfg,
		digiflazzSvc: digiflazzSvc,
		productRepo:  productRepo,
		orderRepo:    orderRepo,
		syncLogRepo:  syncLogRepo,
		paymentRepo:  paymentRepo,
		pakasirSvc:   pakasirSvc,
	}
}

// GetBalance handles GET /api/v1/admin/balance
func (h *AdminHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	resp, err := h.digiflazzSvc.CheckBalance()
	if err != nil {
		InternalError(w, "Gagal mengecek saldo")
		return
	}

	Success(w, "", map[string]interface{}{
		"deposit": resp.Data.Deposit,
	})
}

// SyncProducts handles POST /api/v1/admin/sync/products
func (h *AdminHandler) SyncProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start sync log
	logID, _ := h.syncLogRepo.StartSync(ctx, "prepaid")

	log.Println("[Sync] Starting product sync from Digiflazz...")

	// Fetch products from Digiflazz
	products, err := h.digiflazzSvc.GetPriceList("prepaid")
	if err != nil {
		log.Printf("[Sync] Failed to fetch products: %v", err)
		h.syncLogRepo.CompleteSync(ctx, logID, 0, 0, 0, 0, err.Error())
		InternalError(w, "Gagal mengambil data produk dari Digiflazz")
		return
	}

	log.Printf("[Sync] Received %d products from Digiflazz", len(products))

	// Upsert products
	var created, updated, failed int
	skuCodes := make([]string, 0, len(products))

	for _, p := range products {
		skuCodes = append(skuCodes, p.BuyerSKUCode)

		err := h.productRepo.UpsertProduct(ctx, p, h.config.DefaultMarkupPercent)
		if err != nil {
			log.Printf("[Sync] Failed to upsert product %s: %v", p.BuyerSKUCode, err)
			failed++
		} else {
			updated++ // For simplicity, count all as updated
		}
	}

	// Mark products not in sync as unavailable
	if err := h.productRepo.MarkUnavailable(ctx, skuCodes); err != nil {
		log.Printf("[Sync] Failed to mark unavailable products: %v", err)
	}

	// Complete sync log
	h.syncLogRepo.CompleteSync(ctx, logID, len(products), created, updated, failed, "")

	log.Printf("[Sync] Sync completed: total=%d, updated=%d, failed=%d", len(products), updated, failed)

	Success(w, "Sync berhasil", map[string]interface{}{
		"total":   len(products),
		"updated": updated,
		"failed":  failed,
	})
}

// GetOrders handles GET /api/v1/admin/orders
func (h *AdminHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get pagination params
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := parseInt(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")

	orders, total, err := h.orderRepo.GetAll(ctx, limit, offset, search, status)
	if err != nil {
		InternalError(w, "Gagal mengambil data order")
		return
	}

	// Enrich orders with payment status
	type AdminOrderResponse struct {
		ID              string  `json:"id"`
		RefID           string  `json:"ref_id"`
		BuyerSKUCode    string  `json:"buyer_sku_code"`
		ProductName     string  `json:"product_name"`
		CustomerNo      string  `json:"customer_no"`
		CustomerEmail   string  `json:"customer_email,omitempty"`
		CustomerPhone   string  `json:"customer_phone,omitempty"`
		SellingPrice    float64 `json:"selling_price"`
		Status          string  `json:"status"`
		StatusLabel     string  `json:"status_label"`
		PaymentStatus   string  `json:"payment_status"`
		DigiflazzStatus string  `json:"digiflazz_status,omitempty"`
		SerialNumber    string  `json:"serial_number,omitempty"`
		Message         string  `json:"message,omitempty"`
		CreatedAt       string  `json:"created_at"`
	}

	var enrichedOrders []AdminOrderResponse
	for _, order := range orders {
		resp := AdminOrderResponse{
			ID:              order.ID,
			RefID:           order.RefID,
			BuyerSKUCode:    order.BuyerSKUCode,
			ProductName:     order.ProductName,
			CustomerNo:      order.CustomerNo,
			CustomerEmail:   order.CustomerEmail,
			CustomerPhone:   order.CustomerPhone,
			SellingPrice:    order.SellingPrice,
			Status:          string(order.Status),
			StatusLabel:     order.GetStatusLabel(),
			DigiflazzStatus: order.DigiflazzStatus,
			SerialNumber:    order.SerialNumber,
			Message:         order.DigiflazzMsg,
			CreatedAt:       order.CreatedAt.Format(time.RFC3339),
		}

		// Get payment status if exists
		if payment, err := h.paymentRepo.GetByOrderID(ctx, order.ID); err == nil && payment != nil {
			resp.PaymentStatus = string(payment.Status)
		} else {
			resp.PaymentStatus = "-"
		}

		enrichedOrders = append(enrichedOrders, resp)
	}

	Success(w, "", map[string]interface{}{
		"orders": enrichedOrders,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetDashboard handles GET /api/v1/admin/dashboard
func (h *AdminHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get balance
	var deposit float64
	if balance, err := h.digiflazzSvc.CheckBalance(); err == nil && balance != nil {
		deposit = balance.Data.Deposit
	} else {
		log.Printf("[Dashboard] Failed to check balance: %v", err)
	}

	// Get order counts
	orderCounts, err := h.orderRepo.CountByStatus(ctx)
	if err != nil {
		log.Printf("[Dashboard] Failed to count orders: %v", err)
		orderCounts = map[string]int{"pending": 0, "success": 0, "failed": 0}
	}

	// Merge "waiting_payment" into "pending" for dashboard display
	if count, ok := orderCounts["waiting_payment"]; ok {
		orderCounts["pending"] += count
		delete(orderCounts, "waiting_payment") // Optional: remove the raw key if frontend purely relies on 'pending'
	}

	// Get today's stats
	todayOrders, todayRevenue, err := h.orderRepo.GetTodayStats(ctx)
	if err != nil {
		log.Printf("[Dashboard] Failed to get today stats: %v", err)
	}

	// Get last sync info
	lastSync, err := h.syncLogRepo.GetLastSync(ctx, "prepaid")
	if err != nil {
		log.Printf("[Dashboard] Failed to get last sync: %v", err)
	}

	// Get total revenue (all time)
	totalRevenue, err := h.orderRepo.GetTotalRevenue(ctx)
	if err != nil {
		log.Printf("[Dashboard] Failed to get total revenue: %v", err)
	}

	Success(w, "", map[string]interface{}{
		"deposit":       deposit,
		"order_counts":  orderCounts,
		"today_orders":  todayOrders,
		"today_revenue": todayRevenue,
		"total_revenue": totalRevenue,
		"last_sync":     lastSync,
	})
}

// SimulatePayment handles POST /api/v1/admin/simulate-payment (development only)
func (h *AdminHandler) SimulatePayment(w http.ResponseWriter, r *http.Request) {
	if !h.config.IsDevelopment() {
		BadRequest(w, "Simulasi pembayaran hanya tersedia di mode development")
		return
	}

	// This would trigger payment simulation via Pakasir
	// For now, just return a message
	Success(w, "Gunakan endpoint Pakasir untuk simulasi pembayaran", nil)
}

// StartProductSyncJob starts the background product sync job
func StartProductSyncJob(ctx context.Context, cfg *config.Config, digiflazzSvc *digiflazz.Service, productRepo *repository.ProductRepository, syncLogRepo *repository.SyncLogRepository) {
	// This would be called from main.go to start a ticker
	// For now, just log
	log.Println("[Sync] Product sync job initialized")
}

// ==========================================
// ADMIN PRODUCT CRUD HANDLERS
// ==========================================

// GetAdminProducts handles GET /api/v1/admin/products
func (h *AdminHandler) GetAdminProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get pagination params
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := parseInt(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	search := r.URL.Query().Get("search")
	category := r.URL.Query().Get("category")
	brand := r.URL.Query().Get("brand")
	typeStr := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")

	products, total, err := h.productRepo.GetAllForAdmin(ctx, limit, offset, search, category, brand, typeStr, status)
	if err != nil {
		InternalError(w, "Gagal mengambil data produk")
		return
	}

	Success(w, "", map[string]interface{}{
		"products": products,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// GetProductFilters retrieves unique categories and types for filtering
func (h *AdminHandler) GetProductFilters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	categories, err := h.productRepo.GetCategories(ctx)
	if err != nil {
		log.Printf("Failed to get categories: %v", err)
		categories = []string{}
	}

	types, err := h.productRepo.GetTypes(ctx)
	if err != nil {
		log.Printf("Failed to get types: %v", err)
		types = []string{}
	}

	brands, err := h.productRepo.GetAllBrands(ctx)
	if err != nil {
		log.Printf("Failed to get brands: %v", err)
		brands = []string{}
	}

	Success(w, "", map[string]interface{}{
		"categories": categories,
		"types":      types,
		"brands":     brands,
	})
}

// GetAdminProduct handles GET /api/v1/admin/products/{sku}
func (h *AdminHandler) GetAdminProduct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	if sku == "" {
		BadRequest(w, "SKU tidak valid")
		return
	}

	product, err := h.productRepo.GetBySKU(ctx, sku)
	if err != nil {
		NotFound(w, "Produk tidak ditemukan")
		return
	}

	Success(w, "", product)
}

// UpdateProductRequest is the request body for updating product
type UpdateProductRequest struct {
	DisplayName   *string  `json:"display_name,omitempty"`
	IsBestSeller  *bool    `json:"is_best_seller,omitempty"`
	MarkupPercent *float64 `json:"markup_percent,omitempty"`
	DiscountPrice *float64 `json:"discount_price,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	ImageURL      *string  `json:"image_url,omitempty"`
	Description   *string  `json:"description,omitempty"`
}

// UpdateAdminProduct handles PUT /api/v1/admin/products/{sku}
func (h *AdminHandler) UpdateAdminProduct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	if sku == "" {
		BadRequest(w, "SKU tidak valid")
		return
	}

	var req UpdateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if err := h.productRepo.UpdateCustomFields(ctx, sku, req.DisplayName, req.IsBestSeller, req.MarkupPercent, req.DiscountPrice, req.Tags, req.ImageURL, req.Description); err != nil {
		if err.Error() == "product not found" {
			NotFound(w, "Produk tidak ditemukan")
			return
		}
		InternalError(w, "Gagal update produk")
		return
	}

	// Fetch updated product
	product, _ := h.productRepo.GetBySKU(ctx, sku)
	Success(w, "Produk berhasil diupdate", product)
}

// AddTagRequest is the request body for adding a tag
type AddTagRequest struct {
	Tag string `json:"tag"`
}

// AddProductTag handles POST /api/v1/admin/products/{sku}/tags
func (h *AdminHandler) AddProductTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	if sku == "" {
		BadRequest(w, "SKU tidak valid")
		return
	}

	var req AddTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.Tag == "" {
		BadRequest(w, "Tag tidak boleh kosong")
		return
	}

	if err := h.productRepo.AddTag(ctx, sku, req.Tag); err != nil {
		InternalError(w, "Gagal menambahkan tag")
		return
	}

	Success(w, "Tag berhasil ditambahkan", nil)
}

// RemoveProductTag handles DELETE /api/v1/admin/products/{sku}/tags/{tag}
func (h *AdminHandler) RemoveProductTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")
	tag := r.PathValue("tag")

	if sku == "" || tag == "" {
		BadRequest(w, "SKU atau tag tidak valid")
		return
	}

	if err := h.productRepo.RemoveTag(ctx, sku, tag); err != nil {
		InternalError(w, "Gagal menghapus tag")
		return
	}

	Success(w, "Tag berhasil dihapus", nil)
}

// GetAllTags handles GET /api/v1/admin/products/tags
func (h *AdminHandler) GetAllTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tags, err := h.productRepo.GetAllTags(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil data tag")
		return
	}

	Success(w, "", map[string]interface{}{
		"tags": tags,
	})
}

// GetBestSellers handles GET /api/v1/admin/products/best-sellers
func (h *AdminHandler) GetBestSellers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	products, err := h.productRepo.GetBestSellers(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil data produk best seller")
		return
	}

	var responses []model.ProductResponse
	for _, p := range products {
		responses = append(responses, p.ToResponse())
	}

	Success(w, "", map[string]interface{}{
		"products": responses,
		"total":    len(responses),
	})
}

// Helper function to parse int from string
// UpdateProductImage handles PUT /api/v1/admin/products/{sku}/image
func (h *AdminHandler) UpdateProductImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	var req struct {
		ImageURL string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.ImageURL == "" {
		BadRequest(w, "Image URL wajib diisi")
		return
	}

	if err := h.productRepo.UpdateImageURL(ctx, sku, &req.ImageURL); err != nil {
		log.Printf("[Admin] Failed to update product image: %v", err)
		InternalError(w, "Gagal mengupdate gambar produk")
		return
	}

	Success(w, "Gambar produk berhasil diupdate", nil)
}

// DeleteProductImage handles DELETE /api/v1/admin/products/{sku}/image
func (h *AdminHandler) DeleteProductImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	// Set image_url to nil/null
	if err := h.productRepo.UpdateImageURL(ctx, sku, nil); err != nil {
		log.Printf("[Admin] Failed to delete product image: %v", err)
		InternalError(w, "Gagal menghapus gambar produk")
		return
	}

	Success(w, "Gambar produk berhasil dihapus", nil)
}

// Helper function to parse int from string
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

// CheckOrderStatus handles POST /api/v1/admin/orders/{id}/check-status
// Checks if order is expired and cancels it if necessary
func (h *AdminHandler) CheckOrderStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("id")

	if orderID == "" {
		BadRequest(w, "Order ID tidak valid")
		return
	}

	order, err := h.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		NotFound(w, "Order tidak ditemukan")
		return
	}

	// Get payment info
	payment, err := h.paymentRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		NotFound(w, "Payment tidak ditemukan")
		return
	}

	// Check if already final status
	if order.Status == model.OrderStatusSuccess || order.Status == model.OrderStatusFailed || order.Status == model.OrderStatusCancelled {
		Success(w, "Order sudah dalam status final", map[string]interface{}{
			"order_id": order.ID,
			"ref_id":   order.RefID,
			"status":   order.Status,
			"changed":  false,
		})
		return
	}

	// Check if expired
	if !payment.ExpiredAt.IsZero() && time.Now().After(payment.ExpiredAt) {
		// Order is expired, cancel via Pakasir
		log.Printf("[Admin] Order %s is expired, cancelling via Pakasir", order.RefID)

		// Cancel in Pakasir
		if err := h.pakasirSvc.CancelTransaction(order.RefID, order.SellingPrice); err != nil {
			log.Printf("[Admin] Failed to cancel Pakasir transaction: %v", err)
			// Continue anyway, update local status
		}

		// Update payment status
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusExpired); err != nil {
			log.Printf("[Admin] Failed to update payment status: %v", err)
		}

		// Update order status to expired
		if err := h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusExpired); err != nil {
			InternalError(w, "Gagal mengupdate status order")
			return
		}

		Success(w, "Order telah kadaluwarsa dan dibatalkan", map[string]interface{}{
			"order_id": order.ID,
			"ref_id":   order.RefID,
			"status":   model.OrderStatusExpired,
			"changed":  true,
			"reason":   "expired",
		})
		return
	}

	// Check Pakasir transaction status
	detail, err := h.pakasirSvc.GetTransactionDetail(order.RefID, order.SellingPrice)
	if err != nil {
		log.Printf("[Admin] Failed to get Pakasir transaction detail: %v", err)
		// Return current status if Pakasir check fails
		Success(w, "", map[string]interface{}{
			"order_id": order.ID,
			"ref_id":   order.RefID,
			"status":   order.Status,
			"changed":  false,
			"message":  "Tidak dapat mengecek status Pakasir",
		})
		return
	}

	// If Pakasir says expired
	if detail.Transaction.Status == "expired" {
		// Update payment status
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusExpired); err != nil {
			log.Printf("[Admin] Failed to update payment status: %v", err)
		}

		// Update order status to expired
		if err := h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusExpired); err != nil {
			InternalError(w, "Gagal mengupdate status order")
			return
		}

		Success(w, "Order telah kadaluwarsa (dari Pakasir)", map[string]interface{}{
			"order_id": order.ID,
			"ref_id":   order.RefID,
			"status":   model.OrderStatusExpired,
			"changed":  true,
			"reason":   "pakasir_expired",
		})
		return
	}

	// No change needed
	Success(w, "", map[string]interface{}{
		"order_id":       order.ID,
		"ref_id":         order.RefID,
		"status":         order.Status,
		"pakasir_status": detail.Transaction.Status,
		"changed":        false,
	})
}
