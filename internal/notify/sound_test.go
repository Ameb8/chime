package notify

import (
	"errors"
	"strings"
	"testing"
)

type fakeSoundPlayer struct {
	err   error
	paths []string
}

func (p *fakeSoundPlayer) Play(path string) error {
	p.paths = append(p.paths, path)
	return p.err
}

type fakeSoundStore struct {
	err       error
	path      string
	filenames []string
	data      [][]byte
}

func (s *fakeSoundStore) SoundPath(filename string, data []byte) (string, error) {
	s.filenames = append(s.filenames, filename)
	s.data = append(s.data, append([]byte(nil), data...))
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

func TestSoundBackendSupportsReturnsFalseWhenDisabled(t *testing.T) {
	backend := newSoundBackend(SoundOptions{
		Enabled: false,
		Events:  []Event{EventComplete, EventWaiting},
	}, &fakeSoundPlayer{}, &fakeSoundStore{})

	for _, event := range []Event{EventComplete, EventWaiting} {
		if backend.Supports(event) {
			t.Fatalf("Supports(%q) = true, want false", event)
		}
	}
}

func TestSoundBackendSupportsOnlyConfiguredEvents(t *testing.T) {
	backend := newSoundBackend(SoundOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, &fakeSoundPlayer{}, &fakeSoundStore{})

	if !backend.Supports(EventComplete) {
		t.Fatal("Supports(EventComplete) = false, want true")
	}
	if backend.Supports(EventWaiting) {
		t.Fatal("Supports(EventWaiting) = true, want false")
	}
}

func TestSoundBackendSupportsNoEventsWhenEventListEmpty(t *testing.T) {
	backend := newSoundBackend(SoundOptions{
		Enabled: true,
		Events:  nil,
	}, &fakeSoundPlayer{}, &fakeSoundStore{})

	if backend.Supports(EventComplete) || backend.Supports(EventWaiting) {
		t.Fatal("backend supports events with an empty event list")
	}
}

func TestSoundBackendConstructorCopiesEventList(t *testing.T) {
	events := []Event{EventComplete}
	backend := newSoundBackend(SoundOptions{
		Enabled: true,
		Events:  events,
	}, &fakeSoundPlayer{}, &fakeSoundStore{})

	events[0] = EventWaiting

	if !backend.Supports(EventComplete) {
		t.Fatal("Supports(EventComplete) changed after caller mutated events")
	}
	if backend.Supports(EventWaiting) {
		t.Fatal("Supports(EventWaiting) changed after caller mutated events")
	}
}

func TestSoundBackendFireUsesCustomPaths(t *testing.T) {
	player := &fakeSoundPlayer{}
	store := &fakeSoundStore{path: "unused-default"}
	backend := newSoundBackend(SoundOptions{
		Enabled:       true,
		Events:        []Event{EventComplete, EventWaiting},
		CompleteSound: "/sounds/done.aiff",
		WaitingSound:  "/sounds/waiting.aiff",
	}, player, store)

	for _, tc := range []struct {
		event Event
		path  string
	}{
		{event: EventComplete, path: "/sounds/done.aiff"},
		{event: EventWaiting, path: "/sounds/waiting.aiff"},
	} {
		if err := backend.Fire(Notification{Event: tc.event}); err != nil {
			t.Fatalf("Fire(%q) returned error: %v", tc.event, err)
		}
		if got := player.paths[len(player.paths)-1]; got != tc.path {
			t.Fatalf("Fire(%q) played %q, want %q", tc.event, got, tc.path)
		}
	}

	if len(store.filenames) != 0 {
		t.Fatalf("default store calls = %d, want 0", len(store.filenames))
	}
}

func TestSoundBackendFireFallsBackToEmbeddedDefaults(t *testing.T) {
	player := &fakeSoundPlayer{}
	store := &fakeSoundStore{path: "/cache/default.aiff"}
	backend := newSoundBackend(SoundOptions{
		Enabled: true,
		Events:  []Event{EventComplete, EventWaiting},
	}, player, store)

	if err := backend.Fire(Notification{Event: EventComplete}); err != nil {
		t.Fatalf("Fire(EventComplete) returned error: %v", err)
	}
	if err := backend.Fire(Notification{Event: EventWaiting}); err != nil {
		t.Fatalf("Fire(EventWaiting) returned error: %v", err)
	}

	if got, want := player.paths, []string{"/cache/default.aiff", "/cache/default.aiff"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("played paths = %v, want %v", got, want)
	}
	if got, want := store.filenames, []string{"complete.aiff", "waiting.aiff"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("default filenames = %v, want %v", got, want)
	}
	for i, data := range store.data {
		if len(data) == 0 {
			t.Fatalf("default sound data[%d] is empty", i)
		}
	}
}

func TestSoundBackendFirePropagatesPlayerErrors(t *testing.T) {
	backend := newSoundBackend(SoundOptions{
		Enabled:       true,
		Events:        []Event{EventComplete},
		CompleteSound: "/sounds/done.aiff",
	}, &fakeSoundPlayer{err: errors.New("boom")}, &fakeSoundStore{})

	err := backend.Fire(Notification{Event: EventComplete})
	if err == nil {
		t.Fatal("Fire returned nil, want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Fire error = %q, want player error", err)
	}
}

func TestSoundBackendFireReturnsErrorForUnsupportedEvent(t *testing.T) {
	player := &fakeSoundPlayer{}
	backend := newSoundBackend(SoundOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, player, &fakeSoundStore{})

	err := backend.Fire(Notification{Event: EventWaiting})
	if err == nil {
		t.Fatal("Fire returned nil, want error")
	}
	if len(player.paths) != 0 {
		t.Fatalf("player calls = %d, want 0", len(player.paths))
	}
}
