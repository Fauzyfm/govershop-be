package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"govershop-api/internal/model"
)

// ContentRepository handles database operations for homepage content
type ContentRepository struct {
	db *pgxpool.Pool
}

// NewContentRepository creates a new ContentRepository
func NewContentRepository(db *pgxpool.Pool) *ContentRepository {
	return &ContentRepository{db: db}
}

// GetAll retrieves all content items for admin
func (r *ContentRepository) GetAll(ctx context.Context, contentType string) ([]model.HomepageContent, error) {
	query := `
		SELECT id, content_type, brand_name, image_url, title, description, link_url,
		       sort_order, is_active, start_date, end_date, created_at, updated_at
		FROM homepage_content
	`
	var args []interface{}

	if contentType != "" && contentType != "all" {
		query += " WHERE content_type = $1"
		args = append(args, contentType)
	}

	query += " ORDER BY content_type, sort_order ASC, created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query content: %w", err)
	}
	defer rows.Close()

	var items []model.HomepageContent
	for rows.Next() {
		var c model.HomepageContent
		err := rows.Scan(
			&c.ID, &c.ContentType, &c.BrandName, &c.ImageURL, &c.Title, &c.Description, &c.LinkURL,
			&c.SortOrder, &c.IsActive, &c.StartDate, &c.EndDate, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan content: %w", err)
		}
		items = append(items, c)
	}

	return items, nil
}

// GetByID retrieves a content item by ID
func (r *ContentRepository) GetByID(ctx context.Context, id int64) (*model.HomepageContent, error) {
	query := `
		SELECT id, content_type, brand_name, image_url, title, description, link_url,
		       sort_order, is_active, start_date, end_date, created_at, updated_at
		FROM homepage_content
		WHERE id = $1
	`

	var c model.HomepageContent
	err := r.db.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.ContentType, &c.BrandName, &c.ImageURL, &c.Title, &c.Description, &c.LinkURL,
		&c.SortOrder, &c.IsActive, &c.StartDate, &c.EndDate, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get content: %w", err)
	}

	return &c, nil
}

// Create creates a new content item
func (r *ContentRepository) Create(ctx context.Context, c *model.HomepageContent) error {
	query := `
		INSERT INTO homepage_content (
			content_type, brand_name, image_url, title, description, link_url,
			sort_order, is_active, start_date, end_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		c.ContentType, c.BrandName, c.ImageURL, c.Title, c.Description, c.LinkURL,
		c.SortOrder, c.IsActive, c.StartDate, c.EndDate,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create content: %w", err)
	}

	return nil
}

// Update updates a content item
func (r *ContentRepository) Update(ctx context.Context, c *model.HomepageContent) error {
	query := `
		UPDATE homepage_content SET
			content_type = $2, brand_name = $3, image_url = $4, title = $5,
			description = $6, link_url = $7, sort_order = $8, is_active = $9,
			start_date = $10, end_date = $11, updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query,
		c.ID, c.ContentType, c.BrandName, c.ImageURL, c.Title, c.Description, c.LinkURL,
		c.SortOrder, c.IsActive, c.StartDate, c.EndDate,
	)
	if err != nil {
		return fmt.Errorf("failed to update content: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("content not found")
	}

	return nil
}

// Delete deletes a content item
func (r *ContentRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM homepage_content WHERE id = $1`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete content: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("content not found")
	}

	return nil
}

// GetActiveCarousel retrieves active carousel items
func (r *ContentRepository) GetActiveCarousel(ctx context.Context) ([]model.CarouselResponse, error) {
	query := `
		SELECT id, image_url, title, link_url
		FROM homepage_content
		WHERE content_type = 'carousel' AND is_active = true
		ORDER BY sort_order ASC
		LIMIT 5
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query carousel: %w", err)
	}
	defer rows.Close()

	var items []model.CarouselResponse
	for rows.Next() {
		var c model.CarouselResponse
		err := rows.Scan(&c.ID, &c.ImageURL, &c.Title, &c.LinkURL)
		if err != nil {
			return nil, fmt.Errorf("failed to scan carousel: %w", err)
		}
		items = append(items, c)
	}

	return items, nil
}

