package notify

import (
	"errors"
	"strings"
	"testing"
)

type fakeToastSender struct {
	err   error
	shows []toastShow
}

type toastShow struct {
	title string
	body  string
}

func (s *fakeToastSender) Show(title, body string) error {
	s.shows = append(s.shows, toastShow{
		title: title,
		body:  body,
	})
	return s.err
}

func TestToastBackendSupportsReturnsFalseWhenDisabled(t *testing.T) {
	backend := newToastBackend(ToastOptions{
		Enabled: false,
		Events:  []Event{EventComplete, EventWaiting},
	}, &fakeToastSender{})

	for _, event := range []Event{EventComplete, EventWaiting} {
		if backend.Supports(event) {
			t.Fatalf("Supports(%q) = true, want false", event)
		}
	}
}

func TestToastBackendSupportsOnlyConfiguredEvents(t *testing.T) {
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, &fakeToastSender{})

	if !backend.Supports(EventComplete) {
		t.Fatal("Supports(EventComplete) = false, want true")
	}
	if backend.Supports(EventWaiting) {
		t.Fatal("Supports(EventWaiting) = true, want false")
	}
}

func TestToastBackendSupportsNoEventsWhenEventListEmpty(t *testing.T) {
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  nil,
	}, &fakeToastSender{})

	if backend.Supports(EventComplete) || backend.Supports(EventWaiting) {
		t.Fatal("backend supports events with an empty event list")
	}
}

func TestToastBackendConstructorCopiesEventList(t *testing.T) {
	events := []Event{EventComplete}
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  events,
	}, &fakeToastSender{})

	events[0] = EventWaiting

	if !backend.Supports(EventComplete) {
		t.Fatal("Supports(EventComplete) changed after caller mutated events")
	}
	if backend.Supports(EventWaiting) {
		t.Fatal("Supports(EventWaiting) changed after caller mutated events")
	}
}

func TestToastBackendFireFormatsSupportedEvents(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    Notification
		want toastShow
	}{
		{
			name: "complete",
			n: Notification{
				Event:   EventComplete,
				Agent:   "codex",
				Message: "Tests passed",
			},
			want: toastShow{
				title: "chime \u2014 codex",
				body:  "Task complete: Tests passed",
			},
		},
		{
			name: "waiting",
			n: Notification{
				Event:   EventWaiting,
				Agent:   "claude-code",
				Message: "Needs approval",
			},
			want: toastShow{
				title: "chime \u2014 claude-code",
				body:  "Waiting for input: Needs approval",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeToastSender{}
			backend := newToastBackend(ToastOptions{
				Enabled: true,
				Events:  []Event{EventComplete, EventWaiting},
			}, sender)

			if err := backend.Fire(tc.n); err != nil {
				t.Fatalf("Fire returned error: %v", err)
			}
			if got := len(sender.shows); got != 1 {
				t.Fatalf("sender calls = %d, want 1", got)
			}
			if got := sender.shows[0]; got != tc.want {
				t.Fatalf("toast = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestToastBackendFireFormatsOptionalAgentAndMessageCleanly(t *testing.T) {
	sender := &fakeToastSender{}
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, sender)

	if err := backend.Fire(Notification{
		Event:   EventComplete,
		Agent:   "   ",
		Message: "",
	}); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}

	want := toastShow{
		title: "chime",
		body:  "Task complete",
	}
	if got := sender.shows[0]; got != want {
		t.Fatalf("toast = %+v, want %+v", got, want)
	}
}

func TestToastBackendFirePropagatesSenderErrors(t *testing.T) {
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, &fakeToastSender{err: errors.New("boom")})

	err := backend.Fire(Notification{Event: EventComplete})
	if err == nil {
		t.Fatal("Fire returned nil, want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Fire error = %q, want sender error", err)
	}
}

func TestToastBackendFireReturnsErrorForUnsupportedEvent(t *testing.T) {
	sender := &fakeToastSender{}
	backend := newToastBackend(ToastOptions{
		Enabled: true,
		Events:  []Event{EventComplete},
	}, sender)

	err := backend.Fire(Notification{Event: EventWaiting})
	if err == nil {
		t.Fatal("Fire returned nil, want error")
	}
	if len(sender.shows) != 0 {
		t.Fatalf("sender calls = %d, want 0", len(sender.shows))
	}
}
