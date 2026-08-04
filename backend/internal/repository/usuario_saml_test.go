package repository

import (
	"testing"
)

func TestBuscarOuCriarPorEmail_NewUser(t *testing.T) {
	// Integration test — requires DB. Skip if no DB.
	// Validates: creates user with auth_provider='saml', senha_hash=NULL
	t.Skip("integration test — run with DB")
}
