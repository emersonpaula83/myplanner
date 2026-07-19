package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type OAuthHandler struct {
	oauthSvc *jira.OAuthService
	fdRepo   *repository.FonteDadosRepository
	states   sync.Map
	logger   *zap.Logger
}

type oauthState struct {
	CreatedAt time.Time
	FonteID   *uuid.UUID
}

func NewOAuthHandler(oauthSvc *jira.OAuthService, fdRepo *repository.FonteDadosRepository, logger *zap.Logger) *OAuthHandler {
	return &OAuthHandler{
		oauthSvc: oauthSvc,
		fdRepo:   fdRepo,
		logger:   logger,
	}
}

func (h *OAuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	sd := oauthState{CreatedAt: time.Now()}

	if fonteIDStr := r.URL.Query().Get("fonte_id"); fonteIDStr != "" {
		id, err := uuid.Parse(fonteIDStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "fonte_id inválido")
			return
		}
		sd.FonteID = &id
	}

	h.states.Store(state, sd)
	http.Redirect(w, r, h.oauthSvc.AuthorizeURL(state), http.StatusFound)
}

func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		h.logger.Warn("oauth callback error", zap.String("error", errMsg))
		http.Redirect(w, r, "/?oauth_error="+errMsg, http.StatusFound)
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if state == "" || code == "" {
		respondError(w, http.StatusBadRequest, "state e code são obrigatórios")
		return
	}

	raw, ok := h.states.LoadAndDelete(state)
	if !ok {
		respondError(w, http.StatusBadRequest, "state inválido ou expirado")
		return
	}
	sd := raw.(oauthState)

	if time.Since(sd.CreatedAt) > 10*time.Minute {
		respondError(w, http.StatusBadRequest, "state expirado")
		return
	}

	ctx := r.Context()

	tokens, err := h.oauthSvc.ExchangeCode(ctx, code)
	if err != nil {
		h.logger.Error("oauth code exchange failed", zap.Error(err))
		http.Redirect(w, r, "/?oauth_error=token_exchange_failed", http.StatusFound)
		return
	}

	resources, err := h.oauthSvc.GetAccessibleResources(ctx, tokens.AccessToken)
	if err != nil {
		h.logger.Error("failed to get accessible resources", zap.Error(err))
		http.Redirect(w, r, "/?oauth_error=resources_failed", http.StatusFound)
		return
	}

	if len(resources) == 0 {
		http.Redirect(w, r, "/?oauth_error=no_jira_sites", http.StatusFound)
		return
	}

	site := resources[0]
	baseURL := jira.CloudBaseURL(site.ID)
	expiry := tokens.Expiry()

	if sd.FonteID != nil {
		err = h.fdRepo.SaveOAuthTokens(ctx, *sd.FonteID, baseURL, tokens.AccessToken, tokens.RefreshToken, expiry)
		if err != nil {
			h.logger.Error("failed to save oauth tokens", zap.Error(err))
			http.Redirect(w, r, "/?oauth_error=save_failed", http.StatusFound)
			return
		}
	} else {
		cfm := json.RawMessage(`{}`)
		_, err = h.fdRepo.Create(ctx, &repository.CreateFonteDadosRequest{
			Nome:               site.Name,
			Tipo:               "jira",
			BaseURL:            baseURL,
			AuthType:           "oauth2",
			OAuth2AccessToken:  &tokens.AccessToken,
			OAuth2RefreshToken: &tokens.RefreshToken,
			OAuth2TokenExpiry:  &expiry,
			CustomFieldMap:     cfm,
		})
		if err != nil {
			h.logger.Error("failed to create oauth fonte", zap.Error(err))
			http.Redirect(w, r, "/?oauth_error=create_failed", http.StatusFound)
			return
		}
	}

	h.logger.Info("oauth connected", zap.String("site", site.Name), zap.String("cloud_id", site.ID))
	http.Redirect(w, r, "/?oauth_success=true", http.StatusFound)
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
