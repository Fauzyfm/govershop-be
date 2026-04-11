package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/email"
	"govershop-api/internal/service/ipaymu"
	"govershop-api/internal/service/telegram"
)

// OrderHandler handles order-related HTTP requests
type OrderHandler struct {
	config        *config.Config
	orderRepo     *repository.OrderRepository
	paymentRepo   *repository.PaymentRepository
	productRepo   *repository.ProductRepository
	userRepo      *repository.UserRepository
	affiliateRepo *repository.AffiliateRepository
	digiflazzSvc  *digiflazz.Service
	ipaymuSvc     *ipaymu.Service
	emailSvc      *email.Service
	telegramSvc   *telegram.Service
}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler(
	cfg *config.Config,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	productRepo *repository.ProductRepository,
	userRepo *repository.UserRepository,
	affiliateRepo *repository.AffiliateRepository,
	digiflazzSvc *digiflazz.Service,
	ipaymuSvc *ipaymu.Service,
	emailSvc *email.Service,
	telegramSvc *telegram.Service,
) *OrderHandler {
	return &OrderHandler{
		config:        cfg,
		orderRepo:     orderRepo,
		paymentRepo:   paymentRepo,
		productRepo:   productRepo,
		userRepo:      userRepo,
		affiliateRepo: affiliateRepo,
		digiflazzSvc:  digiflazzSvc,
		ipaymuSvc:     ipaymuSvc,
		emailSvc:      emailSvc,
		telegramSvc:   telegramSvc,
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

			// Send admin Telegram alert asynchronously
			go h.telegramSvc.NotifyInsufficientBalance(product.ProductName, product.BuyerSKUCode, product.BuyPrice, balance, deficit, req.CustomerPhone, req.CustomerEmail)

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

	// Attach affiliate info if provided (from link ref or code input)
	if req.AffiliateCode != "" {
		affiliate, affErr := h.affiliateRepo.GetByCode(ctx, req.AffiliateCode)
		if affErr == nil && affiliate != nil && affiliate.Status == model.AffiliateStatusActive {
			order.AffiliateID = &affiliate.ID
			channel := req.AffiliateChannel
			if channel == "" {
				channel = model.AffiliateChannelLink
			}
			order.AffiliateChannel = &channel
			log.Printf("[CreateOrder] Affiliate attached: code=%s id=%d channel=%s", affiliate.Code, affiliate.ID, channel)
		}
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

	// Calculate payment fee from iPaymu channels data
	var paymentFee float64 = 0
	if channels, err := h.ipaymuSvc.GetPaymentChannels(); err == nil {
		for _, category := range channels.Data {
			for _, ch := range category.Channels {
				if strings.EqualFold(ch.Code, req.PaymentMethod) || strings.EqualFold(ch.Code, req.PaymentChannel) {
					if ch.TransactionFee.ActualFeeType == "PERCENT" {
						paymentFee = math.Ceil((ch.TransactionFee.ActualFee / 100) * sellingPrice)
					} else {
						paymentFee = ch.TransactionFee.ActualFee
					}
					paymentFee += ch.TransactionFee.AdditionalFee
					break
				}
			}
		}
	}

	// Total = product + admin + transaction fee (this is what customer pays)
	totalFee := adminFee + paymentFee
	totalPrice := sellingPrice + totalFee

	// Send full total to iPaymu — iPaymu deducts its fee from merchant, NOT from customer
	ipaymuAmount := int(totalPrice)

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
		// HARD DELETE the order so it doesn't get stuck in 'pending' without payment info
		if delErr := h.orderRepo.Delete(ctx, order.ID); delErr != nil {
			log.Printf("[OrderHandler] Failed to delete failed payment order %s: %v", order.ID, delErr)
		}

		InternalError(w, fmt.Sprintf("Gagal membuat pembayaran: %v", err))
		return
	}

	// Parse expiry time from iPaymu (format: "2025-01-01 12:00:00")
	// iPaymu returns time in WIB (Asia/Jakarta), so we must parse with that timezone
	var expiredAt time.Time
	wib, _ := time.LoadLocation("Asia/Jakarta")
	t, parseErr := time.ParseInLocation("2006-01-02 15:04:05", ipaymuResp.Data.Expired, wib)
	if parseErr == nil && !t.IsZero() {
		expiredAt = t
	} else {
		// Fallback: 24 hours from now
		expiredAt = time.Now().Add(24 * time.Hour)
	}

	// Determine QR image URL for QRIS
	qrImageURL := ""

	// Safely parse PaymentNo to string since ShopeePay returns numbers
	paymentNumber := ""
	if ipaymuResp.Data.PaymentNo != nil {
		if fmt.Sprintf("%T", ipaymuResp.Data.PaymentNo) == "float64" {
			paymentNumber = fmt.Sprintf("%.0f", ipaymuResp.Data.PaymentNo)
		} else {
			paymentNumber = fmt.Sprintf("%v", ipaymuResp.Data.PaymentNo)
		}
	}

	if strings.ToLower(req.PaymentMethod) == "qris" && ipaymuResp.Data.QrUrl != "" {
		qrImageURL = ipaymuResp.Data.QrUrl
	}

	paymentChannelValue := ipaymuResp.Data.PaymentChannel
	if paymentChannelValue == "" {
		paymentChannelValue = req.PaymentChannel // Fallback because iPaymu response might be empty
	}

	payment := &model.Payment{
		OrderID:             orderID,
		Amount:              sellingPrice, // Pure product price (e.g., 1466)
		Fee:                 totalFee,     // Admin (10) + transaction fee (e.g., 11) = 21
		TotalPayment:        totalPrice,   // amount + fee = 1487 (what customer pays)
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

	// Send Telegram notification — only after payment is successfully created
	go h.telegramSvc.NotifyOrderCreated(order)

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
	// This acts as a fallback checking mechanism in case the webhook callback from iPaymu fails
	if payment != nil && payment.Status == model.PaymentStatusPending && payment.IpaymuTransactionID > 0 {
		log.Printf("[GetOrderStatus] (Fallback) Checking iPaymu status for TransactionID: %d", payment.IpaymuTransactionID)
		ipaymuStatus, err := h.ipaymuSvc.CheckTransaction(payment.IpaymuTransactionID)

		if err != nil {
			log.Printf("[GetOrderStatus] (Fallback) Error checking iPaymu status: %v", err)
		} else if ipaymuStatus.Success {
			log.Printf("[GetOrderStatus] (Fallback) iPaymu status response: Status=%d (1=success, 0=pending, -1=failed)", ipaymuStatus.Data.Status)

			if ipaymuStatus.Data.Status == 1 { // 1 = success
				log.Printf("[GetOrderStatus] (Fallback) Updating order %s to PAID", orderID)
				_ = h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusCompleted)
				_ = h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusPaid)
				payment.Status = model.PaymentStatusCompleted
				order.Status = model.OrderStatusPaid

				// Process topup to Digiflazz manually!
				go h.processTopup(order)
			} else if ipaymuStatus.Data.Status == -1 || ipaymuStatus.Data.Status == -2 { // failed/expired
				log.Printf("[GetOrderStatus] (Fallback) Updating order %s to EXPIRED/FAILED", orderID)
				_ = h.paymentRepo.UpdateStatusByOrderID(ctx, orderID, model.PaymentStatusExpired)
				_ = h.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusExpired)
				payment.Status = model.PaymentStatusExpired
				order.Status = model.OrderStatusExpired
			}
		} else {
			log.Printf("[GetOrderStatus] (Fallback) iPaymu returned success=false: %s", ipaymuStatus.Message)
		}
	}

	// Also check Digiflazz status if payment is completed but order is still processing
	// We only want to check if the Digiflazz status is pending or empty, AND we have sent it to Digiflazz
	if order.Status == model.OrderStatusProcessing || order.Status == model.OrderStatusPaid {
		// Verify if it needs checking (it has been paid)
		if order.DigiflazzStatus == "" || order.DigiflazzStatus == "Pending" {
			log.Printf("[GetOrderStatus] Checking Digiflazz status for OrderID: %s, RefID: %s", order.ID, order.RefID)

			dfResp, err := h.digiflazzSvc.CheckTransactionStatus(order.BuyerSKUCode, order.CustomerNo, order.RefID)
			if err == nil && dfResp != nil {
				// Update order based on Digiflazz response
				var newOrderStatus model.OrderStatus

				switch dfResp.Data.Status {
				case "Sukses":
					newOrderStatus = model.OrderStatusSuccess
				case "Gagal":
					newOrderStatus = model.OrderStatusFailed
				default:
					newOrderStatus = model.OrderStatusProcessing
				}

				// Only update if status actually changed or we got new info (like SN)
				if newOrderStatus != order.Status || dfResp.Data.SN != order.SerialNumber || dfResp.Data.Status != order.DigiflazzStatus || dfResp.Data.Message != order.DigiflazzMsg {
					log.Printf("[GetOrderStatus] Updating order %s from Digiflazz Check: status=%s, SN=%s", order.ID, dfResp.Data.Status, dfResp.Data.SN)

					errUpdate := h.orderRepo.UpdateDigiflazzResponse(
						ctx,
						order.ID,
						newOrderStatus,
						dfResp.Data.Status,
						dfResp.Data.RC,
						dfResp.Data.SN,
						dfResp.Data.Message,
					)

					if errUpdate == nil {
						// Process refund if failed
						if newOrderStatus == model.OrderStatusFailed && order.MemberID != nil && order.Status != model.OrderStatusFailed {
							amount := order.MemberPrice
							if amount == nil {
								amount = &order.SellingPrice
							}
							desc := fmt.Sprintf("Refund Gagal Transaksi (Cek Status) %s", order.RefID)
							if errRefund := h.userRepo.TopupBalance(ctx, *order.MemberID, *amount, desc, "SYSTEM"); errRefund != nil {
								log.Printf("CRITICAL: Failed to refund member balance for order %s: %v", order.ID, errRefund)
							}
						}

						// Process affiliate commission if success and previously not success
						if newOrderStatus == model.OrderStatusSuccess && order.AffiliateID != nil && order.Status != model.OrderStatusSuccess {
							go h.processAffiliateCommission(order)
						}

						// Update the order object for the API response
						order.Status = newOrderStatus
						order.DigiflazzStatus = dfResp.Data.Status
						order.DigiflazzRC = dfResp.Data.RC
						order.SerialNumber = dfResp.Data.SN
						order.DigiflazzMsg = dfResp.Data.Message
					} else {
						log.Printf("[GetOrderStatus] Failed to update order %s to DB: %v", order.ID, errUpdate)
					}
				}
			} else {
				if err != nil {
					log.Printf("[GetOrderStatus] Error checking Digiflazz status: %v", err)
				}
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
			// Only include channels that are active in iPaymu dashboard
			if !strings.EqualFold(ch.FeatureStatus, "active") {
				continue
			}

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

// processTopup processes the topup transaction with Digiflazz
func (h *OrderHandler) processTopup(order *model.Order) {
	// Use background context for goroutine operations
	ctx := context.Background()

	log.Printf("[Topup-Manual] Processing topup for order %s", order.ID)

	// Update status to processing
	_ = h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusProcessing)

	// Create transaction with Digiflazz
	// Force Testing: false because user wants real transactions even if ENV is not explicitly set to production
	req := digiflazz.TopupRequest{
		BuyerSKUCode: order.BuyerSKUCode,
		CustomerNo:   order.CustomerNo,
		RefID:        order.RefID,
		Testing:      false,
	}

	resp, err := h.digiflazzSvc.CreateTransaction(req)

	if err != nil {
		log.Printf("[Topup-Manual] Failed to create transaction: %v", err)
		// Check if it's a "Signature Anda salah" error or IP error
		_ = h.orderRepo.UpdateDigiflazzResponse(ctx, order.ID, model.OrderStatusFailed, "", "", "", err.Error())

		// REFUND IF MEMBER
		if order.MemberID != nil {
			amount := order.MemberPrice
			if amount == nil {
				amount = &order.SellingPrice
			}
			desc := fmt.Sprintf("Refund Gagal Transaksi (Initial) %s", order.RefID)
			if err := h.userRepo.TopupBalance(ctx, *order.MemberID, *amount, desc, "SYSTEM"); err != nil {
				log.Printf("CRITICAL: Failed to refund member balance for order %s: %v", order.ID, err)
			}
		}

		return
	}

	log.Printf("[Topup-Manual] Digiflazz response: order=%s status=%s", order.ID, resp.Data.Status)

	// Map Digiflazz status to order status
	var orderStatus model.OrderStatus
	switch resp.Data.Status {
	case "Sukses":
		orderStatus = model.OrderStatusSuccess
	case "Gagal":
		orderStatus = model.OrderStatusFailed
	default:
		orderStatus = model.OrderStatusProcessing
	}

	// Update order with Digiflazz response
	_ = h.orderRepo.UpdateDigiflazzResponse(
		ctx,
		order.ID,
		orderStatus,
		resp.Data.Status,
		resp.Data.RC,
		resp.Data.SN,
		resp.Data.Message,
	)

	// REFUND IF MEMBER AND FAILED
	if orderStatus == model.OrderStatusFailed && order.MemberID != nil {
		amount := order.MemberPrice
		if amount == nil {
			amount = &order.SellingPrice
		}
		desc := fmt.Sprintf("Refund Gagal Transaksi %s", order.RefID)
		if err := h.userRepo.TopupBalance(ctx, *order.MemberID, *amount, desc, "SYSTEM"); err != nil {
			log.Printf("CRITICAL: Failed to refund member balance for order %s: %v", order.ID, err)
		}
	}

	// PROCESS AFFILIATE COMMISSION ON SYNCHRONOUS SUCCESS
	if orderStatus == model.OrderStatusSuccess && order.AffiliateID != nil {
		go h.processAffiliateCommission(order)
	}
}

// processAffiliateCommission handles affiliate commission after successful transaction
func (h *OrderHandler) processAffiliateCommission(order *model.Order) {
	ctx := context.Background()

	if order.AffiliateID == nil {
		return
	}

	affiliate, err := h.affiliateRepo.GetByID(ctx, *order.AffiliateID)
	if err != nil || affiliate == nil {
		log.Printf("[Affiliate] Failed to get affiliate %d for order %s: %v", *order.AffiliateID, order.ID, err)
		return
	}

	// Check min transaction amount
	if order.SellingPrice < affiliate.MinTransactionAmount {
		log.Printf("[Affiliate] Order %s below min transaction (%.0f < %.0f), no commission",
			order.ID, order.SellingPrice, affiliate.MinTransactionAmount)

		// Still log the usage but with commission_applied = false
		channel := model.AffiliateChannelLink
		if order.AffiliateChannel != nil {
			channel = *order.AffiliateChannel
		}
		usage := &model.AffiliateUsage{
			AffiliateID:       affiliate.ID,
			CustomerNo:        order.CustomerNo,
			OrderID:           order.ID,
			Channel:           channel,
			TransactionAmount: order.SellingPrice,
			DiscountApplied:   false,
			DiscountAmount:    0,
			CommissionApplied: false,
			CommissionAmount:  0,
		}
		_ = h.affiliateRepo.CreateUsage(ctx, usage)
		return
	}

	// Check max commission uses per customer per month
	usageCount, err := h.affiliateRepo.CountUsagesByCustomerThisMonth(ctx, affiliate.ID, order.CustomerNo)
	if err != nil {
		log.Printf("[Affiliate] Failed to count usages for order %s: %v", order.ID, err)
		return
	}

	channel := model.AffiliateChannelLink
	if order.AffiliateChannel != nil {
		channel = *order.AffiliateChannel
	}

	commissionApplied := usageCount < affiliate.MaxCommissionUses
	var commissionAmount float64

	if commissionApplied {
		profit := order.SellingPrice - order.BuyPrice
		effectivePercent := affiliate.CommissionPercent
		if channel == model.AffiliateChannelCode && affiliate.DiscountEnabled {
			effectivePercent = affiliate.CommissionPercent - affiliate.DiscountPercent
		}
		if effectivePercent < 0 {
			effectivePercent = 0
		}
		commissionAmount = profit * (effectivePercent / 100.0)
		if commissionAmount < 0 {
			commissionAmount = 0
		}

		if commissionAmount > 0 {
			if err := h.affiliateRepo.AddAffiliateBalance(ctx, affiliate.UserID, commissionAmount); err != nil {
				log.Printf("CRITICAL: Failed to add affiliate balance for user %d, order %s: %v",
					affiliate.UserID, order.ID, err)
			} else {
				log.Printf("[Affiliate] Commission Rp %.0f credited to user %d for order %s (channel=%s)",
					commissionAmount, affiliate.UserID, order.ID, channel)
			}
		} else {
			log.Printf("[Affiliate] No profit on order %s, commission is 0", order.ID)
		}
	} else {
		log.Printf("[Affiliate] Max commission uses reached for customer %s on affiliate %d",
			order.CustomerNo, affiliate.ID)
	}

	// Log usage
	usage := &model.AffiliateUsage{
		AffiliateID:       affiliate.ID,
		CustomerNo:        order.CustomerNo,
		OrderID:           order.ID,
		Channel:           channel,
		TransactionAmount: order.SellingPrice,
		DiscountApplied:   order.AffiliateDiscount > 0,
		DiscountAmount:    order.AffiliateDiscount,
		CommissionApplied: commissionApplied,
		CommissionAmount:  commissionAmount,
	}
	_ = h.affiliateRepo.CreateUsage(ctx, usage)
}
