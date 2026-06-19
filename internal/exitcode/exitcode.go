package exitcode

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

type SilentError struct {
	Code int
}

func (e *SilentError) Error() string {
	return ""
}
