package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminSecurity represents the admin security settings
type AdminSecurity struct {
	ID              int
	TOTPSecret      string
	TOTPEnabled     bool
	AdminIdentifier string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AdminAuditLog represents an audit log entry
type AdminAuditLog struct {
	ID           int
	Action       string
	OrderID      *string
	Details      map[string]interface{}
	IPAddress    string
	Success      bool
	ErrorMessage string
	CreatedAt    time.Time
}

// AdminSecurityRepository handles admin security operations
type AdminSecurityRepository struct {
	db *pgxpool.Pool
}

// NewAdminSecurityRepository creates a new AdminSecurityRepository
func NewAdminSecurityRepository(db *pgxpool.Pool) *AdminSecurityRepository {
	return &AdminSecurityRepository{db: db}
}

// GetPrimary gets the primary admin security settings
func (r *AdminSecurityRepository) GetPrimary(ctx context.Context) (*AdminSecurity, error) {
	query := `
		SELECT id, COALESCE(totp_secret, ''), totp_enabled, admin_identifier, created_at, updated_at
		FROM admin_security
		WHERE admin_identifier = 'primary'
		LIMIT 1
	`

	var sec AdminSecurity
	err := r.db.QueryRow(ctx, query).Scan(
		&sec.ID,
		&sec.TOTPSecret,
		&sec.TOTPEnabled,
		&sec.AdminIdentifier,
		&sec.CreatedAt,
		&sec.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin security: %w", err)
	}

	return &sec, nil
}

// SetTOTPSecret sets the TOTP secret for primary admin
func (r *AdminSecurityRepository) SetTOTPSecret(ctx context.Context, secret string) error {
	query := `
		UPDATE admin_security 
		SET totp_secret = $1, updated_at = NOW()
		WHERE admin_identifier = 'primary'
	`

	_, err := r.db.Exec(ctx, query, secret)
	if err != nil {
		return fmt.Errorf("failed to set TOTP secret: %w", err)
	}

	return nil
}

// EnableTOTP enables TOTP for primary admin
func (r *AdminSecurityRepository) EnableTOTP(ctx context.Context, enabled bool) error {
	query := `
		UPDATE admin_security 
		SET totp_enabled = $1, updated_at = NOW()
		WHERE admin_identifier = 'primary'
	`

	_, err := r.db.Exec(ctx, query, enabled)
	if err != nil {
		return fmt.Errorf("failed to enable TOTP: %w", err)
	}

	return nil
}

// CreateAuditLog creates an audit log entry
func (r *AdminSecurityRepository) CreateAuditLog(ctx context.Context, action, orderID, ipAddress string, details map[string]interface{}, success bool, errMsg string) error {
	query := `
		INSERT INTO admin_audit_logs (action, order_id, details, ip_address, success, error_message)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	var orderIDPtr *string
	if orderID != "" {
		orderIDPtr = &orderID
	}

	_, err := r.db.Exec(ctx, query, action, orderIDPtr, details, ipAddress, success, errMsg)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// CountRecentManualTopups counts manual topups in the last hour for rate limiting
func (r *AdminSecurityRepository) CountRecentManualTopups(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM admin_audit_logs 
		WHERE action = 'manual_topup' 
		AND success = true
		AND created_at > NOW() - INTERVAL '1 hour'
	`

	var count int
	err := r.db.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count manual topups: %w", err)
	}

	return count, nil
}