// GetActiveBrandImages retrieves public brand data (images + settings)
// Returns all brands including hidden ones - frontend will filter by is_visible
func (r *ContentRepository) GetActiveBrandImages(ctx context.Context) (map[string]model.BrandPublicData, error) {
	// We want to combine data from:
	// 1. homepage_content (for custom images uploaded via 'Game Card Images')
	// 2. brand_settings (for status, best_seller, and visibility flags)
	// We prioritize the image from homepage_content if it exists.
	// Return all brands so frontend can filter by is_visible
	query := `
		SELECT 
			COALESCE(hc.brand_name, bs.brand_name) as brand_name,
			COALESCE(hc.image_url, bs.custom_image_url, '') as image_url,
			COALESCE(bs.is_best_seller, false) as is_best_seller,
			COALESCE(bs.is_visible, true) as is_visible,
			COALESCE(bs.status, 'active') as status
		FROM brand_settings bs
		FULL OUTER JOIN (
			SELECT brand_name, image_url 
			FROM homepage_content 
			WHERE content_type = 'brand_image' AND is_active = true
		) hc ON bs.brand_name = hc.brand_name
		WHERE hc.image_url IS NOT NULL 
		   OR bs.is_best_seller = true 
		   OR bs.status != 'active'
		   OR bs.custom_image_url != ''
		   OR bs.is_visible = false
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query brand data: %w", err)
	}
	defer rows.Close()

	data := make(map[string]model.BrandPublicData)
	for rows.Next() {
		var b model.BrandPublicData
		err := rows.Scan(&b.BrandName, &b.ImageURL, &b.IsBestSeller, &b.IsVisible, &b.Status)
		if err != nil {
			return nil, fmt.Errorf("failed to scan brand data: %w", err)
		}
		data[b.BrandName] = b
	}

	return data, nil
}

// GetActivePopup retrieves the active popup (within date range)
func (r *ContentRepository) GetActivePopup(ctx context.Context) (*model.PopupResponse, error) {
	now := time.Now()
	query := `
		SELECT id, image_url, title, description, link_url
		FROM homepage_content
		WHERE content_type = 'popup' AND is_active = true
		  AND (start_date IS NULL OR start_date <= $1)
		  AND (end_date IS NULL OR end_date >= $1)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p model.PopupResponse
	err := r.db.QueryRow(ctx, query, now).Scan(&p.ID, &p.ImageURL, &p.Title, &p.Description, &p.LinkURL)
	if err != nil {
		return nil, nil // No active popup
	}

	return &p, nil
}

// GetBrandPopup retrieves the active brand popup for a specific brand
func (r *ContentRepository) GetBrandPopup(ctx context.Context, brandName string) (*model.PopupResponse, error) {
	now := time.Now()
	query := `
		SELECT id, image_url, title, description, link_url
		FROM homepage_content
		WHERE content_type = 'brand_popup' AND is_active = true
		  AND brand_name = $1
		  AND (start_date IS NULL OR start_date <= $2)
		  AND (end_date IS NULL OR end_date >= $2)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p model.PopupResponse
	err := r.db.QueryRow(ctx, query, brandName, now).Scan(&p.ID, &p.ImageURL, &p.Title, &p.Description, &p.LinkURL)
	if err != nil {
		return nil, nil // No active brand popup
	}

	return &p, nil
}

// GetAllBrandSettings retrieves all brand settings
func (r *ContentRepository) GetAllBrandSettings(ctx context.Context) ([]model.BrandSetting, error) {
	query := `
		SELECT brand_name, slug, custom_image_url, is_best_seller, COALESCE(is_visible, true) as is_visible, status, 
		       COALESCE(topup_steps, '[]'::jsonb) as topup_steps, 
		       COALESCE(description, '') as description,
		       display_category, COALESCE(display_sort_order, 0) as display_sort_order,
		       created_at, updated_at
		FROM brand_settings
		ORDER BY brand_name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query brand settings: %w", err)
	}
	defer rows.Close()

	var items []model.BrandSetting
	for rows.Next() {
		var b model.BrandSetting
		var topupStepsJSON []byte
		err := rows.Scan(
			&b.BrandName, &b.Slug, &b.CustomImageURL, &b.IsBestSeller, &b.IsVisible, &b.Status,
			&topupStepsJSON, &b.Description,
			&b.DisplayCategory, &b.DisplaySortOrder,
			&b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan brand setting: %w", err)
		}
		// Parse JSONB topup_steps
		if len(topupStepsJSON) > 0 {
			if err := json.Unmarshal(topupStepsJSON, &b.TopupSteps); err != nil {
				b.TopupSteps = []model.TopupStep{} // Default empty if parse fails
			}
		}
		items = append(items, b)
	}

	return items, nil
}

