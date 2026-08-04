package saml

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/emersonpaula83/myplanner/backend/internal/config"
)

// SAMLProvider wraps a configured crewjam/saml Service Provider, exposing it
// for use by the SAML middleware/handlers (Task 5).
type SAMLProvider struct {
	sp *saml.ServiceProvider
}

// NewSAMLProvider loads the SP certificate/key, fetches the IdP metadata and
// builds a fully configured saml.ServiceProvider.
func NewSAMLProvider(cfg config.SAMLConfig) (*SAMLProvider, error) {
	keyPair, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading SP cert/key: %w", err)
	}

	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parsing SP certificate: %w", err)
	}

	idpMetadataURL, err := url.Parse(cfg.IDPMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("parsing IDP metadata URL: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(
		context.Background(),
		http.DefaultClient,
		*idpMetadataURL,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching IDP metadata: %w", err)
	}

	acsURL, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("parsing ACS URL: %w", err)
	}

	// The ACS URL config value is the full path (scheme+host+path); the
	// MetadataURL must be the SP root (scheme+host only).
	rootURL := *acsURL
	rootURL.Path = ""
	rootURL.RawQuery = ""

	sp := saml.ServiceProvider{
		EntityID:    cfg.EntityID,
		Key:         keyPair.PrivateKey.(*rsa.PrivateKey),
		Certificate: keyPair.Leaf,
		IDPMetadata: idpMetadata,
		AcsURL:      *acsURL,
		MetadataURL: rootURL,
	}

	return &SAMLProvider{sp: &sp}, nil
}

// GetServiceProvider returns the underlying saml.ServiceProvider for use by
// middleware/handlers.
func (p *SAMLProvider) GetServiceProvider() *saml.ServiceProvider {
	return p.sp
}

// ExtractEmailFromAssertion extracts the user's email (from the assertion's
// NameID) and display name (from displayName/cn attributes, falling back to
// the email) from a validated SAML assertion.
func ExtractEmailFromAssertion(assertion *saml.Assertion) (email string, nome string, err error) {
	if assertion == nil || assertion.Subject == nil || assertion.Subject.NameID == nil {
		return "", "", fmt.Errorf("assertion missing NameID")
	}

	email = assertion.Subject.NameID.Value
	if email == "" {
		return "", "", fmt.Errorf("NameID value is empty")
	}

	// Try to get display name from attributes
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			if attr.Name == "displayName" || attr.Name == "cn" || attr.Name == "urn:oid:2.16.840.1.113730.3.1.241" {
				if len(attr.Values) > 0 {
					nome = attr.Values[0].Value
				}
			}
		}
	}

	if nome == "" {
		nome = email
	}

	return email, nome, nil
}
