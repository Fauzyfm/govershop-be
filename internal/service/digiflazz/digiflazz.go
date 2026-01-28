package digiflazz

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
)

const (
	BaseURL           = "https://api.digiflazz.com/v1"
	EndpointBalance   = "/cek-saldo"
	EndpointPriceList = "/price-list"
	EndpointTransact  = "/transaction"
)

// Service handles all Digiflazz API interactions
type Service struct {
	config     *config.Config
	httpClient *http.Client
}

// NewService creates a new Digiflazz service
func NewService(cfg *config.Config) *Service {
	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GenerateSignature generates MD5 signature for API requests
func (s *Service) GenerateSignature(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

// BalanceResponse represents the response from check balance API
type BalanceResponse struct {
	Data struct {
		Deposit float64 `json:"deposit"`
	} `json:"data"`
}

// CheckBalance checks the current deposit balance
func (s *Service) CheckBalance() (*BalanceResponse, error) {
	// Signature: md5(username + key + "depo")
	signature := s.GenerateSignature(s.config.DigiflazzUsername + s.config.GetDigiflazzKey() + "depo")

	payload := map[string]string{
		"cmd":      "deposit",
		"username": s.config.DigiflazzUsername,
		"sign":     signature,
	}

	resp, err := s.doRequest(EndpointBalance, payload)
	if err != nil {
		return nil, err
	}

	var result BalanceResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse balance response: %w", err)
	}

	return &result, nil
}

// PriceListResponse represents the response from price list API
type PriceListResponse struct {
	Data []model.DigiflazzProduct `json:"data"`
}

// GetPriceList fetches the price list from Digiflazz
// cmd: "prepaid" for prepaid products, "pasca" for postpaid
func (s *Service) GetPriceList(cmd string) ([]model.DigiflazzProduct, error) {
	// Signature: md5(username + key + "depo")
	signature := s.GenerateSignature(s.config.DigiflazzUsername + s.config.GetDigiflazzKey() + "depo")

	payload := map[string]string{
		"cmd":      cmd,
		"username": s.config.DigiflazzUsername,
		"sign":     signature,
	}

	resp, err := s.doRequest(EndpointPriceList, payload)
	if err != nil {
		return nil, err
	}

	var result PriceListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse price list response: %w", err)
	}

	return result.Data, nil
}

// TopupRequest represents a topup transaction request
type TopupRequest struct {
	BuyerSKUCode string `json:"buyer_sku_code"`
	CustomerNo   string `json:"customer_no"`
	RefID        string `json:"ref_id"`
	CallbackURL  string `json:"cb_url,omitempty"`
	Testing      bool   `json:"testing,omitempty"`
	Msg          string `json:"msg,omitempty"`
}

// TopupResponse represents the response from topup transaction
type TopupResponse struct {
	Data struct {
		RefID          string  `json:"ref_id"`
		CustomerNo     string  `json:"customer_no"`
		BuyerSKUCode   string  `json:"buyer_sku_code"`
		Message        string  `json:"message"`
		Status         string  `json:"status"` // "Pending", "Sukses", "Gagal"
		RC             string  `json:"rc"`     // Response code
		SN             string  `json:"sn"`     // Serial number
		BuyerLastSaldo float64 `json:"buyer_last_saldo"`
		Price          float64 `json:"price"`
		Tele           string  `json:"tele"`
		WA             string  `json:"wa"`
	} `json:"data"`
}

// CreateTransaction creates a topup transaction
func (s *Service) CreateTransaction(req TopupRequest) (*TopupResponse, error) {
	// Signature: md5(username + key + ref_id)
	signature := s.GenerateSignature(s.config.DigiflazzUsername + s.config.GetDigiflazzKey() + req.RefID)

	payload := map[string]interface{}{
		"username":       s.config.DigiflazzUsername,
		"buyer_sku_code": req.BuyerSKUCode,
		"customer_no":    req.CustomerNo,
		"ref_id":         req.RefID,
		"sign":           signature,
	}

	// Optional fields
	if req.Testing {
		payload["testing"] = true
	}
	if req.CallbackURL != "" {
		payload["cb_url"] = req.CallbackURL
	}
	if req.Msg != "" {
		payload["msg"] = req.Msg
	}

	resp, err := s.doRequest(EndpointTransact, payload)
	if err != nil {
		return nil, err
	}

	var result TopupResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse topup response: %w", err)
	}

	return &result, nil
}

// CheckTransactionStatus checks the status of a prepaid transaction
// For prepaid, you resend the same transaction with the same ref_id
func (s *Service) CheckTransactionStatus(buyerSKUCode, customerNo, refID string) (*TopupResponse, error) {
	return s.CreateTransaction(TopupRequest{
		BuyerSKUCode: buyerSKUCode,
		CustomerNo:   customerNo,
		RefID:        refID,
	})
}

// WebhookPayload represents the webhook payload from Digiflazz
type WebhookPayload struct {
	Data struct {
		TrxID        string  `json:"trx_id"`
		RefID        string  `json:"ref_id"`
		CustomerNo   string  `json:"customer_no"`
		BuyerSKUCode string  `json:"buyer_sku_code"`
		Message      string  `json:"message"`
		Status       string  `json:"status"`
		RC           string  `json:"rc"`
		SN           string  `json:"sn"`
		Price        float64 `json:"price"`
	} `json:"data"`
}

// doRequest performs HTTP POST request to Digiflazz API
func (s *Service) doRequest(endpoint string, payload interface{}) ([]byte, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", BaseURL+endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
