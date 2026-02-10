package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// OrderRepository handles database operations for orders
type OrderRepository struct {
	db *pgxpool.Pool
}

// NewOrderRepository creates a new OrderRepository
func NewOrderRepository(db *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{db: db}
}

// GetDB returns the database pool for direct queries
func (r *OrderRepository) GetDB() *pgxpool.Pool {
	return r.db
}

// CreateWithSource creates an order with custom source (for admin topups)
func (r *OrderRepository) CreateWithSource(ctx context.Context, refID, sku, productName, customerNo string, buyPrice, sellingPrice float64, source, notes string) (string, error) {
	query := `
		INSERT INTO orders (
			ref_id, buyer_sku_code, product_name, customer_no,
			buy_price, selling_price, status, order_source, admin_notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'processing', $7, $8
		)
		RETURNING id
	`

	var orderID string
	err := r.db.QueryRow(ctx, query, refID, sku, productName, customerNo, buyPrice, sellingPrice, source, notes).Scan(&orderID)
	if err != nil {
		return "", fmt.Errorf("failed to create order with source: %w", err)
	}

	return orderID, nil
}

// Create creates a new order
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	// Default order source to website if empty
	if order.OrderSource == "" {
		order.OrderSource = "website"
	}

	query := `
		INSERT INTO orders (
			ref_id, buyer_sku_code, product_name, customer_no,
			buy_price, selling_price, status,
			customer_email, customer_phone, customer_name,
			member_id, member_price, order_source
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		order.RefID, order.BuyerSKUCode, order.ProductName, order.CustomerNo,
		order.BuyPrice, order.SellingPrice, order.Status,
		order.CustomerEmail, order.CustomerPhone, order.CustomerName,
		order.MemberID, order.MemberPrice, order.OrderSource,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// GetByID retrieves an order by ID
func (r *OrderRepository) GetByID(ctx context.Context, id string) (*model.Order, error) {
	fmt.Printf("[DEBUG DB] Getting Order ID: %s\n", id)
	query := `
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       member_id, member_price,
		       created_at, updated_at, completed_at
		FROM orders
		WHERE id = $1
	`

	var o model.Order
	err := r.db.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.RefID, &o.BuyerSKUCode, &o.ProductName, &o.CustomerNo,
		&o.BuyPrice, &o.SellingPrice, &o.Status,
		&o.DigiflazzStatus, &o.DigiflazzRC, &o.SerialNumber, &o.DigiflazzMsg,
		&o.CustomerEmail, &o.CustomerPhone, &o.CustomerName,
		&o.MemberID, &o.MemberPrice,
		&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &o, nil
}

// GetByRefID retrieves an order by RefID (for Digiflazz webhook)
func (r *OrderRepository) GetByRefID(ctx context.Context, refID string) (*model.Order, error) {
	query := `
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       member_id, member_price,
		       created_at, updated_at, completed_at
		FROM orders
		WHERE ref_id = $1
	`

	var o model.Order
	err := r.db.QueryRow(ctx, query, refID).Scan(
		&o.ID, &o.RefID, &o.BuyerSKUCode, &o.ProductName, &o.CustomerNo,
		&o.BuyPrice, &o.SellingPrice, &o.Status,
		&o.DigiflazzStatus, &o.DigiflazzRC, &o.SerialNumber, &o.DigiflazzMsg,
		&o.CustomerEmail, &o.CustomerPhone, &o.CustomerName,
		&o.MemberID, &o.MemberPrice,
		&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order by ref_id: %w", err)
	}

	return &o, nil
}

// UpdateStatus updates the order status
func (r *OrderRepository) UpdateStatus(ctx context.Context, id string, status model.OrderStatus) error {
	query := `UPDATE orders SET status = $2, updated_at = NOW() WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	return nil
}

