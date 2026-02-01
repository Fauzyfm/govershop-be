package handler

import (
	"encoding/json"
	"math"
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

	// Calculate Payment Fee
	var paymentFee float64 = 0
	method := req.PaymentMethod

	switch {
	case method == "qris" || method == "shopeepay_qris" || method == "dana_qris":
		// QRIS: 0.7% + 310
		paymentFee = (sellingPrice * 0.007) + 310
	case method == "paypal":
		// Paypal: 1%
		paymentFee = sellingPrice * 0.01
	case method == "artha_va" || method == "sampoerna_va":
		// Specific VAs: 2000
		paymentFee = 2000
	case method == "bri_va" || method == "bni_va" || method == "mandiri_va" ||
		method == "cimb_va" || method == "danamon_va" || method == "permata_va" ||
		method == "maybank_va" || method == "bnc_va" || method == "atm_bersama_va":
		// Bank VAs: 3500
		paymentFee = 3500
	default:
		// Default fallback for VAs if not matched but contains "va"
		if len(method) > 3 && method[len(method)-3:] == "_va" {
			paymentFee = 3500
		}
	}

	// Round up payment fee to nearest integer if needed, or keeping float for precision
	// Usually fees are integers in IDR, but QRIS % might result in decimals.
	// Let's use math.Ceil for paymentFee to be safe
	paymentFee = math.Ceil(paymentFee)

	// Calculate total
	totalPrice := sellingPrice + adminFee + paymentFee

	// Build breakdown
	breakdown := PriceBreakdown{
		Items: []PriceItem{
			{Label: product.ProductName, Amount: sellingPrice},
			{Label: "Biaya Admin", Amount: adminFee},
			{Label: "Biaya Transaksi", Amount: paymentFee},
		},
	}

	Success(w, "Kalkulasi harga berhasil", CalculatePriceResponse{
		ProductPrice:       sellingPrice,
		AdminFee:           adminFee,
		PaymentFee:         paymentFee,
		TotalPrice:         totalPrice,
		ProductName:        product.ProductName,
		PaymentMethodLabel: req.PaymentMethod,
		Breakdown:          breakdown,
	})
}
