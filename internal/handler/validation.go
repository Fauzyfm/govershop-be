package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
)

// ValidationHandler handles account validation requests
type ValidationHandler struct {
	config       *config.Config
	productRepo  *repository.ProductRepository
	digiflazzSvc *digiflazz.Service
}

// NewValidationHandler creates a new ValidationHandler
func NewValidationHandler(
	cfg *config.Config,
	productRepo *repository.ProductRepository,
	digiflazzSvc *digiflazz.Service,
) *ValidationHandler {
	return &ValidationHandler{
		config:       cfg,
		productRepo:  productRepo,
		digiflazzSvc: digiflazzSvc,
	}
}

// ValidateAccountRequest is the request body for account validation
type ValidateAccountRequest struct {
	Brand      string `json:"brand"`       // e.g., "MOBILE LEGENDS", "FREE FIRE"
	CustomerNo string `json:"customer_no"` // User ID + Zone ID
}

// ValidateAccountResponse is the response for account validation
type ValidateAccountResponse struct {
	IsValid       bool    `json:"is_valid"`
	AccountName   string  `json:"account_name,omitempty"`
	CustomerNo    string  `json:"customer_no"`
	Brand         string  `json:"brand"`
	Message       string  `json:"message,omitempty"`
	ValidationFee float64 `json:"validation_fee"` // Biaya check username
}

// ValidateAccount handles POST /api/v1/validate-account
func (h *ValidationHandler) ValidateAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ValidateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Validate required fields
	if req.Brand == "" {
		BadRequest(w, "brand wajib diisi")
		return
	}
	if req.CustomerNo == "" {
		BadRequest(w, "customer_no wajib diisi")
		return
	}

	// Find check username product for this brand
	// Pattern: checkuser{brand} e.g., checkusermobilelegends
	brandSlug := strings.ToLower(strings.ReplaceAll(req.Brand, " ", ""))
	checkUserSKU := fmt.Sprintf("checkuser%s", brandSlug)

	// Get check username product
	checkProduct, err := h.productRepo.GetBySKU(ctx, checkUserSKU)
	if err != nil {
		// If check username product not found, return error
		NotFound(w, fmt.Sprintf("Produk validasi untuk %s tidak ditemukan", req.Brand))
		return
	}

	if !checkProduct.IsAvailable {
		BadRequest(w, "Produk validasi sedang tidak tersedia")
		return
	}

	// Call Digiflazz to validate account
	refID := fmt.Sprintf("VAL-%d", time.Now().UnixMilli())

	resp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: checkUserSKU,
		CustomerNo:   req.CustomerNo,
		RefID:        refID,
		Testing:      false, // Force production mode as requested
	})

	if err != nil {
		InternalError(w, fmt.Sprintf("Gagal validasi akun: %v", err))
		return
	}

	// Check response
	isValid := resp.Data.Status == "Sukses"
	accountName := ""
	message := resp.Data.Message

	if isValid {
		// Extract account name from serial number or message
		// Format SN biasanya: "USERNAME" atau ada di message
		accountName = resp.Data.SN
		if accountName == "" {
			accountName = "Akun Valid"
		}
	}

	Success(w, "Validasi berhasil", ValidateAccountResponse{
		IsValid:       isValid,
		AccountName:   accountName,
		CustomerNo:    req.CustomerNo,
		Brand:         req.Brand,
		Message:       message,
		ValidationFee: checkProduct.SellingPrice,
	})
}
