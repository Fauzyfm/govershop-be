package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// UserRepository handles database operations for users
type UserRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	query := `
		INSERT INTO users (username, password, email, full_name, role, balance, status, whatsapp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		user.Username,
		user.Password,
		user.Email,
		user.FullName,
		user.Role,
		user.Balance,
		user.Status,
		user.WhatsApp,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id int) (*model.User, error) {
	query := `
		SELECT id, username, password, email, full_name, role, balance, status, whatsapp, created_at, updated_at
		FROM users WHERE id = $1
	`

	var user model.User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.FullName,
		&user.Role,
		&user.Balance,
		&user.Status,
		&user.WhatsApp,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by username (for login)
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `
		SELECT id, username, password, email, full_name, role, balance, status, whatsapp, created_at, updated_at
		FROM users WHERE username = $1
	`

	var user model.User
	err := r.db.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.FullName,
		&user.Role,
		&user.Balance,
		&user.Status,
		&user.WhatsApp,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// GetByEmail retrieves a user by email (for password reset)
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, username, password, email, full_name, role, balance, status, whatsapp, created_at, updated_at
		FROM users WHERE email = $1
	`

	var user model.User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.Email,
		&user.FullName,
		&user.Role,
		&user.Balance,
		&user.Status,
		&user.WhatsApp,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}

// GetAllMembers retrieves all members with pagination
func (r *UserRepository) GetAllMembers(ctx context.Context, limit, offset int, search string) ([]model.User, int, error) {
	// Count query
	countQuery := `SELECT COUNT(*) FROM users WHERE role = 'member'`
	args := []interface{}{}
	argIndex := 1

	if search != "" {
		countQuery += fmt.Sprintf(" AND (username ILIKE $%d OR full_name ILIKE $%d OR email ILIKE $%d)", argIndex, argIndex, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count members: %w", err)
	}

	// Data query
	dataQuery := `
		SELECT id, username, password, email, full_name, role, balance, status, whatsapp, created_at, updated_at
		FROM users WHERE role = 'member'
	`

	dataArgs := []interface{}{}
	dataArgIndex := 1

	if search != "" {
		dataQuery += fmt.Sprintf(" AND (username ILIKE $%d OR full_name ILIKE $%d OR email ILIKE $%d)", dataArgIndex, dataArgIndex, dataArgIndex)
		dataArgs = append(dataArgs, "%"+search+"%")
		dataArgIndex++
	}

	dataQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", dataArgIndex, dataArgIndex+1)
	dataArgs = append(dataArgs, limit, offset)

	rows, err := r.db.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get members: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var user model.User
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Password,
			&user.Email,
			&user.FullName,
			&user.Role,
			&user.Balance,
			&user.Status,
			&user.WhatsApp,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, total, nil
}

// Update updates user data
func (r *UserRepository) Update(ctx context.Context, id int, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	query := "UPDATE users SET "
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

	query += fmt.Sprintf(" WHERE id = $%d", i)
	args = append(args, id)

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// UpdatePassword updates user password
func (r *UserRepository) UpdatePassword(ctx context.Context, id int, hashedPassword string) error {
	query := `UPDATE users SET password = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, hashedPassword, id)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

// UpdateBalance updates user balance
func (r *UserRepository) UpdateBalance(ctx context.Context, id int, newBalance float64) error {
	query := `UPDATE users SET balance = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, newBalance, id)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}
	return nil
}

// TopupBalance adds balance and creates deposit log (transaction)
func (r *UserRepository) TopupBalance(ctx context.Context, userID int, amount float64, description, createdBy string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current balance
	var currentBalance float64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("failed to get current balance: %w", err)
	}

	newBalance := currentBalance + amount

	// Update balance
	_, err = tx.Exec(ctx, "UPDATE users SET balance = $1 WHERE id = $2", newBalance, userID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Create deposit log
	_, err = tx.Exec(ctx, `
		INSERT INTO deposits (user_id, amount, type, description, status, created_by)
		VALUES ($1, $2, 'credit', $3, 'success', $4)
	`, userID, amount, description, createdBy)
	if err != nil {
		return fmt.Errorf("failed to create deposit log: %w", err)
	}

	return tx.Commit(ctx)
}

