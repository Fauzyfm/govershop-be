package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
	"govershop-api/internal/repository"
	"govershop-api/internal/service/digiflazz"
	"govershop-api/internal/service/ipaymu"
	"govershop-api/internal/service/pakasir"
	"govershop-api/internal/service/qrispw"
	"govershop-api/internal/service/telegram"
)

// WebhookHandler handles webhook callbacks from external services
type WebhookHandler struct {
	config        *config.Config
	orderRepo     *repository.OrderRepository
	paymentRepo   *repository.PaymentRepository
	webhookRepo   *repository.WebhookLogRepository
	userRepo      *repository.UserRepository
	affiliateRepo *repository.AffiliateRepository
	digiflazzSvc  *digiflazz.Service
	ipaymuSvc     *ipaymu.Service
	telegramSvc   *telegram.Service
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(
	cfg *config.Config,
	orderRepo *repository.OrderRepository,
	paymentRepo *repository.PaymentRepository,
	webhookRepo *repository.WebhookLogRepository,
	userRepo *repository.UserRepository,
	affiliateRepo *repository.AffiliateRepository,
	digiflazzSvc *digiflazz.Service,
	ipaymuSvc *ipaymu.Service,
	telegramSvc *telegram.Service,
) *WebhookHandler {
	return &WebhookHandler{
		config:        cfg,
		orderRepo:     orderRepo,
		paymentRepo:   paymentRepo,
		webhookRepo:   webhookRepo,
		userRepo:      userRepo,
		affiliateRepo: affiliateRepo,
		digiflazzSvc:  digiflazzSvc,
		ipaymuSvc:     ipaymuSvc,
		telegramSvc:   telegramSvc,
	}
}

// parseFormBody parses a URL-encoded form body string into a map
func parseFormBody(body string) (map[string]string, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range values {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result, nil
}

// HandleIpaymuWebhook handles POST /api/v1/webhook/ipaymu
func (h *WebhookHandler) HandleIpaymuWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read raw body (needed for signature verification)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Webhook] Failed to read iPaymu webhook body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Log webhook
	logID, _ := h.webhookRepo.Create(ctx, "ipaymu", string(body))

	// Get signature from header
	signature := r.Header.Get("X-Signature")

	// Verify signature (log warning if fails, but don't block — exact algorithm TBD)
	if signature != "" {
		if !h.ipaymuSvc.VerifyWebhookSignature(body, signature) {
			log.Printf("[Webhook] ⚠️ iPaymu signature verification failed (continuing anyway)")
		} else {
			log.Printf("[Webhook] iPaymu signature verified ✅")
		}
	} else {
		log.Printf("[Webhook] iPaymu webhook received without signature (sandbox mode)")
	}

	// Parse payload — try JSON first, then form-encoded
	var payload ipaymu.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Try form-encoded parsing
		if parseErr := r.ParseForm(); parseErr != nil {
			log.Printf("[Webhook] Failed to parse iPaymu webhook (neither JSON nor form): %v", parseErr)
			h.webhookRepo.MarkProcessed(ctx, logID, "parse error")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		// Manually rebuild body from form values for iPaymu form-urlencoded
		// Re-read from the raw body using net/url
		formValues, parseErr := parseFormBody(string(body))
		if parseErr != nil {
			log.Printf("[Webhook] Failed to parse form body: %v", parseErr)
			h.webhookRepo.MarkProcessed(ctx, logID, "form parse error")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		payload.ReferenceID = formValues["reference_id"]
		payload.SID = formValues["sid"]
		payload.Status = formValues["status"]
		payload.Channel = formValues["channel"]
		payload.Via = formValues["via"]
		payload.Amount = formValues["amount"]
		payload.Fee = formValues["fee"]
		payload.BuyerName = formValues["buyer_name"]
		payload.BuyerEmail = formValues["buyer_email"]
		payload.BuyerPhone = formValues["buyer_phone"]
		payload.PaymentNo = formValues["payment_no"]

		// Parse numeric fields
		if v := formValues["trx_id"]; v != "" {
			fmt.Sscanf(v, "%d", &payload.TrxID)
		}
		if v := formValues["status_code"]; v != "" {
			fmt.Sscanf(v, "%d", &payload.StatusCode)
		}
	}

	log.Printf("[Webhook] iPaymu webhook received: trx_id=%d, reference_id=%s, sid=%s, status=%s, status_code=%d",
		payload.TrxID, payload.ReferenceID, payload.SID, payload.Status, payload.StatusCode)

	// Find order by RefID — try reference_id first, then sid
	refID := payload.ReferenceID
	if refID == "" {
		refID = payload.SID
	}

	order, err := h.orderRepo.GetByRefID(ctx, refID)
	if err != nil {
		log.Printf("[Webhook] iPaymu order not found for ref: %s", refID)
		h.webhookRepo.MarkProcessed(ctx, logID, "order not found: "+refID)
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Process status
	if payload.StatusCode == 1 {
		// SUCCESS
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, order.ID, model.PaymentStatusCompleted); err != nil {
			log.Printf("[Webhook] Failed to update payment: %v", err)
		}
		if err := h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusPaid); err != nil {
			log.Printf("[Webhook] Failed to update order status: %v", err)
			h.webhookRepo.MarkProcessed(ctx, logID, err.Error())
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}
		// Send Telegram notification for payment received
		go h.telegramSvc.NotifyPaymentReceived(order)

		// Process topup to Digiflazz
		go h.processTopup(order)
	} else if payload.StatusCode == -1 || payload.StatusCode == -2 {
		// FAILED or EXPIRED
		log.Printf("[Webhook] Processing failed/expired iPaymu status: %s (code=%d) for order %s", payload.Status, payload.StatusCode, order.ID)
		if err := h.paymentRepo.UpdateStatusByOrderID(ctx, order.ID, model.PaymentStatusExpired); err != nil {
			log.Printf("[Webhook] Failed to update payment to expired: %v", err)
		}
		if err := h.orderRepo.UpdateStatus(ctx, order.ID, model.OrderStatusExpired); err != nil {
			log.Printf("[Webhook] Failed to update order status to expired: %v", err)
		}
	} else {
		// Pending or other unknown codes
		log.Printf("[Webhook] Ignoring non-success iPaymu status: %s (code=%d)", payload.Status, payload.StatusCode)
	}

	h.webhookRepo.MarkProcessed(ctx, logID, "")
	log.Printf("[Webhook] iPaymu order %s processed successfully → topup triggered", order.ID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
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

		// Send Telegram notification for failed topup
		go h.telegramSvc.NotifyTopupResult(order, "Gagal", "", "", err.Error())

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

	log.Printf("[Topup] Digiflazz response: order=%s status=%s", order.ID, resp.Data.Status)

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

	// Send Telegram notification for final results, or start polling if Pending
	if resp.Data.Status == "Sukses" || resp.Data.Status == "Gagal" {
		go h.telegramSvc.NotifyTopupResult(order, resp.Data.Status, resp.Data.RC, resp.Data.SN, resp.Data.Message)

		// PROCESS AFFILIATE COMMISSION ON SYNCHRONOUS SUCCESS
		if orderStatus == model.OrderStatusSuccess && order.AffiliateID != nil {
			go h.processAffiliateCommission(order)
		}
	} else {
		// Status is Pending — start polling DB until final result
		go h.pollOrderStatus(order.ID)
	}

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

// pollOrderStatus polls the database for order status changes and sends
// a Telegram notification when the order reaches a final status (success/failed).
// It checks every 2 minutes with a maximum timeout of 30 minutes.
func (h *WebhookHandler) pollOrderStatus(orderID string) {
	ctx := context.Background()
	pollInterval := 2 * time.Minute
	maxTimeout := 30 * time.Minute
	deadline := time.Now().Add(maxTimeout)

	log.Printf("[PollStatus] Started polling for order %s (every %v, max %v)", orderID, pollInterval, maxTimeout)

	for {
		// Wait before checking
		time.Sleep(pollInterval)

		// Check if we've exceeded the deadline
		if time.Now().After(deadline) {
			log.Printf("[PollStatus] Timeout reached for order %s — stopping polling", orderID)
			return
		}

		// Query order from DB
		order, err := h.orderRepo.GetByID(ctx, orderID)
		if err != nil {
			log.Printf("[PollStatus] Failed to get order %s: %v", orderID, err)
			continue
		}

		log.Printf("[PollStatus] Order %s current status: %s (digiflazz: %s)", orderID, order.Status, order.DigiflazzStatus)

		// Check if order has reached a final status
		switch order.Status {
		case model.OrderStatusSuccess:
			log.Printf("[PollStatus] Order %s is SUCCESS — sending notification", orderID)
			go h.telegramSvc.NotifyTopupResult(order, order.DigiflazzStatus, order.DigiflazzRC, order.SerialNumber, order.DigiflazzMsg)
			return

		case model.OrderStatusFailed:
			log.Printf("[PollStatus] Order %s is FAILED — sending notification", orderID)
			go h.telegramSvc.NotifyTopupResult(order, order.DigiflazzStatus, order.DigiflazzRC, order.SerialNumber, order.DigiflazzMsg)
			return

		default:
			// Still processing/pending — continue polling
			log.Printf("[PollStatus] Order %s still processing, will check again in %v", orderID, pollInterval)
		}
	}
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

	// Note: Telegram notification is handled by pollOrderStatus goroutine
	// which was started after processTopup. No need to notify here.

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

	// PROCESS AFFILIATE COMMISSION ON SUCCESS
	if orderStatus == model.OrderStatusSuccess && order.AffiliateID != nil {
		go h.processAffiliateCommission(order)
	}

	h.webhookRepo.MarkProcessed(ctx, logID, "")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// processAffiliateCommission handles affiliate commission after successful transaction
func (h *WebhookHandler) processAffiliateCommission(order *model.Order) {
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

	// Check max commission uses per customer per month (anti-abuse: 10x default)
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
		
		// Commission is based on the total transaction amount (SellingPrice)
		effectivePercent := affiliate.CommissionPercent
		if channel == model.AffiliateChannelCode && affiliate.DiscountEnabled {
			effectivePercent = affiliate.CommissionPercent - affiliate.DiscountPercent
		}
		if effectivePercent < 0 {
			effectivePercent = 0
		}
		
		// Commission is calculated from the transaction amount, not just the profit margin
		commissionAmount = order.SellingPrice * (effectivePercent / 100.0)
		
		// Failsafe: Ensure commission never exceeds actual profit (so admin doesn't lose money)
		if commissionAmount > profit {
			commissionAmount = profit
		}
		if commissionAmount < 0 {
			commissionAmount = 0
		}

		// Add to streamer's affiliate balance
		if commissionAmount > 0 {
			if err := h.affiliateRepo.AddAffiliateBalance(ctx, affiliate.UserID, commissionAmount); err != nil {
				log.Printf("CRITICAL: Failed to add affiliate balance for user %d, order %s: %v",
					affiliate.UserID, order.ID, err)
			} else {
				log.Printf("[Affiliate] Commission Rp %.0f credited to user %d for order %s (channel=%s)",
					commissionAmount, affiliate.UserID, order.ID, channel)
			}
		}
	} else {
		log.Printf("[Affiliate] Max commission uses reached for customer %s on affiliate %d (count=%d, max=%d)",
			order.CustomerNo, affiliate.ID, usageCount, affiliate.MaxCommissionUses)
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
	if err := h.affiliateRepo.CreateUsage(ctx, usage); err != nil {
		log.Printf("[Affiliate] Failed to create usage log for order %s: %v", order.ID, err)
	}
}
