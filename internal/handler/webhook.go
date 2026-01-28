package handler

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/pakasir"
)

// WebhookHandler handles webhook callbacks from external services
type WebhookHandler struct {
	config       *config.Config
	orderRepo    *repository.OrderRepository
	paymentRepo  *repository.PaymentRepository
	webhookRepo  *repository.WebhookLogRepository
	digiflazzSvc *digiflazz.Service
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(
	cfg *config.Config,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	webhookRepo *repository.WebhookLogRepository,
	digiflazzSvc *digiflazz.Service,
) *WebhookHandler {
	return &WebhookHandler{
		config:       cfg,
		orderRepo:    orderRepo,
		paymentRepo:  paymentRepo,
		webhookRepo:  webhookRepo,
		digiflazzSvc: digiflazzSvc,
	}
}

// HandlePakasirWebhook handles POST /api/v1/webhook/pakasir
func (h *WebhookHandler) HandlePakasirWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Webhook] Failed to read Pakasir webhook body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Log webhook
	logID, _ := h.webhookRepo.Create(ctx, "pakasir", string(body))

	// Parse payload
	var payload pakasir.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Webhook] Failed to parse Pakasir webhook: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	log.Printf("[Webhook] Pakasir webhook received: order_id=%s, status=%s", payload.OrderID, payload.Status)

	// Verify project
	if payload.Project != h.config.PakasirProject {
		log.Printf("[Webhook] Invalid project: %s", payload.Project)
		h.webhookRepo.MarkProcessed(ctx, logID, "invalid project")
		http.Error(w, "Invalid project", http.StatusBadRequest)
		return
	}

	// Only process completed payments
	if payload.Status != "completed" {
		log.Printf("[Webhook] Ignoring non-completed status: %s", payload.Status)
		h.webhookRepo.MarkProcessed(ctx, logID, "")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Find order by RefID (which is used as order_id in Pakasir)
	order, err := h.orderRepo.GetByRefID(ctx, payload.OrderID)
	if err != nil {
		log.Printf("[Webhook] Order not found: %s", payload.OrderID)
		h.webhookRepo.MarkProcessed(ctx, logID, "order not found")
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Verify amount (compare rounded values since Pakasir sends integers)
	expectedAmount := float64(int(order.SellingPrice + 0.5))
	if payload.Amount != expectedAmount {
		log.Printf("[Webhook] Amount mismatch: expected %.0f, got %.0f", expectedAmount, payload.Amount)
		h.webhookRepo.MarkProcessed(ctx, logID, "amount mismatch")
		http.Error(w, "Amount mismatch", http.StatusBadRequest)
		return
	}

	// Update payment status
	if err := h.paymentRepo.UpdateStatusByOrderID(ctx, order.ID, model.PaymentStatusCompleted); err != nil {
		log.Printf("[Webhook] Failed to update payment: %v", err)
	}

	// Update order status to paid
	if err := h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusPaid); err != nil {
		log.Printf("[Webhook] Failed to update order status: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	// Process topup to Digiflazz
	go h.processTopup(order)

	h.webhookRepo.MarkProcessed(ctx, logID, "")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// processTopup processes the topup transaction with Digiflazz
func (h *WebhookHandler) processTopup(order *model.Order) {
	// Use background context for goroutine operations
	ctx := context.Background()

	log.Printf("[Topup] Processing topup for order %s", order.ID)

	// Update status to processing
	_ = h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusProcessing)

	// Create transaction with Digiflazz
	resp, err := h.digiflazzSvc.CreateTransaction(digiflazz.TopupRequest{
		BuyerSKUCode: order.BuyerSKUCode,
		CustomerNo:   order.CustomerNo,
		RefID:        order.RefID,
		Testing:      h.config.IsDevelopment(),
	})

	if err != nil {
		log.Printf("[Topup] Failed to create transaction: %v", err)
		_ = h.orderRepo.UpdateDigiflazzResponse(ctx, order.ID, model.OrderStatusFailed, "", "", "", err.Error())
		return
	}

	log.Printf("[Topup] Digiflazz response: status=%s, message=%s", resp.Data.Status, resp.Data.Message)

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

	log.Printf("[Topup] Order %s updated to status %s", order.ID, orderStatus)
}

// HandleDigiflazzWebhook handles POST /api/v1/webhook/digiflazz
func (h *WebhookHandler) HandleDigiflazzWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify IP whitelist (Digiflazz IP: 52.74.250.133)
	// In production, uncomment this check
	/*
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		if !strings.Contains(clientIP, h.config.DigiflazzWebhookIP) {
			log.Printf("[Webhook] Unauthorized IP: %s", clientIP)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	*/

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Webhook] Failed to read Digiflazz webhook body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Log webhook
	logID, _ := h.webhookRepo.Create(ctx, "digiflazz", string(body))

	// Parse payload
	var payload digiflazz.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Webhook] Failed to parse Digiflazz webhook: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	log.Printf("[Webhook] Digiflazz webhook received: ref_id=%s, status=%s", payload.Data.RefID, payload.Data.Status)

	// Find order by RefID
	order, err := h.orderRepo.GetByRefID(ctx, payload.Data.RefID)
	if err != nil {
		log.Printf("[Webhook] Order not found: %s", payload.Data.RefID)
		h.webhookRepo.MarkProcessed(ctx, logID, "order not found")
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Map Digiflazz status to order status
	var orderStatus model.OrderStatus
	switch payload.Data.Status {
	case "Sukses":
		orderStatus = model.OrderStatusSuccess
	case "Gagal":
		orderStatus = model.OrderStatusFailed
	default:
		orderStatus = model.OrderStatusProcessing
	}

	// Update order with Digiflazz response
	err = h.orderRepo.UpdateDigiflazzResponse(
		ctx,
		order.ID,
		orderStatus,
		payload.Data.Status,
		payload.Data.RC,
		payload.Data.SN,
		payload.Data.Message,
	)

	if err != nil {
		log.Printf("[Webhook] Failed to update order: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	log.Printf("[Webhook] Order %s updated to status %s", order.ID, orderStatus)

	h.webhookRepo.MarkProcessed(ctx, logID, "")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
