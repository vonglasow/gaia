package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// Every case here is one way in.

const aToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// served runs one request against the guard and reports what came back.
func served(t *testing.T, build func(*http.Request)) (int, bool) {
	t.Helper()
	reached := false
	handler := guard(aToken, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8765/mcp", nil)
	req.Host = "localhost:8765"
	build(req)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, reached
}

func TestACallCarryingTheTokenGetsThrough(t *testing.T) {
	code, reached := served(t, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+aToken)
	})

	require.Equal(t, http.StatusOK, code)
	require.True(t, reached)
}

// This is the case the daemon shipped with.
func TestACallWithNoTokenIsRefused(t *testing.T) {
	code, reached := served(t, func(*http.Request) {})

	require.Equal(t, http.StatusUnauthorized, code)
	require.False(t, reached, "the handler must not run for an unauthenticated call")
}

func TestAWrongTokenIsRefused(t *testing.T) {
	code, reached := served(t, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer not-the-token")
	})

	require.Equal(t, http.StatusUnauthorized, code)
	require.False(t, reached)
}

// A prefix of the real token must not pass.
func TestAPrefixOfTheTokenIsRefused(t *testing.T) {
	code, _ := served(t, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+aToken[:32])
	})

	require.Equal(t, http.StatusUnauthorized, code)
}

func TestAnUnauthorizedReplySaysHowToAuthenticate(t *testing.T) {
	handler := guard(aToken, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8765/mcp", nil)
	req.Host = "localhost:8765"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
}

// DNS rebinding: the Host header carries the name the browser actually resolved.
func TestARequestForAHostThatIsNotOursIsRefused(t *testing.T) {
	code, reached := served(t, func(r *http.Request) {
		r.Host = "gaia.evil.example:8765"
		r.Header.Set("Authorization", "Bearer "+aToken)
	})

	require.Equal(t, http.StatusForbidden, code)
	require.False(t, reached)
}

// A page cannot read a file under ~/.config, so it cannot present the token.
func TestACrossSiteOriginIsRefusedBeforeTheToken(t *testing.T) {
	code, reached := served(t, func(r *http.Request) {
		r.Header.Set("Origin", "https://evil.example")
		r.Header.Set("Authorization", "Bearer "+aToken)
	})

	require.Equal(t, http.StatusForbidden, code)
	require.False(t, reached)
}

func TestALoopbackOriginIsAccepted(t *testing.T) {
	for _, origin := range []string{
		"http://localhost:8765",
		"http://127.0.0.1:8765",
		"http://[::1]:8765",
		"http://localhost",
	} {
		t.Run(origin, func(t *testing.T) {
			code, _ := served(t, func(r *http.Request) {
				r.Header.Set("Origin", origin)
				r.Header.Set("Authorization", "Bearer "+aToken)
			})
			require.Equal(t, http.StatusOK, code)
		})
	}
}

// A sandboxed iframe or a file:// page sends Origin: null.
func TestANullOriginIsRefused(t *testing.T) {
	code, _ := served(t, func(r *http.Request) {
		r.Header.Set("Origin", "null")
		r.Header.Set("Authorization", "Bearer "+aToken)
	})

	require.Equal(t, http.StatusForbidden, code)
}

// A command-line client sends no Origin.
func TestNoOriginAtAllIsAllowed(t *testing.T) {
	require.True(t, originIsAllowed(""))
	require.True(t, originIsAllowed("   "))
}

func TestAHostThatMerelyResolvesLocallyIsStillRefused(t *testing.T) {
	require.False(t, isLoopbackHost("localtest.me:8765"),
		"localtest.me resolves to 127.0.0.1, and that resolution is what an attacker controls")
	require.False(t, isLoopbackHost("127.0.0.1.evil.example"))
	require.False(t, isLoopbackHost(""))
}

func TestTheLoopbackNamesAreAccepted(t *testing.T) {
	for _, host := range []string{
		"localhost", "localhost:8765", "127.0.0.1", "127.0.0.1:8765",
		"[::1]:8765", "LOCALHOST:8765",
	} {
		require.Truef(t, isLoopbackHost(host), "host %q", host)
	}
}

func TestTheBearerPrefixIsRequiredAndCaseInsensitive(t *testing.T) {
	require.Equal(t, "abc", bearerToken("Bearer abc"))
	require.Equal(t, "abc", bearerToken("bearer abc"))
	require.Equal(t, "abc", bearerToken("Bearer   abc  "))
	require.Empty(t, bearerToken("abc"), "a bare value is not a bearer token")
	require.Empty(t, bearerToken("Basic abc"))
	require.Empty(t, bearerToken(""))
	require.Empty(t, bearerToken("Bearer "))
}

// --- the token file -------------------------------------------------------

func TestTheTokenIsGeneratedOnceAndKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gaia", "serve.token")

	first, err := ensureToken(path)
	require.NoError(t, err)
	require.Len(t, first, 64, "32 random bytes, hex")

	second, err := ensureToken(path)
	require.NoError(t, err)
	require.Equal(t, first, second, "a token regenerated on every start invalidates every client")
}

