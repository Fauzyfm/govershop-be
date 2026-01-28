package model

import "time"

// Promo represents a discount/promo event
type Promo struct {
	ID            int64     `json:"id" db:"id"`
	Name          string    `json:"name" db:"name"`
	Code          *string   `json:"code,omitempty" db:"code"`
	Description   string    `json:"description,omitempty" db:"description"`
	DiscountType  string    `json:"discount_type" db:"discount_type"` // "percent" or "fixed"
	DiscountValue float64   `json:"discount_value" db:"discount_value"`
	MinPurchase   float64   `json:"min_purchase" db:"min_purchase"`
	MaxDiscount   *float64  `json:"max_discount,omitempty" db:"max_discount"`
	UsageLimit    *int      `json:"usage_limit,omitempty" db:"usage_limit"`
	UsageCount    int       `json:"usage_count" db:"usage_count"`
	Category      *string   `json:"category,omitempty" db:"category"`
	Brand         *string   `json:"brand,omitempty" db:"brand"`
	BuyerSKUCode  *string   `json:"buyer_sku_code,omitempty" db:"buyer_sku_code"`
	StartDate     time.Time `json:"start_date" db:"start_date"`
	EndDate       time.Time `json:"end_date" db:"end_date"`
	IsActive      bool      `json:"is_active" db:"is_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// IsValid checks if the promo is currently valid
func (p *Promo) IsValid() bool {
	now := time.Now()

	// Check if active
	if !p.IsActive {
		return false
	}

	// Check date range
	if now.Before(p.StartDate) || now.After(p.EndDate) {
		return false
	}

	// Check usage limit
	if p.UsageLimit != nil && p.UsageCount >= *p.UsageLimit {
		return false
	}

	return true
}

// CalculateDiscount calculates the discount amount for a given price
func (p *Promo) CalculateDiscount(price float64) float64 {
	if price < p.MinPurchase {
		return 0
	}

	var discount float64

	if p.DiscountType == "percent" {
		discount = price * (p.DiscountValue / 100)

		// Apply max discount cap
		if p.MaxDiscount != nil && discount > *p.MaxDiscount {
			discount = *p.MaxDiscount
		}
	} else {
		// Fixed discount
		discount = p.DiscountValue
	}

	// Discount cannot exceed price
	if discount > price {
		discount = price
	}

	return discount
}

// WebhookLog represents a webhook request log
type WebhookLog struct {
	ID           int64     `json:"id" db:"id"`
	Source       string    `json:"source" db:"source"` // "pakasir" or "digiflazz"
	Payload      string    `json:"payload" db:"payload"`
	Processed    bool      `json:"processed" db:"processed"`
	ErrorMessage *string   `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// SyncLog represents a product sync log
type SyncLog struct {
	ID              int64      `json:"id" db:"id"`
	SyncType        string     `json:"sync_type" db:"sync_type"`
	TotalProducts   int        `json:"total_products" db:"total_products"`
	NewProducts     int        `json:"new_products" db:"new_products"`
	UpdatedProducts int        `json:"updated_products" db:"updated_products"`
	FailedProducts  int        `json:"failed_products" db:"failed_products"`
	Status          string     `json:"status" db:"status"` // "running", "success", "failed"
	ErrorMessage    *string    `json:"error_message,omitempty" db:"error_message"`
	StartedAt       time.Time  `json:"started_at" db:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}
