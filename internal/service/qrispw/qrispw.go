package qrispw

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
	"time"

	"govershop-api/internal/config"
)

const (
	BaseURL               = "https://qris.pw/api"
	EndpointCreatePayment = "/create-payment.php"
	EndpointCheckPayment  = "/check-payment.php"
)

// Service handles all QRIS.PW API interactions
type Service struct {
	config     *config.Config
	httpClient *http.Client
}

// NewService creates a new QRIS.PW service
func NewService(cfg *config.Config) *Service {
	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreatePaymentRequest represents the request body for creating a QRIS payment
type CreatePaymentRequest struct {
	Amount        int    `json:"amount"`
	OrderID       string `json:"order_id"`
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone,omitempty"`
	CallbackURL   string `json:"callback_url"`
}

// CreatePaymentResponse represents the response from qris.pw create-payment
type CreatePaymentResponse struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	OrderID       string `json:"order_id"`
	Amount        int    `json:"amount"`
	QRISUrl       string `json:"qris_url"`
	QRISString    string `json:"qris_string"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
	Error         string `json:"error,omitempty"`
}

// CheckPaymentResponse represents the response from qris.pw check-payment
type CheckPaymentResponse struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	OrderID       string `json:"order_id"`
	Amount        int    `json:"amount"`
	Status        string `json:"status"` // "pending", "paid", "expired"
	PaidAt        string `json:"paid_at,omitempty"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
	Error         string `json:"error,omitempty"`
}

// WebhookPayload represents the webhook payload from qris.pw
type WebhookPayload struct {
	TransactionID string  `json:"transaction_id"`
	OrderID       string  `json:"order_id"`
	Amount        float64 `json:"amount"`
	Status        string  `json:"status"` // "paid"
	PaidAt        string  `json:"paid_at"`
	Timestamp     int64   `json:"timestamp"`
	Signature     string  `json:"signature"`
}

// CreatePayment creates a QRIS payment via qris.pw
func (s *Service) CreatePayment(orderID string, amount float64, customerName string, callbackURL string) (*CreatePaymentResponse, error) {
	// Truncate amount to integer (qris.pw expects integer, match displayed price)
	roundedAmount := int(amount)

	payload := CreatePaymentRequest{
		Amount:       roundedAmount,
		OrderID:      orderID,
		CustomerName: customerName,
		CallbackURL:  callbackURL,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	endpoint := BaseURL + EndpointCreatePayment

	// Debug logging
	log.Printf("[QrisPW] Creating payment:")
	log.Printf("[QrisPW] Endpoint: %s", endpoint)
	log.Printf("[QrisPW] OrderID: %s", orderID)
	log.Printf("[QrisPW] Amount: %d (original: %.2f)", roundedAmount, amount)
	log.Printf("[QrisPW] CustomerName: %s", customerName)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (qris.pw auth via headers)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.config.QrisPWAPIKey)
	req.Header.Set("X-API-Secret", s.config.QrisPWSecretKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[QrisPW] HTTP Error: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Debug logging
	log.Printf("[QrisPW] Response Status: %d", resp.StatusCode)
	log.Printf("[QrisPW] Response Body: %s", string(body))

	var result CreatePaymentResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[QrisPW] Parse Error: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		log.Printf("[QrisPW] API Error: %s", errMsg)
		return nil, fmt.Errorf("qrispw error: %s", errMsg)
	}

	log.Printf("[QrisPW] Success - TransactionID: %s, QRISUrl: %s", result.TransactionID, result.QRISUrl)

	return &result, nil
}

// CheckPaymentStatus checks the status of a QRIS payment
func (s *Service) CheckPaymentStatus(transactionID string) (*CheckPaymentResponse, error) {
	endpoint := fmt.Sprintf("%s%s?transaction_id=%s", BaseURL, EndpointCheckPayment, transactionID)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", s.config.QrisPWAPIKey)
	req.Header.Set("X-API-Secret", s.config.QrisPWSecretKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result CheckPaymentResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}

	if !result.Success {
		return nil, fmt.Errorf("qrispw error: %s", result.Error)
	}

	return &result, nil
}

// VerifyWebhookSignature verifies the HMAC-SHA256 signature of a webhook payload
func VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}
