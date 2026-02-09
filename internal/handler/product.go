package handler

import (
	"log"
	"net/http"

	"govershop-api/internal/model"
	"govershop-api/internal/repository"
)

// ProductHandler handles product-related HTTP requests
type ProductHandler struct {
	productRepo *repository.ProductRepository
}

// NewProductHandler creates a new ProductHandler
func NewProductHandler(productRepo *repository.ProductRepository) *ProductHandler {
	return &ProductHandler{
		productRepo: productRepo,
	}
}

// GetProducts handles GET /api/v1/products
func (h *ProductHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get query params for filtering
	category := r.URL.Query().Get("category")
	brand := r.URL.Query().Get("brand")

	var products []interface{}
	var err error

	if category != "" {
		dbProducts, err := h.productRepo.GetByCategory(ctx, category)
		if err != nil {
			InternalError(w, "Gagal mengambil data produk")
			return
		}
		for _, p := range dbProducts {
			products = append(products, p.ToResponse())
		}
	} else if brand != "" {
		dbProducts, err := h.productRepo.GetByBrand(ctx, brand)
		if err != nil {
			InternalError(w, "Gagal mengambil data produk")
			return
		}
		for _, p := range dbProducts {
			products = append(products, p.ToResponse())
		}
	} else {
		dbProducts, err := h.productRepo.GetAll(ctx)
		if err != nil {
			InternalError(w, "Gagal mengambil data produk")
			return
		}
		for _, p := range dbProducts {
			products = append(products, p.ToResponse())
		}
	}

	_ = err // Suppress unused variable warning

	if products == nil {
		products = []interface{}{}
	}

	Success(w, "", map[string]interface{}{
		"products": products,
		"total":    len(products),
	})
}

// GetProductBySKU handles GET /api/v1/products/{sku}
func (h *ProductHandler) GetProductBySKU(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := r.PathValue("sku")

	if sku == "" {
		BadRequest(w, "SKU tidak boleh kosong")
		return
	}

	product, err := h.productRepo.GetBySKU(ctx, sku)
	if err != nil {
		NotFound(w, "Produk tidak ditemukan")
		return
	}

	Success(w, "", product.ToResponse())
}

// GetCategories handles GET /api/v1/products/categories
func (h *ProductHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	categories, err := h.productRepo.GetCategories(ctx)
	if err != nil {
		InternalError(w, "Gagal mengambil data kategori")
		return
	}

	if categories == nil {
		categories = []string{}
	}

	Success(w, "", map[string]interface{}{
		"categories": categories,
	})
}

// GetBrands handles GET /api/v1/products/brands
func (h *ProductHandler) GetBrands(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	category := r.URL.Query().Get("category")

	brands, err := h.productRepo.GetBrands(ctx, category)
	if err != nil {
		InternalError(w, "Gagal mengambil data brand")
		return
	}

	if brands == nil {
		brands = []model.Brand{}
	}

	Success(w, "", map[string]interface{}{
		"brands": brands,
	})
}

// GetFilters handles GET /api/v1/products/filters
func (h *ProductHandler) GetFilters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	categories, err := h.productRepo.GetCategories(ctx)
	if err != nil {
		log.Printf("Error getting categories: %v", err)
		categories = []string{}
	}

	// Get all brands (pass empty category)
	brands, err := h.productRepo.GetBrands(ctx, "")
	if err != nil {
		log.Printf("Error getting brands: %v", err)
		brands = []model.Brand{}
	}

	brandNames := make([]string, len(brands))
	for i, b := range brands {
		brandNames[i] = b.Name
	}

	types, err := h.productRepo.GetUniqueTypes(ctx)
	if err != nil {
		log.Printf("Error getting types: %v", err)
		types = []string{}
	}

	Success(w, "", map[string]interface{}{
		"categories": categories,
		"brands":     brandNames,
		"types":      types,
	})
}
