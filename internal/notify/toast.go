package notify

import (
	"fmt"
	"strings"
)

type ToastOptions struct {
	Enabled bool
	Events  []Event
}

type ToastBackend struct {
	enabled bool
	events  map[Event]struct{}

	sender toastSender
}

type toastSender interface {
	Show(title, body string) error
}

var toastBodyByEvent = map[Event]string{
	EventComplete: "Task complete",
	EventWaiting:  "Waiting for input",
}

func NewToastBackend(options ToastOptions) *ToastBackend {
	return newToastBackend(options, newPlatformToastSender())
}

func newToastBackend(options ToastOptions, sender toastSender) *ToastBackend {
	events := make(map[Event]struct{}, len(options.Events))
	for _, event := range append([]Event(nil), options.Events...) {
		if _, ok := toastBodyByEvent[event]; ok {
			events[event] = struct{}{}
		}
	}

	return &ToastBackend{
		enabled: options.Enabled,
		events:  events,
		sender:  sender,
	}
}

func (b *ToastBackend) Name() string {
	return "toast"
}

func (b *ToastBackend) Supports(event Event) bool {
	if !b.enabled {
		return false
	}
	_, ok := b.events[event]
	return ok
}

func (b *ToastBackend) Fire(n Notification) error {
	if !b.Supports(n.Event) {
		return fmt.Errorf("toast backend does not support event %q", n.Event)
	}
	if b.sender == nil {
		return fmt.Errorf("no toast sender configured")
	}

	title, body, err := formatToast(n)
	if err != nil {
		return err
	}
	if err := b.sender.Show(title, body); err != nil {
		return fmt.Errorf("show %s toast: %w", n.Event, err)
	}
	return nil
}

func formatToast(n Notification) (string, string, error) {
	body, ok := toastBodyByEvent[n.Event]
	if !ok {
		return "", "", fmt.Errorf("no toast text for event %q", n.Event)
	}

	title := "chime"
	if agent := strings.TrimSpace(n.Agent); agent != "" {
		title += " \u2014 " + agent
	}
	if message := strings.TrimSpace(n.Message); message != "" {
		body += ": " + message
	}

	return title, body, nil
}