// UpsertBrandSetting updates or inserts brand setting
func (r *ContentRepository) UpsertBrandSetting(ctx context.Context, bs *model.BrandSetting) error {
	// Marshal topup_steps to JSON
	topupStepsJSON, err := json.Marshal(bs.TopupSteps)
	if err != nil {
		topupStepsJSON = []byte("[]")
	}

	query := `
		INSERT INTO brand_settings (brand_name, slug, custom_image_url, is_best_seller, is_visible, status, topup_steps, description, display_category, display_sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (brand_name) DO UPDATE SET
		slug = EXCLUDED.slug,
		custom_image_url = EXCLUDED.custom_image_url,
		is_best_seller = EXCLUDED.is_best_seller,
		is_visible = EXCLUDED.is_visible,
		status = EXCLUDED.status,
		topup_steps = EXCLUDED.topup_steps,
		description = EXCLUDED.description,
		display_category = EXCLUDED.display_category,
		display_sort_order = EXCLUDED.display_sort_order,
		updated_at = NOW()
	`

	_, err = r.db.Exec(ctx, query,
		bs.BrandName, bs.Slug, bs.CustomImageURL, bs.IsBestSeller, bs.IsVisible, bs.Status, topupStepsJSON, bs.Description,
		bs.DisplayCategory, bs.DisplaySortOrder,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert brand setting: %w", err)
	}

	return nil
}

// GetBrandSetting retrieves a specific brand setting
func (r *ContentRepository) GetBrandSetting(ctx context.Context, brandName string) (*model.BrandSetting, error) {
	query := `
		SELECT brand_name, slug, custom_image_url, is_best_seller, COALESCE(is_visible, true) as is_visible, status,
		       COALESCE(topup_steps, '[]'::jsonb) as topup_steps,
		       COALESCE(description, '') as description,
		       display_category, COALESCE(display_sort_order, 0) as display_sort_order,
		       created_at, updated_at
		FROM brand_settings
		WHERE brand_name = $1
	`

	var b model.BrandSetting
	var topupStepsJSON []byte
	err := r.db.QueryRow(ctx, query, brandName).Scan(
		&b.BrandName, &b.Slug, &b.CustomImageURL, &b.IsBestSeller, &b.IsVisible, &b.Status,
		&topupStepsJSON, &b.Description,
		&b.DisplayCategory, &b.DisplaySortOrder,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get brand setting: %w", err)
	}

	// Parse JSONB topup_steps
	if len(topupStepsJSON) > 0 {
		if err := json.Unmarshal(topupStepsJSON, &b.TopupSteps); err != nil {
			b.TopupSteps = []model.TopupStep{}
		}
	}

	return &b, nil
}

// ==========================================
// DISPLAY CATEGORIES METHODS
// ==========================================