// DeductBalance subtracts balance and creates deposit log (transaction)
func (r *UserRepository) DeductBalance(ctx context.Context, userID int, amount float64, description, referenceID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current balance
	var currentBalance float64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("failed to get current balance: %w", err)
	}

	if currentBalance < amount {
		return fmt.Errorf("insufficient balance")
	}

	newBalance := currentBalance - amount

	// Update balance
	_, err = tx.Exec(ctx, "UPDATE users SET balance = $1 WHERE id = $2", newBalance, userID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Create deposit log
	_, err = tx.Exec(ctx, `
		INSERT INTO deposits (user_id, amount, type, description, reference_id, status, created_by)
		VALUES ($1, $2, 'debit', $3, $4, 'success', 'system')
	`, userID, amount, description, referenceID)
	if err != nil {
		return fmt.Errorf("failed to create deposit log: %w", err)
	}

	return tx.Commit(ctx)
}

// RefundBalance adds balance back and creates refund log (transaction)
func (r *UserRepository) RefundBalance(ctx context.Context, userID int, amount float64, description, referenceID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current balance
	var currentBalance float64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("failed to get current balance: %w", err)
	}

	newBalance := currentBalance + amount

	// Update balance
	_, err = tx.Exec(ctx, "UPDATE users SET balance = $1 WHERE id = $2", newBalance, userID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Create deposit log
	_, err = tx.Exec(ctx, `
		INSERT INTO deposits (user_id, amount, type, description, reference_id, status, created_by)
		VALUES ($1, $2, 'refund', $3, $4, 'success', 'system')
	`, userID, amount, description, referenceID)
	if err != nil {
		return fmt.Errorf("failed to create deposit log: %w", err)
	}

	return tx.Commit(ctx)
}

// GetDeposits retrieves deposit history for a user with filters
func (r *UserRepository) GetDeposits(ctx context.Context, userID int, limit, offset int, dateFrom, dateTo, depositType string) ([]model.Deposit, int, error) {
	// Build dynamic WHERE clause
	whereClause := "WHERE user_id = $1"
	args := []interface{}{userID}
	paramIdx := 2

	if dateFrom != "" {
		whereClause += fmt.Sprintf(" AND created_at >= $%d::date", paramIdx)
		args = append(args, dateFrom)
		paramIdx++
	}
	if dateTo != "" {
		whereClause += fmt.Sprintf(" AND created_at < ($%d::date + interval '1 day')", paramIdx)
		args = append(args, dateTo)
		paramIdx++
	}
	if depositType != "" && depositType != "all" {
		whereClause += fmt.Sprintf(" AND type = $%d", paramIdx)
		args = append(args, depositType)
		paramIdx++
	}

	// Count query
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM deposits %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count deposits: %w", err)
	}

	// Data query
	query := fmt.Sprintf(`
		SELECT id, user_id, amount, type, description, reference_id, status, created_by, created_at
		FROM deposits %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, paramIdx, paramIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get deposits: %w", err)
	}
	defer rows.Close()

	var deposits []model.Deposit
	for rows.Next() {
		var d model.Deposit
		err := rows.Scan(
			&d.ID,
			&d.UserID,
			&d.Amount,
			&d.Type,
			&d.Description,
			&d.ReferenceID,
			&d.Status,
			&d.CreatedBy,
			&d.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan deposit: %w", err)
		}
		deposits = append(deposits, d)
	}

	return deposits, total, nil
}

// GetMemberStats returns order statistics for a member
func (r *UserRepository) GetMemberStats(ctx context.Context, userID int) (total, success, pending, today int, err error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'success') as success,
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing')) as pending,
			COUNT(*) FILTER (WHERE DATE(created_at) = CURRENT_DATE) as today
		FROM orders WHERE member_id = $1
	`

	err = r.db.QueryRow(ctx, query, userID).Scan(&total, &success, &pending, &today)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get member stats: %w", err)
	}

	return total, success, pending, today, nil
}

// Delete soft-deletes a user by setting status to suspended
func (r *UserRepository) Delete(ctx context.Context, id int) error {
	query := `UPDATE users SET status = 'suspended' WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}
