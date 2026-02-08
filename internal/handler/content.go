package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"govershop-api/internal/model"
	"govershop-api/internal/repository"
)

// ContentHandler handles content management HTTP requests
type ContentHandler struct {
	contentRepo *repository.ContentRepository
}

// NewContentHandler creates a new ContentHandler
func NewContentHandler(contentRepo *repository.ContentRepository) *ContentHandler {
	return &ContentHandler{
		contentRepo: contentRepo,
	}
}

// CreateContentRequest is the request body for creating content
type CreateContentRequest struct {
	ContentType model.ContentType `json:"content_type"`
	BrandName   *string           `json:"brand_name,omitempty"`
	ImageURL    string            `json:"image_url"`
	Title       *string           `json:"title,omitempty"`
	Description *string           `json:"description,omitempty"`
	LinkURL     *string           `json:"link_url,omitempty"`
	SortOrder   int               `json:"sort_order"`
	IsActive    bool              `json:"is_active"`
	StartDate   *string           `json:"start_date,omitempty"` // ISO format
	EndDate     *string           `json:"end_date,omitempty"`   // ISO format
}

// GetAllContent handles GET /api/v1/admin/content
func (h *ContentHandler) GetAllContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contentType := r.URL.Query().Get("type")

	items, err := h.contentRepo.GetAll(ctx, contentType)
	if err != nil {
		InternalError(w, "Gagal mengambil data content")
		return
	}

	if items == nil {
		items = []model.HomepageContent{}
	}

	Success(w, "", map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

// GetContentByID handles GET /api/v1/admin/content/{id}
func (h *ContentHandler) GetContentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, "ID tidak valid")
		return
	}

	item, err := h.contentRepo.GetByID(ctx, id)
	if err != nil {
		NotFound(w, "Content tidak ditemukan")
		return
	}

	Success(w, "", item)
}

// CreateContent handles POST /api/v1/admin/content
func (h *ContentHandler) CreateContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	if req.ContentType == "" || req.ImageURL == "" {
		BadRequest(w, "Content type dan image URL wajib diisi")
		return
	}

	// Parse dates if provided
	var startDate, endDate *time.Time
	if req.StartDate != nil && *req.StartDate != "" {
		t, _ := time.Parse(time.RFC3339, *req.StartDate)
		startDate = &t
	}
	if req.EndDate != nil && *req.EndDate != "" {
		t, _ := time.Parse(time.RFC3339, *req.EndDate)
		endDate = &t
	}

	content := &model.HomepageContent{
		ContentType: req.ContentType,
		BrandName:   req.BrandName,
		ImageURL:    req.ImageURL,
		Title:       req.Title,
		Description: req.Description,
		LinkURL:     req.LinkURL,
		SortOrder:   req.SortOrder,
		IsActive:    req.IsActive,
		StartDate:   startDate,
		EndDate:     endDate,
	}

	if err := h.contentRepo.Create(ctx, content); err != nil {
		InternalError(w, "Gagal membuat content")
		return
	}

	Success(w, "Content berhasil dibuat", content)
}

// UpdateContent handles PUT /api/v1/admin/content/{id}
func (h *ContentHandler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, "ID tidak valid")
		return
	}

	var req CreateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Parse dates if provided
	var startDate, endDate *time.Time
	if req.StartDate != nil && *req.StartDate != "" {
		t, _ := time.Parse(time.RFC3339, *req.StartDate)
		startDate = &t
	}
	if req.EndDate != nil && *req.EndDate != "" {
		t, _ := time.Parse(time.RFC3339, *req.EndDate)
		endDate = &t
	}

	content := &model.HomepageContent{
		ID:          id,
		ContentType: req.ContentType,
		BrandName:   req.BrandName,
		ImageURL:    req.ImageURL,
		Title:       req.Title,
		Description: req.Description,
		LinkURL:     req.LinkURL,
		SortOrder:   req.SortOrder,
		IsActive:    req.IsActive,
		StartDate:   startDate,
		EndDate:     endDate,
	}

	if err := h.contentRepo.Update(ctx, content); err != nil {
		if err.Error() == "content not found" {
			NotFound(w, "Content tidak ditemukan")
			return
		}
		InternalError(w, "Gagal update content")
		return
	}

	Success(w, "Content berhasil diupdate", content)
}

