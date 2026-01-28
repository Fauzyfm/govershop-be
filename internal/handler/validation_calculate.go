package handler

import (
	"encoding/json"
	"net/http"
)

// CalculatePriceRequest is the request for price calculation
type CalculatePriceRequest struct {
	BuyerSKUCode  string `json:"buyer_sku_code"` // SKU produk yang dibeli
	PaymentMethod string `json:"payment_method"` // qris, bni_va, dll
	Brand         string `json:"brand"`          // Brand untuk check username
}

// CalculatePriceResponse is the response for price calculation
type CalculatePriceResponse struct {
	ProductPrice       float64        `json:"product_price"`        // Harga produk
	AdminFee           float64        `json:"admin_fee"`            // Biaya check username
	PaymentFee         float64        `json:"payment_fee"`          // Biaya payment gateway
	TotalPrice         float64        `json:"total_price"`          // Total yang harus dibayar
	ProductName        string         `json:"product_name"`         // Nama produk
	PaymentMethodLabel string         `json:"payment_method_label"` // Label metode pembayaran
	Breakdown          PriceBreakdown `json:"breakdown"`            // Detail breakdown
}

// PriceBreak down details each component
type PriceBreakdown struct {
	Items []PriceItem `json:"items"`
}

// PriceItem represents each line item in price calculation
type PriceItem struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

// CalculatePrice handles POST /api/v1/calculate-price
func (h *ValidationHandler) CalculatePrice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CalculatePriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Validate required fields
	if req.BuyerSKUCode == "" {
		BadRequest(w, "buyer_sku_code wajib diisi")
		return
	}
	if req.PaymentMethod == "" {
		BadRequest(w, "payment_method wajib diisi")
		return
	}
	if req.Brand == "" {
		BadRequest(w, "brand wajib diisi")
		return
	}

	// Get product
	product, err := h.productRepo.GetBySKU(ctx, req.BuyerSKUCode)
	if err != nil {
		NotFound(w, "Produk tidak ditemukan")
		return
	}

	if !product.IsAvailable {
		BadRequest(w, "Produk sedang tidak tersedia")
		return
	}

	// Determine base price (checking if promo exists)
	sellingPrice := product.SellingPrice
	if product.DiscountPrice != nil && *product.DiscountPrice > 0 {
		sellingPrice = *product.DiscountPrice
	}

	// Flat admin fee as per requirement
	var adminFee float64 = 10

	// Calculate total
	// Payment fee is handled by payment gateway (Pakasir) automatically
	totalPrice := sellingPrice + adminFee

	// Build breakdown
	breakdown := PriceBreakdown{
		Items: []PriceItem{
			{Label: product.ProductName, Amount: sellingPrice},
			{Label: "Biaya Admin", Amount: adminFee},
		},
	}

	Success(w, "Kalkulasi harga berhasil", CalculatePriceResponse{
		ProductPrice:       sellingPrice,
		AdminFee:           adminFee,
		PaymentFee:         0, // Fee handled by gateway
		TotalPrice:         totalPrice,
		ProductName:        product.ProductName,
		PaymentMethodLabel: req.PaymentMethod,
		Breakdown:          breakdown,
	})
}
