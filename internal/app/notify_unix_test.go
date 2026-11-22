//go:build unix

package app

// Piano 11, review punto 4: allo scadere del timeout muore l'intero gruppo di
// processi del comando, non solo il figlio diretto. Il test usa `sh` solo come
// fixture di test (un figlio che dorme); in produzione nessuna shell.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunNotifyTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	argv := []string{"sh", "-c", "sleep 30 & echo $! > " + pidFile + "; wait"}
	if err := runNotifyTimeout(context.Background(), argv, nil, 1500*time.Millisecond); err == nil {
		t.Fatal("timeout: atteso errore")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("pidfile del figlio: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("pid non valido %q: %v", raw, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return // il figlio è morto con il gruppo
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("il figlio %d sopravvive al timeout del comando", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
