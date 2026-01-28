package pakasir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"govershop-api/internal/config"
)

const (
	BaseURL                   = "https://app.pakasir.com/api"
	EndpointTransactionCreate = "/transactioncreate"
	EndpointTransactionCancel = "/transactioncancel"
	EndpointTransactionDetail = "/transactiondetail"
	EndpointPaymentSimulation = "/paymentsimulation"
)

// Service handles all Pakasir API interactions
type Service struct {
	config     *config.Config
	httpClient *http.Client
}

// NewService creates a new Pakasir service
func NewService(cfg *config.Config) *Service {
	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateTransactionRequest represents the request body for creating transaction
type CreateTransactionRequest struct {
	Project string  `json:"project"`
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
	APIKey  string  `json:"api_key"`
}

// CreateTransactionResponse represents the response from Pakasir
type CreateTransactionResponse struct {
	Payment struct {
		Project       string  `json:"project"`
		OrderID       string  `json:"order_id"`
		Amount        float64 `json:"amount"`
		Fee           float64 `json:"fee"`
		TotalPayment  float64 `json:"total_payment"`
		PaymentMethod string  `json:"payment_method"`
		PaymentNumber string  `json:"payment_number"`
		ExpiredAt     string  `json:"expired_at"`
	} `json:"payment"`
	Error string `json:"error,omitempty"`
}

// CreateTransaction creates a payment transaction via Pakasir
// paymentMethod: qris, bni_va, bri_va, permata_va, cimb_niaga_va, etc.
func (s *Service) CreateTransaction(paymentMethod, orderID string, amount float64) (*CreateTransactionResponse, error) {
	// Round amount to whole number (Pakasir expects integer)
	roundedAmount := float64(int(amount + 0.5))

	payload := CreateTransactionRequest{
		Project: s.config.PakasirProject,
		OrderID: orderID,
		Amount:  roundedAmount,
		APIKey:  s.config.PakasirAPIKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s%s/%s", BaseURL, EndpointTransactionCreate, paymentMethod)

	// Debug logging
	log.Printf("[Pakasir] Creating transaction:")
	log.Printf("[Pakasir] Endpoint: %s", endpoint)
	log.Printf("[Pakasir] Project: %s", s.config.PakasirProject)
	log.Printf("[Pakasir] OrderID: %s", orderID)
	log.Printf("[Pakasir] Amount: %.0f (original: %.2f)", roundedAmount, amount)
	log.Printf("[Pakasir] PaymentMethod: %s", paymentMethod)
	log.Printf("[Pakasir] Request Body: %s", string(jsonPayload))

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[Pakasir] HTTP Error: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Debug logging - raw response
	log.Printf("[Pakasir] Response Status: %d", resp.StatusCode)
	log.Printf("[Pakasir] Response Body: %s", string(body))

	var result CreateTransactionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[Pakasir] Parse Error: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}

	if result.Error != "" {
		log.Printf("[Pakasir] API Error: %s", result.Error)
		return nil, fmt.Errorf("pakasir error: %s", result.Error)
	}

	log.Printf("[Pakasir] Success - PaymentNumber: %s, TotalPayment: %.0f", result.Payment.PaymentNumber, result.Payment.TotalPayment)

	return &result, nil
}

// CancelTransaction cancels a payment transaction
func (s *Service) CancelTransaction(orderID string, amount float64) error {
	payload := CreateTransactionRequest{
		Project: s.config.PakasirProject,
		OrderID: orderID,
		Amount:  amount,
		APIKey:  s.config.PakasirAPIKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", BaseURL+EndpointTransactionCancel, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// TransactionDetailResponse represents the detail response
type TransactionDetailResponse struct {
	Transaction struct {
		Amount        float64 `json:"amount"`
		OrderID       string  `json:"order_id"`
		Project       string  `json:"project"`
		Status        string  `json:"status"` // "pending", "completed", "expired"
		PaymentMethod string  `json:"payment_method"`
		CompletedAt   string  `json:"completed_at,omitempty"`
	} `json:"transaction"`
	Error string `json:"error,omitempty"`
}

// GetTransactionDetail gets the detail/status of a transaction
func (s *Service) GetTransactionDetail(orderID string, amount float64) (*TransactionDetailResponse, error) {
	params := url.Values{}
	params.Add("project", s.config.PakasirProject)
	params.Add("order_id", orderID)
	params.Add("amount", fmt.Sprintf("%.0f", amount))
	params.Add("api_key", s.config.PakasirAPIKey)

	endpoint := fmt.Sprintf("%s%s?%s", BaseURL, EndpointTransactionDetail, params.Encode())

	resp, err := s.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result TransactionDetailResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("pakasir error: %s", result.Error)
	}

	return &result, nil
}

// SimulatePayment simulates a payment (sandbox mode only)
func (s *Service) SimulatePayment(orderID string, amount float64) error {
	if !s.config.IsDevelopment() {
		return fmt.Errorf("payment simulation is only available in development mode")
	}

	payload := CreateTransactionRequest{
		Project: s.config.PakasirProject,
		OrderID: orderID,
		Amount:  amount,
		APIKey:  s.config.PakasirAPIKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", BaseURL+EndpointPaymentSimulation, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("simulation failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// WebhookPayload represents the webhook payload from Pakasir
type WebhookPayload struct {
	Amount        float64 `json:"amount"`
	OrderID       string  `json:"order_id"`
	Project       string  `json:"project"`
	Status        string  `json:"status"` // "completed"
	PaymentMethod string  `json:"payment_method"`
	CompletedAt   string  `json:"completed_at"`
}
