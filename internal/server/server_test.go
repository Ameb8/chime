package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/notify"
)

type recordingBackend struct {
	supports bool
	err      error

	mu    sync.Mutex
	calls []notify.Notification
}

func (b *recordingBackend) Name() string {
	return "recording"
}

func (b *recordingBackend) Supports(notify.Event) bool {
	return b.supports
}

func (b *recordingBackend) Fire(n notify.Notification) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.calls = append(b.calls, n)
	return b.err
}

func (b *recordingBackend) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.calls)
}

func (b *recordingBackend) lastCall() notify.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[len(b.calls)-1]
}

func TestRoutesAndMethods(t *testing.T) {
	srv, backend := newTestServer("secret")

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResp := httptest.NewRecorder()
	srv.mux.ServeHTTP(healthResp, healthReq)
	if got := healthResp.Code; got != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", got, http.StatusOK)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/missing", nil)
	missingResp := httptest.NewRecorder()
	srv.mux.ServeHTTP(missingResp, missingReq)
	if got := missingResp.Code; got != http.StatusNotFound {
		t.Fatalf("GET /missing status = %d, want %d", got, http.StatusNotFound)
	}

	wrongMethodReq := httptest.NewRequest(http.MethodGet, "/notify", nil)
	wrongMethodResp := httptest.NewRecorder()
	srv.mux.ServeHTTP(wrongMethodResp, wrongMethodReq)
	if got := wrongMethodResp.Code; got != http.StatusMethodNotAllowed {
		t.Fatalf("GET /notify status = %d, want %d", got, http.StatusMethodNotAllowed)
	}
	if got := backend.callCount(); got != 0 {
		t.Fatalf("backend calls after wrong method = %d, want 0", got)
	}
}

func TestNotifyAuth(t *testing.T) {
	tests := []struct {
		name   string
		header string
		status int
		calls  int
	}{
		{name: "missing", status: http.StatusUnauthorized},
		{name: "malformed", header: "Basic secret", status: http.StatusUnauthorized},
		{name: "wrong", header: "Bearer wrong", status: http.StatusUnauthorized},
		{name: "correct", header: "Bearer secret", status: http.StatusOK, calls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, backend := newTestServer("secret")
			req := newNotifyRequest(`{"event":"complete"}`, tt.header)
			resp := httptest.NewRecorder()

			srv.mux.ServeHTTP(resp, req)

			if got := resp.Code; got != tt.status {
				t.Fatalf("status = %d, want %d; body=%s", got, tt.status, resp.Body.String())
			}
			if got := backend.callCount(); got != tt.calls {
				t.Fatalf("backend calls = %d, want %d", got, tt.calls)
			}
			if tt.status == http.StatusUnauthorized {
				var envelope responseEnvelope
				if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if envelope.Error != "unauthorized" {
					t.Fatalf("error = %q, want unauthorized", envelope.Error)
				}
			}
		})
	}
}

func TestNotifyValidation(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{name: "malformed json", body: `{`, status: http.StatusBadRequest},
		{name: "missing event", body: `{}`, status: http.StatusBadRequest},
		{name: "blank event", body: `{"event":"   "}`, status: http.StatusBadRequest},
		{name: "unknown event", body: `{"event":"done"}`, status: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, backend := newTestServer("secret")
			req := newNotifyRequest(tt.body, "Bearer secret")
			resp := httptest.NewRecorder()

			srv.mux.ServeHTTP(resp, req)

			if got := resp.Code; got != tt.status {
				t.Fatalf("status = %d, want %d; body=%s", got, tt.status, resp.Body.String())
			}
			if got := backend.callCount(); got != 0 {
				t.Fatalf("backend calls = %d, want 0", got)
			}
		})
	}
}

