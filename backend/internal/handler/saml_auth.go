package handler

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	samlpkg "github.com/emersonpaula83/myplanner/backend/internal/saml"
	"go.uber.org/zap"
)

// SAMLUserStore is the persistence dependency required by SAMLAuthHandler to
// find or create a usuario from a validated SAML assertion.
type SAMLUserStore interface {
	BuscarOuCriarPorEmail(ctx context.Context, email, nomeCompleto, authProvider string) (*domain.Usuario, error)
}

// SAMLAuthHandler exposes the SP-initiated SSO endpoints: Login (starts the
// AuthnRequest), ACS (consumes the IdP response) and Metadata (serves the SP
// metadata XML consumed by the IdP).
type SAMLAuthHandler struct {
	provider     *samlpkg.SAMLProvider
	store        SAMLUserStore
	tokenService *auth.TokenService
	frontendURL  string
	logger       *zap.Logger
}

func NewSAMLAuthHandler(provider *samlpkg.SAMLProvider, store SAMLUserStore, tokenService *auth.TokenService, frontendURL string, logger *zap.Logger) *SAMLAuthHandler {
	// The handler is stateless (no server-side session/cookie tracking pending
	// AuthnRequest IDs), so the SP must accept assertions whose InResponseTo
	// cannot be matched to a request it issued. Signature and time-window
	// validation on the assertion still apply.
	provider.GetServiceProvider().AllowIDPInitiated = true

	return &SAMLAuthHandler{
		provider:     provider,
		store:        store,
		tokenService: tokenService,
		frontendURL:  frontendURL,
		logger:       logger,
	}
}

// Login initiates the SAML authentication flow by redirecting the browser to
// the IdP's SSO endpoint with a signed AuthnRequest (HTTP-Redirect binding).
func (h *SAMLAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	sp := h.provider.GetServiceProvider()

	redirectURL, err := sp.MakeRedirectAuthenticationRequest("")
	if err != nil {
		h.logger.Error("failed to create SAML AuthnRequest", zap.Error(err))
		http.Error(w, "falha ao iniciar SSO", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// ACS is the Assertion Consumer Service endpoint. It validates the SAML
// response posted by the IdP, finds or creates the corresponding usuario,
// issues a JWT and redirects the browser back to the frontend with the token.
func (h *SAMLAuthHandler) ACS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.logger.Error("failed to parse SAML ACS form", zap.Error(err))
		h.redirectWithError(w, r, "falha ao processar resposta SAML")
		return
	}

	sp := h.provider.GetServiceProvider()

	assertion, err := sp.ParseResponse(r, nil)
	if err != nil {
		h.logger.Error("SAML assertion validation failed", zap.Error(err))
		h.redirectWithError(w, r, "autenticação SAML falhou")
		return
	}

	email, nome, err := samlpkg.ExtractEmailFromAssertion(assertion)
	if err != nil {
		h.logger.Error("failed to extract email from SAML assertion", zap.Error(err))
		h.redirectWithError(w, r, "email não encontrado na resposta SAML")
		return
	}

	usuario, err := h.store.BuscarOuCriarPorEmail(r.Context(), email, nome, "saml")
	if err != nil {
		h.logger.Error("failed to find/create usuario from SAML", zap.Error(err), zap.String("email", email))
		h.redirectWithError(w, r, "erro ao criar usuário")
		return
	}

	token, err := h.tokenService.GenerateToken(usuario.ID, usuario.Email, usuario.Cargo)
	if err != nil {
		h.logger.Error("failed to generate JWT after SAML login", zap.Error(err))
		h.redirectWithError(w, r, "erro ao gerar token")
		return
	}

	redirectURL := fmt.Sprintf("%s/#auth_token=%s", h.frontendURL, url.QueryEscape(token))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Metadata serves the SP metadata XML document that the IdP administrator
// imports to configure the trust relationship.
func (h *SAMLAuthHandler) Metadata(w http.ResponseWriter, r *http.Request) {
	sp := h.provider.GetServiceProvider()

	buf, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		h.logger.Error("failed to marshal SAML SP metadata", zap.Error(err))
		http.Error(w, "falha ao gerar metadata", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}

func (h *SAMLAuthHandler) redirectWithError(w http.ResponseWriter, r *http.Request, msg string) {
	redirectURL := fmt.Sprintf("%s/#auth_error=%s", h.frontendURL, url.QueryEscape(msg))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}
