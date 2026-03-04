package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/email"
	"govershop-api/internal/service/ipaymu"
)

// OrderHandler handles order-related HTTP requests
type OrderHandler struct {
	config       *config.Config
	orderRepo    *repository.OrderRepository
	paymentRepo  *repository.PaymentRepository
	productRepo  *repository.ProductRepository
	digiflazzSvc *digiflazz.Service
	ipaymuSvc    *ipaymu.Service
	emailSvc     *email.Service
}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler(
	cfg *config.Config,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	productRepo *repository.ProductRepository,
	digiflazzSvc *digiflazz.Service,
	ipaymuSvc *ipaymu.Service,
	emailSvc *email.Service,
) *OrderHandler {
	return &OrderHandler{
		config:       cfg,
		orderRepo:    orderRepo,
		paymentRepo:  paymentRepo,
		productRepo:  productRepo,
		digiflazzSvc: digiflazzSvc,
		ipaymuSvc:    ipaymuSvc,
		emailSvc:     emailSvc,
	}
}

// CreateOrder handles POST /api/v1/orders
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req model.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Validate required fields
	if req.BuyerSKUCode == "" {
		BadRequest(w, "buyer_sku_code wajib diisi")
		return
	}
	if req.CustomerNo == "" {
		BadRequest(w, "customer_no wajib diisi")
		return
	}

	// Get product details
	product, err := h.productRepo.GetBySKU(ctx, req.BuyerSKUCode)
	if err != nil {
		NotFound(w, "Produk tidak ditemukan")
		return
	}

	if !product.IsAvailable {
		BadRequest(w, "Produk sedang tidak tersedia")
		return
	}

	// ============================================================
	// CHECK DIGIFLAZZ BALANCE (cached, fail-open strategy)
	// ============================================================
	balance, _, balanceErr := h.digiflazzSvc.GetCachedBalance()
	if balanceErr == nil {
		// Balance check successful - compare with buy price (modal)
		if balance < product.BuyPrice {
			deficit := product.BuyPrice - balance
			log.Printf("[CreateOrder] Insufficient provider balance for product=%s", product.ProductName)

			// Send admin email alert asynchronously
			if h.config.AdminAlertEmail != "" {
				go func() {
					now := time.Now()
					alertData := email.BalanceAlertData{
						Date:           now.Format("02 January 2006"),
						Time:           now.Format("15:04") + " WIB",
						ProductName:    product.ProductName,
						ProductSKU:     product.BuyerSKUCode,
						CustomerPhone:  req.CustomerPhone,
						CustomerEmail:  req.CustomerEmail,
						BuyPrice:       product.BuyPrice,
						CurrentBalance: balance,
						Deficit:        deficit,
					}
					if err := h.emailSvc.SendAdminBalanceAlert(h.config.AdminAlertEmail, alertData); err != nil {
						log.Printf("[CreateOrder] Failed to send admin alert email")
					} else {
						log.Printf("[CreateOrder] Admin balance alert sent")
					}
				}()
			}

			// Return specific error code for frontend to handle
			JSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"success": false,
				"error":   "Maaf, saat ini sedang ada kendala teknis untuk topup produk ini.",
				"code":    "INSUFFICIENT_PROVIDER_BALANCE",
			})
			return
		}
	} else {
		// Balance check failed - fail-open: allow the order to proceed
		log.Printf("[CreateOrder] Balance check failed (fail-open, allowing order)")
	}

	// Generate unique ref_id for Digiflazz
	refID := fmt.Sprintf("GVS-%d-%s", time.Now().UnixMilli(), generateRandomString(6))

	// Determine selling price (use discount if available)
	// Plus flat admin fee (validasi akun) of Rp 10
	sellingPrice := product.SellingPrice
	if product.DiscountPrice != nil && *product.DiscountPrice > 0 {
		sellingPrice = *product.DiscountPrice
	}
	sellingPrice += 10 // Flat admin fee

	// Create order
	order := &model.Order{
		RefID:         refID,
		BuyerSKUCode:  req.BuyerSKUCode,
		ProductName:   product.ProductName,
		CustomerNo:    req.CustomerNo,
		BuyPrice:      product.BuyPrice,
		SellingPrice:  sellingPrice,
		Status:        model.OrderStatusPending,
		CustomerEmail: req.CustomerEmail,
		CustomerPhone: req.CustomerPhone,
		CustomerName:  req.CustomerName,
	}

	if err := h.orderRepo.Create(ctx, order); err != nil {
		InternalError(w, "Gagal membuat order")
		return
	}

	Created(w, "Order berhasil dibuat", order.ToResponse(nil))
}

