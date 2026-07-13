package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var cargosValidos = map[string]bool{
	"coordenador":      true,
	"gerente":          true,
	"gerente_projetos": true,
}

type UsuarioHandler struct {
	store  UsuarioStore
	logger *zap.Logger
}

func NewUsuarioHandler(store UsuarioStore, logger *zap.Logger) *UsuarioHandler {
	return &UsuarioHandler{store: store, logger: logger}
}

func (h *UsuarioHandler) List(w http.ResponseWriter, r *http.Request) {
	usuarios, err := h.store.ListarTodos(r.Context())
	if err != nil {
		h.logger.Error("failed to list usuarios", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar usuários")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"usuarios": usuarios})
}

func (h *UsuarioHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CriarUsuarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.NomeCompleto == "" {
		respondError(w, http.StatusBadRequest, "nome_completo é obrigatório")
		return
	}
	if req.Apelido == "" || len(req.Apelido) > 50 {
		respondError(w, http.StatusBadRequest, "apelido é obrigatório (max 50 chars)")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil || req.Email == "" {
		respondError(w, http.StatusBadRequest, "email inválido")
		return
	}
	if len(req.Senha) < 8 {
		respondError(w, http.StatusBadRequest, "senha deve ter no mínimo 8 caracteres")
		return
	}
	if !cargosValidos[req.Cargo] {
		respondError(w, http.StatusBadRequest, "cargo inválido: deve ser coordenador, gerente ou gerente_projetos")
		return
	}

	senhaHash, err := bcrypt.GenerateFromPassword([]byte(req.Senha), 12)
	if err != nil {
		h.logger.Error("failed to hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}

	usuario, err := h.store.Criar(r.Context(), &req, string(senhaHash))
	if err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "email ou apelido já existe")
			return
		}
		h.logger.Error("failed to create usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar usuário")
		return
	}

	respondJSON(w, http.StatusCreated, usuario)
}

func (h *UsuarioHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	usuario, err := h.store.BuscarPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar usuário")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, usuario)
}

func (h *UsuarioHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AtualizarUsuarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.Apelido != nil && (len(*req.Apelido) == 0 || len(*req.Apelido) > 50) {
		respondError(w, http.StatusBadRequest, "apelido inválido (1-50 chars)")
		return
	}
	if req.Email != nil {
		if _, err := mail.ParseAddress(*req.Email); err != nil {
			respondError(w, http.StatusBadRequest, "email inválido")
			return
		}
	}
	if req.Cargo != nil && !cargosValidos[*req.Cargo] {
		respondError(w, http.StatusBadRequest, "cargo inválido: deve ser coordenador, gerente ou gerente_projetos")
		return
	}

	usuario, err := h.store.Atualizar(r.Context(), id, &req)
	if err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "email ou apelido já existe")
			return
		}
		h.logger.Error("failed to update usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar usuário")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, usuario)
}

func (h *UsuarioHandler) AlterarSenha(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AlterarSenhaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if len(req.NovaSenha) < 8 {
		respondError(w, http.StatusBadRequest, "nova senha deve ter no mínimo 8 caracteres")
		return
	}

	usuario, err := h.store.BuscarPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get usuario for password change", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.SenhaAtual)); err != nil {
		respondError(w, http.StatusUnauthorized, "senha atual incorreta")
		return
	}

	novaHash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), 12)
	if err != nil {
		h.logger.Error("failed to hash new password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}

	if err := h.store.AtualizarSenha(r.Context(), id, string(novaHash)); err != nil {
		h.logger.Error("failed to update password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alterar senha")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "senha alterada"})
}

func (h *UsuarioHandler) ListProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	projetos, err := h.store.ListarProjetos(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar projetos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"projetos": projetos})
}

func (h *UsuarioHandler) UpdateProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AlcadaProjetosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if err := h.store.ValidarProjetosExistem(r.Context(), req.ProjetoIDs); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	projetos, err := h.store.AtualizarProjetos(r.Context(), id, req.ProjetoIDs)
	if err != nil {
		h.logger.Error("failed to update projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar projetos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"projetos": projetos})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
