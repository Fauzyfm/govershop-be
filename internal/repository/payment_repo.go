package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// PaymentRepository handles database operations for payments
type PaymentRepository struct {
	db *pgxpool.Pool
}

// NewPaymentRepository creates a new PaymentRepository
func NewPaymentRepository(db *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{db: db}
}

// Create creates a new payment
func (r *PaymentRepository) Create(ctx context.Context, payment *model.Payment) error {
	query := `
		INSERT INTO payments (
			order_id, amount, fee, total_payment,
			payment_method, payment_number, status, expired_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		payment.OrderID, payment.Amount, payment.Fee, payment.TotalPayment,
		payment.PaymentMethod, payment.PaymentNumber, payment.Status, payment.ExpiredAt,
	).Scan(&payment.ID, &payment.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	return nil
}

// GetByOrderID retrieves payment by order ID
func (r *PaymentRepository) GetByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	query := `
		SELECT id, order_id, amount, fee, total_payment,
		       payment_method, payment_number, status,
		       expired_at, completed_at, created_at
		FROM payments
		WHERE order_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p model.Payment
	err := r.db.QueryRow(ctx, query, orderID).Scan(
		&p.ID, &p.OrderID, &p.Amount, &p.Fee, &p.TotalPayment,
		&p.PaymentMethod, &p.PaymentNumber, &p.Status,
		&p.ExpiredAt, &p.CompletedAt, &p.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	return &p, nil
}

// GetByID retrieves payment by ID
func (r *PaymentRepository) GetByID(ctx context.Context, id string) (*model.Payment, error) {
	query := `
		SELECT id, order_id, amount, fee, total_payment,
		       payment_method, payment_number, status,
		       expired_at, completed_at, created_at
		FROM payments
		WHERE id = $1
	`

	var p model.Payment
	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.OrderID, &p.Amount, &p.Fee, &p.TotalPayment,
		&p.PaymentMethod, &p.PaymentNumber, &p.Status,
		&p.ExpiredAt, &p.CompletedAt, &p.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	return &p, nil
}

// UpdateStatus updates payment status
func (r *PaymentRepository) UpdateStatus(ctx context.Context, id string, status model.PaymentStatus) error {
	var query string
	if status == model.PaymentStatusCompleted {
		query = `UPDATE payments SET status = $2, completed_at = NOW() WHERE id = $1`
	} else {
		query = `UPDATE payments SET status = $2 WHERE id = $1`
	}

	_, err := r.db.Exec(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	return nil
}

// UpdateStatusByOrderID updates payment status by order ID
func (r *PaymentRepository) UpdateStatusByOrderID(ctx context.Context, orderID string, status model.PaymentStatus) error {
	var query string
	if status == model.PaymentStatusCompleted {
		query = `UPDATE payments SET status = $2, completed_at = NOW() WHERE order_id = $1`
	} else {
		query = `UPDATE payments SET status = $2 WHERE order_id = $1`
	}

	_, err := r.db.Exec(ctx, query, orderID, status)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	return nil
}

// MarkExpiredPayments marks payments past their expiry as expired
func (r *PaymentRepository) MarkExpiredPayments(ctx context.Context) (int, error) {
	query := `
		UPDATE payments 
		SET status = 'expired'
		WHERE status = 'pending' AND expired_at < NOW()
		RETURNING id
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to mark expired payments: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	return count, nil
}

// GetPendingPayments retrieves all pending payments (for status checking)
func (r *PaymentRepository) GetPendingPayments(ctx context.Context) ([]model.Payment, error) {
	query := `
		SELECT id, order_id, amount, fee, total_payment,
		       payment_method, payment_number, status,
		       expired_at, completed_at, created_at
		FROM payments
		WHERE status = 'pending' AND expired_at > NOW()
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending payments: %w", err)
	}
	defer rows.Close()

	var payments []model.Payment
	for rows.Next() {
		var p model.Payment
		err := rows.Scan(
			&p.ID, &p.OrderID, &p.Amount, &p.Fee, &p.TotalPayment,
			&p.PaymentMethod, &p.PaymentNumber, &p.Status,
			&p.ExpiredAt, &p.CompletedAt, &p.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}
		payments = append(payments, p)
	}

	return payments, nil
}

// WebhookLogRepository handles database operations for webhook logs
type WebhookLogRepository struct {
	db *pgxpool.Pool
}

// NewWebhookLogRepository creates a new WebhookLogRepository
func NewWebhookLogRepository(db *pgxpool.Pool) *WebhookLogRepository {
	return &WebhookLogRepository{db: db}
}

// Create logs a webhook request
func (r *WebhookLogRepository) Create(ctx context.Context, source, payload string) (int64, error) {
	query := `INSERT INTO webhook_logs (source, payload) VALUES ($1, $2) RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, query, source, payload).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to log webhook: %w", err)
	}

	return id, nil
}

// MarkProcessed marks a webhook as processed
func (r *WebhookLogRepository) MarkProcessed(ctx context.Context, id int64, errorMsg string) error {
	var query string
	if errorMsg != "" {
		query = `UPDATE webhook_logs SET processed = true, error_message = $2 WHERE id = $1`
		_, err := r.db.Exec(ctx, query, id, errorMsg)
		return err
	}

	query = `UPDATE webhook_logs SET processed = true WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// SyncLogRepository handles database operations for sync logs
type SyncLogRepository struct {
	db *pgxpool.Pool
}

// NewSyncLogRepository creates a new SyncLogRepository
func NewSyncLogRepository(db *pgxpool.Pool) *SyncLogRepository {
	return &SyncLogRepository{db: db}
}

// StartSync creates a new sync log entry
func (r *SyncLogRepository) StartSync(ctx context.Context, syncType string) (int64, error) {
	query := `INSERT INTO sync_logs (sync_type, status) VALUES ($1, 'running') RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, query, syncType).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to start sync log: %w", err)
	}

	return id, nil
}

// CompleteSync updates the sync log with results
func (r *SyncLogRepository) CompleteSync(ctx context.Context, id int64, total, created, updated, failed int, errorMsg string) error {
	status := "success"
	if errorMsg != "" {
		status = "failed"
	}

	query := `
		UPDATE sync_logs SET 
			total_products = $2, new_products = $3, updated_products = $4, failed_products = $5,
			status = $6, error_message = $7, completed_at = $8
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id, total, created, updated, failed, status, errorMsg, time.Now())
	if err != nil {
		return fmt.Errorf("failed to complete sync log: %w", err)
	}

	return nil
}

// GetLastSync retrieves the last sync log
func (r *SyncLogRepository) GetLastSync(ctx context.Context, syncType string) (*model.SyncLog, error) {
	query := `
		SELECT id, sync_type, total_products, new_products, updated_products, failed_products,
		       status, error_message, started_at, completed_at
		FROM sync_logs
		WHERE sync_type = $1
		ORDER BY started_at DESC
		LIMIT 1
	`

	var s model.SyncLog
	err := r.db.QueryRow(ctx, query, syncType).Scan(
		&s.ID, &s.SyncType, &s.TotalProducts, &s.NewProducts, &s.UpdatedProducts, &s.FailedProducts,
		&s.Status, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get last sync: %w", err)
	}

	return &s, nil
}
