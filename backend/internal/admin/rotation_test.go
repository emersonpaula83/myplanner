package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockAdminStore struct {
	buscarPorEmailFn func(ctx context.Context, email string) (uuid.UUID, error)
	atualizarSenhaFn func(ctx context.Context, id uuid.UUID, senhaHash string) error

	gotEmail     string
	gotID        uuid.UUID
	gotSenhaHash string
}

func (m *mockAdminStore) BuscarPorEmail(ctx context.Context, email string) (uuid.UUID, error) {
	m.gotEmail = email
	if m.buscarPorEmailFn != nil {
		return m.buscarPorEmailFn(ctx, email)
	}
	return uuid.Nil, nil
}

func (m *mockAdminStore) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	m.gotID = id
	m.gotSenhaHash = senhaHash
	if m.atualizarSenhaFn != nil {
		return m.atualizarSenhaFn(ctx, id, senhaHash)
	}
	return nil
}

type mockSecretWriter struct {
	writePasswordFn func(ctx context.Context, password string, expiresAt time.Time) error

	called      bool
	gotPassword string
}

func (m *mockSecretWriter) WritePassword(ctx context.Context, password string, expiresAt time.Time) error {
	m.called = true
	m.gotPassword = password
	if m.writePasswordFn != nil {
		return m.writePasswordFn(ctx, password, expiresAt)
	}
	return nil
}

func TestNewAdminRotator(t *testing.T) {
	store := &mockAdminStore{}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())
	if rotator == nil {
		t.Fatal("expected non-nil rotator")
	}
}

func TestGeneratePassword(t *testing.T) {
	p1, err := generatePassword()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p1) != passwordLength {
		t.Errorf("expected length %d, got %d", passwordLength, len(p1))
	}
	for _, ch := range p1 {
		if !strings.ContainsRune(passwordCharset, ch) {
			t.Errorf("password contains char %q not in charset", ch)
		}
	}

	p2, err := generatePassword()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1 == p2 {
		t.Error("expected two generated passwords to differ")
	}
}

func TestStart_RunsInitialRotationAndStopsOnCancel(t *testing.T) {
	adminID := uuid.New()
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return adminID, nil
		},
	}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rotator.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	if !writer.called {
		t.Error("expected initial RotateNow to have run WritePassword")
	}
}

func TestRotateNow_HappyPath(t *testing.T) {
	adminID := uuid.New()
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return adminID, nil
		},
	}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	err := rotator.RotateNow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.gotEmail != "admin@example.com" {
		t.Errorf("expected BuscarPorEmail called with admin@example.com, got %q", store.gotEmail)
	}
	if store.gotID != adminID {
		t.Errorf("expected AtualizarSenha called with admin ID %s, got %s", adminID, store.gotID)
	}
	if store.gotSenhaHash == "" {
		t.Error("expected non-empty senha hash passed to AtualizarSenha")
	}
	if !writer.called {
		t.Error("expected WritePassword to be called")
	}
	if writer.gotPassword == "" {
		t.Error("expected non-empty password passed to WritePassword")
	}
}

func TestRotateNow_BuscarError(t *testing.T) {
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return uuid.Nil, errors.New("db error")
		},
	}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	err := rotator.RotateNow(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRotateNow_NilID(t *testing.T) {
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return uuid.Nil, nil
		},
	}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	err := rotator.RotateNow(context.Background())
	if err == nil {
		t.Fatal("expected error for nil admin ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}

func TestRotateNow_AtualizarSenhaError(t *testing.T) {
	adminID := uuid.New()
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return adminID, nil
		},
		atualizarSenhaFn: func(ctx context.Context, id uuid.UUID, senhaHash string) error {
			return errors.New("update failed")
		},
	}
	writer := &mockSecretWriter{}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	err := rotator.RotateNow(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if writer.called {
		t.Error("expected WritePassword not to be called when AtualizarSenha fails")
	}
}

func TestRotateNow_WritePasswordError(t *testing.T) {
	adminID := uuid.New()
	store := &mockAdminStore{
		buscarPorEmailFn: func(ctx context.Context, email string) (uuid.UUID, error) {
			return adminID, nil
		},
	}
	writer := &mockSecretWriter{
		writePasswordFn: func(ctx context.Context, password string, expiresAt time.Time) error {
			return errors.New("write failed")
		},
	}
	rotator := NewAdminRotator(store, writer, "admin@example.com", zap.NewNop())

	err := rotator.RotateNow(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

type mockRepo struct {
	buscarPorEmailFn func(ctx context.Context, email string) (*domain.Usuario, error)
	atualizarSenhaFn func(ctx context.Context, id uuid.UUID, senhaHash string) error

	gotID        uuid.UUID
	gotSenhaHash string
}

func (m *mockRepo) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	return m.buscarPorEmailFn(ctx, email)
}

func (m *mockRepo) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	m.gotID = id
	m.gotSenhaHash = senhaHash
	if m.atualizarSenhaFn != nil {
		return m.atualizarSenhaFn(ctx, id, senhaHash)
	}
	return nil
}

func TestRepoAdapter_BuscarPorEmail_Found(t *testing.T) {
	id := uuid.New()
	repo := &mockRepo{
		buscarPorEmailFn: func(ctx context.Context, email string) (*domain.Usuario, error) {
			return &domain.Usuario{ID: id}, nil
		},
	}
	adapter := NewRepoAdapter(repo)

	gotID, err := adapter.BuscarPorEmail(context.Background(), "test@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotID != id {
		t.Errorf("expected ID %s, got %s", id, gotID)
	}
}

func TestRepoAdapter_BuscarPorEmail_NotFound(t *testing.T) {
	repo := &mockRepo{
		buscarPorEmailFn: func(ctx context.Context, email string) (*domain.Usuario, error) {
			return nil, nil
		},
	}
	adapter := NewRepoAdapter(repo)

	gotID, err := adapter.BuscarPorEmail(context.Background(), "missing@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotID != uuid.Nil {
		t.Errorf("expected uuid.Nil, got %s", gotID)
	}
}

func TestRepoAdapter_BuscarPorEmail_Error(t *testing.T) {
	repo := &mockRepo{
		buscarPorEmailFn: func(ctx context.Context, email string) (*domain.Usuario, error) {
			return nil, errors.New("db error")
		},
	}
	adapter := NewRepoAdapter(repo)

	gotID, err := adapter.BuscarPorEmail(context.Background(), "err@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
	if gotID != uuid.Nil {
		t.Errorf("expected uuid.Nil on error, got %s", gotID)
	}
}

func TestRepoAdapter_AtualizarSenha(t *testing.T) {
	id := uuid.New()
	repo := &mockRepo{}
	adapter := NewRepoAdapter(repo)

	err := adapter.AtualizarSenha(context.Background(), id, "hashed-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotID != id || repo.gotSenhaHash != "hashed-value" {
		t.Errorf("expected passthrough of id=%s hash=%q, got id=%s hash=%q", id, "hashed-value", repo.gotID, repo.gotSenhaHash)
	}
}
