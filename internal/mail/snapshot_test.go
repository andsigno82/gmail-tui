package mail

import (
	"errors"
	"testing"
)

// Piano 16, Task 2: il Mock non implementa Snapshotter, quindi la demo non ha
// istantanee per costruzione (D16-T1).
func TestMockIsNotSnapshotter(t *testing.T) {
	var s MailStore = &Mock{}
	if _, ok := s.(Snapshotter); ok {
		t.Fatal("il Mock non deve implementare Snapshotter: in demo nessuna istantanea")
	}
}

// Piano 16, Task 2: ErrNotConfirmed e una sentinella riconoscibile con errors.Is.
func TestErrNotConfirmedIsSentinel(t *testing.T) {
	if !errors.Is(ErrNotConfirmed, ErrNotConfirmed) || ErrNotConfirmed.Error() == "" {
		t.Fatal("ErrNotConfirmed deve essere un errore sentinella non vuoto")
	}
}
