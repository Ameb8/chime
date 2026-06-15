package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/exitcode"
)

func TestNotifyCommandSendsPayload(t *testing.T) {
	const apiKey = "secret"

	gotReq := make(chan notifyHTTPCall, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotReq <- notifyHTTPCall{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		}
		_, _ = w.Write([]byte(`{"ok":true,"event":"waiting"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Client: config.ClientConfig{Server: srv.URL + "/"},
		Auth:   config.AuthConfig{Key: apiKey},
	}
	stdout, stderr, err := executeNotifyCommand(t, cfg,
		"--event", "waiting",
		"--agent", "codex",
		"--message", "needs input",
	)
	if err != nil {
		t.Fatalf("notify command error: %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	got := <-gotReq
	if got.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", got.method)
	}
	if got.path != "/notify" {
		t.Fatalf("path = %s, want /notify", got.path)
	}
	if got.auth != "Bearer "+apiKey {
		t.Fatalf("authorization = %q, want bearer token", got.auth)
	}
	if got.body["event"] != "waiting" {
		t.Fatalf("event = %q, want waiting", got.body["event"])
	}
	if got.body["agent"] != "codex" {
		t.Fatalf("agent = %q, want codex", got.body["agent"])
	}
	if got.body["message"] != "needs input" {
		t.Fatalf("message = %q, want needs input", got.body["message"])
	}
}

func TestNotifyCommandValidation(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Client: config.ClientConfig{Server: srv.URL},
		Auth:   config.AuthConfig{Key: "secret"},
	}

	tests := []struct {
		name string
		cfg  *config.Config
		args []string
		code int
	}{
		{
			name: "missing event",
			cfg:  cfg,
			args: nil,
			code: exitcode.General,
		},
		{
			name: "unknown event",
			cfg:  cfg,
			args: []string{"--event", "unknown"},
			code: exitcode.General,
		},
		{
			name: "positional argument",
			cfg:  cfg,
			args: []string{"--event", "complete", "extra"},
			code: exitcode.General,
		},
		{
			name: "missing server url",
			cfg:  &config.Config{Auth: config.AuthConfig{Key: "secret"}},
			args: []string{"--event", "complete"},
			code: exitcode.General,
		},
		{
			name: "bad server url",
			cfg:  &config.Config{Client: config.ClientConfig{Server: "://bad"}, Auth: config.AuthConfig{Key: "secret"}},
			args: []string{"--event", "complete"},
			code: exitcode.General,
		},
		{
			name: "missing api key",
			cfg:  &config.Config{Client: config.ClientConfig{Server: srv.URL}},
			args: []string{"--event", "complete"},
			code: exitcode.AuthFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := requests.Load()
			_, _, err := executeNotifyCommand(t, tt.cfg, tt.args...)
			if err == nil {
				t.Fatal("notify command succeeded, want error")
			}
			assertExitCode(t, err, tt.code)
			if got := requests.Load(); got != before {
				t.Fatalf("requests = %d after validation failure, want %d", got, before)
			}
		})
	}
}

func TestNotifyCommandMapsHTTPFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   int
	}{
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
			body:   `{"ok":false,"error":"unauthorized"}`,
			code:   exitcode.AuthFailure,
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
			body:   `{"ok":false,"error":"internal server error"}`,
			code:   exitcode.ServerUnreachable,
		},
		{
			name:   "bad success response",
			status: http.StatusOK,
			body:   `{`,
			code:   exitcode.General,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			cfg := &config.Config{
				Client: config.ClientConfig{Server: srv.URL},
				Auth:   config.AuthConfig{Key: "secret"},
			}
			_, _, err := executeNotifyCommand(t, cfg, "--event", "complete")
			if err == nil {
				t.Fatal("notify command succeeded, want error")
			}
			assertExitCode(t, err, tt.code)
		})
	}
}

func TestNotifyCommandResolutionPrecedence(t *testing.T) {
	configServer, configHits := notifyTestServer(t, "config-key")
	envServer, envHits := notifyTestServer(t, "env-key")
	flagServer, flagHits := notifyTestServer(t, "flag-key")

	t.Setenv("CHIME_SERVER", envServer.URL)
	t.Setenv("CHIME_KEY", "env-key")

	cfg := &config.Config{
		Client: config.ClientConfig{Server: configServer.URL},
		Auth:   config.AuthConfig{Key: "config-key"},
	}

	_, _, err := executeNotifyCommand(t, cfg, "--event", "complete")
	if err != nil {
		t.Fatalf("notify with env values: %v", err)
	}
	if envHits.Load() != 1 || configHits.Load() != 0 || flagHits.Load() != 0 {
		t.Fatalf("hits after env case: config=%d env=%d flag=%d, want 0/1/0", configHits.Load(), envHits.Load(), flagHits.Load())
	}

	_, _, err = executeNotifyCommand(t, cfg,
		"--event", "complete",
		"--server", flagServer.URL,
		"--key", "flag-key",
	)
	if err != nil {
		t.Fatalf("notify with flag values: %v", err)
	}
	if envHits.Load() != 1 || configHits.Load() != 0 || flagHits.Load() != 1 {
		t.Fatalf("hits after flag case: config=%d env=%d flag=%d, want 0/1/1", configHits.Load(), envHits.Load(), flagHits.Load())
	}
}

type notifyHTTPCall struct {
	method string
	path   string
	auth   string
	body   map[string]string
}

func executeNotifyCommand(t *testing.T, cfg *config.Config, args ...string) (string, string, error) {
	t.Helper()

	cfgPtr := cfg
	cmd := newNotifyCmd(&cfgPtr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()

	var codeErr *exitcode.Error
	if errors.As(err, &codeErr) {
		if codeErr.Code != want {
			t.Fatalf("exit code = %d, want %d", codeErr.Code, want)
		}
		return
	}
	if want != exitcode.General {
		t.Fatalf("error = %v, want exit code %d", err, want)
	}
}

func notifyTestServer(t *testing.T, key string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}
