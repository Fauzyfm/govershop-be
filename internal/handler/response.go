package handler

import (
	"encoding/json"
	"net/http"
)

// Response is a standard JSON response structure
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// PaginatedResponse is a paginated JSON response structure
type PaginatedResponse struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination contains pagination info
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// JSON sends a JSON response
func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// Success sends a success response
func Success(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusOK, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Created sends a 201 created response
func Created(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusCreated, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Error sends an error response
func Error(w http.ResponseWriter, statusCode int, message string) {
	JSON(w, statusCode, Response{
		Success: false,
		Error:   message,
	})
}

// BadRequest sends a 400 bad request response
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, message)
}

// NotFound sends a 404 not found response
func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, message)
}

// InternalError sends a 500 internal server error response
func InternalError(w http.ResponseWriter, message string) {
	Error(w, http.StatusInternalServerError, message)
}

// Unauthorized sends a 401 unauthorized response
func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, message)
}