// DeleteContent handles DELETE /api/v1/admin/content/{id}
func (h *ContentHandler) DeleteContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(w, "ID tidak valid")
		return
	}

	if err := h.contentRepo.Delete(ctx, id); err != nil {
		if err.Error() == "content not found" {
			NotFound(w, "Content tidak ditemukan")
			return
		}
		InternalError(w, "Gagal menghapus content")
		return
	}

	Success(w, "Content berhasil dihapus", nil)
}

// --- Public Endpoints ---

// GetCarousel handles GET /api/v1/content/carousel
func (h *ContentHandler) GetCarousel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := h.contentRepo.GetActiveCarousel(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil carousel")
		return
	}

	if items == nil {
		items = []model.CarouselResponse{}
	}

	Success(w, "", map[string]interface{}{
		"carousel": items,
	})
}

// GetBrandImages handles GET /api/v1/content/brands
func (h *ContentHandler) GetBrandImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	images, err := h.contentRepo.GetActiveBrandImages(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil brand images")
		return
	}

	if images == nil {
		images = make(map[string]model.BrandPublicData)
	}

	Success(w, "", map[string]interface{}{
		"brand_images": images,
	})
}

// GetPopup handles GET /api/v1/content/popup
func (h *ContentHandler) GetPopup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	popup, err := h.contentRepo.GetActivePopup(ctx)
	if err != nil || popup == nil {
		// No active popup
		Success(w, "", map[string]interface{}{
			"popup": nil,
		})
		return
	}

	Success(w, "", map[string]interface{}{
		"popup": popup,
	})
}

// GetBrandSettings handles GET /api/v1/admin/brands
func (h *ContentHandler) GetBrandSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := h.contentRepo.GetAllBrandSettings(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil data branding")
		return
	}

	if items == nil {
		items = []model.BrandSetting{}
	}

	Success(w, "", map[string]interface{}{
		"brands": items,
	})
}

// UpdateBrandSettingRequest is the request body for updating brand settings
type UpdateBrandSettingRequest struct {
	BrandName      string            `json:"brand_name"`
	Slug           string            `json:"slug"`
	CustomImageURL string            `json:"custom_image_url"`
	IsBestSeller   bool              `json:"is_best_seller"`
	IsVisible      bool              `json:"is_visible"`
	Status         string            `json:"status"` // 'active', 'coming_soon', 'maintenance'
	TopupSteps     []model.TopupStep `json:"topup_steps"`
	Description    string            `json:"description"`
}

// UpdateBrandSetting handles PUT /api/v1/admin/brands/{brand}
func (h *ContentHandler) UpdateBrandSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	brandName := r.PathValue("brand") // Assuming go 1.22+ router

	if brandName == "" {
		BadRequest(w, "Brand name tidak valid")
		return
	}

	var req UpdateBrandSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Format request tidak valid")
		return
	}

	// Ensure brand name from path is used
	req.BrandName = brandName

	setting := &model.BrandSetting{
		BrandName:      req.BrandName,
		Slug:           req.Slug,
		CustomImageURL: req.CustomImageURL,
		IsBestSeller:   req.IsBestSeller,
		IsVisible:      req.IsVisible,
		Status:         req.Status,
		TopupSteps:     req.TopupSteps,
		Description:    req.Description,
	}

	if err := h.contentRepo.UpsertBrandSetting(ctx, setting); err != nil {
		InternalError(w, "Gagal update brand setting")
		return
	}

	Success(w, "Brand setting berhasil diupdate", setting)
}

// GetPublicBrandSetting handles GET /api/v1/brands/{brand} (Public endpoint)
func (h *ContentHandler) GetPublicBrandSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	brandName := r.PathValue("brand")

	if brandName == "" {
		BadRequest(w, "Brand name tidak valid")
		return
	}

	setting, err := h.contentRepo.GetBrandSetting(ctx, brandName)
	if err != nil {
		// Return empty data if not found
		Success(w, "", map[string]interface{}{
			"brand_name":     brandName,
			"topup_steps":    []model.TopupStep{},
			"description":    "",
			"status":         "active",
			"is_best_seller": false,
			"is_visible":     true,
		})
		return
	}

	Success(w, "", map[string]interface{}{
		"brand_name":     setting.BrandName,
		"topup_steps":    setting.TopupSteps,
		"description":    setting.Description,
		"status":         setting.Status,
		"is_best_seller": setting.IsBestSeller,
		"is_visible":     setting.IsVisible,
		"image_url":      setting.CustomImageURL,
	})
}
