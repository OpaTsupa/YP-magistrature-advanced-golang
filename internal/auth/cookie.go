// Package auth issues and validates HMAC-signed user cookies.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const CookieName = "user_id"

var (
	ErrMalformedCookie  = errors.New("malformed user cookie")
	ErrInvalidSignature = errors.New("invalid cookie signature")
)

type Manager struct {
	secret []byte
}

func NewManager(secret string) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("cookie signing secret must not be empty")
	}
	return &Manager{secret: []byte(secret)}, nil
}

// NewCookie generates a fresh random user ID and a signed cookie.
func (m *Manager) NewCookie() (string, *http.Cookie, error) {
	randomID := make([]byte, 16)
	if _, err := rand.Read(randomID); err != nil {
		return "", nil, fmt.Errorf("generate user ID: %w", err)
	}
	userID := hex.EncodeToString(randomID)
	value := userID + ":" + hex.EncodeToString(m.sign(userID))
	return userID, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// Validate distinguishes malformed values from correctly structured,
// but tampered, signed cookies.
func (m *Manager) Validate(value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) != 64 {
		return "", ErrMalformedCookie
	}
	idBytes, errID := hex.DecodeString(parts[0])
	actualSignature, errSignature := hex.DecodeString(parts[1])
	if errID != nil || len(idBytes) != 16 || errSignature != nil || len(actualSignature) != sha256.Size {
		return "", ErrMalformedCookie
	}
	expectedSignature := m.sign(parts[0])
	if !hmac.Equal(expectedSignature, actualSignature) {
		return "", ErrInvalidSignature
	}
	return parts[0], nil
}

func (m *Manager) sign(userID string) []byte {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte(userID))
	return h.Sum(nil)
}
