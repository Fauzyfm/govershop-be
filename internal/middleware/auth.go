package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"govershop-api/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

type AuthMiddleware struct {
	config *config.Config
}

func NewAuthMiddleware(cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
	}
}

// AdminAuth validates JWT token for admin routes
func (m *AuthMiddleware) AdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenString string

		// Try to get token from Authorization header first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			bearerToken := strings.Split(authHeader, " ")
			if len(bearerToken) == 2 && strings.ToLower(bearerToken[0]) == "bearer" {
				tokenString = bearerToken[1]
			}
		}

		// If no header, check auth_token cookie
		if tokenString == "" {
			cookie, err := r.Cookie("auth_token")
			if err == nil {
				tokenString = cookie.Value
			}
		}

		if tokenString == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(m.config.JWTSecretGovershop), nil
		})

		if err != nil {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			// Check expiration
			if float64(time.Now().Unix()) > claims["exp"].(float64) {
				http.Error(w, "Unauthorized: Token expired", http.StatusUnauthorized)
				return
			}

			// Verify role is admin
			role, ok := claims["role"].(string)
			if !ok || role != "admin" {
				http.Error(w, "Unauthorized: Admin access required", http.StatusUnauthorized)
				return
			}

			// Add user info to context
			userID := int(claims["user_id"].(float64))
			ctx := context.WithValue(r.Context(), "user", claims["sub"])
			ctx = context.WithValue(ctx, "user_id", userID)
			ctx = context.WithValue(ctx, "role", role)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	}
}

// MemberAuth validates JWT token for member routes
func (m *AuthMiddleware) MemberAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenString string
		authHeader := r.Header.Get("Authorization")

		if authHeader != "" {
			bearerToken := strings.Split(authHeader, " ")
			if len(bearerToken) == 2 && strings.ToLower(bearerToken[0]) == "bearer" {
				tokenString = bearerToken[1]
			}
		}

		// If no header, check auth_token cookie first (unified), then member_token (legacy)
		if tokenString == "" {
			cookie, err := r.Cookie("auth_token")
			if err == nil {
				tokenString = cookie.Value
			}
		}
		if tokenString == "" {
			cookie, err := r.Cookie("member_token")
			if err == nil {
				tokenString = cookie.Value
			}
		}

		if tokenString == "" {
			http.Error(w, `{"success":false,"error":"Unauthorized: No token provided"}`, http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(m.config.JWTSecretGovershop), nil
		})

		if err != nil {
			http.Error(w, `{"success":false,"error":"Unauthorized: Invalid token"}`, http.StatusUnauthorized)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			// Check expiration
			if float64(time.Now().Unix()) > claims["exp"].(float64) {
				http.Error(w, `{"success":false,"error":"Unauthorized: Token expired"}`, http.StatusUnauthorized)
				return
			}

			// Verify role is member
			role := claims["role"].(string)
			if role != "member" {
				http.Error(w, `{"success":false,"error":"Unauthorized: Invalid role"}`, http.StatusUnauthorized)
				return
			}

			// Add user info to context
			userID := int(claims["user_id"].(float64))
			ctx := context.WithValue(r.Context(), "user", claims["sub"])
			ctx = context.WithValue(ctx, "user_id", userID)
			ctx = context.WithValue(ctx, "role", role)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, `{"success":false,"error":"Unauthorized"}`, http.StatusUnauthorized)
		}
	}
}
