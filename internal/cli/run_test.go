package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/exitcode"
)

func TestRunCommandArgumentParsing(t *testing.T) {
	cfg := &config.Config{}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing command",
			args: nil,
			want: runUsage,
		},
		{
			name: "delimiter only",
			args: []string{"--"},
			want: "missing command after --",
		},
		{
			name: "missing delimiter",
			args: []string{"test-command"},
			want: "missing -- delimiter",
		},
		{
			name: "missing delimiter before child flag",
			args: []string{"test-command", "--child-flag"},
			want: "missing -- delimiter",
		},
		{
			name: "argument before delimiter",
			args: []string{"test-command", "--", "child"},
			want: "missing -- delimiter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &recordingRunExecutor{}
			_, _, err := executeRunCommand(t, cfg, runDeps{
				execute: exec.execute,
			}, tt.args...)
			if err == nil {
				t.Fatal("run command succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want to contain %q", err, tt.want)
			}
			if exec.calls != 0 {
				t.Fatalf("executor calls = %d, want 0", exec.calls)
			}
		})
	}
}

func TestRunCommandPassesWrappedArgs(t *testing.T) {
	cfg := &config.Config{}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: exitcode.Success,
			Duration: 1500 * time.Millisecond,
		},
	}
	notifier := &recordingRunNotifier{}

	_, stderr, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
		notify:  notifier.notify,
	}, "--", "test-command", "--child-flag")
	if err != nil {
		t.Fatalf("run command error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	wantArgv := []string{"test-command", "--child-flag"}
	if !reflect.DeepEqual(exec.argv, wantArgv) {
		t.Fatalf("argv = %#v, want %#v", exec.argv, wantArgv)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
}

func TestRunCommandOptionsReachNotifier(t *testing.T) {
	cfg := &config.Config{}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: exitcode.Success,
			Duration: time.Second,
		},
	}
	notifier := &recordingRunNotifier{}

	_, _, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
		notify:  notifier.notify,
	}, "--agent", "terminal", "--message", "frontend build", "--", "test-command")
	if err != nil {
		t.Fatalf("run command error: %v", err)
	}
	if notifier.opts.agent != "terminal" {
		t.Fatalf("agent = %q, want terminal", notifier.opts.agent)
	}
	if notifier.opts.message != "frontend build" {
		t.Fatalf("message = %q, want frontend build", notifier.opts.message)
	}
}

func TestRunCommandSendsNotificationPayload(t *testing.T) {
	const apiKey = "secret"

	gotReq := make(chan runHTTPCall, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotReq <- runHTTPCall{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		}
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Client: config.ClientConfig{Server: srv.URL},
		Auth:   config.AuthConfig{Key: apiKey},
	}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: exitcode.Success,
			Duration: 151 * time.Second,
		},
	}

	_, _, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
	}, "--agent", "terminal", "--message", "frontend build", "--", "npm", "run", "build")
	if err != nil {
		t.Fatalf("run command error: %v", err)
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
	if got.body["event"] != "complete" {
		t.Fatalf("event = %q, want complete", got.body["event"])
	}
	if got.body["agent"] != "terminal" {
		t.Fatalf("agent = %q, want terminal", got.body["agent"])
	}
	wantMessage := "frontend build: npm run build completed successfully in 2m31s"
	if got.body["message"] != wantMessage {
		t.Fatalf("message = %q, want %q", got.body["message"], wantMessage)
	}
}

func TestRunCommandDefaultAgent(t *testing.T) {
	gotReq := make(chan map[string]string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotReq <- body
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Client: config.ClientConfig{Server: srv.URL},
		Auth:   config.AuthConfig{Key: "secret"},
	}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: exitcode.Success,
			Duration: time.Second,
		},
	}

	_, _, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
	}, "--", "/usr/local/bin/docker", "compose", "build")
	if err != nil {
		t.Fatalf("run command error: %v", err)
	}
	body := <-gotReq
	if body["agent"] != "docker" {
		t.Fatalf("agent = %q, want docker", body["agent"])
	}
}

