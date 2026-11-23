package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Revisione Piano 15, punto 8: la radice degli allegati aperti non sta nel /tmp
// condiviso ma in una cartella privata dell'utente.

// Con XDG_RUNTIME_DIR (cartella per-utente 0700 del sistema) la radice e li.
func TestRadiceAllegatiInXDGRuntimeDir(t *testing.T) {
	rt := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", rt)
	want := filepath.Join(rt, "gmail-tui", "open")
	if got := defaultAttachRoot(); got != want {
		t.Errorf("radice = %q, voglio %q", got, want)
	}
}

// Senza XDG_RUNTIME_DIR: os.UserCacheDir()/gmail-tui/open, mai /tmp condiviso.
func TestRadiceAllegatiInUserCacheDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("UserCacheDir: %v", err)
	}
	want := filepath.Join(cache, "gmail-tui", "open")
	got := defaultAttachRoot()
	if got != want {
		t.Errorf("radice = %q, voglio %q", got, want)
	}
	if strings.HasPrefix(got, filepath.Join(os.TempDir(), attachRootName)) {
		t.Errorf("radice nel tmp condiviso: %q", got)
	}
}

// XDG_RUNTIME_DIR relativo o inesistente non si usa (non e una cartella fidata).
func TestRadiceAllegatiIgnoraXDGRuntimeDirInvalido(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("UserCacheDir: %v", err)
	}
	want := filepath.Join(cache, "gmail-tui", "open")
	for _, v := range []string{"relativo/dir", filepath.Join(t.TempDir(), "non-esiste")} {
		t.Setenv("XDG_RUNTIME_DIR", v)
		if got := defaultAttachRoot(); got != want {
			t.Errorf("XDG_RUNTIME_DIR=%q: radice = %q, voglio %q", v, got, want)
		}
	}
}

// La radice e i suoi contenitori si creano 0700 e una radice con permessi piu
// larghi si riporta a 0700.
func TestEnsureAttachRootPermessi(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "gmail-tui", "open")
	if err := ensureAttachRoot(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Dir(root), root} {
		if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v, voglio 0700", p, st, err)
		}
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureAttachRoot(root); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(root); st.Mode().Perm() != 0o700 {
		t.Errorf("permessi = %v, voglio 0700", st.Mode().Perm())
	}
}

// SweepAttachments ripulisce anche la vecchia radice nel tmp condiviso (versioni
// precedenti): sessioni s-* vecchie tolte, il resto no.
func TestSweepLegacyRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.TempDir su Windows non usa TMPDIR")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	old := filepath.Join(tmp, attachRootName, attachSessPrefix+"vecchia")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	longAgo := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, longAgo, longAgo); err != nil {
		t.Fatal(err)
	}
	orig := attachRoot
	attachRoot = func() string { return filepath.Join(t.TempDir(), "nuova") }
	t.Cleanup(func() { attachRoot = orig })

	SweepAttachments()
	if exists(old) {
		t.Errorf("sessione vecchia nella radice legacy non rimossa")
	}
}
