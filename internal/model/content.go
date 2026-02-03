package model

import "time"

// ContentType defines the type of homepage content
type ContentType string

const (
	ContentTypeCarousel   ContentType = "carousel"
	ContentTypeBrandImage ContentType = "brand_image"
	ContentTypePopup      ContentType = "popup"
)

// HomepageContent represents a piece of content for the homepage
type HomepageContent struct {
	ID          int64       `json:"id" db:"id"`
	ContentType ContentType `json:"content_type" db:"content_type"`
	BrandName   *string     `json:"brand_name,omitempty" db:"brand_name"`
	ImageURL    string      `json:"image_url" db:"image_url"`
	Title       *string     `json:"title,omitempty" db:"title"`
	Description *string     `json:"description,omitempty" db:"description"`
	LinkURL     *string     `json:"link_url,omitempty" db:"link_url"`
	SortOrder   int         `json:"sort_order" db:"sort_order"`
	IsActive    bool        `json:"is_active" db:"is_active"`
	StartDate   *time.Time  `json:"start_date,omitempty" db:"start_date"`
	EndDate     *time.Time  `json:"end_date,omitempty" db:"end_date"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
}

// CarouselResponse is the response format for carousel items
type CarouselResponse struct {
	ID       int64   `json:"id"`
	ImageURL string  `json:"image_url"`
	Title    *string `json:"title,omitempty"`
	LinkURL  *string `json:"link_url,omitempty"`
}

// BrandImageResponse is the response format for brand images
type BrandImageResponse struct {
	BrandName string `json:"brand_name"`
	ImageURL  string `json:"image_url"`
}

// PopupResponse is the response format for popup
type PopupResponse struct {
	ID          int64   `json:"id"`
	ImageURL    string  `json:"image_url"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	LinkURL     *string `json:"link_url,omitempty"`
}

// BrandSetting represents settings for a specific brand
type BrandSetting struct {
	BrandName      string      `json:"brand_name" db:"brand_name"`
	Slug           string      `json:"slug" db:"slug"`
	CustomImageURL string      `json:"custom_image_url" db:"custom_image_url"`
	IsBestSeller   bool        `json:"is_best_seller" db:"is_best_seller"`
	Status         string      `json:"status" db:"status"` // 'active', 'coming_soon', 'maintenance'
	TopupSteps     []TopupStep `json:"topup_steps" db:"topup_steps"`
	Description    string      `json:"description" db:"description"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at" db:"updated_at"`
}

// TopupStep represents a single step in the topup guide
type TopupStep struct {
	Step  int    `json:"step"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// BrandPublicData represents public brand settings
type BrandPublicData struct {
	BrandName    string      `json:"brand_name"`
	ImageURL     string      `json:"image_url"`
	IsBestSeller bool        `json:"is_best_seller"`
	Status       string      `json:"status"`
	TopupSteps   []TopupStep `json:"topup_steps,omitempty"`
	Description  string      `json:"description,omitempty"`
}
