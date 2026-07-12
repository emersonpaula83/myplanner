package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidateToken(t *testing.T) {
	ts := NewTokenService("test-secret-key-minimum-32-chars!!", 24)

	userID := uuid.New()
	email := "test@example.com"
	cargo := "coordenador"

	token, err := ts.GenerateToken(userID, email, cargo)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}

	claims, err := ts.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Errorf("subject = %q, want %q", claims.Subject, userID.String())
	}
	if claims.Email != email {
		t.Errorf("email = %q, want %q", claims.Email, email)
	}
	if claims.Cargo != cargo {
		t.Errorf("cargo = %q, want %q", claims.Cargo, cargo)
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	ts1 := NewTokenService("secret-key-one-32-chars-long!!!!", 24)
	ts2 := NewTokenService("secret-key-two-32-chars-long!!!!", 24)

	token, err := ts1.GenerateToken(uuid.New(), "test@example.com", "gerente")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ts2.ValidateToken(token)
	if err == nil {
		t.Fatal("ValidateToken should fail with wrong secret")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	ts := &TokenService{
		secret:     []byte("test-secret-key-minimum-32-chars!!"),
		expiration: -1 * time.Hour,
	}

	token, err := ts.GenerateToken(uuid.New(), "test@example.com", "gerente")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ts.ValidateToken(token)
	if err == nil {
		t.Fatal("ValidateToken should fail with expired token")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	ts := NewTokenService("test-secret-key-minimum-32-chars!!", 24)

	_, err := ts.ValidateToken("not.a.valid.token")
	if err == nil {
		t.Fatal("ValidateToken should fail with malformed token")
	}
}
