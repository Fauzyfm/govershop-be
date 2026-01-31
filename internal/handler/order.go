package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/pakasir"
)

// OrderHandler handles order-related HTTP requests
type OrderHandler struct {
	orderRepo    *repository.OrderRepository
	paymentRepo  *repository.PaymentRepository
	productRepo  *repository.ProductRepository
	digiflazzSvc *digiflazz.Service
	pakasirSvc   *pakasir.Service
}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler(
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	productRepo *repository.ProductRepository,
	digiflazzSvc *digiflazz.Service,
	pakasirSvc *pakasir.Service,
) *OrderHandler {
	return &OrderHandler{
		orderRepo:    orderRepo,
		paymentRepo:  paymentRepo,
		productRepo:  productRepo,
		digiflazzSvc: digiflazzSvc,
		pakasirSvc:   pakasirSvc,
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

	// Create payment via Pakasir
	pakasirResp, err := h.pakasirSvc.CreateTransaction(
		string(req.PaymentMethod),
		order.RefID,
		order.SellingPrice,
	)
	if err != nil {
		InternalError(w, fmt.Sprintf("Gagal membuat pembayaran: %v", err))
		return
	}

	// Parse expiry time
	expiredAt, _ := time.Parse(time.RFC3339, pakasirResp.Payment.ExpiredAt)

	// Save payment to database
	payment := &model.Payment{
		OrderID:       orderID,
		Amount:        pakasirResp.Payment.Amount,
		Fee:           pakasirResp.Payment.Fee,
		TotalPayment:  pakasirResp.Payment.TotalPayment,
		PaymentMethod: model.PaymentMethod(pakasirResp.Payment.PaymentMethod),
		PaymentNumber: pakasirResp.Payment.PaymentNumber,
		Status:        model.PaymentStatusPending,
		ExpiredAt:     expiredAt,
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

	// Cancel payment in Pakasir if exists
	if order.Status == model.OrderStatusWaitingPayment {
		_ = h.pakasirSvc.CancelTransaction(order.RefID, order.SellingPrice)
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

	// Build response
	response := map[string]interface{}{
		"order_id":      order.ID,
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
func (h *OrderHandler) GetPaymentMethods(w http.ResponseWriter, r *http.Request) {
	methods := model.GetAvailablePaymentMethods()
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
