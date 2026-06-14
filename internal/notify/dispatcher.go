package notify

import (
	"context"
	"log/slog"
	"sync"
)

type Dispatcher struct {
	backends []Backend
}

func NewDispatcher(backends []Backend) *Dispatcher {
	return &Dispatcher{
		backends: append([]Backend(nil), backends...),
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, n Notification) error {
	_ = ctx

	var wg sync.WaitGroup
	for _, b := range d.backends {
		if !b.Supports(n.Event) {
			continue
		}

		wg.Add(1)
		go func(b Backend) {
			defer wg.Done()

			if err := b.Fire(n); err != nil {
				slog.Error(
					"backend fire error",
					"backend", b.Name(),
					"event", string(n.Event),
					"err", err,
				)
			}
		}(b)
	}

	wg.Wait()
	return nil
}
