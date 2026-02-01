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
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 || strings.ToLower(bearerToken[0]) != "bearer" {
			http.Error(w, "Unauthorized: Invalid token format", http.StatusUnauthorized)
			return
		}

		tokenString := bearerToken[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(m.config.JWTSecret), nil
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

			// Add username to context
			ctx := context.WithValue(r.Context(), "user", claims["sub"])
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	}
}
