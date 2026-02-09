package model

import (
	"time"
)

// OrderStatus represents the status of an order
type OrderStatus string

const (
	OrderStatusPending        OrderStatus = "pending"         // Order created, no payment yet
	OrderStatusWaitingPayment OrderStatus = "waiting_payment" // Payment initiated, waiting for user payment
	OrderStatusPaid           OrderStatus = "paid"            // Payment confirmed by Pakasir
	OrderStatusProcessing     OrderStatus = "processing"      // Topup in progress at Digiflazz
	OrderStatusSuccess        OrderStatus = "success"         // Topup completed successfully
	OrderStatusFailed         OrderStatus = "failed"          // Topup failed (Digiflazz error)
	OrderStatusExpired        OrderStatus = "expired"         // Payment expired (timeout)
	OrderStatusCancelled      OrderStatus = "cancelled"       // Cancelled by user
	OrderStatusRefunded       OrderStatus = "refunded"        // Payment refunded
)

// Order represents a customer order
type Order struct {
	ID              string      `json:"id" db:"id"`
	RefID           string      `json:"ref_id" db:"ref_id"`
	BuyerSKUCode    string      `json:"buyer_sku_code" db:"buyer_sku_code"`
	ProductName     string      `json:"product_name" db:"product_name"`
	CustomerNo      string      `json:"customer_no" db:"customer_no"`
	BuyPrice        float64     `json:"-" db:"buy_price"` // Hidden from FE
	SellingPrice    float64     `json:"selling_price" db:"selling_price"`
	Status          OrderStatus `json:"status" db:"status"`
	DigiflazzStatus string      `json:"digiflazz_status,omitempty" db:"digiflazz_status"`
	DigiflazzRC     string      `json:"digiflazz_rc,omitempty" db:"digiflazz_rc"`
	SerialNumber    string      `json:"serial_number,omitempty" db:"serial_number"`
	DigiflazzMsg    string      `json:"message,omitempty" db:"digiflazz_message"`
	CustomerEmail   string      `json:"customer_email,omitempty" db:"customer_email"`
	CustomerPhone   string      `json:"customer_phone,omitempty" db:"customer_phone"`
	CustomerName    string      `json:"customer_name,omitempty" db:"customer_name"`
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at" db:"updated_at"`
	CompletedAt     *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
	OrderSource     string      `json:"order_source" db:"order_source"`
	AdminNotes      string      `json:"admin_notes,omitempty" db:"admin_notes"`
	MemberID        *int        `json:"member_id,omitempty" db:"member_id"`
	MemberPrice     *float64    `json:"member_price,omitempty" db:"member_price"`
}

// CreateOrderRequest is the request body for creating an order
type CreateOrderRequest struct {
	BuyerSKUCode  string `json:"buyer_sku_code" validate:"required"`
	CustomerNo    string `json:"customer_no" validate:"required"`
	CustomerEmail string `json:"customer_email,omitempty"`
	CustomerPhone string `json:"customer_phone,omitempty"`
	CustomerName  string `json:"customer_name,omitempty"`
}

// OrderResponse is the response format for FE
type OrderResponse struct {
	ID           string      `json:"id"`
	RefID        string      `json:"ref_id"`
	ProductName  string      `json:"product_name"`
	CustomerNo   string      `json:"customer_no"`
	Price        float64     `json:"price"`
	Status       OrderStatus `json:"status"`
	StatusLabel  string      `json:"status_label"`
	SerialNumber string      `json:"serial_number,omitempty"`
	Message      string      `json:"message,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	Payment      *Payment    `json:"payment,omitempty"`
	IsMember     bool        `json:"is_member,omitempty"`
}

// GetStatusLabel returns human-readable status label
func (o *Order) GetStatusLabel() string {
	labels := map[OrderStatus]string{
		OrderStatusPending:        "Menunggu Pembayaran",
		OrderStatusWaitingPayment: "Menunggu Pembayaran",
		OrderStatusPaid:           "Pembayaran Berhasil",
		OrderStatusProcessing:     "Sedang Diproses",
		OrderStatusSuccess:        "Berhasil",
		OrderStatusFailed:         "Gagal",
		OrderStatusExpired:        "Kadaluwarsa",
		OrderStatusCancelled:      "Dibatalkan",
		OrderStatusRefunded:       "Dana Dikembalikan",
	}
	if label, ok := labels[o.Status]; ok {
		return label
	}
	return string(o.Status)
}

// ToResponse converts Order to OrderResponse for FE
func (o *Order) ToResponse(payment *Payment) OrderResponse {
	return OrderResponse{
		ID:           o.ID,
		RefID:        o.RefID,
		ProductName:  o.ProductName,
		CustomerNo:   o.CustomerNo,
		Price:        o.SellingPrice,
		Status:       o.Status,
		StatusLabel:  o.GetStatusLabel(),
		SerialNumber: o.SerialNumber,
		Message:      o.DigiflazzMsg,
		CreatedAt:    o.CreatedAt,
		CompletedAt:  o.CompletedAt,
		Payment:      payment,
	}
}