// UpdateDigiflazzResponse updates the order with Digiflazz response
func (r *OrderRepository) UpdateDigiflazzResponse(ctx context.Context, id string, status model.OrderStatus, dfStatus, rc, sn, message string) error {
	var query string
	var args []interface{}

	if status == model.OrderStatusSuccess || status == model.OrderStatusFailed {
		query = `
			UPDATE orders SET 
				status = $2, digiflazz_status = $3, digiflazz_rc = $4, 
				serial_number = $5, digiflazz_message = $6,
				updated_at = NOW(), completed_at = NOW()
			WHERE id = $1
		`
		args = []interface{}{id, status, dfStatus, rc, sn, message}
	} else {
		query = `
			UPDATE orders SET 
				status = $2, digiflazz_status = $3, digiflazz_rc = $4, 
				serial_number = $5, digiflazz_message = $6,
				updated_at = NOW()
			WHERE id = $1
		`
		args = []interface{}{id, status, dfStatus, rc, sn, message}
	}

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update digiflazz response: %w", err)
	}

	return nil
}

// UpdateCustomerNo updates the customer number (for manual topup retries)
func (r *OrderRepository) UpdateCustomerNo(ctx context.Context, id, customerNo string) error {
	query := `UPDATE orders SET customer_no = $2, updated_at = NOW() WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, customerNo)
	if err != nil {
		return fmt.Errorf("failed to update customer_no: %w", err)
	}

	return nil
}

// GetTotalRevenue calculate total revenue from successful orders
func (r *OrderRepository) GetTotalRevenue(ctx context.Context) (float64, error) {
	query := `
		SELECT COALESCE(SUM(selling_price), 0)
		FROM orders
		WHERE status IN ('success', 'processing', 'paid')
	`
	var total float64
	err := r.db.QueryRow(ctx, query).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total revenue: %w", err)
	}
	return total, nil
}

// GetAll retrieves all orders with optional filtering (for admin)
func (r *OrderRepository) GetAll(ctx context.Context, limit, offset int, search, status, dateFrom, dateTo, paymentStatus, digiflazzStatus string) ([]model.Order, int, error) {
	// Use the internal implementation with date range support
	return r.getAllInternal(ctx, limit, offset, search, status, dateFrom, dateTo, paymentStatus, digiflazzStatus)
}

func (r *OrderRepository) getAllInternal(ctx context.Context, limit, offset int, search, status, dateFrom, dateTo, paymentStatus, digiflazzStatus string) ([]model.Order, int, error) {
	var conditions []string
	var args []interface{}
	argCounter := 1

	if status != "" && status != "all" {
		if status == "pending" {
			conditions = append(conditions, fmt.Sprintf("status IN ($%d, $%d)", argCounter, argCounter+1))
			args = append(args, "pending", "waiting_payment")
			argCounter += 2
		} else {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argCounter))
			args = append(args, status)
			argCounter++
		}
	}

	if search != "" {
		// PostgreSQL is case insensitive with ILIKE
		searchCondition := fmt.Sprintf("(ref_id ILIKE $%d OR customer_no ILIKE $%d OR product_name ILIKE $%d OR serial_number ILIKE $%d OR customer_email ILIKE $%d OR customer_phone ILIKE $%d)", argCounter, argCounter, argCounter, argCounter, argCounter, argCounter)
		conditions = append(conditions, searchCondition)
		args = append(args, "%"+search+"%")
		argCounter++
	}

	// Date range filter
	if dateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("DATE(created_at) >= $%d", argCounter))
		args = append(args, dateFrom)
		argCounter++
	}
	if dateTo != "" {
		conditions = append(conditions, fmt.Sprintf("DATE(created_at) <= $%d", argCounter))
		args = append(args, dateTo)
		argCounter++
	}

	// Digiflazz status filter
	if digiflazzStatus != "" && digiflazzStatus != "all" {
		conditions = append(conditions, fmt.Sprintf("digiflazz_status = $%d", argCounter))
		args = append(args, digiflazzStatus)
		argCounter++
	}

	whereStmt := ""
	if len(conditions) > 0 {
		whereStmt = " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			whereStmt += " AND " + conditions[i]
		}
	}

	// Count Total
	var total int
	countQuery := "SELECT COUNT(*) FROM orders" + whereStmt
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count: %w", err)
	}

	// Get Data
	query := fmt.Sprintf(`
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       created_at, updated_at, completed_at,
		       COALESCE(order_source, 'website'), COALESCE(admin_notes, ''),
		       member_id, member_price
		FROM orders
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereStmt, argCounter, argCounter+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query orders: %w", err)
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.RefID, &o.BuyerSKUCode, &o.ProductName, &o.CustomerNo,
			&o.BuyPrice, &o.SellingPrice, &o.Status,
			&o.DigiflazzStatus, &o.DigiflazzRC, &o.SerialNumber, &o.DigiflazzMsg,
			&o.CustomerEmail, &o.CustomerPhone, &o.CustomerName,
			&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
			&o.OrderSource, &o.AdminNotes,
			&o.MemberID, &o.MemberPrice,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, o)
	}

	return orders, total, nil
}

