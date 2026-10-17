package api

import (
	"net/http"
	"strings"

	"fleetflow/internal/auth"
)

// apiKeyScheme is the Authorization scheme an API key is presented with. It is
// the standard one, so a client does not need FleetFlow-specific documentation to
// know where a credential belongs.
const apiKeyScheme = "Bearer"

// requireAPIKey refuses a request that does not carry a valid API key, and does
// nothing else: no weight is written and no route is computed before it returns
// (DESIGN 3.5, PRD US-13).
//
// A key that is missing, malformed, or simply unknown is refused identically, so
// probing cannot tell those three cases apart. A key store that is unreachable is
// not one of them: it is the server's problem, and the auth service reports it as
// unavailable rather than as a bad key.
func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.keys == nil {
			writeError(w, r, errNoAuth)
			return
		}

		presented, ok := bearerKey(r)
		if !ok {
			// Nothing usable to check, which is the same answer as a key that
			// does not exist.
			writeError(w, r, auth.ErrUnauthenticated)
			return
		}
		if _, err := s.keys.Authenticate(r.Context(), presented); err != nil {
			writeError(w, r, err)
			return
		}

		next(w, r)
	}
}

// bearerKey reads the API key out of the Authorization header. It reports false
// for a header that is absent, uses another scheme, or carries an empty
// credential, so every one of those reaches the client as the same refusal.
func bearerKey(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}

	scheme, credential, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, apiKeyScheme) {
		return "", false
	}
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return "", false
	}
	return credential, true
}
