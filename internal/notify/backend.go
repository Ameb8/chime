package notify

type Event string

const (
	EventComplete Event = "complete"
	EventWaiting  Event = "waiting"
)

type Notification struct {
	Event   Event
	Agent   string
	Message string
}

type Backend interface {
	Name() string
	Supports(event Event) bool
	Fire(n Notification) error
}
