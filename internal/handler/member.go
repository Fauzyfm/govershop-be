package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/email"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// MemberHandler handles member-related HTTP requests
type MemberHandler struct {
	config       *config.Config
	userRepo     *repository.UserRepository
	productRepo  *repository.ProductRepository
	orderRepo    *repository.OrderRepository
	digiflazzSvc *digiflazz.Service
	emailSvc     *email.Service
}

// NewMemberHandler creates a new MemberHandler
func NewMemberHandler(
	cfg *config.Config,
	userRepo *repository.UserRepository,
	productRepo *repository.ProductRepository,
	orderRepo *repository.OrderRepository,
	digiflazzSvc *digiflazz.Service,
	emailSvc *email.Service,
) *MemberHandler {
	return &MemberHandler{
		config:       cfg,
		userRepo:     userRepo,
		productRepo:  productRepo,
		orderRepo:    orderRepo,
		digiflazzSvc: digiflazzSvc,
		emailSvc:     emailSvc,
	}
}

// ==========================================
// MEMBER AUTH ENDPOINTS
// ==========================================

// Login handles POST /api/v1/member/login
func (h *MemberHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.MemberLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	// Get user by username
	user, err := h.userRepo.GetByUsername(r.Context(), req.Username)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	if user == nil {
		Unauthorized(w, "Username atau password salah")
		return
	}

	// Verify only members can login here
	if user.Role != model.UserRoleMember {
		Unauthorized(w, "Username atau password salah")
		return
	}

	// Check if user is active
	if user.Status != model.UserStatusActive {
		Error(w, http.StatusForbidden, "Akun Anda telah dinonaktifkan. Hubungi admin.")
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		Unauthorized(w, "Username atau password salah")
		return
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":     user.Username,
		"user_id": user.ID,
		"role":    user.Role,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.config.JWTSecretGovershop))
	if err != nil {
		log.Printf("Error signing token: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	// Set HTTP-only cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "member_token",
		Value:    tokenString,
		Expires:  time.Now().Add(24 * time.Hour),
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.Env == "production", // Secure in production
		SameSite: http.SameSiteLaxMode,         // Lax is usually checking for CSRF but allows top-level nav. For API, None might be needed if cross-site?
		// Since we are likely on same domain or handling via CORS with credentials:
		// If frontend is localhost:3000 and backend localhost:8080, they are cross-port but same-site (localhost).
		// However, some browsers treat ports differently.
		// Let's use SameSiteLaxMode which usually works for localhost.
	})

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user":    user.ToResponse(),
	})
}

// ForgotPassword handles POST /api/v1/member/forgot-password
func (h *MemberHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req model.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	// Get user by email
	user, err := h.userRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		log.Printf("Error getting user by email: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	// Return error if email not found
	if user == nil {
		NotFound(w, "Email tidak ditemukan")
		return
	}

	// Generate JWT reset token (valid for 1 hour)
	resetToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"type":    "password_reset",
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
	})

	tokenString, err := resetToken.SignedString([]byte(h.config.JWTSecretGovershop))
	if err != nil {
		log.Printf("Error signing reset token: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	// Send email with reset link
	resetLink := fmt.Sprintf("%s/member/reset-password?token=%s", h.config.FrontendURL, tokenString)

	// Send email synchronously to support frontend loading state
	if err := h.emailSvc.SendResetPasswordEmail(*user.Email, resetLink); err != nil {
		log.Printf("❌ Failed to send reset email to %s: %v", *user.Email, err)
		InternalError(w, "Gagal mengirim email reset password. Silakan coba lagi.")
		return
	}

	log.Printf("📧 Reset email sent to %s", *user.Email)

	Success(w, "Link reset password telah dikirim ke email Anda.", nil)
}

// ResetPassword handles POST /api/v1/member/reset-password
func (h *MemberHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req model.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	// Parse and validate JWT token
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(h.config.JWTSecretGovershop), nil
	})

	if err != nil || !token.Valid {
		BadRequest(w, "Token tidak valid atau sudah kadaluarsa")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		BadRequest(w, "Token tidak valid")
		return
	}

	// Verify token type
	if claims["type"] != "password_reset" {
		BadRequest(w, "Token tidak valid")
		return
	}

	// Get user ID from token
	userID := int(claims["user_id"].(float64))

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	// Update password
	if err := h.userRepo.UpdatePassword(r.Context(), userID, string(hashedPassword)); err != nil {
		log.Printf("Error updating password: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	Success(w, "Password berhasil diubah. Silakan login dengan password baru.", nil)
}

// ==========================================
// MEMBER DASHBOARD ENDPOINTS
// ==========================================

// GetDashboard handles GET /api/v1/member/dashboard
func (h *MemberHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	// Get user for balance
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		NotFound(w, "User not found")
		return
	}

	// Get order stats
	total, success, pending, today, err := h.userRepo.GetMemberStats(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting member stats: %v", err)
		total, success, pending, today = 0, 0, 0, 0
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": model.MemberDashboardResponse{
			Balance:       user.Balance,
			TotalOrders:   total,
			SuccessOrders: success,
			PendingOrders: pending,
			TodayOrders:   today,
		},
	})
}

