package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	userEmailKey contextKey = "user_email"
	userCargoKey contextKey = "user_cargo"
)

func AuthJWT(tokenService *auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				respondUnauthorized(w, "token não fornecido")
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				respondUnauthorized(w, "formato de authorization inválido")
				return
			}

			claims, err := tokenService.ValidateToken(parts[1])
			if err != nil {
				respondUnauthorized(w, "token inválido ou expirado")
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				respondUnauthorized(w, "token inválido")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, userEmailKey, claims.Email)
			ctx = context.WithValue(ctx, userCargoKey, claims.Cargo)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userIDKey).(uuid.UUID)
	return id
}

func UserEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(userEmailKey).(string)
	return email
}

func UserCargoFromContext(ctx context.Context) string {
	cargo, _ := ctx.Value(userCargoKey).(string)
	return cargo
}

func respondUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
