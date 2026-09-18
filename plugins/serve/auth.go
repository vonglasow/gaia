// Package serve runs gaia as an MCP daemon, behind a token and an origin check.
package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const tokenFileName = "serve.token"

// tokenPath is where the daemon keeps the secret a client must present.
func tokenPath() string { return filepath.Join(configDir(), tokenFileName) }

// ensureToken generates once, 0600: a token everyone can read authenticates nobody.
func ensureToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read serve token: %w", err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate serve token: %w", err)
	}
	token := hex.EncodeToString(raw)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write serve token: %w", err)
	}
	return token, nil
}

// bearerToken pulls the token out of an Authorization header, or returns "".
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// isLoopbackHost matches by name: the resolution is what an attacker controls.
func isLoopbackHost(hostPort string) bool {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return false
	}
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// originIsAllowed: absent is allowed, since a browser cannot omit it cross-site.
func originIsAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	if strings.EqualFold(origin, "null") {
		return false
	}
	trimmed := origin
	for _, scheme := range []string{"http://", "https://"} {
		if strings.HasPrefix(strings.ToLower(trimmed), scheme) {
			trimmed = trimmed[len(scheme):]
			break
		}
	}
	if i := strings.IndexAny(trimmed, "/?#"); i >= 0 {
		trimmed = trimmed[:i]
	}
	return isLoopbackHost(trimmed)
}

// guard checks the origin first, so a cross-site request never reaches the token.
func guard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !originIsAllowed(r.Header.Get("Origin")) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		// Constant-time: an early return leaks the token a byte at a time.
		presented := bearerToken(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="gaia"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
