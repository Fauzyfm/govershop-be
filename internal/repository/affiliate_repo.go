package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// AffiliateRepository handles database operations for affiliate partners
type AffiliateRepository struct {
	db *pgxpool.Pool
}

// NewAffiliateRepository creates a new AffiliateRepository
func NewAffiliateRepository(db *pgxpool.Pool) *AffiliateRepository {
	return &AffiliateRepository{db: db}
}

// GetByCode retrieves an affiliate partner by their unique code
func (r *AffiliateRepository) GetByCode(ctx context.Context, code string) (*model.AffiliatePartner, error) {
	query := `
		SELECT id, user_id, code, commission_percent, discount_enabled, discount_percent,
			   min_discount_amount, min_transaction_amount, max_discount_uses, max_commission_uses,
			   status, created_at, updated_at
		FROM affiliate_partners WHERE LOWER(code) = LOWER($1)
	`

	var a model.AffiliatePartner
	err := r.db.QueryRow(ctx, query, code).Scan(
		&a.ID, &a.UserID, &a.Code, &a.CommissionPercent,
		&a.DiscountEnabled, &a.DiscountPercent, &a.MinDiscountAmount,
		&a.MinTransactionAmount, &a.MaxDiscountUses, &a.MaxCommissionUses,
		&a.Status, &a.CreatedAt, &a.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get affiliate by code: %w", err)
	}

	return &a, nil
}

// GetByID retrieves an affiliate partner by ID
func (r *AffiliateRepository) GetByID(ctx context.Context, id int) (*model.AffiliatePartner, error) {
	query := `
		SELECT id, user_id, code, commission_percent, discount_enabled, discount_percent,
			   min_discount_amount, min_transaction_amount, max_discount_uses, max_commission_uses,
			   status, created_at, updated_at
		FROM affiliate_partners WHERE id = $1
	`

	var a model.AffiliatePartner
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Code, &a.CommissionPercent,
		&a.DiscountEnabled, &a.DiscountPercent, &a.MinDiscountAmount,
		&a.MinTransactionAmount, &a.MaxDiscountUses, &a.MaxCommissionUses,
		&a.Status, &a.CreatedAt, &a.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get affiliate by id: %w", err)
	}

	return &a, nil
}

// GetByUserID retrieves affiliate partner data for a specific member
func (r *AffiliateRepository) GetByUserID(ctx context.Context, userID int) (*model.AffiliatePartner, error) {
	query := `
		SELECT id, user_id, code, commission_percent, discount_enabled, discount_percent,
			   min_discount_amount, min_transaction_amount, max_discount_uses, max_commission_uses,
			   status, created_at, updated_at
		FROM affiliate_partners WHERE user_id = $1
	`

	var a model.AffiliatePartner
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&a.ID, &a.UserID, &a.Code, &a.CommissionPercent,
		&a.DiscountEnabled, &a.DiscountPercent, &a.MinDiscountAmount,
		&a.MinTransactionAmount, &a.MaxDiscountUses, &a.MaxCommissionUses,
		&a.Status, &a.CreatedAt, &a.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get affiliate by user_id: %w", err)
	}

	return &a, nil
}

// CountUsagesByCustomerThisMonth counts how many times a customer_no has used
// a specific affiliate code this month (global across all brands).
// Returns total usage count for anti-abuse checks.
func (r *AffiliateRepository) CountUsagesByCustomerThisMonth(ctx context.Context, affiliateID int, customerNo string) (int, error) {
	// Get first day of current month
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	query := `
		SELECT COUNT(*) FROM affiliate_usages
		WHERE affiliate_id = $1 AND customer_no = $2 AND created_at >= $3
	`

	var count int
	err := r.db.QueryRow(ctx, query, affiliateID, customerNo, firstOfMonth).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count this month usages: %w", err)
	}

	return count, nil
}

