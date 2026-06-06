package jwt

import (
	"fmt"
	"os"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// minJWTSecretBytes is the floor enforced at process startup. 32 bytes is
// enough entropy that brute-force is infeasible for a 256-bit HMAC and matches
// common guidance (e.g. RFC 7518 §3.2 for HS256). Below that we refuse to mint
// tokens at all rather than ship a guessable secret.
const minJWTSecretBytes = 32

type NivekClaims struct {
	UserId int `json:"user_id"`

	jwtlib.RegisteredClaims
}

func (s *TokenService) getClaims(tokenString string) (*NivekClaims, error) {
	token, err := jwtlib.ParseWithClaims(
		tokenString,
		&NivekClaims{},
		func(token *jwtlib.Token) (interface{}, error) {
			return []byte(s.secret), nil
		},
	)

	if err != nil {
		logrus.Errorf("error parsing token: %s", err.Error())
	}

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return token.Claims.(*NivekClaims), nil
}

type TokenService struct {
	secret string
}

func newTokenService(_ nivek.NivekService) *TokenService {
	// Read from env at construction time so callers don't have to thread the
	// secret through. The startup validator in cmd/core-api/main.go guarantees
	// this is set + long enough before any handler runs, but we double-check
	// here so a misconfigured non-prod binary can't accidentally mint tokens
	// signed with an empty key.
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < minJWTSecretBytes {
		logrus.Fatalf("JWT_SECRET must be set and at least %d bytes", minJWTSecretBytes)
	}
	return &TokenService{secret: secret}
}

// ValidateJWTSecret is the startup check. Call it from main() so a missing or
// too-short JWT_SECRET fails the process immediately instead of surfacing as a
// 500 on the first auth request.
func ValidateJWTSecret() error {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < minJWTSecretBytes {
		return fmt.Errorf("JWT_SECRET must be set and at least %d bytes (got %d)", minJWTSecretBytes, len(secret))
	}
	return nil
}

func (s *TokenService) buildToken(
	userID int,
) (
	string,
	error,
) {
	// Create the claims
	claims := NivekClaims{
		UserId: userID,

		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour * 24)), // Expires in 24 hours
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),                     // Issued at
		},
	}

	// Create token with claims
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)

	// Generate encoded token
	tokenString, err := token.SignedString([]byte(s.secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (s *TokenService) validateToken(tokenString string) error {
	claims, err := s.getClaims(tokenString)
	if err != nil {
		return err
	}

	if claims.UserId == 0 {
		return fmt.Errorf("invalid token")
	}

	return nil
}

func (s *TokenService) GetUserId(tokenString string) (int, error) {
	claims, err := s.getClaims(tokenString)
	if err != nil {
		return 0, err
	}

	if claims.UserId == 0 {
		return 0, fmt.Errorf("invalid token")
	}

	return claims.UserId, nil
}
