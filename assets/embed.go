package assets

import _ "embed"

//go:embed sounds/complete.aiff
var CompleteSound []byte

//go:embed sounds/waiting.aiff
var WaitingSound []byte
