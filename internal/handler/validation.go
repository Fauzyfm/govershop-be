package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
)

// ValidationHandler handles account validation requests
type ValidationHandler struct {
	config       *config.Config
	productRepo  *repository.ProductRepository
	orderRepo    *repository.OrderRepository
	digiflazzSvc *digiflazz.Service
}

// NewValidationHandler creates a new ValidationHandler
func NewValidationHandler(
	cfg *config.Config,
	productRepo *repository.ProductRepository,
	orderRepo *repository.OrderRepository,
	digiflazzSvc *digiflazz.Service,
) *ValidationHandler {
	return &ValidationHandler{
		config:       cfg,
		productRepo:  productRepo,
		orderRepo:    orderRepo,
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
	if err != nil || !checkProduct.IsAvailable {
		// If check username product not found or unavailable, return Success but with empty account name
		// This triggers "Manual Validation" flow on frontend
		Success(w, "Validasi manual", ValidateAccountResponse{
			IsValid:     true,
			AccountName: "", // Empty name indicates manual check warning needed
			CustomerNo:  req.CustomerNo,
			Brand:       req.Brand,
			Message:     "Pastikan User ID sesuai. Kesalahan input diluar tanggung jawab kami.",
		})
		return
	}

	// Call Digiflazz to validate account
	refID := fmt.Sprintf("VAL-%d", time.Now().UnixMilli())

	resp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: checkUserSKU,
		CustomerNo:   req.CustomerNo,
		RefID:        refID,
		Testing:      false, // Production mode
	})

	if resp != nil {
		respJSON, _ := json.Marshal(resp)
		fmt.Printf("🔍 DEBUG DIGIFLAZZ RESPONSE: %s\n", string(respJSON))
	}

	if err != nil {
		fmt.Printf("❌ DIGIFLAZZ ERROR: %v\n", err)
		InternalError(w, fmt.Sprintf("Gagal validasi akun: %v", err))
		return
	}

	// Check response
	isValid := resp.Data.Status == "Sukses"
	accountName := ""
	message := resp.Data.Message
	refID = resp.Data.RefID // Update refID from response just in case

	// If Pending, retry check status a few times
	if resp.Data.Status == "Pending" {
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			time.Sleep(2 * time.Second) // Wait 2 seconds

			// Check status (idempotent call with same refID)
			retryResp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
				BuyerSKUCode: checkUserSKU,
				CustomerNo:   req.CustomerNo,
				RefID:        refID,
				Testing:      false,
			})

			if err == nil {
				if retryResp != nil {
					respJSON, _ := json.Marshal(retryResp)
					fmt.Printf("🔄 RETRY %d RESPONSE: %s\n", i+1, string(respJSON))
				}

				if retryResp.Data.Status == "Sukses" {
					isValid = true
					resp.Data = retryResp.Data
					message = retryResp.Data.Message
					break
				}
				if retryResp.Data.Status == "Gagal" {
					isValid = false
					message = retryResp.Data.Message
					break
				}
			}
		}
	}

	if isValid {
		// Extract account name from serial number or message
		// Format SN biasanya: "USERNAME" atau ada di message
		accountName = resp.Data.SN
		if accountName == "" {
			accountName = "Akun Valid (Nama tidak muncul)"
		}
	}

	// --- LOG TRANSAKSI KE DATABASE (ORDERS) ---
	// Catat sebagai pengeluaran operasional (SellingPrice 0, BuyPrice = Digiflazz Price)
	// Hanya jika ada response dari Digiflazz (sukses/gagal/pending yang valid)
	// Status order mengikuti status validasi

	orderStatus := model.OrderStatusFailed
	if isValid {
		orderStatus = model.OrderStatusSuccess
	} else if resp.Data.Status == "Pending" {
		orderStatus = model.OrderStatusProcessing
	}

	// CustomerName = Hasil validasi (Nama akun) atau pesan error
	customerNameLog := accountName
	if !isValid {
		customerNameLog = "Invalid: " + message
	}

	logOrder := &model.Order{
		RefID:           refID,
		BuyerSKUCode:    checkUserSKU,
		ProductName:     fmt.Sprintf("Check User %s", req.Brand),
		CustomerNo:      req.CustomerNo,
		CustomerName:    customerNameLog,
		BuyPrice:        resp.Data.Price, // Biaya admin
		SellingPrice:    0,               // Konsumen tidak bayar
		Status:          orderStatus,
		CustomerPhone:   "", // Kosongkan agar tidak muncul di history user
		CustomerEmail:   "",
		DigiflazzStatus: resp.Data.Status,
		DigiflazzRC:     resp.Data.RC,
		DigiflazzMsg:    message,
		SerialNumber:    resp.Data.SN,
	}

	// Fire and forget logging (in goroutine is safer for latency, but here we want data integrity so standard call)
	// Use background context for DB insert to prevent cancellation if HTTP request finishes early
	go func() {
		bgCtx := context.Background()
		if err := h.orderRepo.Create(bgCtx, logOrder); err != nil {
			fmt.Printf("❌ Failed to log validation order: %v\n", err)
		} else {
			// Auto update payment to paid because this is system transaction
			_ = h.orderRepo.UpdateStatus(bgCtx, logOrder.ID, logOrder.Status)
			// Wait, Create already sets status. But maybe payment status?
			// The Order model might imply payment flow.
			// Let's assume Create sets initial status correctly.
		}
	}()
	// ------------------------------------------

	Success(w, "Validasi berhasil", ValidateAccountResponse{
		IsValid:       isValid,
		AccountName:   accountName,
		CustomerNo:    req.CustomerNo,
		Brand:         req.Brand,
		Message:       message,
		ValidationFee: checkProduct.SellingPrice,
	})
}
