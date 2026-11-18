// Package app implements the HTTP handlers of the URL shortener.
package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"shortener/internal/auth"
	"shortener/internal/storage"
)

const maxRequestBytes = 1 << 20

type App struct {
	store   *storage.Memory
	cookies *auth.Manager
	baseURL string
}

func New(store *storage.Memory, cookies *auth.Manager, baseURL string) *App {
	return &App{
		store:   store,
		cookies: cookies,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/shorten", a.shorten)
	mux.HandleFunc("GET /api/user/urls", a.userURLs)
	mux.HandleFunc("GET /{id}", a.redirect)
	return mux
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Result string `json:"result"`
}

func (a *App) shorten(w http.ResponseWriter, r *http.Request) {
	userID, _, err := a.user(w, r, false)
	if err != nil {
		http.Error(w, "cannot issue user cookie", http.StatusInternalServerError)
		return
	}
	var request shortenRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	// Reject trailing JSON values.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "request must contain one JSON object", http.StatusBadRequest)
		return
	}
	originalURL := strings.TrimSpace(request.URL)
	parsedURL, err := url.Parse(originalURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		http.Error(w, "url must be an absolute http or https URL", http.StatusBadRequest)
		return
	}

	for attempts := 0; attempts < 8; attempts++ {
		id, err := newShortID()
		if err != nil {
			http.Error(w, "cannot generate short identifier", http.StatusInternalServerError)
			return
		}
		shortURL := a.baseURL + "/" + id
		if a.store.Save(id, userID, originalURL, shortURL) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if err := json.NewEncoder(w).Encode(shortenResponse{Result: shortURL}); err != nil {
				// The response writer or client connection may have failed.
				return
			}
			return
		}
	}
	http.Error(w, "cannot reserve unique short identifier", http.StatusInternalServerError)
}

func (a *App) redirect(w http.ResponseWriter, r *http.Request) {
	originalURL, ok := a.store.Resolve(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
}

func (a *App) userURLs(w http.ResponseWriter, r *http.Request) {
	userID, malformed, err := a.user(w, r, true)
	if malformed {
		http.Error(w, "cookie has no valid user ID", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "cannot issue user cookie", http.StatusInternalServerError)
		return
	}
	items := a.store.List(userID)
	if len(items) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		return
	}
}

// user reports malformed=true only when a cookie is present but structurally
// invalid. Invalid HMACs instead receive a new identity (per assignment).
func (a *App) user(w http.ResponseWriter, r *http.Request, rejectMalformed bool) (id string, malformed bool, err error) {
	cookie, err := r.Cookie(auth.CookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return a.issueCookie(w)
	}
	if err != nil {
		if rejectMalformed {
			return "", true, nil
		}
		return a.issueCookie(w)
	}
	id, err = a.cookies.Validate(cookie.Value)
	if err == nil {
		return id, false, nil
	}
	if errors.Is(err, auth.ErrMalformedCookie) {
		if rejectMalformed {
			return "", true, nil
		}
		return a.issueCookie(w)
	}
	return a.issueCookie(w)
}

func (a *App) issueCookie(w http.ResponseWriter) (string, bool, error) {
	id, cookie, err := a.cookies.NewCookie()
	if err != nil {
		return "", false, fmt.Errorf("new cookie: %w", err)
	}
	http.SetCookie(w, cookie)
	return id, false, nil
}

func newShortID() (string, error) {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
