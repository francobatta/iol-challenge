// Package token issues and verifies the tokens apps authenticate with.
//
// A token is a JWT signed with HMAC-SHA256 whose subject is the app's ID. It carries
// no expiry and is verified by signature alone, so it stays valid until the signing
// secret changes.
package token

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// A Signer issues and verifies tokens with one secret.
type Signer struct {
	secret []byte
}

// NewSigner returns a Signer that uses secret, which must not be empty.
func NewSigner(secret string) (*Signer, error) {
	if secret == "" {
		return nil, errors.New("token: empty signing secret")
	}
	return &Signer{secret: []byte(secret)}, nil
}

// Issue returns a token for the app.
func (s *Signer) Issue(appID string) (string, error) {
	claims := jwt.RegisteredClaims{Subject: appID}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Verify returns the ID of the app that tok was issued to. It returns an error if tok
// was not issued by a Signer with the same secret.
func (s *Signer) Verify(tok string) (appID string, err error) {
	var claims jwt.RegisteredClaims
	keyFunc := func(*jwt.Token) (any, error) { return s.secret, nil }
	// Pinning the method stops a token signed with another algorithm, or none, from
	// being accepted.
	if _, err := jwt.ParseWithClaims(tok, &claims, keyFunc, jwt.WithValidMethods([]string{"HS256"})); err != nil {
		return "", fmt.Errorf("token: %v", err)
	}
	if claims.Subject == "" {
		return "", errors.New("token: no subject")
	}
	return claims.Subject, nil
}
