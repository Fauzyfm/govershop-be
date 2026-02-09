package model

import (
	"time"
)

// User represents a member/reseller user
type User struct {
	ID        int       `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"-" db:"password"` // Never expose password
	Email     *string   `json:"email,omitempty" db:"email"`
	FullName  string    `json:"full_name" db:"full_name"`
	Role      string    `json:"role" db:"role"` // member, admin
	Balance   float64   `json:"balance" db:"balance"`
	Status    string    `json:"status" db:"status"` // active, suspended
	WhatsApp  *string   `json:"whatsapp,omitempty" db:"whatsapp"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Deposit represents a balance transaction (topup, debit, refund)
type Deposit struct {
	ID          int       `json:"id" db:"id"`
	UserID      int       `json:"user_id" db:"user_id"`
	Amount      float64   `json:"amount" db:"amount"`
	Type        string    `json:"type" db:"type"` // credit, debit, refund
	Description *string   `json:"description,omitempty" db:"description"`
	ReferenceID *string   `json:"reference_id,omitempty" db:"reference_id"`
	Status      string    `json:"status" db:"status"` // success, pending, failed
	CreatedBy   string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// DepositType constants
const (
	DepositTypeCredit = "credit" // Top-up
	DepositTypeDebit  = "debit"  // Order/Purchase
	DepositTypeRefund = "refund" // Refund from failed order
)

// UserStatus constants
const (
	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"
)

// UserRole constants
const (
	UserRoleMember = "member"
	UserRoleAdmin  = "admin"
)

// CreateUserRequest for admin creating a new member
type CreateUserRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=6"`
	Email    string `json:"email,omitempty" validate:"omitempty,email"`
	FullName string `json:"full_name" validate:"required"`
	WhatsApp string `json:"whatsapp,omitempty"`
}

// UpdateUserRequest for updating user data
type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	WhatsApp *string `json:"whatsapp,omitempty"`
	Status   *string `json:"status,omitempty"`
}

// TopupRequest for admin topping up member balance
type TopupRequest struct {
	Amount      float64 `json:"amount" validate:"required,min=20000"`
	Description string  `json:"description,omitempty"`
}

// MemberLoginRequest for member login
type MemberLoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// MemberLoginResponse for login response
type MemberLoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// MemberOrderRequest for creating a new order
type MemberOrderRequest struct {
	BuyerSKUCode      string `json:"buyer_sku_code"`
	DestinationNumber string `json:"destination_number"`
	Pin               string `json:"pin,omitempty"` // Optional security pin
}

// ForgotPasswordRequest for password reset request
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest for resetting password with token
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=6"`
}

// MemberDashboardResponse for member dashboard overview
type MemberDashboardResponse struct {
	Balance       float64 `json:"balance"`
	TotalOrders   int     `json:"total_orders"`
	SuccessOrders int     `json:"success_orders"`
	PendingOrders int     `json:"pending_orders"`
	TodayOrders   int     `json:"today_orders"`
}

// UserResponse is a safe user response without password
type UserResponse struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Email     *string   `json:"email,omitempty"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	Balance   float64   `json:"balance"`
	Status    string    `json:"status"`
	WhatsApp  *string   `json:"whatsapp,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ToResponse converts User to UserResponse (safe for frontend)
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		FullName:  u.FullName,
		Role:      u.Role,
		Balance:   u.Balance,
		Status:    u.Status,
		WhatsApp:  u.WhatsApp,
		CreatedAt: u.CreatedAt,
	}
}
