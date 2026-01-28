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

// Create creates a new order
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	query := `
		INSERT INTO orders (
			ref_id, buyer_sku_code, product_name, customer_no,
			buy_price, selling_price, status,
			customer_email, customer_phone, customer_name
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		order.RefID, order.BuyerSKUCode, order.ProductName, order.CustomerNo,
		order.BuyPrice, order.SellingPrice, order.Status,
		order.CustomerEmail, order.CustomerPhone, order.CustomerName,
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

// GetAll retrieves all orders (for admin)
func (r *OrderRepository) GetAll(ctx context.Context, limit, offset int) ([]model.Order, error) {
	query := `
		SELECT id, ref_id, buyer_sku_code, product_name, customer_no,
		       buy_price, selling_price, status,
		       COALESCE(digiflazz_status, ''), COALESCE(digiflazz_rc, ''), COALESCE(serial_number, ''), COALESCE(digiflazz_message, ''),
		       COALESCE(customer_email, ''), COALESCE(customer_phone, ''), COALESCE(customer_name, ''),
		       created_at, updated_at, completed_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
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