func TestNotifyDispatchesSanitizedNotification(t *testing.T) {
	srv, backend := newTestServer("secret")
	longAgent := strings.Repeat("a", maxAgentRunes+10)
	longMessage := strings.Repeat("m", maxMessageRunes+10)
	body := `{"event":"waiting","agent":"  ` + longAgent + `  ","message":"  ` + longMessage + `  "}`
	req := newNotifyRequest(body, "Bearer secret")
	resp := httptest.NewRecorder()

	srv.mux.ServeHTTP(resp, req)

	if got := resp.Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", got, http.StatusOK, resp.Body.String())
	}

	got := backend.lastCall()
	if got.Event != notify.EventWaiting {
		t.Fatalf("event = %q, want %q", got.Event, notify.EventWaiting)
	}
	if len([]rune(got.Agent)) != maxAgentRunes {
		t.Fatalf("agent length = %d, want %d", len([]rune(got.Agent)), maxAgentRunes)
	}
	if strings.HasPrefix(got.Agent, " ") || strings.HasSuffix(got.Agent, " ") {
		t.Fatalf("agent was not trimmed: %q", got.Agent)
	}
	if len([]rune(got.Message)) != maxMessageRunes {
		t.Fatalf("message length = %d, want %d", len([]rune(got.Message)), maxMessageRunes)
	}
	if strings.HasPrefix(got.Message, " ") || strings.HasSuffix(got.Message, " ") {
		t.Fatalf("message was not trimmed: %q", got.Message)
	}
}

func TestNotifyOversizedBodyDoesNotDispatch(t *testing.T) {
	srv, backend := newTestServer("secret")
	body := strings.Repeat("x", maxRequestBodyBytes+1)
	req := newNotifyRequest(body, "Bearer secret")
	resp := httptest.NewRecorder()

	srv.mux.ServeHTTP(resp, req)

	if got := resp.Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := backend.callCount(); got != 0 {
		t.Fatalf("backend calls = %d, want 0", got)
	}
}

func TestBackendErrorStillReturnsOK(t *testing.T) {
	backend := &recordingBackend{supports: true, err: errors.New("boom")}
	cfg := &config.Config{Auth: config.AuthConfig{Key: "secret"}}
	srv := New(cfg, notify.NewDispatcher([]notify.Backend{backend}), Options{Version: "test"})

	req := newNotifyRequest(`{"event":"complete"}`, "Bearer secret")
	resp := httptest.NewRecorder()

	srv.mux.ServeHTTP(resp, req)

	if got := resp.Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", got, http.StatusOK, resp.Body.String())
	}
	if got := backend.callCount(); got != 1 {
		t.Fatalf("backend calls = %d, want 1", got)
	}
}

func TestHealthResponse(t *testing.T) {
	srv, _ := newTestServer("secret")

	first := requestHealth(t, srv)
	time.Sleep(10 * time.Millisecond)
	second := requestHealth(t, srv)

	if !first.OK {
		t.Fatal("first health ok = false, want true")
	}
	if first.Version != "test" {
		t.Fatalf("version = %q, want test", first.Version)
	}
	if first.UptimeSeconds < 0 {
		t.Fatalf("uptime = %d, want non-negative", first.UptimeSeconds)
	}
	if second.UptimeSeconds < first.UptimeSeconds {
		t.Fatalf("second uptime = %d, first = %d", second.UptimeSeconds, first.UptimeSeconds)
	}
}

func TestStartConfiguresTimeoutsAndShutsDownOnContextCancel(t *testing.T) {
	srv, _ := newTestServer("secret")
	srv.opts.Bind = "127.0.0.1:0"
	ready := make(chan struct{})
	srv.opts.Ready = ready
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Start(ctx)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server start")
	}

	if got := srv.httpServer.ReadTimeout; got != readTimeout {
		t.Fatalf("read timeout = %s, want %s", got, readTimeout)
	}
	if got := srv.httpServer.WriteTimeout; got != writeTimeout {
		t.Fatalf("write timeout = %s, want %s", got, writeTimeout)
	}
	if got := srv.httpServer.IdleTimeout; got != idleTimeout {
		t.Fatalf("idle timeout = %s, want %s", got, idleTimeout)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Start to return")
	}
}

func newTestServer(key string) (*Server, *recordingBackend) {
	backend := &recordingBackend{supports: true}
	cfg := &config.Config{Auth: config.AuthConfig{Key: key}}
	srv := New(cfg, notify.NewDispatcher([]notify.Backend{backend}), Options{Version: "test"})
	return srv, backend
}

func newNotifyRequest(body, auth string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return req
}

func requestHealth(t *testing.T, srv *Server) healthResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	srv.mux.ServeHTTP(resp, req)
	if got := resp.Code; got != http.StatusOK {
		t.Fatalf("health status = %d, want %d", got, http.StatusOK)
	}

	var health healthResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	return health
}
