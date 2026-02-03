package model

import (
	"time"
)

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusExpired   PaymentStatus = "expired"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

// PaymentMethod represents available payment methods
type PaymentMethod string

const (
	PaymentMethodQRIS         PaymentMethod = "qris"
	PaymentMethodBNIVA        PaymentMethod = "bni_va"
	PaymentMethodBRIVA        PaymentMethod = "bri_va"
	PaymentMethodMandiriVA    PaymentMethod = "mandiri_va"
	PaymentMethodPermataVA    PaymentMethod = "permata_va"
	PaymentMethodCIMBNiagaVA  PaymentMethod = "cimb_niaga_va"
	PaymentMethodSampoernaVA  PaymentMethod = "sampoerna_va"
	PaymentMethodBNCVA        PaymentMethod = "bnc_va"
	PaymentMethodMaybankVA    PaymentMethod = "maybank_va"
	PaymentMethodArthaGrahaVA PaymentMethod = "artha_graha_va"
	PaymentMethodATMBersamaVA PaymentMethod = "atm_bersama_va"
)

// Payment represents a payment transaction via Pakasir
type Payment struct {
	ID            string        `json:"id" db:"id"`
	OrderID       string        `json:"order_id" db:"order_id"`
	Amount        float64       `json:"amount" db:"amount"`
	Fee           float64       `json:"fee" db:"fee"`
	TotalPayment  float64       `json:"total_payment" db:"total_payment"`
	PaymentMethod PaymentMethod `json:"payment_method" db:"payment_method"`
	PaymentNumber string        `json:"payment_number" db:"payment_number"` // QR string or VA number
	Status        PaymentStatus `json:"status" db:"status"`
	ExpiredAt     time.Time     `json:"expired_at" db:"expired_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
}

// InitiatePaymentRequest is the request body for initiating payment
type InitiatePaymentRequest struct {
	PaymentMethod PaymentMethod `json:"payment_method" validate:"required"`
}

// PaymentResponse is the response format for FE
type PaymentResponse struct {
	ID            string        `json:"id"`
	OrderID       string        `json:"order_id"`
	Amount        float64       `json:"amount"`
	Fee           float64       `json:"fee"`
	TotalPayment  float64       `json:"total_payment"`
	PaymentMethod PaymentMethod `json:"payment_method"`
	PaymentNumber string        `json:"payment_number"`
	QRString      string        `json:"qr_string,omitempty"` // Derived from PaymentNumber for QRIS
	VANumber      string        `json:"va_number,omitempty"` // Derived from PaymentNumber for VA
	Status        PaymentStatus `json:"status"`
	ExpiredAt     time.Time     `json:"expired_at"`
	ExpiredIn     int64         `json:"expired_in"` // Seconds until expiration
}

// ToResponse converts Payment to PaymentResponse for FE
func (p *Payment) ToResponse() PaymentResponse {
	expiredIn := time.Until(p.ExpiredAt).Seconds()
	if expiredIn < 0 {
		expiredIn = 0
	}

	resp := PaymentResponse{
		ID:            p.ID,
		OrderID:       p.OrderID,
		Amount:        p.Amount,
		Fee:           p.Fee,
		TotalPayment:  p.TotalPayment,
		PaymentMethod: p.PaymentMethod,
		PaymentNumber: p.PaymentNumber,
		Status:        p.Status,
		ExpiredAt:     p.ExpiredAt,
		ExpiredIn:     int64(expiredIn),
	}

	// Map PaymentNumber to specific fields for frontend convenience
	if p.PaymentMethod == PaymentMethodQRIS {
		resp.QRString = p.PaymentNumber
	} else {
		resp.VANumber = p.PaymentNumber
	}

	return resp
}

// PakasirCreateResponse represents the response from Pakasir transaction create
type PakasirCreateResponse struct {
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
}

// PakasirWebhookPayload represents the webhook payload from Pakasir
type PakasirWebhookPayload struct {
	Amount        float64 `json:"amount"`
	OrderID       string  `json:"order_id"`
	Project       string  `json:"project"`
	Status        string  `json:"status"` // "completed"
	PaymentMethod string  `json:"payment_method"`
	CompletedAt   string  `json:"completed_at"`
}

// GetAvailablePaymentMethods returns list of available payment methods
// Based on Pakasir API documentation: https://pakasir.com/p/docs
func GetAvailablePaymentMethods() []map[string]string {
	return []map[string]string{
		// QRIS
		{"code": "qris", "name": "QRIS", "type": "qris"},
		// Virtual Account - Bank Besar
		{"code": "bni_va", "name": "BNI Virtual Account", "type": "va"},
		{"code": "bri_va", "name": "BRI Virtual Account", "type": "va"},
		{"code": "permata_va", "name": "Permata Virtual Account", "type": "va"},
		{"code": "cimb_niaga_va", "name": "CIMB Niaga Virtual Account", "type": "va"},
		{"code": "maybank_va", "name": "Maybank Virtual Account", "type": "va"},
		// Virtual Account - Bank Lainnya
		{"code": "bnc_va", "name": "BNC Virtual Account", "type": "va"},
		{"code": "sampoerna_va", "name": "Bank Sampoerna Virtual Account", "type": "va"},
		{"code": "artha_graha_va", "name": "Artha Graha Virtual Account", "type": "va"},
		{"code": "atm_bersama_va", "name": "ATM Bersama Virtual Account", "type": "va"},
		// PayPal
		{"code": "paypal", "name": "PayPal", "type": "paypal"},
	}
}
