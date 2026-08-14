package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// cargosQuePodemVerSalario são os cargos com acesso a valores salariais. A
// conta admin passa independente do cargo.
var cargosQuePodemVerSalario = map[string]bool{
	"coordenador": true,
	"gerente":     true,
	"diretor":     true,
}

type SalarioLockStore interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
}

type SalarioLockHandler struct {
	store        SalarioLockStore
	tokenService *auth.TokenService
	adminEmail   string
	logger       *zap.Logger
}

func NewSalarioLockHandler(store SalarioLockStore, tokenService *auth.TokenService, adminEmail string, logger *zap.Logger) *SalarioLockHandler {
	return &SalarioLockHandler{store: store, tokenService: tokenService, adminEmail: adminEmail, logger: logger}
}

// Desbloquear troca senha correta por um token com a claim de salários. O token
// novo herda a expiração do atual: destravar não renova a sessão.
func (h *SalarioLockHandler) Desbloquear(w http.ResponseWriter, r *http.Request) {
	email := middleware.UserEmailFromContext(r.Context())
	cargo := middleware.UserCargoFromContext(r.Context())

	if email != h.adminEmail && !cargosQuePodemVerSalario[cargo] {
		respondError(w, http.StatusForbidden, "seu cargo não permite ver valores salariais")
		return
	}

	var req struct {
		Senha string `json:"senha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Senha == "" {
		respondError(w, http.StatusBadRequest, "informe a senha")
		return
	}

	usuario, err := h.store.BuscarPorEmail(r.Context(), email)
	if err != nil {
		h.logger.Error("failed to find usuario for salary unlock", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil || usuario.SenhaHash == nil {
		respondError(w, http.StatusUnauthorized, "senha incorreta")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*usuario.SenhaHash), []byte(req.Senha)); err != nil {
		respondError(w, http.StatusUnauthorized, "senha incorreta")
		return
	}

	h.responderComToken(w, usuario.ID, email, cargo, true, middleware.TokenExpiraEm(r.Context()))
}

// Travar devolve o token sem a claim. Não pede senha: fechar a cortina é sempre
// permitido.
func (h *SalarioLockHandler) Travar(w http.ResponseWriter, r *http.Request) {
	h.responderComToken(w,
		middleware.UserIDFromContext(r.Context()),
		middleware.UserEmailFromContext(r.Context()),
		middleware.UserCargoFromContext(r.Context()),
		false,
		middleware.TokenExpiraEm(r.Context()),
	)
}

func (h *SalarioLockHandler) responderComToken(w http.ResponseWriter, userID uuid.UUID, email, cargo string, salarios bool, expiraEm time.Time) {
	// TokenExpiraEm só devolve o zero-value quando o contexto não carrega uma
	// expiração — hoje inalcançável, pois o AuthJWT sempre grava a expiração de
	// todo token emitido (login, SAML, etc). Mas se algum dia deixar de ser
	// verdade, um zero aqui geraria um token que já nasce expirado: o usuário
	// seria deslogado sem nenhuma mensagem explicando o motivo. Preferimos
	// falhar de forma clara a produzir esse logout silencioso.
	//
	// Usamos 500 e não 401: o frontend costuma ter um interceptor genérico que
	// trata 401 como "sessão expirou, manda pro login". Isso reproduziria
	// exatamente o logout silencioso que estamos evitando. 500 sinaliza o que
	// isso realmente é — uma quebra de invariante do servidor, não uma
	// credencial inválida do cliente.
	if expiraEm.IsZero() {
		h.logger.Error("token de identidade sem expiração no contexto ao gerar token de salários")
		respondError(w, http.StatusInternalServerError, "erro interno: sessão sem expiração")
		return
	}

	token, err := h.tokenService.GenerateTokenComExpiracao(userID, email, cargo, salarios, expiraEm)
	if err != nil {
		h.logger.Error("failed to generate salary token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}