// GetOrder handles GET /api/v1/orders/{id}
func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
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

	// Get payment if exists
	payment, _ := h.paymentRepo.GetByOrderID(ctx, orderID)

	Success(w, "", order.ToResponse(payment))
}

// InitiatePayment handles POST /api/v1/orders/{id}/pay
func (h *OrderHandler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("id")

	if orderID == "" {
		BadRequest(w, "Order ID tidak valid")
		return
	}

	var req model.InitiatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.PaymentMethod == "" {
		BadRequest(w, "payment_method wajib diisi")
		return
	}
	if req.PaymentChannel == "" {
		BadRequest(w, "payment_channel wajib diisi")
		return
	}

	// Get order
	log.Printf("[InitiatePayment] Looking for order ID: %s", orderID)
	order, err := h.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[InitiatePayment] Order not found error: %v", err)
		NotFound(w, "Order tidak ditemukan")
		return
	}

	// Check order status
	if order.Status != model.OrderStatusPending {
		BadRequest(w, "Order tidak dalam status pending")
		return
	}

	// Build notify URL (webhook callback for iPaymu)
	scheme := "https"
	if r.TLS == nil && r.Host == "localhost:8080" {
		scheme = "http"
	}
	notifyURL := fmt.Sprintf("%s://%s/api/v1/webhook/ipaymu", scheme, r.Host)

	customerName := "Customer"
	if order.CustomerName != "" {
		customerName = order.CustomerName
	}
	customerEmail := order.CustomerEmail
	if customerEmail == "" {
		customerEmail = "customer@restopup.com"
	}
	customerPhone := order.CustomerPhone
	if customerPhone == "" {
		customerPhone = "08123456789"
	}

	// Determine base price
	sellingPrice := order.SellingPrice

	// Flat admin fee
	var adminFee float64 = 10

	// Amount to send to iPaymu = selling price + admin fee
	// NOTE: Do NOT add paymentFee here — iPaymu adds its own transaction fee automatically
	// If we add it, the customer gets charged double fee
	ipaymuAmount := int(sellingPrice + adminFee)

	// Create payment via iPaymu Direct Payment
	ipaymuReq := ipaymu.DirectPaymentRequest{
		Name:           customerName,
		Phone:          customerPhone,
		Email:          customerEmail,
		Amount:         ipaymuAmount,
		NotifyURL:      notifyURL,
		PaymentMethod:  req.PaymentMethod,
		PaymentChannel: req.PaymentChannel,
		ReferenceID:    order.RefID,
		BuyerName:      customerName,
		BuyerEmail:     customerEmail,
		BuyerPhone:     customerPhone,
	}

	ipaymuResp, err := h.ipaymuSvc.CreateDirectPayment(ipaymuReq)
	if err != nil {
		InternalError(w, fmt.Sprintf("Gagal membuat pembayaran: %v", err))
		return
	}

	// Parse expiry time from iPaymu (format: "2025-01-01 12:00:00")
	var expiredAt time.Time
	t, parseErr := time.Parse("2006-01-02 15:04:05", ipaymuResp.Data.Expired)
	if parseErr == nil && !t.IsZero() {
		expiredAt = t
	} else {
		// Fallback: 24 hours from now
		expiredAt = time.Now().Add(24 * time.Hour)
	}

	// Determine QR image URL for QRIS
	qrImageURL := ""
	paymentNumber := ipaymuResp.Data.PaymentNo
	if strings.ToLower(req.PaymentMethod) == "qris" && ipaymuResp.Data.QrUrl != "" {
		qrImageURL = ipaymuResp.Data.QrUrl
	}

	paymentChannelValue := ipaymuResp.Data.PaymentChannel
	if paymentChannelValue == "" {
		paymentChannelValue = req.PaymentChannel // Fallback because iPaymu response might be empty
	}

	payment := &model.Payment{
		OrderID:             orderID,
		Amount:              sellingPrice,                                           // Pure product price (e.g., 1466)
		Fee:                 float64(ipaymuResp.Data.Fee),                           // iPaymu's transaction fee
		TotalPayment:        sellingPrice + adminFee + float64(ipaymuResp.Data.Fee), // Actual total customer pays
		PaymentMethod:       model.PaymentMethod(strings.ToLower(paymentChannelValue)),
		PaymentNumber:       paymentNumber,
		QRImageURL:          qrImageURL,
		IpaymuTransactionID: ipaymuResp.Data.TransactionID,
		PaymentGateway:      "ipaymu",
		Status:              model.PaymentStatusPending,
		ExpiredAt:           expiredAt,
	}

	if err := h.paymentRepo.Create(ctx, payment); err != nil {
		InternalError(w, "Gagal menyimpan data pembayaran")
		return
	}

	// Update order status
	if err := h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusWaitingPayment); err != nil {
		InternalError(w, "Gagal update status order")
		return
	}

	Success(w, "Pembayaran berhasil dibuat", payment.ToResponse())
}

