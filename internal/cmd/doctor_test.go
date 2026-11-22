package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorReportsDefaultsAndMissingPassword(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	code := Doctor(&out, filepath.Join(dir, "config.toml"), func(string) string { return "" }, nil)
	if code != 0 {
		t.Fatalf("config assente non è errore fatale: code=%d", code)
	}
	s := out.String()
	if !strings.Contains(s, "defaults") {
		t.Errorf("attesa riga config defaults, avuto:\n%s", s)
	}
	if !strings.Contains(s, "password: missing") {
		t.Errorf("attesa riga password missing, avuto:\n%s", s)
	}
}

func TestDoctorReportsBadTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfgToml := "address = \"a@example.com\"\ntheme = \"non-esiste\"\n"
	if err := osWriteFile(path, cfgToml); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Doctor(&out, path, func(string) string { return "" }, nil)
	if code != 1 {
		t.Fatalf("tema rotto deve dare exit 1, avuto %d", code)
	}
	if !strings.Contains(out.String(), "theme non-esiste:") {
		t.Errorf("atteso dettaglio tema rotto, avuto:\n%s", out.String())
	}
}

func TestDoctorOKWithEnvPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := osWriteFile(path, "address = \"a@example.com\"\ntheme = \"dracula\"\n"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Doctor(&out, path, func(k string) string {
		if k == "GMAIL_APP_PASSWORD" {
			return "xxxx xxxx xxxx xxxx"
		}
		return ""
	}, nil)
	if code != 0 {
		t.Fatalf("tutto ok atteso exit 0, avuto %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "theme: dracula ok") {
		t.Errorf("attesa riga theme ok, avuto:\n%s", out.String())
	}
}

// capsProbe è un CapsProbe finto: restituisce caps/err e registra la chiamata.
func capsProbe(caps []string, err error) (CapsProbe, *bool) {
	called := false
	return func(context.Context) ([]string, error) {
		called = true
		return caps, err
	}, &called
}

// allGmailCaps sono le capability che doctor richiede (Piano 7.1, Task 4).
var allGmailCaps = []string{"MOVE", "UIDPLUS", "SPECIAL-USE", "LIST-EXTENDED", "IMAP4rev1"}

// TestDoctorIMAPCapsOK: tutte le capability richieste ⇒ riga ok, exit 0.
func TestDoctorIMAPCapsOK(t *testing.T) {
	probe, called := capsProbe(allGmailCaps, nil)
	var out bytes.Buffer
	code := Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" }, probe)
	if !*called {
		t.Fatal("probe non chiamato")
	}
	if !strings.Contains(out.String(), "imap: ok (MOVE UIDPLUS SPECIAL-USE LIST-EXTENDED)") {
		t.Errorf("attesa riga imap ok, avuto:\n%s", out.String())
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

// TestDoctorIMAPMissingCap: una capability mancante è bloccante ⇒ exit 1.
func TestDoctorIMAPMissingCap(t *testing.T) {
	probe, _ := capsProbe([]string{"MOVE", "UIDPLUS", "LIST-EXTENDED"}, nil)
	var out bytes.Buffer
	code := Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" }, probe)
	if !strings.Contains(out.String(), "imap: MANCA SPECIAL-USE") {
		t.Errorf("attesa riga MANCA SPECIAL-USE, avuto:\n%s", out.String())
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// TestDoctorIMAPError: errore di connessione ⇒ riga ERRORE, exit 1.
func TestDoctorIMAPError(t *testing.T) {
	probe, _ := capsProbe(nil, errors.New("boom"))
	var out bytes.Buffer
	code := Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" }, probe)
	if !strings.Contains(out.String(), "imap: ERRORE connessione: boom") {
		t.Errorf("attesa riga ERRORE connessione, avuto:\n%s", out.String())
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// TestDoctorIMAPNilProbeSkips: probe nil ⇒ nessuna riga imap (controllo saltato).
func TestDoctorIMAPNilProbeSkips(t *testing.T) {
	var out bytes.Buffer
	Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" }, nil)
	if strings.Contains(out.String(), "imap:") {
		t.Errorf("probe nil: riga imap inattesa:\n%s", out.String())
	}
}

// TestDoctorIMAPTimeout: il probe che non risponde entro il budget ⇒ errore di
// connessione (non blocca doctor).
func TestDoctorIMAPTimeout(t *testing.T) {
	old := probeTimeout
	probeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { probeTimeout = old })

	hang := func(ctx context.Context) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var out bytes.Buffer
	code := Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(string) string { return "" }, hang)
	if !strings.Contains(out.String(), "imap: ERRORE connessione") || code != 1 {
		t.Errorf("timeout: code=%d, output:\n%s", code, out.String())
	}
}

// TestDoctorIMAPNeverPrintsPassword: la password (env) non compare mai nel report,
// nemmeno quando il probe fallisce.
func TestDoctorIMAPNeverPrintsPassword(t *testing.T) {
	const secret = "p4ss-SEGRETA-xyz"
	probe, _ := capsProbe(nil, errors.New("login fallito"))
	var out bytes.Buffer
	Doctor(&out, filepath.Join(t.TempDir(), "config.toml"), func(k string) string {
		if k == "GMAIL_APP_PASSWORD" {
			return secret
		}
		return ""
	}, probe)
	if strings.Contains(out.String(), secret) {
		t.Errorf("password nel report:\n%s", out.String())
	}
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