// GetProfile handles GET /api/v1/member/profile
func (h *MemberHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		NotFound(w, "User not found")
		return
	}

	Success(w, "", user.ToResponse())
}

// UpdateProfile handles PUT /api/v1/member/profile
func (h *MemberHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	var req model.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	updates := map[string]interface{}{}
	if req.FullName != nil {
		updates["full_name"] = *req.FullName
	}
	if req.WhatsApp != nil {
		updates["whatsapp"] = *req.WhatsApp
	}

	if err := h.userRepo.Update(r.Context(), userID, updates); err != nil {
		log.Printf("Error updating profile: %v", err)
		InternalError(w, "Failed to update profile")
		return
	}

	Success(w, "Profile berhasil diupdate", nil)
}

// GetDeposits handles GET /api/v1/member/deposits
func (h *MemberHandler) GetDeposits(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 10
	}

	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	depositType := r.URL.Query().Get("type")

	deposits, total, err := h.userRepo.GetDeposits(r.Context(), userID, limit, offset, dateFrom, dateTo, depositType)
	if err != nil {
		log.Printf("Error getting deposits: %v", err)
		InternalError(w, "Failed to get deposits")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"deposits": deposits,
		"total":    total,
	})
}

// GetProducts handles GET /api/v1/member/products
func (h *MemberHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	category := r.URL.Query().Get("category")
	brand := r.URL.Query().Get("brand")
	pType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 20
	}

	products, total, err := h.productRepo.GetProductsWithFilters(r.Context(), search, category, brand, pType, status, limit, offset)
	if err != nil {
		log.Printf("Error getting products: %v", err)
		InternalError(w, "Failed to get products")
		return
	}

	// Get default member markup
	defaultMarkup := 0.0

	var responses []model.ProductResponse
	for _, p := range products {
		responses = append(responses, p.ToMemberResponse(defaultMarkup))
	}

	if responses == nil {
		responses = []model.ProductResponse{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"products": responses,
			"total":    total,
		},
	})
}

// GetProductBySku handles GET /api/v1/member/products/{sku}
func (h *MemberHandler) GetProductBySku(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	if sku == "" {
		BadRequest(w, "SKU required")
		return
	}

	product, err := h.productRepo.GetBySKU(r.Context(), sku)
	if err != nil {
		log.Printf("Error getting product: %v", err)
		InternalError(w, "Failed to get product")
		return
	}

	if product == nil {
		NotFound(w, "Product not found")
		return
	}

	// Use default member markup if not set
	defaultMarkup := 0.0

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    product.ToMemberResponse(defaultMarkup),
	})
}

