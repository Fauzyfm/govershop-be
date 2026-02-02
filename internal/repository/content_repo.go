package repository

import (
	"context"
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

// GetActiveBrandImages retrieves active brand images as a map
func (r *ContentRepository) GetActiveBrandImages(ctx context.Context) (map[string]string, error) {
	query := `
		SELECT brand_name, image_url
		FROM homepage_content
		WHERE content_type = 'brand_image' AND is_active = true AND brand_name IS NOT NULL
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query brand images: %w", err)
	}
	defer rows.Close()

	images := make(map[string]string)
	for rows.Next() {
		var brandName, imageURL string
		err := rows.Scan(&brandName, &imageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to scan brand image: %w", err)
		}
		images[brandName] = imageURL
	}

	return images, nil
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
