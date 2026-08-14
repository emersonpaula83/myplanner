package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	Email string `json:"email"`
	Cargo string `json:"cargo"`
	// Salarios diz se este token pode ver valores salariais. Fica no token
	// porque o desbloqueio dura o que a sessão durar — sem estado no servidor.
	Salarios bool `json:"salarios,omitempty"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret     []byte
	expiration time.Duration
}

func NewTokenService(secret string, expirationHours int) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		expiration: time.Duration(expirationHours) * time.Hour,
	}
}

func (ts *TokenService) GenerateToken(userID uuid.UUID, email, cargo string) (string, error) {
	now := time.Now()
	claims := Claims{
		Email: email,
		Cargo: cargo,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ts.expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ts.secret)
}

// GenerateTokenComExpiracao emite um token com expiração ditada por quem chama.
// É o que permite destravar salários sem renovar a sessão: o token novo herda o
// prazo que restava do antigo.
func (ts *TokenService) GenerateTokenComExpiracao(userID uuid.UUID, email, cargo string, salarios bool, expiraEm time.Time) (string, error) {
	claims := Claims{
		Email:    email,
		Cargo:    cargo,
		Salarios: salarios,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiraEm),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ts.secret)
}

func (ts *TokenService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return ts.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
