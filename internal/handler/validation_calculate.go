package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
)

// CalculatePriceRequest is the request for price calculation
type CalculatePriceRequest struct {
	BuyerSKUCode  string `json:"buyer_sku_code"` // SKU produk yang dibeli
	PaymentMethod string `json:"payment_method"` // qris, bni, bri, etc (iPaymu channel code)
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

	// Calculate Payment Fee from iPaymu channels data
	var paymentFee float64 = 0
	paymentFee = h.getIpaymuFee(req.PaymentMethod, sellingPrice)

	// Round up payment fee to nearest integer
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

// getIpaymuFee fetches the fee for a given payment channel from iPaymu
func (h *ValidationHandler) getIpaymuFee(channelCode string, amount float64) float64 {
	if h.ipaymuSvc == nil {
		return 0
	}

	channels, err := h.ipaymuSvc.GetPaymentChannels()
	if err != nil {
		return 0
	}

	// Find the matching channel in any category
	for _, category := range channels.Data {
		for _, ch := range category.Channels {
			if strings.EqualFold(ch.Code, channelCode) {
				var fee float64
				if ch.TransactionFee.ActualFeeType == "PERCENT" {
					fee = (ch.TransactionFee.ActualFee / 100) * amount
				} else {
					// FLAT fee
					fee = ch.TransactionFee.ActualFee
				}
				fee += ch.TransactionFee.AdditionalFee
				return fee
			}
		}
	}

	return 0
}
