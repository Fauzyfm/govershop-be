package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"

	"github.com/pquerna/otp/totp"
)

// TOTPHandler handles TOTP-related operations
type TOTPHandler struct {
	config           *config.Config
	securityRepo     *repository.AdminSecurityRepository
	orderRepo        *repository.OrderRepository
	paymentRepo      *repository.PaymentRepository
	digiflazzSvc     *digiflazz.Service
	maxTopupsPerHour int
}

// NewTOTPHandler creates a new TOTPHandler
func NewTOTPHandler(
	cfg *config.Config,
	securityRepo *repository.AdminSecurityRepository,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	digiflazzSvc *digiflazz.Service,
) *TOTPHandler {
	return &TOTPHandler{
		config:           cfg,
		securityRepo:     securityRepo,
		orderRepo:        orderRepo,
		paymentRepo:      paymentRepo,
		digiflazzSvc:     digiflazzSvc,
		maxTopupsPerHour: 20, // Rate limit
	}
}

// GetTOTPStatus handles GET /api/v1/admin/totp/status
func (h *TOTPHandler) GetTOTPStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	security, err := h.securityRepo.GetPrimary(ctx)
	if err != nil {
		// If not found, TOTP is not set up
		Success(w, "", map[string]interface{}{
			"enabled": false,
			"setup":   false,
		})
		return
	}

	Success(w, "", map[string]interface{}{
		"enabled": security.TOTPEnabled,
		"setup":   security.TOTPSecret != "",
	})
}

// SetupTOTP handles POST /api/v1/admin/totp/setup
// Generates a new TOTP secret and returns QR code
func (h *TOTPHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if already enabled
	security, _ := h.securityRepo.GetPrimary(ctx)
	if security != nil && security.TOTPEnabled {
		BadRequest(w, "TOTP sudah aktif. Nonaktifkan dulu untuk setup ulang.")
		return
	}

	// Generate new TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Govershop Admin",
		AccountName: "admin@govershop",
		SecretSize:  32,
	})
	if err != nil {
		InternalError(w, "Gagal generate TOTP key")
		return
	}

	// Save secret (not enabled yet until verified)
	err = h.securityRepo.SetTOTPSecret(ctx, key.Secret())
	if err != nil {
		InternalError(w, "Gagal menyimpan TOTP secret")
		return
	}

	// Generate QR code image
	img, err := key.Image(200, 200)
	if err != nil {
		InternalError(w, "Gagal generate QR code")
		return
	}

	// Convert to base64
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		InternalError(w, "Gagal encode QR code")
		return
	}
	qrBase64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Log action
	h.securityRepo.CreateAuditLog(ctx, "totp_setup", "", getClientIP(r), nil, true, "")

	Success(w, "Scan QR code dengan Google Authenticator", map[string]interface{}{
		"qr_code": "data:image/png;base64," + qrBase64,
		"secret":  key.Secret(), // For manual entry
		"issuer":  "Govershop Admin",
		"account": "admin@govershop",
	})
}

// EnableTOTP handles POST /api/v1/admin/totp/enable
// Verifies TOTP code and enables 2FA
func (h *TOTPHandler) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.Code == "" || len(req.Code) != 6 {
		BadRequest(w, "Kode TOTP harus 6 digit")
		return
	}

	// Get secret
	security, err := h.securityRepo.GetPrimary(ctx)
	if err != nil || security.TOTPSecret == "" {
		BadRequest(w, "TOTP belum di-setup. Jalankan setup dulu.")
		return
	}

	// Verify code
	valid := totp.Validate(req.Code, security.TOTPSecret)
	if !valid {
		h.securityRepo.CreateAuditLog(ctx, "totp_enable", "", getClientIP(r), nil, false, "Invalid code")
		Unauthorized(w, "Kode TOTP tidak valid")
		return
	}

	// Enable TOTP
	err = h.securityRepo.EnableTOTP(ctx, true)
	if err != nil {
		InternalError(w, "Gagal mengaktifkan TOTP")
		return
	}

	h.securityRepo.CreateAuditLog(ctx, "totp_enable", "", getClientIP(r), nil, true, "")

	Success(w, "2FA berhasil diaktifkan!", nil)
}

