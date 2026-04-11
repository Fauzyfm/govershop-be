package handler

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"govershop-api/internal/model"
	"govershop-api/internal/repository"
)

// AffiliateHandler handles affiliate-related HTTP requests
type AffiliateHandler struct {
	affiliateRepo *repository.AffiliateRepository
	userRepo      *repository.UserRepository
}

// NewAffiliateHandler creates a new AffiliateHandler
func NewAffiliateHandler(
	affiliateRepo *repository.AffiliateRepository,
	userRepo *repository.UserRepository,
) *AffiliateHandler {
	return &AffiliateHandler{
		affiliateRepo: affiliateRepo,
		userRepo:      userRepo,
	}
}

// ==========================================================
// PUBLIC API
// ==========================================================

// ValidateAffiliate handles POST /api/v1/affiliate/validate
// Used by frontend at checkout to validate a ref code (from link or manual input)
func (h *AffiliateHandler) ValidateAffiliate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req model.ValidateAffiliateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.Code == "" {
		BadRequest(w, "Kode affiliate wajib diisi")
		return
	}
	if req.CustomerNo == "" {
		BadRequest(w, "Customer number wajib diisi")
		return
	}

	// Find affiliate by code
	affiliate, err := h.affiliateRepo.GetByCode(ctx, req.Code)
	if err != nil {
		log.Printf("[Affiliate] Error getting affiliate: %v", err)
		InternalError(w, "Gagal memvalidasi kode")
		return
	}
	if affiliate == nil {
		Success(w, "", model.ValidateAffiliateResponse{
			Valid:   false,
			Message: "Kode affiliate tidak ditemukan",
		})
		return
	}

	if affiliate.Status != model.AffiliateStatusActive {
		Success(w, "", model.ValidateAffiliateResponse{
			Valid:   false,
			Message: "Kode affiliate tidak aktif",
		})
		return
	}

	// Count usages for this customer_no this month (global across brands)
	usageCount, err := h.affiliateRepo.CountUsagesByCustomerThisMonth(ctx, affiliate.ID, req.CustomerNo)
	if err != nil {
		log.Printf("[Affiliate] Error counting usages: %v", err)
		InternalError(w, "Gagal memvalidasi kode")
		return
	}

	// Determine channel (default to link if not specified)
	channel := req.Channel
	if channel == "" {
		channel = model.AffiliateChannelLink
	}

	// Check if max commission limit reached (10x default)
	if usageCount >= affiliate.MaxCommissionUses {
		Success(w, "", model.ValidateAffiliateResponse{
			Valid:         false,
			AffiliateID:   affiliate.ID,
			Channel:       channel,
			Message:       "Batas pemakaian kode ini sudah tercapai bulan ini untuk akun Anda",
			UsageCount:    usageCount,
			MaxCommission: affiliate.MaxCommissionUses,
		})
		return
	}

	// Calculate discount (only for code channel + discount enabled)
	var discountAmount float64
	var discountPercent float64

	if channel == model.AffiliateChannelCode && affiliate.DiscountEnabled {
		// Check if still within discount limit (3x default)
		if usageCount < affiliate.MaxDiscountUses {
			// Check min discount amount
			if req.TransactionAmount >= affiliate.MinDiscountAmount {
				discountPercent = affiliate.DiscountPercent
				// Discount is calculated from the profit (markup) portion, not total price
				// profit = selling_price * (markup_percent / (100 + markup_percent))
				// For 3% markup: profit = price * 3/103 ≈ 2.91% of selling price
				// Discount % is taken from this profit
				profit := req.TransactionAmount * 3.0 / 103.0
				discountAmount = math.Floor(profit * (affiliate.DiscountPercent / 100.0) * 100) / 100
			}
		}
	}

	// Build response message
	message := "Kode affiliate valid ✓"
	if channel == model.AffiliateChannelCode && affiliate.DiscountEnabled {
		if usageCount >= affiliate.MaxDiscountUses {
			message = "Anda sudah tidak mendapat diskon, tapi kode tetap aktif untuk mendukung kreator"
		} else if discountAmount > 0 {
			message = "Kode affiliate valid, diskon diterapkan ✓"
		}
	}

	Success(w, "", model.ValidateAffiliateResponse{
		Valid:           true,
		AffiliateID:     affiliate.ID,
		DiscountAmount:  discountAmount,
		DiscountPercent: discountPercent,
		Channel:         channel,
		Message:         message,
		UsageCount:      usageCount,
		MaxDiscount:     affiliate.MaxDiscountUses,
		MaxCommission:   affiliate.MaxCommissionUses,
	})
}

