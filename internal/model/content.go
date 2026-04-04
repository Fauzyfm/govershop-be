package model

import "time"

// ContentType defines the type of homepage content
type ContentType string

const (
	ContentTypeCarousel   ContentType = "carousel"
	ContentTypeBrandImage ContentType = "brand_image"
	ContentTypePopup      ContentType = "popup"
	ContentTypeBrandPopup ContentType = "brand_popup"
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

// DisplayCategory represents a custom category for frontend display tabs
type DisplayCategory struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	SortOrder int       `json:"sort_order" db:"sort_order"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// InputField represents a configurable input field for a brand's order form.
// Admins can define these via the dashboard to customize what data is collected per game.
type InputField struct {
	Key         string   `json:"key"`                    // Unique identifier (e.g. "user_id", "zone_id", "server")
	Type        string   `json:"type"`                   // "text" or "select"
	Label       string   `json:"label"`                  // Display label (e.g. "User ID", "Server")
	Placeholder string   `json:"placeholder"`            // Input placeholder text
	Required    bool     `json:"required"`               // Whether field is mandatory
	Options     []string `json:"options,omitempty"`       // Options for "select" type
}

// BrandSetting represents settings for a specific brand
type BrandSetting struct {
	BrandName        string       `json:"brand_name" db:"brand_name"`
	Slug             string       `json:"slug" db:"slug"`
	CustomImageURL   string       `json:"custom_image_url" db:"custom_image_url"`
	IsBestSeller     bool         `json:"is_best_seller" db:"is_best_seller"`
	IsVisible        bool         `json:"is_visible" db:"is_visible"`
	Status           string       `json:"status" db:"status"` // 'active', 'coming_soon', 'maintenance'
	TopupSteps       []TopupStep  `json:"topup_steps" db:"topup_steps"`
	Description      string       `json:"description" db:"description"`
	DisplayCategory  *string      `json:"display_category" db:"display_category"`
	DisplaySortOrder int          `json:"display_sort_order" db:"display_sort_order"`
	InputFields      []InputField `json:"input_fields" db:"input_fields"`
	InputSeparator   string       `json:"input_separator" db:"input_separator"`
	CreatedAt        time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at" db:"updated_at"`
}

// TopupStep represents a single step in the topup guide
type TopupStep struct {
	Step  int    `json:"step"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// BrandPublicData represents public brand settings
type BrandPublicData struct {
	BrandName      string       `json:"brand_name"`
	ImageURL       string       `json:"image_url"`
	IsBestSeller   bool         `json:"is_best_seller"`
	IsVisible      bool         `json:"is_visible"`
	Status         string       `json:"status"`
	TopupSteps     []TopupStep  `json:"topup_steps,omitempty"`
	Description    string       `json:"description,omitempty"`
	InputFields    []InputField `json:"input_fields,omitempty"`
	InputSeparator string       `json:"input_separator,omitempty"`
}
