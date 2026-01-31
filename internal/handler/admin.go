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
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	config       *config.Config
	digiflazzSvc *digiflazz.Service
	productRepo  *repository.ProductRepository
	orderRepo    *repository.OrderRepository
	syncLogRepo  *repository.SyncLogRepository
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(
	cfg *config.Config,
	digiflazzSvc *digiflazz.Service,
	productRepo *repository.ProductRepository,
	orderRepo *repository.OrderRepository,
	syncLogRepo *repository.SyncLogRepository,
) *AdminHandler {
	return &AdminHandler{
		config:       cfg,
		digiflazzSvc: digiflazzSvc,
		productRepo:  productRepo,
		orderRepo:    orderRepo,
		syncLogRepo:  syncLogRepo,
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

	orders, err := h.orderRepo.GetAll(ctx, limit, offset)
	if err != nil {
		InternalError(w, "Gagal mengambil data order")
		return
	}

	Success(w, "", map[string]interface{}{
		"orders": orders,
		"total":  len(orders),
	})
}

// GetDashboard handles GET /api/v1/admin/dashboard
func (h *AdminHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get balance
	balance, _ := h.digiflazzSvc.CheckBalance()

	// Get order counts
	orderCounts, _ := h.orderRepo.CountByStatus(ctx)

	// Get today's stats
	todayOrders, todayRevenue, _ := h.orderRepo.GetTodayStats(ctx)

	// Get last sync info
	lastSync, _ := h.syncLogRepo.GetLastSync(ctx, "prepaid")

	Success(w, "", map[string]interface{}{
		"deposit":       balance.Data.Deposit,
		"order_counts":  orderCounts,
		"today_orders":  todayOrders,
		"today_revenue": todayRevenue,
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

	products, total, err := h.productRepo.GetAllForAdmin(ctx, limit, offset)
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

	if err := h.productRepo.UpdateCustomFields(ctx, sku, req.DisplayName, req.IsBestSeller, req.MarkupPercent, req.DiscountPrice); err != nil {
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