// GetAffiliateByCode handles GET /api/v1/affiliate/{code}
// Public endpoint to check if a code exists (for link referral landing)
func (h *AffiliateHandler) GetAffiliateByCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.PathValue("code")

	if code == "" {
		BadRequest(w, "Kode affiliate wajib diisi")
		return
	}

	affiliate, err := h.affiliateRepo.GetByCode(ctx, code)
	if err != nil {
		log.Printf("[Affiliate] Error getting affiliate: %v", err)
		InternalError(w, "Gagal mengambil data affiliate")
		return
	}
	if affiliate == nil || affiliate.Status != model.AffiliateStatusActive {
		NotFound(w, "Kode affiliate tidak ditemukan")
		return
	}

	// Return minimal info (don't expose internal details)
	Success(w, "", map[string]interface{}{
		"code":   affiliate.Code,
		"valid":  true,
	})
}

// ==========================================================
// MEMBER API (Streamer Dashboard)
// ==========================================================

// GetMyAffiliate handles GET /api/v1/member/affiliate
// Returns affiliate data for the logged-in member
func (h *AffiliateHandler) GetMyAffiliate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from context (set by auth middleware)
	userID, ok := ctx.Value("user_id").(int)
	if !ok {
		Unauthorized(w, "User ID tidak valid")
		return
	}

	affiliate, err := h.affiliateRepo.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[Affiliate] Error getting affiliate for user %d: %v", userID, err)
		InternalError(w, "Gagal mengambil data affiliate")
		return
	}
	if affiliate == nil {
		NotFound(w, "Anda belum terdaftar sebagai affiliate")
		return
	}

	// Get monthly stats
	totalUsages, linkUsages, codeUsages, totalCommission, err := h.affiliateRepo.GetMonthlyStats(ctx, affiliate.ID)
	if err != nil {
		log.Printf("[Affiliate] Error getting stats: %v", err)
		InternalError(w, "Gagal mengambil statistik")
		return
	}

	// Get affiliate balance
	balance, err := h.affiliateRepo.GetAffiliateBalance(ctx, userID)
	if err != nil {
		log.Printf("[Affiliate] Error getting balance: %v", err)
		balance = 0
	}

	Success(w, "", model.AffiliateStatsResponse{
		Code:              affiliate.Code,
		Link:              "https://restopup.com/?ref=" + strings.ToLower(affiliate.Code),
		TotalUsages:       totalUsages,
		LinkUsages:        linkUsages,
		CodeUsages:        codeUsages,
		TotalCommission:   totalCommission,
		AffiliateBalance:  balance,
		CommissionPercent: affiliate.CommissionPercent,
		DiscountEnabled:   affiliate.DiscountEnabled,
		DiscountPercent:   affiliate.DiscountPercent,
		MinDiscountAmount: affiliate.MinDiscountAmount,
	})
}

