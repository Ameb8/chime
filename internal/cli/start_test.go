package cli

import (
	"testing"

	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/notify"
)

func TestBuildBackendsUsesNotificationConfig(t *testing.T) {
	cfg := &appconfig.Config{
		Notifications: appconfig.NotificationsConfig{
			Toast: appconfig.ToastConfig{
				Enabled: true,
				Events:  []string{"waiting"},
			},
			Sound: appconfig.SoundConfig{
				Enabled: true,
				Events:  []string{"complete"},
			},
		},
	}

	backends := buildBackends(cfg)
	if len(backends) != 2 {
		t.Fatalf("backend count = %d, want 2", len(backends))
	}

	toast := findBackend(t, backends, "toast")
	if !toast.Supports(notify.EventWaiting) {
		t.Fatal("toast backend does not support configured waiting event")
	}
	if toast.Supports(notify.EventComplete) {
		t.Fatal("toast backend supports unconfigured complete event")
	}

	sound := findBackend(t, backends, "sound")
	if !sound.Supports(notify.EventComplete) {
		t.Fatal("sound backend does not support configured complete event")
	}
	if sound.Supports(notify.EventWaiting) {
		t.Fatal("sound backend supports unconfigured waiting event")
	}
}

func TestBuildBackendsSkipsDisabledBackends(t *testing.T) {
	cfg := &appconfig.Config{
		Notifications: appconfig.NotificationsConfig{
			Toast: appconfig.ToastConfig{
				Enabled: false,
				Events:  []string{"complete", "waiting"},
			},
			Sound: appconfig.SoundConfig{
				Enabled: false,
				Events:  []string{"complete", "waiting"},
			},
		},
	}

	backends := buildBackends(cfg)
	if len(backends) != 0 {
		t.Fatalf("backend count = %d, want 0", len(backends))
	}
}

func findBackend(t *testing.T, backends []notify.Backend, name string) notify.Backend {
	t.Helper()

	for _, backend := range backends {
		if backend.Name() == name {
			return backend
		}
	}
	t.Fatalf("backend %q not found", name)
	return nil
}
