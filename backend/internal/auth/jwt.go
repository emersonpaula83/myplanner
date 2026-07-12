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