// CancelOrder handles POST /api/v1/orders/{id}/cancel
func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
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

	// Only allow cancellation for pending or waiting_payment status
	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusWaitingPayment {
		BadRequest(w, "Order tidak dapat dibatalkan")
		return
	}

	// Cancel payment if exists
	if order.Status == model.OrderStatusWaitingPayment {
		// iPaymu payments auto-expire, no cancel API needed
		_ = h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusCancelled)
	}

	// Update order status
	if err := h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusCancelled); err != nil {
		InternalError(w, "Gagal membatalkan order")
		return
	}

	Success(w, "Order berhasil dibatalkan", nil)
}

// GetOrderStatus handles GET /api/v1/orders/{id}/status
func (h *OrderHandler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
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

	// Get payment if exists
	payment, _ := h.paymentRepo.GetByOrderID(ctx, orderID)

	// Check status with iPaymu if pending and has iPaymu transaction ID
	if payment != nil && payment.Status == model.PaymentStatusPending && payment.IpaymuTransactionID > 0 {
		ipaymuStatus, err := h.ipaymuSvc.CheckTransaction(payment.IpaymuTransactionID)
		if err == nil && ipaymuStatus.Success {
			if ipaymuStatus.Data.Status == 1 { // 1 = success
				_ = h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusCompleted)
				_ = h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusPaid)
				payment.Status = model.PaymentStatusCompleted
				order.Status = model.OrderStatusPaid
			} else if ipaymuStatus.Data.Status == -1 { // -1 = failed/expired
				_ = h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusExpired)
				payment.Status = model.PaymentStatusExpired
			}
		}
	}

	// Build response
	response := map[string]interface{}{
		"order_id":      order.ID,
		"ref_id":        order.RefID,
		"status":        order.Status,
		"status_label":  order.GetStatusLabel(),
		"serial_number": order.SerialNumber,
		"message":       order.DigiflazzMsg,
	}

	// Add payment info with proper response format
	if payment != nil {
		response["payment"] = payment.ToResponse()
	}

	Success(w, "", response)
}

