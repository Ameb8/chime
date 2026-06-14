package notify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	name     string
	supports bool
	err      error
	onFire   func(Notification)

	started     chan struct{}
	startedOnce sync.Once
	release     <-chan struct{}

	mu    sync.Mutex
	calls []Notification
}

func (b *fakeBackend) Name() string {
	return b.name
}

func (b *fakeBackend) Supports(Event) bool {
	return b.supports
}

func (b *fakeBackend) Fire(n Notification) error {
	b.mu.Lock()
	b.calls = append(b.calls, n)
	b.mu.Unlock()

	if b.started != nil {
		b.startedOnce.Do(func() {
			close(b.started)
		})
	}

	if b.onFire != nil {
		b.onFire(n)
	}

	if b.release != nil {
		<-b.release
	}

	return b.err
}

func (b *fakeBackend) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.calls)
}

func TestNewDispatcherAcceptsNoBackends(t *testing.T) {
	for _, backends := range [][]Backend{nil, {}} {
		dispatcher := NewDispatcher(backends)
		if dispatcher == nil {
			t.Fatal("NewDispatcher returned nil")
		}
		if got := len(dispatcher.backends); got != 0 {
			t.Fatalf("len(dispatcher.backends) = %d, want 0", got)
		}
		if err := dispatcher.Dispatch(context.Background(), Notification{Event: EventComplete}); err != nil {
			t.Fatalf("Dispatch returned error: %v", err)
		}
	}
}

func TestNewDispatcherStoresBackendsInOrder(t *testing.T) {
	first := &fakeBackend{name: "first"}
	second := &fakeBackend{name: "second"}

	dispatcher := NewDispatcher([]Backend{first, second})

	if got, want := dispatcher.backends[0].Name(), "first"; got != want {
		t.Fatalf("first backend name = %q, want %q", got, want)
	}
	if got, want := dispatcher.backends[1].Name(), "second"; got != want {
		t.Fatalf("second backend name = %q, want %q", got, want)
	}
}

func TestDispatchFiresOnlySupportedBackends(t *testing.T) {
	unsupported := &fakeBackend{name: "unsupported", supports: false}
	supported := &fakeBackend{name: "supported", supports: true}
	dispatcher := NewDispatcher([]Backend{unsupported, supported})

	err := dispatcher.Dispatch(context.Background(), Notification{
		Event:   EventWaiting,
		Agent:   "codex",
		Message: "permission needed",
	})

	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if got := unsupported.callCount(); got != 0 {
		t.Fatalf("unsupported backend Fire calls = %d, want 0", got)
	}
	if got := supported.callCount(); got != 1 {
		t.Fatalf("supported backend Fire calls = %d, want 1", got)
	}
}

func TestDispatchLogsBackendErrorsWithoutReturningThem(t *testing.T) {
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
	})

	dispatcher := NewDispatcher([]Backend{
		&fakeBackend{name: "sound", supports: true, err: errors.New("boom")},
	})

	err := dispatcher.Dispatch(context.Background(), Notification{Event: EventComplete})

	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	logOutput := buf.String()
	for _, want := range []string{
		`msg="backend fire error"`,
		`backend=sound`,
		`event=complete`,
		`err=boom`,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output %q does not contain %q", logOutput, want)
		}
	}
}

func TestDispatchRunsBackendsConcurrentlyAndWaits(t *testing.T) {
	release := make(chan struct{})
	blockedStarted := make(chan struct{})
	fastFired := make(chan struct{})
	dispatchReturned := make(chan struct{})

	blocked := &fakeBackend{
		name:     "blocked",
		supports: true,
		started:  blockedStarted,
		release:  release,
	}
	fast := &fakeBackend{
		name:     "fast",
		supports: true,
		onFire: func(Notification) {
			close(fastFired)
		},
	}

	dispatcher := NewDispatcher([]Backend{blocked, fast})
	go func() {
		_ = dispatcher.Dispatch(context.Background(), Notification{Event: EventComplete})
		close(dispatchReturned)
	}()

	waitForSignal(t, blockedStarted, "blocked backend to start")
	waitForSignal(t, fastFired, "fast backend to fire")

	select {
	case <-dispatchReturned:
		t.Fatal("Dispatch returned before all backend goroutines completed")
	default:
	}

	close(release)
	waitForSignal(t, dispatchReturned, "Dispatch to return")
}

func waitForSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