// A token every account on the machine can read authenticates nobody.
func TestTheTokenFileIsPrivateToItsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gaia", "serve.token")
	_, err := ensureToken(path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	dir, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
}

// An empty file is what a failed write leaves behind.
func TestAnEmptyTokenFileIsReplacedRatherThanTrusted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.token")
	require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))

	token, err := ensureToken(path)

	require.NoError(t, err)
	require.Len(t, token, 64)
}

func TestTwoMachinesDoNotGetTheSameToken(t *testing.T) {
	first, err := ensureToken(filepath.Join(t.TempDir(), "serve.token"))
	require.NoError(t, err)
	second, err := ensureToken(filepath.Join(t.TempDir(), "serve.token"))
	require.NoError(t, err)

	require.NotEqual(t, first, second)
}

func TestATokenThatCannotBeReadIsReported(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file should be.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "serve.token"), 0o700))

	_, err := ensureToken(filepath.Join(dir, "serve.token"))

	require.Error(t, err)
}

// --- the real MCP surface, end to end -------------------------------------

// probePlugin is a plugin of the test's own, carrying one MCP tool.
type probePlugin struct {
	id      string
	enabled bool
	calls   *int
}

func (p *probePlugin) ID() string             { return p.id }
func (p *probePlugin) DefaultEnabled() bool   { return p.enabled }
func (p *probePlugin) DependsOn() []string    { return nil }
func (p *probePlugin) ConfigSchema() []string { return nil }

func (p *probePlugin) Register(*kernel.Kernel) ([]*cobra.Command, error) {
	return []*cobra.Command{{Use: p.id}}, nil
}

func (p *probePlugin) MCPTools() []kernel.MCPTool {
	return []kernel.MCPTool{{
		Name:        p.id + "_probe",
		Description: "a tool that records that it was called",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		Handler: func(context.Context, map[string]interface{}) (string, error) {
			*p.calls++
			return "called", nil
		},
	}}
}

// servedByTheDaemon builds the handler the daemon really serves.
func servedByTheDaemon(t *testing.T, plugins ...kernel.Plugin) (*httptest.Server, *ServePlugin) {
	t.Helper()
	aHomeWithoutADaemon(t)

	k := kernel.NewKernel()
	for _, plugin := range plugins {
		require.NoError(t, k.RegisterPlugin(plugin))
	}
	require.NoError(t, k.ResolveEnabled())

	p := NewServePlugin()
	_, err := p.Register(k)
	require.NoError(t, err)

	handler, err := p.mcpHandler()
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, p
}

func callMCP(t *testing.T, server *httptest.Server, token string) int {
	t.Helper()
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Host = "localhost:8765"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// The handler here is the one the daemon serves, with the guard on it.
func TestTheRealHandlerRefusesAnUnauthenticatedCall(t *testing.T) {
	calls := 0
	server, _ := servedByTheDaemon(t, &probePlugin{id: "probe", enabled: true, calls: &calls})

	require.Equal(t, http.StatusUnauthorized, callMCP(t, server, ""))
}

func TestTheRealHandlerAnswersACallCarryingTheToken(t *testing.T) {
	calls := 0
	server, _ := servedByTheDaemon(t, &probePlugin{id: "probe", enabled: true, calls: &calls})

	token, err := ensureToken(tokenPath())
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, callMCP(t, server, token))
}

// A tool belonging to a disabled plugin must not be on the surface at all.
func TestADisabledPluginIsNotOnTheMCPSurface(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	k := kernel.NewKernel()
	calls := 0
	require.NoError(t, k.RegisterPlugin(&probePlugin{id: "wanted", enabled: true, calls: &calls}))
	require.NoError(t, k.RegisterPlugin(&probePlugin{id: "unwanted", enabled: false, calls: &calls}))
	require.NoError(t, k.ResolveEnabled())

	exposed := map[string]bool{}
	for _, plugin := range k.EnabledPlugins() {
		for _, tool := range plugin.MCPTools() {
			exposed[tool.Name] = true
		}
	}

	require.True(t, exposed["wanted_probe"])
	require.False(t, exposed["unwanted_probe"],
		"a plugin that is off must not reach the loop that registers MCP tools")
}
