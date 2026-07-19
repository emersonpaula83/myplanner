package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type UsuarioStore interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error)
	ListarTodos(ctx context.Context) ([]domain.Usuario, error)
	Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error)
	Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
	ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error)
	AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error)
	BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
	ValidarProjetosExistem(ctx context.Context, projetoIDs []uuid.UUID) error
}

type AuthHandler struct {
	store        UsuarioStore
	tokenService *auth.TokenService
	logger       *zap.Logger
}

func NewAuthHandler(store UsuarioStore, tokenService *auth.TokenService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		store:        store,
		tokenService: tokenService,
		logger:       logger,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.Email == "" || req.Senha == "" {
		respondError(w, http.StatusBadRequest, "email e senha são obrigatórios")
		return
	}

	usuario, err := h.store.BuscarPorEmail(r.Context(), req.Email)
	if err != nil {
		h.logger.Error("failed to find usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusUnauthorized, "credenciais inválidas")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.Senha)); err != nil {
		respondError(w, http.StatusUnauthorized, "credenciais inválidas")
		return
	}

	token, err := h.tokenService.GenerateToken(usuario.ID, usuario.Email, usuario.Cargo)
	if err != nil {
		h.logger.Error("failed to generate token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}

	respondJSON(w, http.StatusOK, domain.LoginResponse{
		Token:   token,
		Usuario: *usuario,
	})
}
