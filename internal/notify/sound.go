package notify

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Ameb8/chime/assets"
	"github.com/Ameb8/chime/internal/paths"
)

type SoundOptions struct {
	Enabled       bool
	Events        []Event
	CompleteSound string
	WaitingSound  string
}

type SoundBackend struct {
	enabled bool
	events  map[Event]struct{}
	paths   map[Event]string

	player soundPlayer
	store  soundFileStore
}

type soundPlayer interface {
	Play(path string) error
}

type soundFileStore interface {
	SoundPath(filename string, data []byte) (string, error)
}

type embeddedSound struct {
	filename string
	data     []byte
}

type cachedSoundStore struct {
	dir string
	mu  sync.Mutex
}

var defaultSounds = map[Event]embeddedSound{
	EventComplete: {
		filename: "complete.aiff",
		data:     assets.CompleteSound,
	},
	EventWaiting: {
		filename: "waiting.aiff",
		data:     assets.WaitingSound,
	},
}

func NewSoundBackend(options SoundOptions) *SoundBackend {
	return newSoundBackend(
		options,
		newPlatformSoundPlayer(),
		&cachedSoundStore{dir: filepath.Join(paths.DataDir(), "sounds")},
	)
}

func newSoundBackend(options SoundOptions, player soundPlayer, store soundFileStore) *SoundBackend {
	events := make(map[Event]struct{}, len(options.Events))
	for _, event := range append([]Event(nil), options.Events...) {
		if _, ok := defaultSounds[event]; ok {
			events[event] = struct{}{}
		}
	}

	return &SoundBackend{
		enabled: options.Enabled,
		events:  events,
		paths: map[Event]string{
			EventComplete: options.CompleteSound,
			EventWaiting:  options.WaitingSound,
		},
		player: player,
		store:  store,
	}
}

func (b *SoundBackend) Name() string {
	return "sound"
}

func (b *SoundBackend) Supports(event Event) bool {
	if !b.enabled {
		return false
	}
	_, ok := b.events[event]
	return ok
}

func (b *SoundBackend) Fire(n Notification) error {
	if !b.Supports(n.Event) {
		return fmt.Errorf("sound backend does not support event %q", n.Event)
	}

	path, err := b.pathForEvent(n.Event)
	if err != nil {
		return err
	}
	if err := b.player.Play(path); err != nil {
		return fmt.Errorf("play %s sound %q: %w", n.Event, path, err)
	}
	return nil
}

func (b *SoundBackend) pathForEvent(event Event) (string, error) {
	if path := b.paths[event]; path != "" {
		return path, nil
	}

	sound, ok := defaultSounds[event]
	if !ok {
		return "", fmt.Errorf("no default sound for event %q", event)
	}
	if b.store == nil {
		return "", fmt.Errorf("no sound file store configured")
	}

	path, err := b.store.SoundPath(sound.filename, sound.data)
	if err != nil {
		return "", fmt.Errorf("prepare default %s sound: %w", event, err)
	}
	return path, nil
}

func (s *cachedSoundStore) SoundPath(filename string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("create sound cache directory: %w", err)
	}

	path := filepath.Join(s.dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write sound cache file: %w", err)
	}
	return path, nil
}