// CreateOrder handles POST /api/v1/member/orders
func (h *MemberHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("user_id").(int)
	// username is not used

	var req model.MemberOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	if req.BuyerSKUCode == "" || req.DestinationNumber == "" {
		BadRequest(w, "Produk dan nomor tujuan wajib diisi")
		return
	}

	// 1. Get Product
	product, err := h.productRepo.GetBySKU(ctx, req.BuyerSKUCode)
	if err != nil {
		log.Printf("Error getting product: %v", err)
		InternalError(w, "Internal server error")
		return
	}
	if product == nil || !product.IsAvailable {
		BadRequest(w, "Produk tidak tersedia")
		return
	}

	// 2. Calculate Member Price
	defaultMarkup := 0.0 // Default member markup
	resp := product.ToMemberResponse(defaultMarkup)
	amount := resp.Price

	// 3. Generate Order Ref ID (INV-...)
	refID := fmt.Sprintf("INV-%d-%s", time.Now().Unix(), generateRandomString(5))

	// 4. Deduct Balance (Transaction)
	description := fmt.Sprintf("Pembelian %s - %s", product.ProductName, req.DestinationNumber)
	if err := h.userRepo.DeductBalance(ctx, userID, amount, description, refID); err != nil {
		log.Printf("Error deducting balance: %v", err)
		if err.Error() == "insufficient balance" {
			BadRequest(w, "Saldo tidak mencukupi")
		} else {
			InternalError(w, "Gagal memproses transaksi")
		}
		return
	}

	// 5. Create Order
	memberID := userID
	memberPrice := amount
	order := &model.Order{
		MemberID:     &memberID,
		MemberPrice:  &memberPrice,
		RefID:        refID,
		BuyerSKUCode: product.BuyerSKUCode,
		ProductName:  product.ProductName,
		CustomerNo:   req.DestinationNumber,
		Status:       model.OrderStatusProcessing, // Already paid via balance
		SellingPrice: amount,
		BuyPrice:     product.BuyPrice,
		OrderSource:  "member",
	}

	if err := h.orderRepo.Create(ctx, order); err != nil {
		log.Printf("CRITICAL: Failed to create order after balance deduction. UserID: %d, Amount: %f, RefID: %s. Error: %v", userID, amount, refID, err)

		// REFUND BALANCE
		refundDesc := fmt.Sprintf("Refund Failed Order %s", refID)
		if refundErr := h.userRepo.TopupBalance(ctx, userID, amount, refundDesc, "SYSTEM"); refundErr != nil {
			log.Printf("CRITICAL: Failed to refund balance. UserID: %d, Amount: %f. Error: %v", userID, amount, refundErr)
		}

		InternalError(w, "Gagal membuat order. Saldo telah dikembalikan.")
		return
	}

	// 6. Call Digiflazz
	// Use Digiflazz Service to topup
	dfReq := digiflazz.TopupRequest{
		BuyerSKUCode: product.BuyerSKUCode,
		CustomerNo:   req.DestinationNumber,
		RefID:        refID,
	}

	_, err = h.digiflazzSvc.CreateTransaction(dfReq)

	// Handle failure calling Digiflazz
	if err != nil {
		log.Printf("Error calling Digiflazz: %v", err)

		// Mark order as Failed and Refund
		h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed)

		refundDesc := fmt.Sprintf("Refund Gagal Transaksi %s", refID)
		h.userRepo.TopupBalance(ctx, userID, amount, refundDesc, "SYSTEM")

		InternalError(w, "Gagal memproses ke provider. Saldo dikembalikan.")
		return
	}

	// 7. Return Success
	// Get latest user balance
	user, _ := h.userRepo.GetByID(ctx, userID)

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Transaksi sedang diproses",
		"data": map[string]interface{}{
			"order_id": order.ID,
			"ref_id":   order.RefID,
			"status":   order.Status,
			"balance":  user.Balance,
		},
	})
}

