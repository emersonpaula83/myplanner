package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const equipeIDsKey contextKey = "equipe_ids"

type EquipeIDsFetcher interface {
	BuscarEquipeIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

type AllEquipesFetcher interface {
	ListarTodosIDs(ctx context.Context) ([]uuid.UUID, error)
}

func EquipeFilter(fetcher EquipeIDsFetcher, allFetcher AllEquipesFetcher, adminEmail string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserIDFromContext(r.Context())
			if userID == uuid.Nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "usuário não autenticado"})
				return
			}

			email := UserEmailFromContext(r.Context())

			var ids []uuid.UUID
			var err error

			if email == adminEmail {
				ids, err = allFetcher.ListarTodosIDs(r.Context())
			} else {
				ids, err = fetcher.BuscarEquipeIDsPorUsuario(r.Context(), userID)
			}

			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "falha ao buscar alçada"})
				return
			}

			ctx := context.WithValue(r.Context(), equipeIDsKey, ids)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func EquipeIDsFromContext(ctx context.Context) []uuid.UUID {
	ids, _ := ctx.Value(equipeIDsKey).([]uuid.UUID)
	return ids
}
