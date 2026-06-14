//go:build !darwin

package notify

import "fmt"

type unsupportedSoundPlayer struct{}

func newPlatformSoundPlayer() soundPlayer {
	return unsupportedSoundPlayer{}
}

func (unsupportedSoundPlayer) Play(path string) error {
	return fmt.Errorf("sound playback is not implemented on this platform for %q", path)
}