// GetPaymentMethods handles GET /api/v1/payment-methods
// Fetches payment channels dynamically from iPaymu and maps to frontend format
func (h *OrderHandler) GetPaymentMethods(w http.ResponseWriter, r *http.Request) {
	channels, err := h.ipaymuSvc.GetPaymentChannels()
	if err != nil {
		log.Printf("[PaymentMethods] Failed to fetch from iPaymu: %v", err)
		// Return empty list — no fallback to old static methods
		Success(w, "", map[string]interface{}{
			"payment_methods": []interface{}{},
		})
		return
	}

	// Map iPaymu channels to frontend-compatible format (lowercase)
	type FEPaymentMethod struct {
		Code        string `json:"code"`
		Name        string `json:"name"`
		Type        string `json:"type"` // e.g., "va", "qris", "cc", "cstore"
		Description string `json:"description,omitempty"`
		Logo        string `json:"logo,omitempty"`
		Fee         *struct {
			Flat    float64 `json:"flat"`
			Percent float64 `json:"percent"`
		} `json:"fee,omitempty"`
	}

	var methods []FEPaymentMethod

	// Known logos for payment channels (iPaymu may return empty logos)
	knownLogos := map[string]string{
		"qris":     "https://storage.googleapis.com/ipaymu-docs/assets/qris_default.png",
		"mpm":      "https://storage.googleapis.com/ipaymu-docs/assets/qris_default.png",
		"bca":      "https://upld.zone.id/uploads/exi8kviq/bank-bni-seeklogo.webp",
		"bni":      "https://upld.zone.id/uploads/exi8kviq/bank-bni-seeklogo.webp",
		"bri":      "https://upld.zone.id/uploads/exi8kviq/bank-bri-seeklogo.webp",
		"mandiri":  "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"cimb":     "https://upld.zone.id/uploads/exi8kviq/cimb-niaga.webp",
		"permata":  "https://upld.zone.id/uploads/exi8kviq/permata.webp",
		"bsi":      "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"danamon":  "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"muamalat": "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"bag":      "https://upld.zone.id/uploads/exi8kviq/bank-artha-graha-internasional.webp",
		"cc":       "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
	}
	// Category-level logos (used when individual channel has no logo)
	categoryLogos := map[string]string{
		"qris":   "https://storage.googleapis.com/ipaymu-docs/assets/qris_default.png",
		"va":     "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"cstore": "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
		"cc":     "https://upld.zone.id/uploads/exi8kviq/virtual-account.webp",
	}

	for _, category := range channels.Data {
		categoryType := strings.ToLower(category.Code) // e.g., "va", "qris", "cstore"

		for _, ch := range category.Channels {
			// Determine logo: prefer iPaymu → known channel → category fallback
			logo := ch.Logo
			if logo == "" {
				if kl, ok := knownLogos[strings.ToLower(ch.Code)]; ok {
					logo = kl
				} else if cl, ok := categoryLogos[categoryType]; ok {
					logo = cl
				}
			}

			m := FEPaymentMethod{
				Code:        strings.ToLower(ch.Code),
				Name:        ch.Name,
				Type:        categoryType,
				Description: ch.Description,
				Logo:        logo,
			}

			feePercent := 0.0
			feeFlat := ch.TransactionFee.ActualFee
			if ch.TransactionFee.ActualFeeType == "PERCENT" {
				feePercent = ch.TransactionFee.ActualFee
				feeFlat = 0
			}

			m.Fee = &struct {
				Flat    float64 `json:"flat"`
				Percent float64 `json:"percent"`
			}{
				Flat:    feeFlat,
				Percent: feePercent,
			}
			methods = append(methods, m)
		}
	}

	Success(w, "", map[string]interface{}{
		"payment_methods": methods,
	})
}

// generateRandomString generates a random alphanumeric string
func generateRandomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // Ensure different values
	}
	return string(result)
}

// TrackOrders handles GET /api/v1/orders/track?phone=xxx
func (h *OrderHandler) TrackOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	phone := r.URL.Query().Get("phone")
	if phone == "" {
		BadRequest(w, "Nomor telepon wajib diisi")
		return
	}

	// Validate phone format (basic check)
	if len(phone) < 10 {
		BadRequest(w, "Nomor telepon tidak valid")
		return
	}

	// Get orders by phone
	orders, err := h.orderRepo.GetByCustomerPhone(ctx, phone, 20)
	if err != nil {
		log.Printf("[TrackOrders] Error getting orders: %v", err)
		InternalError(w, "Gagal mengambil data pesanan")
		return
	}

	// Convert to response format
	var responses []map[string]interface{}
	for _, order := range orders {
		// Get payment info for each order
		payment, _ := h.paymentRepo.GetByOrderID(ctx, order.ID)

		resp := map[string]interface{}{
			"id":           order.ID,
			"ref_id":       order.RefID,
			"product_name": order.ProductName,
			"customer_no":  order.CustomerNo,
			"price":        order.SellingPrice,
			"status":       order.Status,
			"status_label": order.GetStatusLabel(),
			"created_at":   order.CreatedAt,
		}

		if order.SerialNumber != "" {
			resp["serial_number"] = order.SerialNumber
		}
		if order.DigiflazzMsg != "" {
			resp["message"] = order.DigiflazzMsg
		}
		if order.CompletedAt != nil {
			resp["completed_at"] = order.CompletedAt
		}
		if payment != nil {
			resp["payment_method"] = payment.PaymentMethod
		}

		responses = append(responses, resp)
	}

	Success(w, "", map[string]interface{}{
		"orders": responses,
		"total":  len(responses),
	})
}
