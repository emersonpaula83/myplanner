package middleware

import (
	"context"
	"encoding/json"
	"fmt"
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

// ContextWithEquipeIDs returns a new context carrying the given equipe IDs,
// as if injected by EquipeFilter. Intended for tests that exercise handlers
// calling ValidateEquipeAccess without going through the full middleware chain.
func ContextWithEquipeIDs(ctx context.Context, ids []uuid.UUID) context.Context {
	return context.WithValue(ctx, equipeIDsKey, ids)
}

// ValidateEquipeAccess checks that every requested equipe ID is within the
// caller's alçada (the set of equipe IDs injected into the context by
// EquipeFilter). It returns an error if the caller has no alçada configured
// or if any requested ID is not allowed.
func ValidateEquipeAccess(ctx context.Context, requestedIDs []uuid.UUID) error {
	allowed := EquipeIDsFromContext(ctx)
	if len(allowed) == 0 {
		return fmt.Errorf("sem alçada configurada")
	}
	allowedSet := make(map[uuid.UUID]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	for _, id := range requestedIDs {
		if !allowedSet[id] {
			return fmt.Errorf("acesso negado à equipe %s", id)
		}
	}
	return nil
}