// UpdateMyAffiliateSettings handles PUT /api/v1/member/affiliate/settings
// Lets streamer toggle discount and set discount percent
func (h *AffiliateHandler) UpdateMyAffiliateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := ctx.Value("user_id").(int)
	if !ok {
		Unauthorized(w, "User ID tidak valid")
		return
	}

	affiliate, err := h.affiliateRepo.GetByUserID(ctx, userID)
	if err != nil || affiliate == nil {
		NotFound(w, "Anda belum terdaftar sebagai affiliate")
		return
	}

	var req struct {
		DiscountEnabled   *bool    `json:"discount_enabled"`
		DiscountPercent   *float64 `json:"discount_percent"`
		MinDiscountAmount *float64 `json:"min_discount_amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	updates := map[string]interface{}{}

	if req.DiscountEnabled != nil {
		updates["discount_enabled"] = *req.DiscountEnabled
	}
	if req.DiscountPercent != nil {
		// Validate: discount cannot exceed commission
		if *req.DiscountPercent > affiliate.CommissionPercent {
			BadRequest(w, "Diskon tidak boleh melebihi komisi Anda")
			return
		}
		if *req.DiscountPercent < 0 {
			BadRequest(w, "Diskon tidak boleh negatif")
			return
		}
		updates["discount_percent"] = *req.DiscountPercent
	}
	if req.MinDiscountAmount != nil {
		if *req.MinDiscountAmount < 0 {
			BadRequest(w, "Minimal belanja tidak boleh negatif")
			return
		}
		updates["min_discount_amount"] = *req.MinDiscountAmount
	}

	if len(updates) == 0 {
		BadRequest(w, "Tidak ada perubahan")
		return
	}

	if err := h.affiliateRepo.Update(ctx, affiliate.ID, updates); err != nil {
		log.Printf("[Affiliate] Error updating settings: %v", err)
		InternalError(w, "Gagal menyimpan pengaturan")
		return
	}

	Success(w, "Pengaturan berhasil disimpan", nil)
}

// ==========================================================
// ADMIN API
// ==========================================================

// AdminListAffiliates handles GET /api/v1/admin/affiliates
func (h *AffiliateHandler) AdminListAffiliates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	affiliates, err := h.affiliateRepo.ListAll(ctx)
	if err != nil {
		log.Printf("[Affiliate] Error listing affiliates: %v", err)
		InternalError(w, "Gagal mengambil data affiliate")
		return
	}

	// Enrich with user info and monthly stats
	type AffiliateWithUser struct {
		model.AffiliatePartner
		Username        string  `json:"username"`
		FullName        string  `json:"full_name"`
		MonthlyUsages   int     `json:"monthly_usages"`
		MonthlyCommission float64 `json:"monthly_commission"`
	}

	var result []AffiliateWithUser
	for _, a := range affiliates {
		user, _ := h.userRepo.GetByID(ctx, a.UserID)
		username, fullName := "", ""
		if user != nil {
			username = user.Username
			fullName = user.FullName
		}

		totalUsages, _, _, totalCommission, _ := h.affiliateRepo.GetMonthlyStats(ctx, a.ID)

		result = append(result, AffiliateWithUser{
			AffiliatePartner:  a,
			Username:          username,
			FullName:          fullName,
			MonthlyUsages:     totalUsages,
			MonthlyCommission: totalCommission,
		})
	}

	Success(w, "", map[string]interface{}{
		"affiliates": result,
		"total":      len(result),
	})
}

// AdminCreateAffiliate handles POST /api/v1/admin/affiliates
func (h *AffiliateHandler) AdminCreateAffiliate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		UserID               int     `json:"user_id"`
		Code                 string  `json:"code"`
		CommissionPercent    float64 `json:"commission_percent"`
		MinTransactionAmount float64 `json:"min_transaction_amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.UserID == 0 {
		BadRequest(w, "User ID wajib diisi")
		return
	}
	if req.Code == "" {
		BadRequest(w, "Kode affiliate wajib diisi")
		return
	}

	// Check if user exists
	user, err := h.userRepo.GetByID(ctx, req.UserID)
	if err != nil || user == nil {
		BadRequest(w, "User tidak ditemukan")
		return
	}

	// Check if code already exists
	existing, _ := h.affiliateRepo.GetByCode(ctx, req.Code)
	if existing != nil {
		BadRequest(w, "Kode affiliate sudah digunakan")
		return
	}

	// Check if user is already an affiliate
	existingUser, _ := h.affiliateRepo.GetByUserID(ctx, req.UserID)
	if existingUser != nil {
		BadRequest(w, "User sudah terdaftar sebagai affiliate")
		return
	}

	// Set defaults
	commissionPercent := req.CommissionPercent
	if commissionPercent == 0 {
		commissionPercent = 2.0 // Default 2%
	}
	minTransaction := req.MinTransactionAmount
	if minTransaction == 0 {
		minTransaction = 20000 // Default Rp 20.000
	}

	affiliate := &model.AffiliatePartner{
		UserID:               req.UserID,
		Code:                 strings.ToUpper(req.Code),
		CommissionPercent:    commissionPercent,
		DiscountEnabled:      false,
		DiscountPercent:      0,
		MinDiscountAmount:    0,
		MinTransactionAmount: minTransaction,
		MaxDiscountUses:      3,
		MaxCommissionUses:    10,
		Status:               model.AffiliateStatusActive,
	}

	if err := h.affiliateRepo.Create(ctx, affiliate); err != nil {
		log.Printf("[Affiliate] Error creating affiliate: %v", err)
		InternalError(w, "Gagal membuat affiliate")
		return
	}

	Created(w, "Affiliate berhasil dibuat", affiliate)
}

// AdminUpdateAffiliate handles PUT /api/v1/admin/affiliates/{id}
func (h *AffiliateHandler) AdminUpdateAffiliate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		BadRequest(w, "ID tidak valid")
		return
	}

	var req struct {
		CommissionPercent    *float64 `json:"commission_percent"`
		MinTransactionAmount *float64 `json:"min_transaction_amount"`
		MaxDiscountUses      *int     `json:"max_discount_uses"`
		MaxCommissionUses    *int     `json:"max_commission_uses"`
		Status               *string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	updates := map[string]interface{}{}

	if req.CommissionPercent != nil {
		updates["commission_percent"] = *req.CommissionPercent
	}
	if req.MinTransactionAmount != nil {
		updates["min_transaction_amount"] = *req.MinTransactionAmount
	}
	if req.MaxDiscountUses != nil {
		updates["max_discount_uses"] = *req.MaxDiscountUses
	}
	if req.MaxCommissionUses != nil {
		updates["max_commission_uses"] = *req.MaxCommissionUses
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if err := h.affiliateRepo.Update(ctx, id, updates); err != nil {
		log.Printf("[Affiliate] Error updating affiliate: %v", err)
		InternalError(w, "Gagal mengupdate affiliate")
		return
	}

	Success(w, "Affiliate berhasil diupdate", nil)
}
