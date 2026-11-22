package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failCloser struct{}

func (failCloser) Close() error { return errors.New("chiusura fallita") }

// Piano 14, fix review 7: l'errore di chiusura dello store finisce su stderr.
func TestCloseStoreLogsError(t *testing.T) {
	var stderr bytes.Buffer
	closeStoreLogged(failCloser{}, &stderr)
	if !strings.Contains(stderr.String(), "chiusura fallita") {
		t.Fatalf("errore di chiusura non registrato: %q", stderr.String())
	}
}
