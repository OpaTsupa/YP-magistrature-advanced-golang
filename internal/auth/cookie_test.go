package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestNewCookieAndValidate(t *testing.T) {
	manager, err := NewManager("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	firstID, cookie, err := manager.NewCookie()
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Name != CookieName || !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatalf("unexpected cookie attributes: %+v", cookie)
	}
	id, err := manager.Validate(cookie.Value)
	if err != nil || id != firstID {
		t.Fatalf("valid signature was rejected: id=%q error=%v", id, err)
	}
	secondID, _, err := manager.NewCookie()
	if err != nil || secondID == firstID {
		t.Fatalf("user IDs must be distinct: first=%q second=%q error=%v", firstID, secondID, err)
	}
}

func TestValidateTamperedCookie(t *testing.T) {
	manager, _ := NewManager("test-secret")
	_, cookie, _ := manager.NewCookie()
	segments := strings.Split(cookie.Value, ":")
	if segments[0][0] == 'a' {
		segments[0] = "b" + segments[0][1:]
	} else {
		segments[0] = "a" + segments[0][1:]
	}
	_, err := manager.Validate(strings.Join(segments, ":"))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestValidateMalformedCookie(t *testing.T) {
	manager, _ := NewManager("test-secret")
	for _, value := range []string{"", "abc", ":", "abc:xyz", "not-hex:" + strings.Repeat("a", 64)} {
		_, err := manager.Validate(value)
		if !errors.Is(err, ErrMalformedCookie) {
			t.Errorf("value %q: expected ErrMalformedCookie, got %v", value, err)
		}
	}
}

func TestEmptySecretIsRejected(t *testing.T) {
	if _, err := NewManager(""); err == nil {
		t.Fatal("empty signing secret must be rejected")
	}
}