// DisableTOTP handles POST /api/v1/admin/totp/disable
func (h *TOTPHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Get secret
	security, err := h.securityRepo.GetPrimary(ctx)
	if err != nil || !security.TOTPEnabled {
		BadRequest(w, "TOTP tidak aktif")
		return
	}

	// Verify code before disabling
	valid := totp.Validate(req.Code, security.TOTPSecret)
	if !valid {
		Unauthorized(w, "Kode TOTP tidak valid")
		return
	}

	// Disable TOTP
	err = h.securityRepo.EnableTOTP(ctx, false)
	if err != nil {
		InternalError(w, "Gagal menonaktifkan TOTP")
		return
	}

	// Clear secret
	h.securityRepo.SetTOTPSecret(ctx, "")

	h.securityRepo.CreateAuditLog(ctx, "totp_disable", "", getClientIP(r), nil, true, "")

	Success(w, "2FA berhasil dinonaktifkan", nil)
}

// ManualTopup handles POST /api/v1/admin/orders/{id}/manual-topup
// Retries a failed order with TOTP verification
func (h *TOTPHandler) ManualTopup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("id")

	var req struct {
		TOTPCode   string `json:"totp_code"`
		CustomerNo string `json:"customer_no"` // Optional: new customer_no if wrong input
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Get TOTP status
	security, err := h.securityRepo.GetPrimary(ctx)
	if err != nil {
		InternalError(w, "Gagal mengecek status TOTP")
		return
	}

	// Verify TOTP if enabled
	if security.TOTPEnabled {
		if req.TOTPCode == "" || len(req.TOTPCode) != 6 {
			BadRequest(w, "Kode TOTP diperlukan (6 digit)")
			return
		}

		valid := totp.Validate(req.TOTPCode, security.TOTPSecret)
		if !valid {
			h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r),
				map[string]interface{}{"reason": "invalid_totp"}, false, "Invalid TOTP code")
			Unauthorized(w, "Kode TOTP tidak valid")
			return
		}
	}

	// Rate limiting check
	recentCount, err := h.securityRepo.CountRecentManualTopups(ctx)
	if err != nil {
		InternalError(w, "Gagal mengecek rate limit")
		return
	}
	if recentCount >= h.maxTopupsPerHour {
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r),
			map[string]interface{}{"reason": "rate_limit"}, false, "Rate limit exceeded")
		BadRequest(w, fmt.Sprintf("Rate limit tercapai (%d topup/jam). Coba lagi nanti.", h.maxTopupsPerHour))
		return
	}

	// Get order
	order, err := h.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		NotFound(w, "Order tidak ditemukan")
		return
	}

	// Validate: order must be failed
	if order.Status != "failed" {
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r),
			map[string]interface{}{"reason": "invalid_status", "status": string(order.Status)}, false, "Order status not failed")
		BadRequest(w, fmt.Sprintf("Order tidak dalam status gagal (status: %s)", order.Status))
		return
	}

	// Validate: payment must be completed
	payment, err := h.paymentRepo.GetByOrderID(ctx, orderID)
	if err != nil || payment == nil || payment.Status != "completed" {
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r),
			map[string]interface{}{"reason": "payment_not_completed"}, false, "Payment not completed")
		BadRequest(w, "Pembayaran belum completed. Manual topup hanya untuk order yang sudah dibayar.")
		return
	}

	// Use new customer_no if provided, otherwise use original
	customerNo := order.CustomerNo
	if req.CustomerNo != "" && req.CustomerNo != order.CustomerNo {
		customerNo = req.CustomerNo
	}

	// Generate new ref_id for retry
	newRefID := fmt.Sprintf("RETRY-%d", time.Now().UnixMilli())

	// Call Digiflazz
	resp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: order.BuyerSKUCode,
		CustomerNo:   customerNo,
		RefID:        newRefID,
		Testing:      false,
	})

	auditDetails := map[string]interface{}{
		"original_ref_id":   order.RefID,
		"new_ref_id":        newRefID,
		"sku":               order.BuyerSKUCode,
		"original_customer": order.CustomerNo,
		"retry_customer":    customerNo,
	}

	if err != nil {
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r), auditDetails, false, err.Error())
		InternalError(w, fmt.Sprintf("Gagal topup: %v", err))
		return
	}

	// Update order based on response
	if resp.Data.Status == "Sukses" {
		// Success!
		err = h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusSuccess, resp.Data.Status, resp.Data.RC, resp.Data.SN, resp.Data.Message)
		if err != nil {
			InternalError(w, "Gagal update order status")
			return
		}

		// Update customer_no if changed
		if customerNo != order.CustomerNo {
			h.orderRepo.UpdateCustomerNo(ctx, orderID, customerNo)
		}

		auditDetails["result"] = "success"
		auditDetails["sn"] = resp.Data.SN
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r), auditDetails, true, "")

		Success(w, "Manual topup berhasil!", map[string]interface{}{
			"order_id":      orderID,
			"status":        "success",
			"serial_number": resp.Data.SN,
			"customer_no":   customerNo,
		})
	} else if resp.Data.Status == "Pending" {
		// Still pending
		err = h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusProcessing, resp.Data.Status, resp.Data.RC, "", resp.Data.Message)

		// Update customer_no if changed
		if customerNo != order.CustomerNo {
			h.orderRepo.UpdateCustomerNo(ctx, orderID, customerNo)
		}

		auditDetails["result"] = "pending"
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r), auditDetails, true, "")

		Success(w, "Topup sedang diproses", map[string]interface{}{
			"order_id": orderID,
			"status":   "processing",
			"message":  resp.Data.Message,
		})
	} else {
		// Failed again
		auditDetails["result"] = "failed"
		auditDetails["error"] = resp.Data.Message
		h.securityRepo.CreateAuditLog(ctx, "manual_topup", orderID, getClientIP(r), auditDetails, false, resp.Data.Message)

		BadRequest(w, fmt.Sprintf("Topup gagal: %s", resp.Data.Message))
	}
}

