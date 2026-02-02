package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// ProductRepository handles database operations for products
type ProductRepository struct {
	db *pgxpool.Pool
}

// NewProductRepository creates a new ProductRepository
func NewProductRepository(db *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{db: db}
}

// GetAll retrieves all available products
func (r *ProductRepository) GetAll(ctx context.Context) ([]model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
		WHERE is_available = true
		ORDER BY category, brand, product_name
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// GetByCategory retrieves products by category
func (r *ProductRepository) GetByCategory(ctx context.Context, category string) ([]model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
		WHERE is_available = true AND LOWER(category) = LOWER($1)
		ORDER BY brand, product_name
	`

	rows, err := r.db.Query(ctx, query, category)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// GetByBrand retrieves products by brand
func (r *ProductRepository) GetByBrand(ctx context.Context, brand string) ([]model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
		WHERE is_available = true AND LOWER(brand) = LOWER($1)
		ORDER BY category, product_name
	`

	rows, err := r.db.Query(ctx, query, brand)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// GetBySKU retrieves a product by SKU code
func (r *ProductRepository) GetBySKU(ctx context.Context, sku string) (*model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
		WHERE buyer_sku_code = $1
	`

	var p model.Product
	err := r.db.QueryRow(ctx, query, sku).Scan(
		&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
		&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
		&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
		&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
		&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	return &p, nil
}

// GetCategories retrieves list of available categories
func (r *ProductRepository) GetCategories(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT category FROM products 
		WHERE is_available = true AND category IS NOT NULL
		ORDER BY category
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories: %w", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		categories = append(categories, category)
	}

	return categories, nil
}

// GetTypes retrieves list of available product types
func (r *ProductRepository) GetTypes(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT type FROM products 
		WHERE type IS NOT NULL AND type != ''
		ORDER BY type
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query types: %w", err)
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("failed to scan type: %w", err)
		}
		types = append(types, t)
	}

	return types, nil
}

// GetBrands retrieves list of available brands (optionally filtered by category)
func (r *ProductRepository) GetBrands(ctx context.Context, category string) ([]model.Brand, error) {
	var query string
	var rows interface {
		Close()
		Next() bool
		Scan(...interface{}) error
	}
	var err error

	if category != "" {
		query = `
			SELECT brand, MAX(image_url) as image_url FROM products 
			WHERE is_available = true AND brand IS NOT NULL AND LOWER(category) = LOWER($1)
			GROUP BY brand
			ORDER BY brand
		`
		rows, err = r.db.Query(ctx, query, category)
	} else {
		query = `
			SELECT brand, MAX(image_url) as image_url FROM products 
			WHERE is_available = true AND brand IS NOT NULL
			GROUP BY brand
			ORDER BY brand
		`
		rows, err = r.db.Query(ctx, query)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query brands: %w", err)
	}
	defer rows.Close()

	var brands []model.Brand
	for rows.Next() {
		var b model.Brand
		if err := rows.Scan(&b.Name, &b.ImageURL); err != nil {
			return nil, fmt.Errorf("failed to scan brand: %w", err)
		}
		brands = append(brands, b)
	}

	return brands, nil
}

// UpsertProduct inserts or updates a product from Digiflazz sync
func (r *ProductRepository) UpsertProduct(ctx context.Context, dfProduct model.DigiflazzProduct, markupPercent float64) error {
	// Calculate selling price with markup
	sellingPrice := dfProduct.Price + (dfProduct.Price * markupPercent / 100)
	sellingPrice = math.Ceil(sellingPrice) // Round up

	isAvailable := dfProduct.BuyerProductStatus && dfProduct.SellerProductStatus

	query := `
		INSERT INTO products (
			buyer_sku_code, product_name, category, brand, type, seller_name,
			buy_price, markup_percent, selling_price, is_available,
			buyer_product_status, seller_product_status, unlimited_stock, stock,
			description, start_cut_off, end_cut_off, is_multi, last_sync_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18, $19
		)
		ON CONFLICT (buyer_sku_code) DO UPDATE SET
			product_name = EXCLUDED.product_name,
			category = EXCLUDED.category,
			brand = EXCLUDED.brand,
			type = EXCLUDED.type,
			seller_name = EXCLUDED.seller_name,
			buy_price = EXCLUDED.buy_price,
			selling_price = CEIL(products.buy_price + (products.buy_price * products.markup_percent / 100)),
			is_available = EXCLUDED.is_available,
			buyer_product_status = EXCLUDED.buyer_product_status,
			seller_product_status = EXCLUDED.seller_product_status,
			unlimited_stock = EXCLUDED.unlimited_stock,
			stock = EXCLUDED.stock,
			description = EXCLUDED.description,
			start_cut_off = EXCLUDED.start_cut_off,
			end_cut_off = EXCLUDED.end_cut_off,
			is_multi = EXCLUDED.is_multi,
			last_sync_at = EXCLUDED.last_sync_at
	`

	_, err := r.db.Exec(ctx, query,
		dfProduct.BuyerSKUCode, dfProduct.ProductName, dfProduct.Category, dfProduct.Brand,
		dfProduct.Type, dfProduct.SellerName, dfProduct.Price, markupPercent, sellingPrice, isAvailable,
		dfProduct.BuyerProductStatus, dfProduct.SellerProductStatus, dfProduct.UnlimitedStock, dfProduct.Stock,
		dfProduct.Desc, dfProduct.StartCutOff, dfProduct.EndCutOff, dfProduct.Multi, time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to upsert product: %w", err)
	}

	return nil
}

// MarkUnavailable marks products not in the sync as unavailable
func (r *ProductRepository) MarkUnavailable(ctx context.Context, skuCodes []string) error {
	if len(skuCodes) == 0 {
		return nil
	}

	query := `
		UPDATE products 
		SET is_available = false, updated_at = NOW()
		WHERE buyer_sku_code NOT IN (SELECT UNNEST($1::text[]))
	`

	_, err := r.db.Exec(ctx, query, skuCodes)
	if err != nil {
		return fmt.Errorf("failed to mark unavailable: %w", err)
	}

	return nil
}

// ==========================================
// ADMIN CRUD METHODS
// ==========================================

// GetAllForAdmin retrieves all products for admin (including unavailable) with filtering
func (r *ProductRepository) GetAllForAdmin(ctx context.Context, limit, offset int, search, category, typeStr, status string) ([]model.Product, int, error) {
	// Build WHERE clause
	whereClause := " WHERE 1=1"
	var args []interface{}
	argCounter := 1

	if search != "" {
		whereClause += fmt.Sprintf(" AND (product_name ILIKE $%d OR buyer_sku_code ILIKE $%d OR brand ILIKE $%d)", argCounter, argCounter, argCounter)
		args = append(args, "%"+search+"%")
		argCounter++
	}

	if category != "" && category != "all" {
		whereClause += fmt.Sprintf(" AND category = $%d", argCounter)
		args = append(args, category)
		argCounter++
	}

	if typeStr != "" && typeStr != "all" {
		whereClause += fmt.Sprintf(" AND type = $%d", argCounter)
		args = append(args, typeStr)
		argCounter++
	}

	if status != "" && status != "all" {
		if status == "active" {
			whereClause += " AND buyer_product_status = true"
		} else {
			whereClause += " AND buyer_product_status = false"
		}
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM products" + whereClause
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count products: %w", err)
	}

	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
	` + whereClause + fmt.Sprintf(" ORDER BY category, brand, product_name LIMIT $%d OFFSET $%d", argCounter, argCounter+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, total, nil
}

// UpdateCustomFields updates admin-editable fields for a product
// When a pointer is nil, the field is not updated. When a pointer has an empty value, the field is set to NULL.
func (r *ProductRepository) UpdateCustomFields(ctx context.Context, sku string, displayName *string, isBestSeller *bool, markupPercent *float64, discountPrice *float64, tags []string, imageURL *string, description *string) error {
	// Build dynamic update query to allow clearing fields
	query := `
		UPDATE products SET
			display_name = $2,
			is_best_seller = COALESCE($3, is_best_seller),
			markup_percent = COALESCE($4, markup_percent),
			discount_price = $5,
			tags = CASE WHEN $6::text[] IS NULL THEN tags ELSE $6::text[] END,
			image_url = $7,
			description = $8,
			selling_price = CEIL(buy_price + (buy_price * COALESCE($4, markup_percent) / 100)),
			updated_at = NOW()
		WHERE buyer_sku_code = $1
	`

	// Handle empty string as NULL for clearing
	var displayNameVal interface{} = displayName
	if displayName != nil && *displayName == "" {
		displayNameVal = nil // Clear to NULL
	}

	var imageURLVal interface{} = imageURL
	if imageURL != nil && *imageURL == "" {
		imageURLVal = nil // Clear to NULL
	}

	var descriptionVal interface{} = description
	if description != nil && *description == "" {
		descriptionVal = nil // Clear to NULL
	}

	var discountPriceVal interface{} = discountPrice
	if discountPrice != nil && *discountPrice == 0 {
		discountPriceVal = nil // Clear to NULL (0 means no discount)
	}

	result, err := r.db.Exec(ctx, query, sku, displayNameVal, isBestSeller, markupPercent, discountPriceVal, tags, imageURLVal, descriptionVal)
	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("product not found")
	}

	return nil
}

// AddTag adds a tag to a product
func (r *ProductRepository) AddTag(ctx context.Context, sku, tag string) error {
	query := `
		UPDATE products 
		SET tags = array_append(tags, $2), updated_at = NOW()
		WHERE buyer_sku_code = $1 AND NOT ($2 = ANY(tags))
	`

	_, err := r.db.Exec(ctx, query, sku, tag)
	if err != nil {
		return fmt.Errorf("failed to add tag: %w", err)
	}

	return nil
}

// RemoveTag removes a tag from a product
func (r *ProductRepository) RemoveTag(ctx context.Context, sku, tag string) error {
	query := `
		UPDATE products 
		SET tags = array_remove(tags, $2), updated_at = NOW()
		WHERE buyer_sku_code = $1
	`

	_, err := r.db.Exec(ctx, query, sku, tag)
	if err != nil {
		return fmt.Errorf("failed to remove tag: %w", err)
	}

	return nil
}

// GetByTag retrieves products by tag
func (r *ProductRepository) GetByTag(ctx context.Context, tag string) ([]model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags
		FROM products
		WHERE is_available = true AND $1 = ANY(tags)
		ORDER BY category, brand, product_name
	`

	rows, err := r.db.Query(ctx, query, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// GetAllTags retrieves all unique tags
func (r *ProductRepository) GetAllTags(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT UNNEST(tags) as tag FROM products
		WHERE tags IS NOT NULL AND array_length(tags, 1) > 0
		ORDER BY tag
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// GetBestSellers retrieves best seller products
func (r *ProductRepository) GetBestSellers(ctx context.Context) ([]model.Product, error) {
	query := `
		SELECT id, buyer_sku_code, product_name, category, brand, type, seller_name,
		       buy_price, markup_percent, selling_price, discount_price, is_available,
		       buyer_product_status, seller_product_status, unlimited_stock, stock,
		       description, start_cut_off, end_cut_off, is_multi, last_sync_at, created_at, updated_at,
		       display_name, is_best_seller, tags, image_url
		FROM products
		WHERE is_available = true AND is_best_seller = true
		ORDER BY category, brand, product_name
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.BuyerSKUCode, &p.ProductName, &p.Category, &p.Brand, &p.Type, &p.SellerName,
			&p.BuyPrice, &p.MarkupPercent, &p.SellingPrice, &p.DiscountPrice, &p.IsAvailable,
			&p.BuyerProductStatus, &p.SellerProductStatus, &p.UnlimitedStock, &p.Stock,
			&p.Description, &p.StartCutOff, &p.EndCutOff, &p.IsMulti, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.DisplayName, &p.IsBestSeller, &p.Tags, &p.ImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// UpdateImageURL updates the image URL for a product
func (r *ProductRepository) UpdateImageURL(ctx context.Context, sku string, imageURL *string) error {
	query := `UPDATE products SET image_url = $1, updated_at = NOW() WHERE buyer_sku_code = $2`

	_, err := r.db.Exec(ctx, query, imageURL, sku)
	if err != nil {
		return fmt.Errorf("failed to update image URL: %w", err)
	}

	return nil
}
