package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewValidatesOptions(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want error
	}{
		{
			name: "missing base url",
			opts: Options{APIKey: "secret"},
			want: ErrInvalidConfig,
		},
		{
			name: "invalid base url",
			opts: Options{BaseURL: "://bad", APIKey: "secret"},
			want: ErrInvalidConfig,
		},
		{
			name: "unsupported scheme",
			opts: Options{BaseURL: "ftp://example.com", APIKey: "secret"},
			want: ErrInvalidConfig,
		},
		{
			name: "missing host",
			opts: Options{BaseURL: "http:///notify", APIKey: "secret"},
			want: ErrInvalidConfig,
		},
		{
			name: "missing key",
			opts: Options{BaseURL: "http://example.com"},
			want: ErrAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if !errors.Is(err, tt.want) {
				t.Fatalf("New() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNotifySendsRequest(t *testing.T) {
	const apiKey = "secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/notify" {
			t.Fatalf("path = %s, want /notify", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("accept = %q, want application/json", got)
		}

		var got Notification
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		want := Notification{Event: "complete", Agent: "claude-code", Message: "done"}
		if got != want {
			t.Fatalf("notification = %+v, want %+v", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"event":"complete"}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL + "/", APIKey: apiKey})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	err = c.Notify(context.Background(), Notification{
		Event:   "complete",
		Agent:   "claude-code",
		Message: "done",
	})
	if err != nil {
		t.Fatalf("Notify(): %v", err)
	}
}

func TestNotifyClassifiesResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
			body:   `{"ok":false,"error":"unauthorized"}`,
			want:   ErrAuth,
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
			body:   `{"ok":false,"error":"internal server error"}`,
			want:   ErrUnavailable,
		},
		{
			name:   "bad success json",
			status: http.StatusOK,
			body:   `{`,
			want:   ErrInvalidResponse,
		},
		{
			name:   "negative success envelope",
			status: http.StatusOK,
			body:   `{"ok":false,"error":"nope"}`,
			want:   ErrInvalidResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const apiKey = "supersecret"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c, err := New(Options{BaseURL: srv.URL, APIKey: apiKey})
			if err != nil {
				t.Fatalf("New(): %v", err)
			}
			err = c.Notify(context.Background(), Notification{Event: "complete"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Notify() error = %v, want %v", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), apiKey) {
				t.Fatalf("error includes api key: %v", err)
			}
		})
	}
}

func TestNotifyClassifiesTransportFailure(t *testing.T) {
	c, err := New(Options{
		BaseURL: "http://example.com",
		APIKey:  "secret",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		},
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	err = c.Notify(context.Background(), Notification{Event: "complete"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Notify() error = %v, want %v", err, ErrUnavailable)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
