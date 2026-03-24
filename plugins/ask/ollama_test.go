package ask

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

type ollamaTestServer struct {
	t         *testing.T
	tags      []string
	pullCalls int32
}

func (s *ollamaTestServer) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/tags":
		w.Header().Set("Content-Type", "application/json")
		type model struct {
			Name string `json:"name"`
		}
		out := struct {
			Models []model `json:"models"`
		}{Models: []model{}}
		for _, name := range s.tags {
			out.Models = append(out.Models, model{Name: name})
		}
		_ = json.NewEncoder(w).Encode(out)
	case "/api/pull":
		atomic.AddInt32(&s.pullCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"status":"pulling","completed":1,"total":2}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, `{"status":"success","completed":2,"total":2,"done":true}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
	default:
		http.NotFound(w, r)
	}
}

func newOllamaTestServer(t *testing.T, tags []string) (*httptest.Server, *ollamaTestServer) {
	t.Helper()
	state := &ollamaTestServer{t: t, tags: tags}
	srv := httptest.NewServer(http.HandlerFunc(state.handler))
	return srv, state
}

func parseHostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

func TestOllamaEnsureModelNoPullWhenExists(t *testing.T) {
	srv, state := newOllamaTestServer(t, []string{"llama3:latest"})
	defer srv.Close()

	host, port := parseHostPort(t, srv.URL)
	req := AskRequest{
		Host:        host,
		Port:        port,
		Model:       "llama3:latest",
		Timeout:     time.Second,
		Pull:        false,
		ProgressOut: io.Discard,
	}
	provider := NewOllamaProvider()
	if err := provider.ensureModel(context.Background(), req); err != nil {
		t.Fatalf("ensureModel error: %v", err)
	}
	if got := atomic.LoadInt32(&state.pullCalls); got != 0 {
		t.Fatalf("expected no pull call, got %d", got)
	}
}

func TestOllamaEnsureModelPullsWhenMissing(t *testing.T) {
	srv, state := newOllamaTestServer(t, []string{"other:latest"})
	defer srv.Close()

	host, port := parseHostPort(t, srv.URL)
	req := AskRequest{
		Host:        host,
		Port:        port,
		Model:       "llama3:latest",
		Timeout:     time.Second,
		Pull:        false,
		ProgressOut: io.Discard,
	}
	provider := NewOllamaProvider()
	if err := provider.ensureModel(context.Background(), req); err != nil {
		t.Fatalf("ensureModel error: %v", err)
	}
	if got := atomic.LoadInt32(&state.pullCalls); got != 1 {
		t.Fatalf("expected pull call, got %d", got)
	}
}

func TestOllamaEnsureModelPullsWhenForced(t *testing.T) {
	srv, state := newOllamaTestServer(t, []string{"llama3:latest"})
	defer srv.Close()

	host, port := parseHostPort(t, srv.URL)
	req := AskRequest{
		Host:        host,
		Port:        port,
		Model:       "llama3:latest",
		Timeout:     time.Second,
		Pull:        true,
		ProgressOut: io.Discard,
	}
	provider := NewOllamaProvider()
	if err := provider.ensureModel(context.Background(), req); err != nil {
		t.Fatalf("ensureModel error: %v", err)
	}
	if got := atomic.LoadInt32(&state.pullCalls); got != 1 {
		t.Fatalf("expected pull call, got %d", got)
	}
}
