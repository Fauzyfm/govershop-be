package model

import (
	"time"
)

// Product represents a digital product from Digiflazz (cached in database)
type Product struct {
	ID                  int64     `json:"id" db:"id"`
	BuyerSKUCode        string    `json:"buyer_sku_code" db:"buyer_sku_code"`
	ProductName         string    `json:"product_name" db:"product_name"`
	Category            string    `json:"category" db:"category"`
	Brand               string    `json:"brand" db:"brand"`
	Type                string    `json:"type" db:"type"`
	SellerName          string    `json:"seller_name,omitempty" db:"seller_name"`
	BuyPrice            float64   `json:"-" db:"buy_price"`                             // Hidden from FE
	MarkupPercent       float64   `json:"-" db:"markup_percent"`                        // Hidden from FE
	SellingPrice        float64   `json:"selling_price" db:"selling_price"`             // Displayed to FE
	DiscountPrice       *float64  `json:"discount_price,omitempty" db:"discount_price"` // Promo price
	IsAvailable         bool      `json:"is_available" db:"is_available"`
	BuyerProductStatus  bool      `json:"-" db:"buyer_product_status"`
	SellerProductStatus bool      `json:"-" db:"seller_product_status"`
	UnlimitedStock      bool      `json:"unlimited_stock" db:"unlimited_stock"`
	Stock               int       `json:"stock,omitempty" db:"stock"`
	Description         string    `json:"description,omitempty" db:"description"`
	StartCutOff         string    `json:"start_cut_off,omitempty" db:"start_cut_off"`
	EndCutOff           string    `json:"end_cut_off,omitempty" db:"end_cut_off"`
	IsMulti             bool      `json:"is_multi" db:"is_multi"`
	LastSyncAt          time.Time `json:"last_sync_at" db:"last_sync_at"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`

	// Custom fields (NOT overwritten by sync)
	DisplayName  *string  `json:"display_name,omitempty" db:"display_name"` // Custom name for FE
	IsBestSeller bool     `json:"is_best_seller" db:"is_best_seller"`       // Best seller flag
	Tags         []string `json:"tags,omitempty" db:"tags"`                 // Product tags (e.g., "diamond", "wdp")
}

// ProductResponse is the response format for FE (with calculated final price)
type ProductResponse struct {
	BuyerSKUCode   string   `json:"buyer_sku_code"`
	ProductName    string   `json:"product_name"`
	Category       string   `json:"category"`
	Brand          string   `json:"brand"`
	Type           string   `json:"type"`
	Price          float64  `json:"price"`          // Final price (discount or selling)
	OriginalPrice  *float64 `json:"original_price"` // Original price if discounted
	IsAvailable    bool     `json:"is_available"`
	UnlimitedStock bool     `json:"unlimited_stock"`
	Stock          int      `json:"stock,omitempty"`
	Description    string   `json:"description,omitempty"`
	IsPromo        bool     `json:"is_promo"`       // True if discounted
	IsBestSeller   bool     `json:"is_best_seller"` // Best seller flag
	Tags           []string `json:"tags,omitempty"` // Product tags
}

// ToResponse converts Product to ProductResponse for FE
func (p *Product) ToResponse() ProductResponse {
	// Use DisplayName if set, otherwise fallback to ProductName
	displayName := p.ProductName
	if p.DisplayName != nil && *p.DisplayName != "" {
		displayName = *p.DisplayName
	}

	resp := ProductResponse{
		BuyerSKUCode:   p.BuyerSKUCode,
		ProductName:    displayName,
		Category:       p.Category,
		Brand:          p.Brand,
		Type:           p.Type,
		Price:          p.SellingPrice,
		IsAvailable:    p.IsAvailable,
		UnlimitedStock: p.UnlimitedStock,
		Stock:          p.Stock,
		Description:    p.Description,
		IsPromo:        false,
		IsBestSeller:   p.IsBestSeller,
		Tags:           p.Tags,
	}

	// If there's a discount price, use it
	if p.DiscountPrice != nil && *p.DiscountPrice > 0 && *p.DiscountPrice < p.SellingPrice {
		resp.Price = *p.DiscountPrice
		resp.OriginalPrice = &p.SellingPrice
		resp.IsPromo = true
	}

	return resp
}

// DigiflazzProduct represents the product structure from Digiflazz API
type DigiflazzProduct struct {
	ProductName         string  `json:"product_name"`
	Category            string  `json:"category"`
	Brand               string  `json:"brand"`
	Type                string  `json:"type"`
	SellerName          string  `json:"seller_name"`
	Price               float64 `json:"price"`
	BuyerSKUCode        string  `json:"buyer_sku_code"`
	BuyerProductStatus  bool    `json:"buyer_product_status"`
	SellerProductStatus bool    `json:"seller_product_status"`
	UnlimitedStock      bool    `json:"unlimited_stock"`
	Stock               int     `json:"stock"`
	Multi               bool    `json:"multi"`
	StartCutOff         string  `json:"start_cut_off"`
	EndCutOff           string  `json:"end_cut_off"`
	Desc                string  `json:"desc"`
}