// CreateUsage records a new affiliate usage log entry
func (r *AffiliateRepository) CreateUsage(ctx context.Context, usage *model.AffiliateUsage) error {
	query := `
		INSERT INTO affiliate_usages (
			affiliate_id, customer_no, order_id, channel,
			transaction_amount, discount_applied, discount_amount,
			commission_applied, commission_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		usage.AffiliateID, usage.CustomerNo, usage.OrderID, usage.Channel,
		usage.TransactionAmount, usage.DiscountApplied, usage.DiscountAmount,
		usage.CommissionApplied, usage.CommissionAmount,
	).Scan(&usage.ID, &usage.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create affiliate usage: %w", err)
	}

	return nil
}

// AddAffiliateBalance adds commission to streamer's affiliate_balance (atomic)
func (r *AffiliateRepository) AddAffiliateBalance(ctx context.Context, userID int, amount float64) error {
	query := `UPDATE users SET affiliate_balance = COALESCE(affiliate_balance, 0) + $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, amount, userID)
	if err != nil {
		return fmt.Errorf("failed to add affiliate balance: %w", err)
	}
	return nil
}

// GetMonthlyStats returns affiliate stats for the current month
func (r *AffiliateRepository) GetMonthlyStats(ctx context.Context, affiliateID int) (totalUsages, linkUsages, codeUsages int, totalCommission float64, err error) {
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE channel = 'link') as link_count,
			COUNT(*) FILTER (WHERE channel = 'code') as code_count,
			COALESCE(SUM(commission_amount) FILTER (WHERE commission_applied = true), 0) as total_commission
		FROM affiliate_usages
		WHERE affiliate_id = $1 AND created_at >= $2
	`

	err = r.db.QueryRow(ctx, query, affiliateID, firstOfMonth).Scan(
		&totalUsages, &linkUsages, &codeUsages, &totalCommission,
	)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get monthly stats: %w", err)
	}

	return
}

// GetAffiliateBalance retrieves the affiliate_balance for a user
func (r *AffiliateRepository) GetAffiliateBalance(ctx context.Context, userID int) (float64, error) {
	query := `SELECT COALESCE(affiliate_balance, 0) FROM users WHERE id = $1`

	var balance float64
	err := r.db.QueryRow(ctx, query, userID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("failed to get affiliate balance: %w", err)
	}

	return balance, nil
}

// ListAll returns all affiliate partners (for admin)
func (r *AffiliateRepository) ListAll(ctx context.Context) ([]model.AffiliatePartner, error) {
	query := `
		SELECT id, user_id, code, commission_percent, discount_enabled, discount_percent,
			   min_discount_amount, min_transaction_amount, max_discount_uses, max_commission_uses,
			   status, created_at, updated_at
		FROM affiliate_partners ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list affiliates: %w", err)
	}
	defer rows.Close()

	var affiliates []model.AffiliatePartner
	for rows.Next() {
		var a model.AffiliatePartner
		err := rows.Scan(
			&a.ID, &a.UserID, &a.Code, &a.CommissionPercent,
			&a.DiscountEnabled, &a.DiscountPercent, &a.MinDiscountAmount,
			&a.MinTransactionAmount, &a.MaxDiscountUses, &a.MaxCommissionUses,
			&a.Status, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan affiliate: %w", err)
		}
		affiliates = append(affiliates, a)
	}

	return affiliates, nil
}

// Create inserts a new affiliate partner (admin only)
func (r *AffiliateRepository) Create(ctx context.Context, a *model.AffiliatePartner) error {
	query := `
		INSERT INTO affiliate_partners (
			user_id, code, commission_percent, discount_enabled, discount_percent,
			min_discount_amount, min_transaction_amount, max_discount_uses, max_commission_uses, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		a.UserID, a.Code, a.CommissionPercent,
		a.DiscountEnabled, a.DiscountPercent,
		a.MinDiscountAmount, a.MinTransactionAmount,
		a.MaxDiscountUses, a.MaxCommissionUses, a.Status,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create affiliate: %w", err)
	}

	return nil
}

// Update updates an affiliate partner
func (r *AffiliateRepository) Update(ctx context.Context, id int, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	query := "UPDATE affiliate_partners SET "
	args := []interface{}{}
	i := 1

	for key, value := range updates {
		if i > 1 {
			query += ", "
		}
		query += fmt.Sprintf("%s = $%d", key, i)
		args = append(args, value)
		i++
	}

	query += fmt.Sprintf(", updated_at = NOW() WHERE id = $%d", i)
	args = append(args, id)

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update affiliate: %w", err)
	}

	return nil
}