// CustomTopup handles POST /api/v1/admin/topup/custom
// Creates a new topup without going through payment flow (for cash/gift)
func (h *TOTPHandler) CustomTopup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		SKU        string `json:"sku"`
		CustomerNo string `json:"customer_no"`
		Source     string `json:"source"`    // "cash" or "gift"
		Notes      string `json:"notes"`     // Optional notes
		Password   string `json:"password"`  // Admin password
		TOTPCode   string `json:"totp_code"` // TOTP code
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Validate required fields
	if req.SKU == "" {
		BadRequest(w, "SKU wajib diisi")
		return
	}
	if req.CustomerNo == "" {
		BadRequest(w, "Customer No wajib diisi")
		return
	}
	if req.Source != "cash" && req.Source != "gift" {
		BadRequest(w, "Source harus 'cash' atau 'gift'")
		return
	}

	// ============ AUTHENTICATION ============

	// 1. Verify Admin Password
	if req.Password != h.config.AdminPassword {
		h.securityRepo.CreateAuditLog(ctx, "custom_topup", "", getClientIP(r),
			map[string]interface{}{"reason": "invalid_password", "sku": req.SKU}, false, "Invalid password")
		Unauthorized(w, "Password admin tidak valid")
		return
	}

	// 2. Verify TOTP if enabled
	security, err := h.securityRepo.GetPrimary(ctx)
	if err == nil && security.TOTPEnabled {
		if req.TOTPCode == "" || len(req.TOTPCode) != 6 {
			BadRequest(w, "Kode TOTP diperlukan (6 digit)")
			return
		}

		valid := totp.Validate(req.TOTPCode, security.TOTPSecret)
		if !valid {
			h.securityRepo.CreateAuditLog(ctx, "custom_topup", "", getClientIP(r),
				map[string]interface{}{"reason": "invalid_totp", "sku": req.SKU}, false, "Invalid TOTP code")
			Unauthorized(w, "Kode TOTP tidak valid")
			return
		}
	}

	// ============ RATE LIMITING ============
	recentCount, _ := h.securityRepo.CountRecentManualTopups(ctx)
	if recentCount >= h.maxTopupsPerHour {
		BadRequest(w, fmt.Sprintf("Rate limit tercapai (%d topup/jam). Coba lagi nanti.", h.maxTopupsPerHour))
		return
	}

	// ============ GET PRODUCT ============
	product, err := h.getProductBySKU(ctx, req.SKU)
	if err != nil || product == nil {
		BadRequest(w, fmt.Sprintf("Product dengan SKU '%s' tidak ditemukan", req.SKU))
		return
	}

	// ============ CREATE ORDER RECORD ============
	orderSource := "admin_" + req.Source // admin_cash or admin_gift
	refID := fmt.Sprintf("ADMIN-%d", time.Now().UnixMilli())

	orderID, err := h.orderRepo.CreateWithSource(ctx, refID, req.SKU, product.ProductName,
		req.CustomerNo, product.BuyPrice, product.SellingPrice, orderSource, req.Notes)
	if err != nil {
		InternalError(w, "Gagal membuat order record")
		return
	}

	// ============ CALL DIGIFLAZZ ============
	resp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: req.SKU,
		CustomerNo:   req.CustomerNo,
		RefID:        refID,
		Testing:      false,
	})

	auditDetails := map[string]interface{}{
		"ref_id":      refID,
		"sku":         req.SKU,
		"customer_no": req.CustomerNo,
		"source":      orderSource,
		"notes":       req.Notes,
	}

	if err != nil {
		// Update order as failed
		h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusFailed, "", "", "", err.Error())
		h.securityRepo.CreateAuditLog(ctx, "custom_topup", orderID, getClientIP(r), auditDetails, false, err.Error())
		InternalError(w, fmt.Sprintf("Gagal topup: %v", err))
		return
	}

	// ============ UPDATE ORDER BASED ON RESPONSE ============
	if resp.Data.Status == "Sukses" {
		h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusSuccess,
			resp.Data.Status, resp.Data.RC, resp.Data.SN, resp.Data.Message)

		auditDetails["result"] = "success"
		auditDetails["sn"] = resp.Data.SN
		h.securityRepo.CreateAuditLog(ctx, "custom_topup", orderID, getClientIP(r), auditDetails, true, "")

		Success(w, "Custom topup berhasil!", map[string]interface{}{
			"order_id":      orderID,
			"ref_id":        refID,
			"status":        "success",
			"serial_number": resp.Data.SN,
			"product":       product.ProductName,
			"customer_no":   req.CustomerNo,
			"source":        orderSource,
		})
	} else if resp.Data.Status == "Pending" {
		h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusProcessing,
			resp.Data.Status, resp.Data.RC, "", resp.Data.Message)

		auditDetails["result"] = "pending"
		h.securityRepo.CreateAuditLog(ctx, "custom_topup", orderID, getClientIP(r), auditDetails, true, "")

		Success(w, "Topup sedang diproses", map[string]interface{}{
			"order_id":    orderID,
			"ref_id":      refID,
			"status":      "processing",
			"product":     product.ProductName,
			"customer_no": req.CustomerNo,
			"source":      orderSource,
		})
	} else {
		h.orderRepo.UpdateDigiflazzResponse(ctx, orderID, model.OrderStatusFailed,
			resp.Data.Status, resp.Data.RC, "", resp.Data.Message)

		auditDetails["result"] = "failed"
		auditDetails["error"] = resp.Data.Message
		h.securityRepo.CreateAuditLog(ctx, "custom_topup", orderID, getClientIP(r), auditDetails, false, resp.Data.Message)

		BadRequest(w, fmt.Sprintf("Topup gagal: %s", resp.Data.Message))
	}
}

// ProductInfo for custom topup
type ProductInfo struct {
	SKU          string
	ProductName  string
	BuyPrice     float64
	SellingPrice float64
}

// getProductBySKU helper to get product info
func (h *TOTPHandler) getProductBySKU(ctx context.Context, sku string) (*ProductInfo, error) {
	// We need to query the product - let's add a simple method
	query := `
		SELECT buyer_sku_code, product_name, COALESCE(buy_price, 0), COALESCE(selling_price, 0)
		FROM products
		WHERE buyer_sku_code = $1 AND buyer_product_status = true
		LIMIT 1
	`

	var p ProductInfo
	err := h.orderRepo.GetDB().QueryRow(ctx, query, sku).Scan(&p.SKU, &p.ProductName, &p.BuyPrice, &p.SellingPrice)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Helper to get client IP
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