// GetByCustomerPhone retrieves orders by customer phone
func (r *OrderRepository) GetByCustomerPhone(ctx context.Context, phone string, limit int) ([]model.Order, error) {
	query := `
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       created_at, updated_at, completed_at
		FROM orders
		WHERE customer_phone = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, phone, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query orders: %w", err)
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.RefID, &o.BuyerSKUCode, &o.ProductName, &o.CustomerNo,
			&o.BuyPrice, &o.SellingPrice, &o.Status,
			&o.DigiflazzStatus, &o.DigiflazzRC, &o.SerialNumber, &o.DigiflazzMsg,
			&o.CustomerEmail, &o.CustomerPhone, &o.CustomerName,
			&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, o)
	}

	return orders, nil
}

// CountByStatus counts orders by status (for dashboard)
func (r *OrderRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	query := `SELECT status, COUNT(*) FROM orders GROUP BY status`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count orders: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[status] = count
	}

	return counts, nil
}

// GetTodayStats returns today's order statistics
func (r *OrderRepository) GetTodayStats(ctx context.Context) (totalOrders int, totalRevenue float64, err error) {
	query := `
		SELECT COUNT(*), COALESCE(SUM(selling_price), 0)
		FROM orders
		WHERE DATE(created_at) = DATE(NOW()) AND status IN ('success', 'processing', 'paid')
	`

	err = r.db.QueryRow(ctx, query).Scan(&totalOrders, &totalRevenue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get today stats: %w", err)
	}

	return totalOrders, totalRevenue, nil
}

// GetByMemberID retrieves orders for a specific member with pagination and filters
func (r *OrderRepository) GetByMemberID(ctx context.Context, memberID, limit, offset int, dateFrom, dateTo, status, search string) ([]model.Order, int, error) {
	// Build dynamic WHERE clause
	whereClause := "WHERE member_id = $1"
	args := []interface{}{memberID}
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
	if status != "" && status != "all" {
		whereClause += fmt.Sprintf(" AND status = $%d", paramIdx)
		args = append(args, status)
		paramIdx++
	}
	if search != "" {
		whereClause += fmt.Sprintf(" AND (buyer_sku_code ILIKE $%d OR product_name ILIKE $%d OR ref_id ILIKE $%d OR customer_no ILIKE $%d)", paramIdx, paramIdx, paramIdx, paramIdx)
		args = append(args, "%"+search+"%")
		paramIdx++
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM orders %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count member orders: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       member_id, member_price,
		       created_at, updated_at, completed_at
		FROM orders
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, paramIdx, paramIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query member orders: %w", err)
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.RefID, &o.BuyerSKUCode, &o.ProductName, &o.CustomerNo,
			&o.BuyPrice, &o.SellingPrice, &o.Status,
			&o.DigiflazzStatus, &o.DigiflazzRC, &o.SerialNumber, &o.DigiflazzMsg,
			&o.CustomerEmail, &o.CustomerPhone, &o.CustomerName,
			&o.MemberID, &o.MemberPrice,
			&o.CreatedAt, &o.UpdatedAt, &o.CompletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan member order: %w", err)
		}
		orders = append(orders, o)
	}

	return orders, total, nil
}

// CleanupExpiredPendingOrders cancels orders that have been pending too long
func (r *OrderRepository) CleanupExpiredPendingOrders(ctx context.Context, maxAge time.Duration) (int, error) {
	query := `
		UPDATE orders 
		SET status = 'cancelled', updated_at = NOW()
		WHERE status = 'pending' AND created_at < NOW() - $1::interval
		RETURNING id
	`

	rows, err := r.db.Query(ctx, query, maxAge.String())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired orders: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	return count, nil
}
