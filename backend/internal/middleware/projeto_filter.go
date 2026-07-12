package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const projetoIDsKey contextKey = "projeto_ids"

type ProjetoIDsFetcher interface {
	BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

func ProjetoFilter(fetcher ProjetoIDsFetcher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserIDFromContext(r.Context())
			if userID == uuid.Nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "usuário não autenticado"})
				return
			}

			ids, err := fetcher.BuscarProjetoIDsPorUsuario(r.Context(), userID)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "falha ao buscar alçada"})
				return
			}

			ctx := context.WithValue(r.Context(), projetoIDsKey, ids)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ProjetoIDsFromContext(ctx context.Context) []uuid.UUID {
	ids, _ := ctx.Value(projetoIDsKey).([]uuid.UUID)
	return ids
}
