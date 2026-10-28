package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A deployment that protects operator writes must refuse one without a key, and
// accept one with it. Reads stay public.
func TestProtectedOperatorWritesRequireAnAPIKey(t *testing.T) {
	protected := testServer(t, WithAPIKeys(testKeyService(t)), WithOperatorWritesProtected(true))

	body := `{"name":"Unit 501","registration":"B-9501-JKT","type":"van"}`
	if rec := postJSONAs(t, protected, "/api/vehicles", body, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("write without a key answered %d, want 401: %s", rec.Code, rec.Body)
	}
	// With the key the request reaches the store, which this test server does not
	// configure: anything but 401 means the gate opened.
	if rec := postJSONAs(t, protected, "/api/vehicles", body, "Bearer "+testAPIKey); rec.Code == http.StatusUnauthorized {
		t.Fatalf("write with a key was refused: %s", rec.Body)
	}

	read := httptest.NewRecorder()
	protected.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/vehicles", nil))
	// Vehicle reads remain public: this server has no fleet store, so the request
	// fails for its own reason, not for want of a key.
	if read.Code == http.StatusUnauthorized {
		t.Fatalf("read was refused: %s", read.Body)
	}
}

func TestDriverReadsStayPublicWhenWritesAreProtected(t *testing.T) {
	s := testServer(t, WithOperatorWritesProtected(true))

	withoutKey := httptest.NewRecorder()
	s.ServeHTTP(withoutKey, httptest.NewRequest(http.MethodGet, "/api/drivers", nil))
	if withoutKey.Code == http.StatusUnauthorized {
		t.Fatalf("driver read without a key was refused: %s", withoutKey.Body)
	}

	withKey := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/drivers", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	s.ServeHTTP(withKey, req)
	if withKey.Code != http.StatusServiceUnavailable {
		t.Fatalf("driver read with a key answered %d, want the same missing-store response: %s", withKey.Code, withKey.Body)
	}
}

// An unprotected process still takes writes: the demonstration console has no key
// to present. Readiness is where the difference is published.
func TestUnprotectedOperatorWritesStayOpen(t *testing.T) {
	s := testServer(t)
	body := `{"name":"Unit 502","registration":"B-9502-JKT","type":"van"}`
	if rec := postJSONAs(t, s, "/api/vehicles", body, ""); rec.Code == http.StatusUnauthorized {
		t.Fatalf("an unprotected process refused a write: %s", rec.Body)
	}
}

func TestProtectedDriverProfileUpdatesRequireAnAPIKey(t *testing.T) {
	protected := testServer(t, WithOperatorWritesProtected(true))
	req := httptest.NewRequest(http.MethodPut, "/api/drivers/driver-1", strings.NewReader(`{"name":"Demo Driver 1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("profile update without a key answered %d, want 401: %s", rec.Code, rec.Body)
	}
}

func TestReadyzReportsWhetherWritesAreProtected(t *testing.T) {
	for _, test := range []struct {
		name string
		opts []Option
		want string
	}{
		{name: "development", want: "unauthenticated"},
		{name: "deployment", opts: []Option{WithOperatorWritesProtected(true)}, want: "protected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testServer(t, test.opts...)
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if got := decodeBody(t, rec)["writes"]; got != test.want {
				t.Fatalf("writes = %v, want %v", got, test.want)
			}
		})
	}
}
