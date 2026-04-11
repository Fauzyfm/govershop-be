package model

import "time"

// AffiliatePartner represents an affiliate/streamer partner
type AffiliatePartner struct {
	ID                    int       `json:"id" db:"id"`
	UserID                int       `json:"user_id" db:"user_id"`
	Code                  string    `json:"code" db:"code"`
	CommissionPercent     float64   `json:"commission_percent" db:"commission_percent"`
	DiscountEnabled       bool      `json:"discount_enabled" db:"discount_enabled"`
	DiscountPercent       float64   `json:"discount_percent" db:"discount_percent"`
	MinDiscountAmount     float64   `json:"min_discount_amount" db:"min_discount_amount"`
	MinTransactionAmount  float64   `json:"min_transaction_amount" db:"min_transaction_amount"`
	MaxDiscountUses       int       `json:"max_discount_uses" db:"max_discount_uses"`
	MaxCommissionUses     int       `json:"max_commission_uses" db:"max_commission_uses"`
	Status                string    `json:"status" db:"status"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// AffiliatePartnerStatus constants
const (
	AffiliateStatusActive   = "active"
	AffiliateStatusInactive = "inactive"
)

// AffiliateUsage tracks each usage of an affiliate code/link
type AffiliateUsage struct {
	ID                int       `json:"id" db:"id"`
	AffiliateID       int       `json:"affiliate_id" db:"affiliate_id"`
	CustomerNo        string    `json:"customer_no" db:"customer_no"`
	OrderID           string    `json:"order_id" db:"order_id"`
	Channel           string    `json:"channel" db:"channel"` // "link" or "code"
	TransactionAmount float64   `json:"transaction_amount" db:"transaction_amount"`
	DiscountApplied   bool      `json:"discount_applied" db:"discount_applied"`
	DiscountAmount    float64   `json:"discount_amount" db:"discount_amount"`
	CommissionApplied bool      `json:"commission_applied" db:"commission_applied"`
	CommissionAmount  float64   `json:"commission_amount" db:"commission_amount"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

// AffiliateChannel constants
const (
	AffiliateChannelLink = "link"
	AffiliateChannelCode = "code"
)

// ValidateAffiliateRequest is the request body for validating an affiliate code/link
type ValidateAffiliateRequest struct {
	Code              string  `json:"code"`
	CustomerNo        string  `json:"customer_no"`
	TransactionAmount float64 `json:"transaction_amount"`
	Channel           string  `json:"channel"` // "link" or "code"
}

// ValidateAffiliateResponse is the response for affiliate validation
type ValidateAffiliateResponse struct {
	Valid           bool    `json:"valid"`
	AffiliateID     int     `json:"affiliate_id,omitempty"`
	DiscountAmount  float64 `json:"discount_amount"`
	DiscountPercent float64 `json:"discount_percent,omitempty"`
	Channel         string  `json:"channel"`
	Message         string  `json:"message"`
	UsageCount      int     `json:"usage_count"`
	MaxDiscount     int     `json:"max_discount_uses"`
	MaxCommission   int     `json:"max_commission_uses"`
}

// AffiliateStatsResponse returns stats for streamer dashboard
type AffiliateStatsResponse struct {
	Code                string  `json:"code"`
	Link                string  `json:"link"`
	TotalUsages         int     `json:"total_usages"`
	LinkUsages          int     `json:"link_usages"`
	CodeUsages          int     `json:"code_usages"`
	TotalCommission     float64 `json:"total_commission"`
	AffiliateBalance    float64 `json:"affiliate_balance"`
	CommissionPercent   float64 `json:"commission_percent"`
	DiscountEnabled     bool    `json:"discount_enabled"`
	DiscountPercent     float64 `json:"discount_percent"`
	MinDiscountAmount   float64 `json:"min_discount_amount"`
}
