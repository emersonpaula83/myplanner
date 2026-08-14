package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"go.uber.org/zap"
)

type ImportStore interface {
	MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (csvContent, id, gid string, err error)
	ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error)
	GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error)
	Sync(ctx context.Context) (*domain.ImportMatchResult, error)
}

type ImportHandler struct {
	store  ImportStore
	logger *zap.Logger
}

func NewImportHandler(store ImportStore, logger *zap.Logger) *ImportHandler {
	return &ImportHandler{store: store, logger: logger}
}

func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	var csvContent string

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			respondError(w, http.StatusBadRequest, "arquivo inválido")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			respondError(w, http.StatusBadRequest, "arquivo não enviado")
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "falha ao ler arquivo")
			return
		}
		csvContent = string(body)
	} else {
		var req struct {
			SheetsURL string `json:"sheets_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SheetsURL == "" {
			respondError(w, http.StatusBadRequest, "informe sheets_url ou envie um arquivo CSV")
			return
		}
		content, _, _, err := h.store.FetchGoogleSheetCSV(r.Context(), req.SheetsURL)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		csvContent = content
	}

	result, err := h.store.MatchPlanilha(r.Context(), csvContent)
	if err != nil {
		h.logger.Error("failed to match planilha", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !middleware.PodeVerSalarios(r.Context()) {
		limparSalariosDoPreview(result)
	}
	respondJSON(w, http.StatusOK, result)
}

// limparSalariosDoPreview tira valor salarial do preview do import, inclusive o
// marcador "salario" em changes — ele sozinho já denunciaria que o valor mudou.
func limparSalariosDoPreview(result *domain.ImportMatchResult) {
	for i := range result.Matched {
		result.Matched[i].Dados.Salario = nil
		result.Matched[i].Changes = semSalario(result.Matched[i].Changes)
	}
	for i := range result.UnmatchedMembros {
		result.UnmatchedMembros[i].Dados.Salario = nil
	}
}

func semSalario(changes []string) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		if c != "salario" {
			out = append(out, c)
		}
	}
	return out
}

func (h *ImportHandler) Confirmar(w http.ResponseWriter, r *http.Request) {
	// Alterar salário sem poder vê-lo seria alterar às cegas — e seria o
	// caminho aberto para quem monta a requisição na mão.
	if !middleware.PodeVerSalarios(r.Context()) {
		respondError(w, http.StatusForbidden, "destrave os valores salariais para alterar salário")
		return
	}

	var req domain.ConfirmImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if len(req.Linhas) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma linha para importar")
		return
	}

	resp, err := h.store.ConfirmImport(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to confirm import", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao confirmar importação")
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *ImportHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetSyncConfig(r.Context())
	if err != nil {
		h.logger.Error("failed to get import config", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar configuração de sync")
		return
	}
	respondJSON(w, http.StatusOK, cfg)
}

func (h *ImportHandler) Sync(w http.ResponseWriter, r *http.Request) {
	result, err := h.store.Sync(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !middleware.PodeVerSalarios(r.Context()) {
		limparSalariosDoPreview(result)
	}
	respondJSON(w, http.StatusOK, result)
}
