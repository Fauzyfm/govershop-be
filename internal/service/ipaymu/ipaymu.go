package ipaymu

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"govershop-api/internal/config"
)

// Service handles all iPaymu API interactions
type Service struct {
	config     *config.Config
	httpClient *http.Client

	// Cache for payment channels
	channelCache   *PaymentChannelResponse
	channelCacheAt time.Time
	channelCacheMu sync.Mutex
}

const channelCacheTTL = 10 * time.Minute

// NewService creates a new iPaymu service
func NewService(cfg *config.Config) *Service {
	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateSignature creates HMAC-SHA256 signature for iPaymu API
// Formula: stringToSign = "METHOD:VA:sha256(body):apiKey"
// Signature = HMAC-SHA256(stringToSign, apiKey)
func (s *Service) generateSignature(method string, body []byte) string {
	// SHA256 hash of body
	bodyHash := sha256.Sum256(body)
	bodyHashStr := strings.ToLower(hex.EncodeToString(bodyHash[:]))

	// Build string to sign
	stringToSign := fmt.Sprintf("%s:%s:%s:%s",
		strings.ToUpper(method),
		s.config.IpaymuVA,
		bodyHashStr,
		s.config.IpaymuKey,
	)

	// HMAC-SHA256
	h := hmac.New(sha256.New, []byte(s.config.IpaymuKey))
	h.Write([]byte(stringToSign))
	return hex.EncodeToString(h.Sum(nil))
}

// doRequest performs an authenticated request to iPaymu API
func (s *Service) doRequest(method, endpoint string, body []byte) ([]byte, error) {
	url := s.config.IpaymuBaseURL + endpoint

	signature := s.generateSignature(method, body)

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("va", s.config.IpaymuVA)
	req.Header.Set("signature", signature)
	req.Header.Set("timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, nil
}

// ============================================================
// Payment Channels
// ============================================================

// PaymentChannelResponse represents the response from payment channels API
type PaymentChannelResponse struct {
	Status  int              `json:"Status"`
	Success bool             `json:"Success"`
	Message string           `json:"Message"`
	Data    []PaymentChannel `json:"Data"`
}

// PaymentChannel represents a category of payment channels (e.g. Virtual Account, QRIS)
type PaymentChannel struct {
	Code        string `json:"Code"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Channels    []struct {
		Code                   string `json:"Code"`
		Name                   string `json:"Name"`
		Description            string `json:"Description"`
		Logo                   string `json:"Logo"`
		PaymentInstructionsDoc string `json:"PaymentInstructionsDoc"`
		FeatureStatus          string `json:"FeatureStatus"`
		HealthStatus           string `json:"HealthStatus"`
		TransactionFee         struct {
			ActualFee     float64 `json:"ActualFee"`
			ActualFeeType string  `json:"ActualFeeType"` // "FLAT"
			AdditionalFee float64 `json:"AdditionalFee"` // Usually 0
		} `json:"TransactionFee"`
	} `json:"Channels"`
}

// GetPaymentChannels fetches available payment channels from iPaymu (with 10-min cache)
func (s *Service) GetPaymentChannels() (*PaymentChannelResponse, error) {
	s.channelCacheMu.Lock()
	defer s.channelCacheMu.Unlock()

	// Return cached data if still valid
	if s.channelCache != nil && time.Since(s.channelCacheAt) < channelCacheTTL {
		return s.channelCache, nil
	}

	body := []byte("{}")

	respBody, err := s.doRequest("GET", "/api/v2/payment-channels", body)
	if err != nil {
		log.Printf("[iPaymu] Error fetching payment channels")
		// Return stale cache if available
		if s.channelCache != nil {
			log.Printf("[iPaymu] Returning stale cache")
			return s.channelCache, nil
		}
		return nil, err
	}

	var result PaymentChannelResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[iPaymu] Error parsing payment channels response: %s", string(respBody[:min(len(respBody), 200)]))
		if s.channelCache != nil {
			return s.channelCache, nil
		}
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		log.Printf("[iPaymu] Error: %s", result.Message)
		return nil, fmt.Errorf("ipaymu error: %s", result.Message)
	}

	// Update cache
	s.channelCache = &result
	s.channelCacheAt = time.Now()
	log.Printf("[iPaymu] Payment channels cached (%d categories)", len(result.Data))

	return &result, nil
}

// ============================================================
// Direct Payment
// ============================================================

// DirectPaymentRequest represents the request for direct payment
type DirectPaymentRequest struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	Amount         int    `json:"amount"`
	NotifyURL      string `json:"notifyUrl"`
	PaymentMethod  string `json:"paymentMethod"`  // "VA", "QRIS", "CC", etc.
	PaymentChannel string `json:"paymentChannel"` // "bni", "bri", "mandiri", "qris", etc.
	ReferenceID    string `json:"referenceId"`
	BuyerName      string `json:"buyerName,omitempty"`
	BuyerEmail     string `json:"buyerEmail,omitempty"`
	BuyerPhone     string `json:"buyerPhone,omitempty"`
}

// DirectPaymentResponse represents the response from direct payment API
type DirectPaymentResponse struct {
	Status  int    `json:"Status"`
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Data    struct {
		TransactionID  int    `json:"TransactionId"`
		SessionID      string `json:"SessionId"`
		ReferenceID    string `json:"ReferenceId"`
		PaymentNo      any    `json:"PaymentNo"` // Can be string (VA) or number (ShopeePay)
		PaymentName    string `json:"PaymentName"`
		PaymentMethod  string `json:"PaymentMethod"`
		PaymentChannel string `json:"PaymentChannel"`
		Expired        string `json:"Expired"`
		Total          int    `json:"Total"`
		Fee            int    `json:"Fee"`
		SubTotal       int    `json:"SubTotal"`
		QrUrl          string `json:"QrUrl,omitempty"`
	} `json:"Data"`
}

// CreateDirectPayment creates a direct payment via iPaymu
func (s *Service) CreateDirectPayment(req DirectPaymentRequest) (*DirectPaymentResponse, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	log.Printf("[iPaymu] CreateDirectPayment: ref=%s amount=%d method=%s channel=%s",
		req.ReferenceID, req.Amount, req.PaymentMethod, req.PaymentChannel)

	respBody, err := s.doRequest("POST", "/api/v2/payment/direct", jsonPayload)
	if err != nil {
		log.Printf("[iPaymu] Error: ref=%s connection failed", req.ReferenceID)
		return nil, err
	}

	var result DirectPaymentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[iPaymu] Error: ref=%s failed to parse response", req.ReferenceID)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		log.Printf("[iPaymu] Error: ref=%s msg=%s", req.ReferenceID, result.Message)
		return nil, fmt.Errorf("ipaymu error: %s", result.Message)
	}

	log.Printf("[iPaymu] Success: ref=%s txn=%d total=%d",
		req.ReferenceID, result.Data.TransactionID, result.Data.Total)

	return &result, nil
}

// ============================================================
// Check Transaction
// ============================================================

// CheckTransactionRequest represents the request for checking transaction status
type CheckTransactionRequest struct {
	TransactionID int `json:"transactionId"`
}

// CheckTransactionResponse represents the response from check transaction API
type CheckTransactionResponse struct {
	Status  int    `json:"Status"`
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Data    struct {
		TransactionID int     `json:"TransactionId"`
		SessionID     string  `json:"SessionId"`
		ReferenceID   string  `json:"ReferenceId"`
		RelatedID     int     `json:"RelatedId"`
		Sender        string  `json:"Sender"`
		Receiver      string  `json:"Receiver"`
		Amount        float64 `json:"Amount"`
		Fee           float64 `json:"Fee"`
		Status        int     `json:"Status"` // 0=pending, 1=success, -1=failed
		StatusDesc    string  `json:"StatusDesc"`
		Type          int     `json:"Type"`
		PaymentNo     string  `json:"PaymentNo"`
		PaymentMethod string  `json:"PaymentChannel"`
		CreatedDate   string  `json:"CreatedDate"`
		ExpiredDate   string  `json:"ExpiredDate"`
		SuccessDate   string  `json:"SuccessDate"`
	} `json:"Data"`
}

// CheckTransaction checks the status of a transaction
func (s *Service) CheckTransaction(transactionID int) (*CheckTransactionResponse, error) {
	payload := CheckTransactionRequest{
		TransactionID: transactionID,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	respBody, err := s.doRequest("POST", "/api/v2/transaction", jsonPayload)
	if err != nil {
		return nil, err
	}

	var result CheckTransactionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("ipaymu error: %s", result.Message)
	}

	return &result, nil
}

// ============================================================
// Webhook Verification
// ============================================================

// WebhookPayload represents the webhook payload from iPaymu
// iPaymu sends webhooks as application/x-www-form-urlencoded
type WebhookPayload struct {
	TrxID       int    `json:"trx_id"`
	SID         string `json:"sid"`
	Status      string `json:"status"`      // "berhasil", "pending", "expired", "gagal"
	StatusCode  int    `json:"status_code"` // 1=success, 0=pending, -1=failed
	ReferenceID string `json:"reference_id"`
	Amount      string `json:"amount"`
	Fee         string `json:"fee"`
	Channel     string `json:"channel"`
	Via         string `json:"via"`
	PaymentNo   string `json:"payment_no"`
	BuyerName   string `json:"buyer_name"`
	BuyerEmail  string `json:"buyer_email"`
	BuyerPhone  string `json:"buyer_phone"`
}

// VerifyWebhookSignature verifies the X-Signature header from iPaymu webhook
// iPaymu signs the raw body with HMAC-SHA256 using the API key
func (s *Service) VerifyWebhookSignature(rawBody []byte, receivedSignature string) bool {
	if receivedSignature == "" {
		log.Printf("[iPaymu] No signature provided in webhook")
		return false
	}

	// HMAC-SHA256 with API key
	h := hmac.New(sha256.New, []byte(s.config.IpaymuKey))
	h.Write(rawBody)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expectedSignature), []byte(receivedSignature))
}
