package admin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*"
const passwordLength = 16

type AdminPasswordStore interface {
	BuscarPorEmail(ctx context.Context, email string) (id uuid.UUID, err error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
}

type SecretWriter interface {
	WritePassword(ctx context.Context, password string, expiresAt time.Time) error
}

type AdminRotator struct {
	store      AdminPasswordStore
	writer     SecretWriter
	adminEmail string
	logger     *zap.Logger
}

func NewAdminRotator(store AdminPasswordStore, writer SecretWriter, adminEmail string, logger *zap.Logger) *AdminRotator {
	return &AdminRotator{
		store:      store,
		writer:     writer,
		adminEmail: adminEmail,
		logger:     logger,
	}
}

func (ar *AdminRotator) Start(ctx context.Context) {
	if err := ar.RotateNow(ctx); err != nil {
		ar.logger.Error("initial admin password rotation failed", zap.Error(err))
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ar.RotateNow(ctx); err != nil {
				ar.logger.Error("admin password rotation failed", zap.Error(err))
			}
		}
	}
}

func (ar *AdminRotator) RotateNow(ctx context.Context) error {
	password, err := generatePassword()
	if err != nil {
		return fmt.Errorf("generating password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	adminID, err := ar.store.BuscarPorEmail(ctx, ar.adminEmail)
	if err != nil {
		return fmt.Errorf("finding admin user: %w", err)
	}
	if adminID == uuid.Nil {
		return fmt.Errorf("admin user %s not found", ar.adminEmail)
	}

	if err := ar.store.AtualizarSenha(ctx, adminID, string(hash)); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := ar.writer.WritePassword(ctx, password, expiresAt); err != nil {
		return fmt.Errorf("writing password to secret store: %w", err)
	}

	ar.logger.Info("admin password rotated successfully")
	return nil
}

func generatePassword() (string, error) {
	b := make([]byte, passwordLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordCharset))))
		if err != nil {
			return "", err
		}
		b[i] = passwordCharset[n.Int64()]
	}
	return string(b), nil
}

// RepoAdapter adapts a *domain.Usuario-returning repository (such as
// *repository.UsuarioRepository) to satisfy AdminPasswordStore, which only
// needs the user's ID.
type RepoAdapter struct {
	repo interface {
		BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
		AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
	}
}

func NewRepoAdapter(repo interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
}) *RepoAdapter {
	return &RepoAdapter{repo: repo}
}

func (a *RepoAdapter) BuscarPorEmail(ctx context.Context, email string) (uuid.UUID, error) {
	u, err := a.repo.BuscarPorEmail(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, nil
	}
	return u.ID, nil
}

func (a *RepoAdapter) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	return a.repo.AtualizarSenha(ctx, id, senhaHash)
}