func TestRunCommandResolutionPrecedence(t *testing.T) {
	configServer, configHits := runTestServer(t, "config-key")
	envServer, envHits := runTestServer(t, "env-key")
	flagServer, flagHits := runTestServer(t, "flag-key")

	t.Setenv("CHIME_SERVER", envServer.URL)
	t.Setenv("CHIME_KEY", "env-key")

	cfg := &config.Config{
		Client: config.ClientConfig{Server: configServer.URL},
		Auth:   config.AuthConfig{Key: "config-key"},
	}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: exitcode.Success,
			Duration: time.Second,
		},
	}

	_, _, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
	}, "--", "test-command")
	if err != nil {
		t.Fatalf("run with env values: %v", err)
	}
	if envHits.Load() != 1 || configHits.Load() != 0 || flagHits.Load() != 0 {
		t.Fatalf("hits after env case: config=%d env=%d flag=%d, want 0/1/0", configHits.Load(), envHits.Load(), flagHits.Load())
	}

	_, _, err = executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
	}, "--server", flagServer.URL, "--key", "flag-key", "--", "test-command")
	if err != nil {
		t.Fatalf("run with flag values: %v", err)
	}
	if envHits.Load() != 1 || configHits.Load() != 0 || flagHits.Load() != 1 {
		t.Fatalf("hits after flag case: config=%d env=%d flag=%d, want 0/1/1", configHits.Load(), envHits.Load(), flagHits.Load())
	}
}

func TestRunCommandNotificationFailureDoesNotChangeExit(t *testing.T) {
	tests := []struct {
		name     string
		child    int
		wantErr  bool
		wantCode int
	}{
		{
			name:    "successful child",
			child:   exitcode.Success,
			wantErr: false,
		},
		{
			name:     "failed child",
			child:    9,
			wantErr:  true,
			wantCode: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			exec := &recordingRunExecutor{
				result: commandResult{
					Started:  true,
					ExitCode: tt.child,
					Duration: time.Second,
				},
			}

			_, stderr, err := executeRunCommand(t, cfg, runDeps{
				execute: exec.execute,
			}, "--", "test-command")
			if tt.wantErr {
				assertSilentExitCode(t, err, tt.wantCode)
			} else if err != nil {
				t.Fatalf("run command error = %v, want nil", err)
			}
			if !strings.Contains(stderr, "chime: notification failed:") {
				t.Fatalf("stderr = %q, want notification warning", stderr)
			}
		})
	}
}

func TestRunCommandChildExitIsSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		Client: config.ClientConfig{Server: srv.URL},
		Auth:   config.AuthConfig{Key: "secret"},
	}
	exec := &recordingRunExecutor{
		result: commandResult{
			Started:  true,
			ExitCode: 7,
			Duration: time.Second,
		},
	}

	_, stderr, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
	}, "--", "test-command")
	assertSilentExitCode(t, err, 7)
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunCommandStartFailureSkipsNotification(t *testing.T) {
	cfg := &config.Config{}
	startErr := errors.New("start failed")
	exec := &recordingRunExecutor{
		err: startErr,
	}
	notifier := &recordingRunNotifier{}

	_, _, err := executeRunCommand(t, cfg, runDeps{
		execute: exec.execute,
		notify:  notifier.notify,
	}, "--", "test-command")
	if !errors.Is(err, startErr) {
		t.Fatalf("error = %v, want %v", err, startErr)
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
}

func TestFormatRunMessage(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		result commandResult
		want   string
	}{
		{
			name:  "success",
			label: "",
			result: commandResult{
				DisplayCommand: "npm run build",
				ExitCode:       exitcode.Success,
				Duration:       48 * time.Second,
			},
			want: "npm run build completed successfully in 48s",
		},
		{
			name:  "failure",
			label: "",
			result: commandResult{
				DisplayCommand: "go test ./...",
				ExitCode:       17,
				Duration:       151 * time.Second,
			},
			want: "go test ./... failed with exit code 17 after 2m31s",
		},
		{
			name:  "signal with label",
			label: "frontend build",
			result: commandResult{
				DisplayCommand: "npm run build",
				ExitCode:       130,
				Duration:       12 * time.Second,
				SignalName:     "SIGINT",
			},
			want: "frontend build: npm run build interrupted by SIGINT with exit code 130 after 12s",
		},
		{
			name:  "subsecond",
			label: "",
			result: commandResult{
				DisplayCommand: "true",
				ExitCode:       exitcode.Success,
				Duration:       500 * time.Millisecond,
			},
			want: "true completed successfully in <1s",
		},
		{
			name:  "hour",
			label: "",
			result: commandResult{
				DisplayCommand: "long-command",
				ExitCode:       exitcode.Success,
				Duration:       2*time.Hour + 7*time.Minute + 30*time.Second,
			},
			want: "long-command completed successfully in 2h7m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRunMessage(tt.label, tt.result); got != tt.want {
				t.Fatalf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecuteWrappedCommandExitCode(t *testing.T) {
	t.Setenv("CHIME_RUN_HELPER", "1")
	t.Setenv("CHIME_RUN_MODE", "exit")
	t.Setenv("CHIME_RUN_EXIT", "7")

	result, err := executeWrappedCommand(context.Background(), runHelperArgs(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("execute wrapped command: %v", err)
	}
	if !result.Started {
		t.Fatal("Started = false, want true")
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestExecuteWrappedCommandStreams(t *testing.T) {
	t.Setenv("CHIME_RUN_HELPER", "1")
	t.Setenv("CHIME_RUN_MODE", "stdio")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := executeWrappedCommand(context.Background(), runHelperArgs(), strings.NewReader("from stdin"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("execute wrapped command: %v", err)
	}
	if result.ExitCode != exitcode.Success {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if stdout.String() != "child stdout\nstdin:from stdin" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "child stderr\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestExecuteWrappedCommandStartFailure(t *testing.T) {
	result, err := executeWrappedCommand(context.Background(), []string{"__chime_missing_executable__"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("execute wrapped command succeeded, want error")
	}
	if result.Started {
		t.Fatal("Started = true, want false")
	}
}

func TestRunHelperProcess(t *testing.T) {
	if os.Getenv("CHIME_RUN_HELPER") != "1" {
		return
	}

	switch os.Getenv("CHIME_RUN_MODE") {
	case "exit":
		code, err := strconv.Atoi(os.Getenv("CHIME_RUN_EXIT"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad CHIME_RUN_EXIT: %v", err)
			os.Exit(1)
		}
		os.Exit(code)
	case "stdio":
		if _, err := fmt.Fprintln(os.Stdout, "child stdout"); err != nil {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "child stderr")
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v", err)
			os.Exit(1)
		}
		if _, err := fmt.Fprintf(os.Stdout, "stdin:%s", stdin); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown CHIME_RUN_MODE %q", os.Getenv("CHIME_RUN_MODE"))
		os.Exit(1)
	}
}

type runHTTPCall struct {
	method string
	path   string
	auth   string
	body   map[string]string
}

type recordingRunExecutor struct {
	calls  int
	argv   []string
	result commandResult
	err    error
}

func (r *recordingRunExecutor) execute(_ context.Context, argv []string, _ io.Reader, _ io.Writer, _ io.Writer) (commandResult, error) {
	r.calls++
	r.argv = append([]string(nil), argv...)
	result := r.result
	if result.DisplayCommand == "" {
		result.DisplayCommand = displayCommand(argv)
	}
	if len(argv) > 0 && result.Executable == "" {
		result.Executable = argv[0]
	}
	if !result.Started && r.err == nil {
		result.Started = true
	}
	return result, r.err
}

type recordingRunNotifier struct {
	calls  int
	opts   runOptions
	result commandResult
	err    error
}

func (r *recordingRunNotifier) notify(_ context.Context, _ *config.Config, opts *runOptions, result commandResult) error {
	r.calls++
	r.opts = *opts
	r.result = result
	return r.err
}

func executeRunCommand(t *testing.T, cfg *config.Config, deps runDeps, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deps.stdin = strings.NewReader("")
	if deps.stdout == nil {
		deps.stdout = &stdout
	}
	if deps.stderr == nil {
		deps.stderr = &stderr
	}

	cfgPtr := cfg
	cmd := newRunCmdWithDeps(&cfgPtr, deps)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func assertSilentExitCode(t *testing.T, err error, want int) {
	t.Helper()

	var silentErr *exitcode.SilentError
	if !errors.As(err, &silentErr) {
		t.Fatalf("error = %v, want silent exit code %d", err, want)
	}
	if silentErr.Code != want {
		t.Fatalf("exit code = %d, want %d", silentErr.Code, want)
	}
}

func runTestServer(t *testing.T, key string) (*httptest.Server, *atomic.Int64) {
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

func runHelperArgs() []string {
	return []string{os.Args[0], "-test.run=TestRunHelperProcess"}
}
