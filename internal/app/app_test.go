package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shortener/internal/auth"
	"shortener/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	manager, err := auth.NewManager("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	return New(storage.NewMemory(), manager, "http://localhost:8080").Handler()
}

func perform(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func firstCookie(t *testing.T, rr *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	t.Fatal("user_id cookie was not issued")
	return nil
}

func shorten(t *testing.T, handler http.Handler, original string, cookie *http.Cookie) (*httptest.ResponseRecorder, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"url": original})
	if err != nil {
		t.Fatal(err)
	}
	response := perform(handler, http.MethodPost, "/api/shorten", string(body), cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%q", response.Code, response.Body.String())
	}
	var result shortenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Result, "http://localhost:8080/") {
		t.Fatalf("invalid short URL %q", result.Result)
	}
	return response, result.Result
}

func TestShortenRedirectAndUserList(t *testing.T) {
	handler := newTestHandler(t)
	original := "https://example.com/some/long/path?x=1"
	response, shortURL := shorten(t, handler, original, nil)
	cookie := firstCookie(t, response)
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("wrong content type: %q", response.Header().Get("Content-Type"))
	}
	id := strings.TrimPrefix(shortURL, "http://localhost:8080")
	redirect := perform(handler, http.MethodGet, id, "", nil)
	if redirect.Code != http.StatusTemporaryRedirect || redirect.Header().Get("Location") != original {
		t.Fatalf("redirect status=%d Location=%q", redirect.Code, redirect.Header().Get("Location"))
	}
	list := perform(handler, http.MethodGet, "/api/user/urls", "", cookie)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%q", list.Code, list.Body.String())
	}
	var urls []storage.UserURL
	if err := json.Unmarshal(list.Body.Bytes(), &urls); err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 || urls[0].ShortURL != shortURL || urls[0].OriginalURL != original {
		t.Fatalf("unexpected links: %#v", urls)
	}
}

func TestUnknownShortURL(t *testing.T) {
	rr := perform(newTestHandler(t), http.MethodGet, "/not-found", "", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", rr.Code)
	}
}

func TestNewUserWithoutLinks(t *testing.T) {
	rr := perform(newTestHandler(t), http.MethodGet, "/api/user/urls", "", nil)
	if rr.Code != http.StatusNoContent || rr.Body.Len() != 0 {
		t.Fatalf("got status=%d body=%q; want empty 204", rr.Code, rr.Body.String())
	}
	firstCookie(t, rr)
}

func TestMalformedCookieGetsUnauthorized(t *testing.T) {
	handler := newTestHandler(t)
	for _, value := range []string{"", "abc", "abc:invalid"} {
		t.Run(fmt.Sprintf("cookie=%q", value), func(t *testing.T) {
			rr := perform(handler, http.MethodGet, "/api/user/urls", "", &http.Cookie{Name: auth.CookieName, Value: value})
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401", rr.Code)
			}
		})
	}
}

func TestInvalidSignatureGetsNewIdentity(t *testing.T) {
	handler := newTestHandler(t)
	firstResponse, _ := shorten(t, handler, "https://example.com", nil)
	first := firstCookie(t, firstResponse)
	modified := *first
	if modified.Value[0] == 'a' {
		modified.Value = "b" + modified.Value[1:]
	} else {
		modified.Value = "a" + modified.Value[1:]
	}
	response := perform(handler, http.MethodGet, "/api/user/urls", "", &modified)
	if response.Code != http.StatusNoContent {
		t.Fatalf("got %d; want 204", response.Code)
	}
	fresh := firstCookie(t, response)
	if fresh.Value == first.Value {
		t.Fatal("a new identity was not issued")
	}
}

func TestUserIsolation(t *testing.T) {
	handler := newTestHandler(t)
	firstResponse, _ := shorten(t, handler, "https://example.com/user-a", nil)
	alice := firstCookie(t, firstResponse)
	secondResponse, _ := shorten(t, handler, "https://example.com/user-b", nil)
	bob := firstCookie(t, secondResponse)
	shorten(t, handler, "https://example.com/user-a-2", alice)

	for _, test := range []struct {
		cookie   *http.Cookie
		want     int
		contains string
	}{
		{alice, 2, "/user-a"},
		{bob, 1, "/user-b"},
	} {
		rr := perform(handler, http.MethodGet, "/api/user/urls", "", test.cookie)
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d", rr.Code)
		}
		var urls []storage.UserURL
		if err := json.Unmarshal(rr.Body.Bytes(), &urls); err != nil {
			t.Fatal(err)
		}
		if len(urls) != test.want {
			t.Fatalf("got %d links; want %d", len(urls), test.want)
		}
		for _, item := range urls {
			if !strings.Contains(item.OriginalURL, test.contains) {
				t.Fatalf("cross-user data leak: %+v", urls)
			}
		}
	}
}

func TestMalformedCookieOnShortenIsReplaced(t *testing.T) {
	handler := newTestHandler(t)
	body := `{"url":"https://example.com"}`
	rr := perform(handler, http.MethodPost, "/api/shorten", body, &http.Cookie{Name: auth.CookieName, Value: "invalid"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	firstCookie(t, rr)
}

func TestInvalidRequests(t *testing.T) {
	handler := newTestHandler(t)
	for _, body := range []string{
		`{}`, `{"url":""}`, `{"url":"ftp://example.com"}`, `{"url":"example.com"}`,
		`{"url":"http://"}`, `{"url":42}`, `{"url":"https://example.com","extra":true}`,
		`{"url":"https://example.com"} {"url":"https://other.example"}`, `not-json`,
	} {
		rr := perform(handler, http.MethodPost, "/api/shorten", body, nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q: status=%d; want 400", body, rr.Code)
		}
	}
	rr := perform(handler, http.MethodGet, "/api/shorten", "", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on POST endpoint: status=%d; want 405", rr.Code)
	}
}

func TestConcurrentShortening(t *testing.T) {
	handler := newTestHandler(t)
	const count = 40
	var wg sync.WaitGroup
	urls := make(chan string, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"url":"https://example.com/%d"}`, i)
			rr := perform(handler, http.MethodPost, "/api/shorten", body, nil)
			if rr.Code != http.StatusOK {
				t.Errorf("concurrent request: got status %d", rr.Code)
				return
			}
			var resp shortenResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Error(err)
				return
			}
			urls <- resp.Result
		}(i)
	}
	wg.Wait()
	close(urls)
	seen := map[string]struct{}{}
	for url := range urls {
		if _, exists := seen[url]; exists {
			t.Errorf("duplicate short URL: %s", url)
		}
		seen[url] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("got %d unique links; want %d", len(seen), count)
	}
}