// GetAllDisplayCategories retrieves all display categories (for admin)
func (r *ContentRepository) GetAllDisplayCategories(ctx context.Context) ([]model.DisplayCategory, error) {
	query := `
		SELECT id, name, slug, sort_order, is_active, created_at, updated_at
		FROM display_categories
		ORDER BY sort_order ASC, name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query display categories: %w", err)
	}
	defer rows.Close()

	var items []model.DisplayCategory
	for rows.Next() {
		var c model.DisplayCategory
		err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.SortOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan display category: %w", err)
		}
		items = append(items, c)
	}

	return items, nil
}

// CreateDisplayCategory creates a new display category
func (r *ContentRepository) CreateDisplayCategory(ctx context.Context, c *model.DisplayCategory) error {
	query := `
		INSERT INTO display_categories (name, slug, sort_order, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query, c.Name, c.Slug, c.SortOrder, c.IsActive).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create display category: %w", err)
	}

	return nil
}

// UpdateDisplayCategory updates a display category
func (r *ContentRepository) UpdateDisplayCategory(ctx context.Context, c *model.DisplayCategory) error {
	query := `
		UPDATE display_categories SET
			name = $2, slug = $3, sort_order = $4, is_active = $5, updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, c.ID, c.Name, c.Slug, c.SortOrder, c.IsActive)
	if err != nil {
		return fmt.Errorf("failed to update display category: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("display category not found")
	}

	return nil
}

// DeleteDisplayCategory deletes a display category and clears brand assignments
func (r *ContentRepository) DeleteDisplayCategory(ctx context.Context, id int64) error {
	// First get the category name to clear brand assignments
	var name string
	err := r.db.QueryRow(ctx, `SELECT name FROM display_categories WHERE id = $1`, id).Scan(&name)
	if err != nil {
		return fmt.Errorf("display category not found")
	}

	// Clear brand assignments for this category
	_, _ = r.db.Exec(ctx, `UPDATE brand_settings SET display_category = NULL WHERE display_category = $1`, name)

	// Delete the category
	result, err := r.db.Exec(ctx, `DELETE FROM display_categories WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete display category: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("display category not found")
	}

	return nil
}

// UpdateDisplayCategorySortOrder updates the sort order of multiple categories
func (r *ContentRepository) UpdateDisplayCategorySortOrder(ctx context.Context, ids []int64) error {
	for i, id := range ids {
		_, err := r.db.Exec(ctx, `UPDATE display_categories SET sort_order = $1, updated_at = NOW() WHERE id = $2`, i, id)
		if err != nil {
			return fmt.Errorf("failed to update sort order: %w", err)
		}
	}
	return nil
}

// GetDisplayCategoriesWithBrands retrieves active categories with their brands (for public frontend)
func (r *ContentRepository) GetDisplayCategoriesWithBrands(ctx context.Context) ([]model.DisplayCategory, map[string][]string, error) {
	// Get active categories
	catQuery := `
		SELECT id, name, slug, sort_order, is_active, created_at, updated_at
		FROM display_categories
		WHERE is_active = true
		ORDER BY sort_order ASC, name ASC
	`

	rows, err := r.db.Query(ctx, catQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query display categories: %w", err)
	}
	defer rows.Close()

	var categories []model.DisplayCategory
	for rows.Next() {
		var c model.DisplayCategory
		err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.SortOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan display category: %w", err)
		}
		categories = append(categories, c)
	}

	// Get brands assigned to categories, ordered by display_sort_order then alphabetically
	brandQuery := `
		SELECT display_category, brand_name
		FROM brand_settings
		WHERE display_category IS NOT NULL
		  AND COALESCE(is_visible, true) = true
		ORDER BY display_category, display_sort_order ASC, brand_name ASC
	`

	brandRows, err := r.db.Query(ctx, brandQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query brands by category: %w", err)
	}
	defer brandRows.Close()

	categoryBrands := make(map[string][]string)
	for brandRows.Next() {
		var cat, brand string
		if err := brandRows.Scan(&cat, &brand); err != nil {
			return nil, nil, fmt.Errorf("failed to scan brand: %w", err)
		}
		categoryBrands[cat] = append(categoryBrands[cat], brand)
	}

	return categories, categoryBrands, nil
}
