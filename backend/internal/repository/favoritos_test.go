package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestFavoritosRepository_List_Empty(t *testing.T) {
	pool := getTestPool(t)
	repo := NewFavoritosRepository(pool)
	ctx := context.Background()

	keys, err := repo.List(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if keys == nil {
		t.Fatal("List returned nil, expected empty slice")
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestFavoritosRepository_Replace(t *testing.T) {
	pool := getTestPool(t)
	repo := NewFavoritosRepository(pool)
	ctx := context.Background()

	userID := createTestUsuario(t, pool)
	fonteID := createTestFonteDados(t, pool)
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM usuario_projeto_favoritos WHERE usuario_id = $1", userID)
		pool.Exec(ctx, "DELETE FROM usuarios WHERE id = $1", userID)
		pool.Exec(ctx, "DELETE FROM fonte_dados WHERE id = $1", fonteID)
	})

	// Replace with 2 keys
	err := repo.Replace(ctx, userID, fonteID, []string{"TCDV", "PLAT"})
	if err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	keys, _ := repo.List(ctx, userID, fonteID)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Replace with different keys (old ones should be gone)
	err = repo.Replace(ctx, userID, fonteID, []string{"DATA"})
	if err != nil {
		t.Fatalf("second Replace returned error: %v", err)
	}

	keys, _ = repo.List(ctx, userID, fonteID)
	if len(keys) != 1 || keys[0] != "DATA" {
		t.Fatalf("expected [DATA], got %v", keys)
	}

	// Replace with empty clears all
	err = repo.Replace(ctx, userID, fonteID, []string{})
	if err != nil {
		t.Fatalf("empty Replace returned error: %v", err)
	}

	keys, _ = repo.List(ctx, userID, fonteID)
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after clear, got %d", len(keys))
	}
}

func createTestUsuario(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuarios (id, nome_completo, apelido, email, senha_hash, cargo)
		VALUES ($1, 'Test User', $2, $3, 'hash', 'coordenador')
	`, id, "test_"+id.String()[:8], "test_"+id.String()[:8]+"@test.com")
	if err != nil {
		t.Fatalf("creating test usuario: %v", err)
	}
	return id
}

func createTestFonteDados(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fonte_dados (id, nome, tipo, base_url, auth_type)
		VALUES ($1, 'Test Fonte', 'jira', 'https://test.atlassian.net', 'basic')
	`, id)
	if err != nil {
		t.Fatalf("creating test fonte_dados: %v", err)
	}
	return id
}