// ValidateMemberAccount handles POST /api/v1/member/validate-account
// Deducts balance for validation, refunds if Digiflazz fails.
func (h *MemberHandler) ValidateMemberAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value("user_id").(int)

	var req struct {
		Brand      string `json:"brand"`
		CustomerNo string `json:"customer_no"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.Brand == "" {
		BadRequest(w, "brand wajib diisi")
		return
	}
	if req.CustomerNo == "" {
		BadRequest(w, "customer_no wajib diisi")
		return
	}

	brandSlug := strings.ToLower(strings.ReplaceAll(req.Brand, " ", ""))
	checkUserSKU := fmt.Sprintf("checkuser%s", brandSlug)

	checkProduct, err := h.productRepo.GetBySKU(ctx, checkUserSKU)
	if err != nil || !checkProduct.IsAvailable {
		// Manual validation, no fee
		Success(w, "Validasi manual", map[string]interface{}{
			"is_valid":       true,
			"account_name":   "",
			"customer_no":    req.CustomerNo,
			"brand":          req.Brand,
			"message":        "Pastikan User ID sesuai. Kesalahan input diluar tanggung jawab kami.",
			"validation_fee": 0,
		})
		return
	}

	// Calculate member price
	defaultMarkup := 0.0
	respData := checkProduct.ToMemberResponse(defaultMarkup)
	validationFee := respData.Price

	// Generate RefID
	refID := fmt.Sprintf("MVAL-%d-%s", time.Now().UnixMilli(), generateRandomString(4))

	// 1. Deduct balance first
	description := fmt.Sprintf("Check User %s - %s", req.Brand, req.CustomerNo)
	if err := h.userRepo.DeductBalance(ctx, userID, validationFee, description, refID); err != nil {
		if err.Error() == "insufficient balance" {
			BadRequest(w, "Saldo tidak mencukupi untuk melakukan pengecekan ID")
		} else {
			InternalError(w, "Gagal memproses transaksi pengecekan ID")
		}
		return
	}

	// 2. Create Pending Order
	memberID := userID
	memberPrice := validationFee
	order := &model.Order{
		MemberID:     &memberID,
		MemberPrice:  &memberPrice,
		RefID:        refID,
		BuyerSKUCode: checkUserSKU,
		ProductName:  fmt.Sprintf("Check User %s", req.Brand),
		CustomerNo:   req.CustomerNo,
		Status:       model.OrderStatusProcessing,
		SellingPrice: validationFee,
		BuyPrice:     checkProduct.BuyPrice,
		OrderSource:  "member",
	}

	if err := h.orderRepo.Create(ctx, order); err != nil {
		// Refund since we can't save order
		h.userRepo.TopupBalance(ctx, userID, validationFee, fmt.Sprintf("Refund Gagal System %s", refID), "SYSTEM")
		InternalError(w, "Gagal membuat riwayat. Saldo dikembalikan.")
		return
	}

	// 3. Call Digiflazz
	resp, digiErr := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: checkUserSKU,
		CustomerNo:   req.CustomerNo,
		RefID:        refID,
		Testing:      false,
	})

	if digiErr != nil {
		h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed)
		h.userRepo.TopupBalance(ctx, userID, validationFee, fmt.Sprintf("Refund Gagal Provider %s", refID), "SYSTEM")
		InternalError(w, "Gagal validasi ke provider. Saldo dikembalikan.")
		return
	}

	// 4. Check Response
	isValid := resp.Data.Status == "Sukses"
	accountName := ""
	message := resp.Data.Message

	if resp.Data.Status == "Pending" {
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			time.Sleep(2 * time.Second)
			retryResp, retryErr := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
				BuyerSKUCode: checkUserSKU,
				CustomerNo:   req.CustomerNo,
				RefID:        resp.Data.RefID,
				Testing:      false,
			})
			if retryErr == nil && retryResp != nil {
				if retryResp.Data.Status == "Sukses" || retryResp.Data.Status == "Gagal" {
					isValid = retryResp.Data.Status == "Sukses"
					message = retryResp.Data.Message
					resp.Data = retryResp.Data
					break
				}
			}
		}
	}

	var updateStatus model.OrderStatus
	if isValid {
		updateStatus = model.OrderStatusSuccess
		accountName = resp.Data.SN
		if accountName == "" {
			accountName = "Akun Valid (Nama tidak muncul)"
		}
	} else if resp.Data.Status == "Pending" {
		updateStatus = model.OrderStatusProcessing
	} else {
		updateStatus = model.OrderStatusFailed
	}

	// Update order
	h.orderRepo.UpdateStatus(ctx, order.ID, updateStatus)

	// User requested: "jika check user itu gagal dari digiflazz itu sendiri tidak akan memotong saldonya"
	// So if status is Failed (isValid == false and not pending), we refund!
	if updateStatus == model.OrderStatusFailed {
		refundDesc := fmt.Sprintf("Refund Invalid ID %s", refID)
		h.userRepo.TopupBalance(ctx, userID, validationFee, refundDesc, "SYSTEM")
	}

	Success(w, "Validasi selesai", map[string]interface{}{
		"is_valid":       isValid,
		"account_name":   accountName,
		"customer_no":    req.CustomerNo,
		"brand":          req.Brand,
		"message":        message,
		"validation_fee": validationFee,
	})
}

// GetOrders handles GET /api/v1/member/orders
func (h *MemberHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 {
		limit = 10
	}

	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")

	orders, total, err := h.orderRepo.GetByMemberID(r.Context(), userID, limit, offset, dateFrom, dateTo, status, search)
	if err != nil {
		log.Printf("Error getting member orders: %v", err)
		InternalError(w, "Failed to get orders")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"orders":  orders,
		"total":   total,
	})
}

// GetOrderByID handles GET /api/v1/member/orders/{id}
func (h *MemberHandler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	orderID := r.PathValue("id")

	if orderID == "" {
		BadRequest(w, "Order ID required")
		return
	}

	order, err := h.orderRepo.GetByID(r.Context(), orderID)
	if err != nil {
		log.Printf("Error getting order: %v", err)
		InternalError(w, "Failed to get order")
		return
	}

	if order == nil {
		NotFound(w, "Order not found")
		return
	}

	// Verify ownership - check if order belongs to this member
	if order.MemberID == nil || *order.MemberID != userID {
		NotFound(w, "Order not found")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    order,
	})
}

// ChangePassword handles PUT /api/v1/member/password
func (h *MemberHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		BadRequest(w, "Password lama dan baru wajib diisi")
		return
	}

	if len(req.NewPassword) < 6 {
		BadRequest(w, "Password baru minimal 6 karakter")
		return
	}

	// Get current user
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		NotFound(w, "User not found")
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		BadRequest(w, "Password lama salah")
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	// Update password
	if err := h.userRepo.UpdatePassword(r.Context(), userID, string(hashedPassword)); err != nil {
		log.Printf("Error updating password: %v", err)
		InternalError(w, "Failed to update password")
		return
	}

	Success(w, "Password berhasil diubah", nil)
}

// ==========================================
// ADMIN MEMBER MANAGEMENT ENDPOINTS
// ==========================================

// GetMembers handles GET /api/v1/admin/members
func (h *MemberHandler) GetMembers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	search := r.URL.Query().Get("search")

	if limit <= 0 {
		limit = 20
	}

	members, total, err := h.userRepo.GetAllMembers(r.Context(), limit, offset, search)
	if err != nil {
		log.Printf("Error getting members: %v", err)
		InternalError(w, "Failed to get members")
		return
	}

	var responses []model.UserResponse
	for _, m := range members {
		responses = append(responses, m.ToResponse())
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"members": responses,
		"total":   total,
	})
}

// CreateMember handles POST /api/v1/admin/members
func (h *MemberHandler) CreateMember(w http.ResponseWriter, r *http.Request) {
	var req model.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" || req.FullName == "" {
		BadRequest(w, "Username, password, dan nama lengkap wajib diisi")
		return
	}

	existing, _ := h.userRepo.GetByUsername(r.Context(), req.Username)
	if existing != nil {
		Error(w, http.StatusConflict, "Username sudah digunakan")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		InternalError(w, "Internal server error")
		return
	}

	user := &model.User{
		Username: req.Username,
		Password: string(hashedPassword),
		FullName: req.FullName,
		Role:     model.UserRoleMember,
		Balance:  0,
		Status:   model.UserStatusActive,
	}

	if req.Email != "" {
		user.Email = &req.Email
	}
	if req.WhatsApp != "" {
		user.WhatsApp = &req.WhatsApp
	}

	if err := h.userRepo.Create(r.Context(), user); err != nil {
		log.Printf("Error creating member: %v", err)
		InternalError(w, "Failed to create member")
		return
	}

	Created(w, "Member berhasil dibuat", user.ToResponse())
}

// GetMember handles GET /api/v1/admin/members/{id}
func (h *MemberHandler) GetMember(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		BadRequest(w, "Invalid member ID")
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("Error getting member: %v", err)
		InternalError(w, "Failed to get member")
		return
	}

	if user == nil || user.Role != model.UserRoleMember {
		NotFound(w, "Member not found")
		return
	}

	Success(w, "", user.ToResponse())
}

// UpdateMember handles PUT /api/v1/admin/members/{id}
func (h *MemberHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		BadRequest(w, "Invalid member ID")
		return
	}

	var req model.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	updates := map[string]interface{}{}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.FullName != nil {
		updates["full_name"] = *req.FullName
	}
	if req.WhatsApp != nil {
		updates["whatsapp"] = *req.WhatsApp
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if err := h.userRepo.Update(r.Context(), id, updates); err != nil {
		log.Printf("Error updating member: %v", err)
		InternalError(w, "Failed to update member")
		return
	}

	Success(w, "Member berhasil diupdate", nil)
}

// DeleteMember handles DELETE /api/v1/admin/members/{id}
func (h *MemberHandler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		BadRequest(w, "Invalid member ID")
		return
	}

	if err := h.userRepo.Delete(r.Context(), id); err != nil {
		log.Printf("Error deleting member: %v", err)
		InternalError(w, "Failed to delete member")
		return
	}

	Success(w, "Member berhasil dinonaktifkan", nil)
}

// TopupMember handles POST /api/v1/admin/members/{id}/topup
func (h *MemberHandler) TopupMember(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		BadRequest(w, "Invalid member ID")
		return
	}

	var req model.TopupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	if req.Amount < 20000 {
		BadRequest(w, "Minimum topup adalah Rp 20.000")
		return
	}

	adminUsername := r.Context().Value("user").(string)

	description := req.Description
	if description == "" {
		description = fmt.Sprintf("Topup saldo oleh admin %s", adminUsername)
	}

	if err := h.userRepo.TopupBalance(r.Context(), id, req.Amount, description, adminUsername); err != nil {
		log.Printf("Error topping up member: %v", err)
		InternalError(w, "Failed to topup member")
		return
	}

	user, _ := h.userRepo.GetByID(r.Context(), id)

	JSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"message":     "Topup berhasil",
		"new_balance": user.Balance,
	})
}
