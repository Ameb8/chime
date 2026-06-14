//go:build !darwin

package notify

import "fmt"

type unsupportedToastSender struct{}

func newPlatformToastSender() toastSender {
	return unsupportedToastSender{}
}

func (unsupportedToastSender) Show(title, body string) error {
	return fmt.Errorf("toast notifications are not implemented on this platform for %q: %q", title, body)
}
