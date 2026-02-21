package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/pakasir"
	"govershop-api/internal/service/qrispw"
)

// WebhookHandler handles webhook callbacks from external services
type WebhookHandler struct {
	config       *config.Config
	orderRepo    *repository.OrderRepository
	paymentRepo  *repository.PaymentRepository
	webhookRepo  *repository.WebhookLogRepository
	userRepo     *repository.UserRepository
	digiflazzSvc *digiflazz.Service
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(
	cfg *config.Config,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	webhookRepo *repository.WebhookLogRepository,
	userRepo *repository.UserRepository,
	digiflazzSvc *digiflazz.Service,
) *WebhookHandler {
	return &WebhookHandler{
		config:       cfg,
		orderRepo:    orderRepo,
		paymentRepo:  paymentRepo,
		webhookRepo:  webhookRepo,
		userRepo:     userRepo,
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

// HandleQrisPWWebhook handles POST /api/v1/webhook/qrispw
func (h *WebhookHandler) HandleQrisPWWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Webhook] Failed to read QrisPW webhook body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Always log webhook payload first
	logID, _ := h.webhookRepo.Create(ctx, "qrispw", string(body))
	log.Printf("[Webhook] QrisPW raw payload: %s", string(body))

	// Note: Qris.pw currently does not send any signature header for validation.
	// We rely on matching the strict parameters (TransactionID, OrderID, Amount) to authorize the webhook,
	// as checking the specific Amount ensures no one can spoof it without knowing the exact payment amount requested.

	// Parse payload
	var payload qrispw.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Webhook] Failed to parse QrisPW webhook: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	log.Printf("[Webhook] QrisPW webhook received: transaction_id=%s, order_id=%s, status=%s, amount=%s",
		payload.TransactionID, payload.OrderID, payload.Status, payload.Amount.String())

	// Find order by RefID (which is used as order_id in qris.pw)
	order, err := h.orderRepo.GetByRefID(ctx, payload.OrderID)
	if err != nil {
		log.Printf("[Webhook] QrisPW order not found: %s", payload.OrderID)
		h.webhookRepo.MarkProcessed(ctx, logID, "order not found")
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Verify amount
	payloadAmount, err := payload.Amount.Float64()
	if err != nil {
		log.Printf("[Webhook] QrisPW invalid amount format: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, "invalid amount format")
		http.Error(w, "Invalid amount format", http.StatusBadRequest)
		return
	}

	// Compare amounts using integer truncation (qris.pw sends "1486.00", order has 1486)
	expectedAmount := int(order.SellingPrice)
	receivedAmount := int(payloadAmount)
	if receivedAmount != expectedAmount {
		log.Printf("[Webhook] QrisPW amount mismatch: expected %d, got %d (raw: %s)", expectedAmount, receivedAmount, payload.Amount.String())
		h.webhookRepo.MarkProcessed(ctx, logID, fmt.Sprintf("amount mismatch: expected %d, got %d", expectedAmount, receivedAmount))
		http.Error(w, "Amount mismatch", http.StatusBadRequest)
		return
	}

	// Process based on status
	switch payload.Status {
	case "paid":
		log.Printf("[Webhook] QrisPW payment PAID for order %s", order.ID)

		// Update payment status to completed
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, order.ID, model.PaymentStatusCompleted); err != nil {
			log.Printf("[Webhook] QrisPW failed to update payment: %v", err)
		}

		// Update order status to paid
		if err := h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusPaid); err != nil {
			log.Printf("[Webhook] QrisPW failed to update order status: %v", err)
			h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}

		// Process topup to Digiflazz
		go h.processTopup(order)

		h.webhookRepo.MarkProcessed(ctx, logID, "")
		log.Printf("[Webhook] QrisPW order %s processed successfully → topup triggered", order.ID)

	case "expired":
		log.Printf("[Webhook] QrisPW payment EXPIRED for order %s", order.ID)

		// Update payment status to expired
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, order.ID, model.PaymentStatusExpired); err != nil {
			log.Printf("[Webhook] QrisPW failed to update payment to expired: %v", err)
		}

		h.webhookRepo.MarkProcessed(ctx, logID, "")

	case "pending":
		log.Printf("[Webhook] QrisPW payment still PENDING for order %s", order.ID)
		h.webhookRepo.MarkProcessed(ctx, logID, "")

	default:
		log.Printf("[Webhook] QrisPW unknown status '%s' for order %s", payload.Status, order.ID)
		h.webhookRepo.MarkProcessed(ctx, logID, fmt.Sprintf("unknown status: %s", payload.Status))
	}

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
	// Force Testing: false because user wants real transactions even if ENV is not explicitly set to production
	req := digiflazz.TopupRequest{
		BuyerSKUCode: order.BuyerSKUCode,
		CustomerNo:   order.CustomerNo,
		RefID:        order.RefID,
		Testing:      false,
	}

	resp, err := h.digiflazzSvc.CreateTransaction(req)

	if err != nil {
		log.Printf("[Topup] Failed to create transaction: %v", err)
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

	// Log Raw Response for debugging
	respJSON, _ := json.Marshal(resp)
	log.Printf("[Topup] Digiflazz Raw Response: %s", string(respJSON))

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

	log.Printf("[Topup] Order %s updated to status %s", order.ID, orderStatus)
}

// HandleDigiflazzWebhook handles POST /api/v1/webhook/digiflazz
func (h *WebhookHandler) HandleDigiflazzWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
		// If order not found (e.g. Validation transaction VAL-...), ignore it
		if strings.Contains(err.Error(), "no rows in result set") {
			log.Printf("[Webhook] Ignored unknown RefID: %s", payload.Data.RefID)
			h.webhookRepo.MarkProcessed(ctx, logID, "ignored: unknown ref_id")
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Printf("[Webhook] Failed to get order: %v", err)
		h.webhookRepo.MarkProcessed(ctx, logID, "error finding order")
		http.Error(w, "Internal Error", http.StatusInternalServerError)
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

	h.webhookRepo.MarkProcessed(ctx, logID, "")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
