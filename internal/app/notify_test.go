package app

// Piano 11, Task 3: sanitizzazione, env e esecuzione senza shell del comando di
// notifica. Nessuna shell, nessuna dipendenza esterna: i binari usati sono
// `true`, `false`, `sleep` (gated per OS) e il test binary stesso.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

func TestSanitizeNotifyTextStripsANSIAndControls(t *testing.T) {
	in := "\x1b[31mRossi\x1b[0m\n\u202eeuro"
	if got := sanitizeNotifyText(in, 100); got != "Rossi euro" {
		t.Fatalf("sanitize = %q, voglio %q", got, "Rossi euro")
	}
}

func TestSanitizeNotifyTextStripsOSCSequence(t *testing.T) {
	in := "\x1b]0;titolo\x07Ciao"
	if got := sanitizeNotifyText(in, 100); got != "Ciao" {
		t.Fatalf("sanitize OSC = %q, voglio %q", got, "Ciao")
	}
}

func TestSanitizeNotifyTextCollapsesSpaces(t *testing.T) {
	if got := sanitizeNotifyText("  a \t b  ", 100); got != "a b" {
		t.Fatalf("sanitize spazi = %q, voglio %q", got, "a b")
	}
}

func TestSanitizeNotifyTextTruncatesToMaxRunes(t *testing.T) {
	// max conta anche l'ellissi: il risultato ha esattamente max rune.
	if got := sanitizeNotifyText("abcdef", 4); got != "abc…" {
		t.Fatalf("troncamento = %q, voglio %q", got, "abc…")
	}
	if got := sanitizeNotifyText("abcd", 4); got != "abcd" {
		t.Fatalf("lunghezza esatta non va troncata: %q", got)
	}
	if got := sanitizeNotifyText("", 4); got != "" {
		t.Fatalf("stringa vuota = %q", got)
	}
}

// envValue restituisce il valore di key in env (formato KEY=VALUE).
func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func TestNotifyEnvCarriesDataAsVariables(t *testing.T) {
	refs := []msg.MessageRef{{From: "Ada <ada@example.org>", Subject: "Ciao\x1b[2J"}}
	env := notifyEnv(refs, 2)
	cases := map[string]string{
		"GMAIL_TUI_FROM":    "Ada <ada@example.org>",
		"GMAIL_TUI_SUBJECT": "Ciao",
		"GMAIL_TUI_COUNT":   "2",
		"GMAIL_TUI_FOLDER":  "INBOX",
	}
	for k, want := range cases {
		got, ok := envValue(env, k)
		if !ok {
			t.Errorf("%s assente nell'env", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, voglio %q", k, got, want)
		}
	}
}

func TestNotifyEnvWithoutRefsIsEmptyButPresent(t *testing.T) {
	env := notifyEnv(nil, 3)
	if v, ok := envValue(env, "GMAIL_TUI_SUBJECT"); !ok || v != "" {
		t.Fatalf("senza Refs: SUBJECT atteso presente e vuoto, avuto (%q, %v)", v, ok)
	}
	if v, _ := envValue(env, "GMAIL_TUI_COUNT"); v != "3" {
		t.Fatalf("COUNT = %q, voglio 3", v)
	}
}

func TestRunNotifyTrueSucceeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binario true non disponibile su Windows")
	}
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true non nel PATH")
	}
	if err := runNotify(context.Background(), []string{"true"}, nil); err != nil {
		t.Fatalf("true: errore inatteso %v", err)
	}
}

func TestRunNotifyFailingCommandReportsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binario false non disponibile su Windows")
	}
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false non nel PATH")
	}
	if err := runNotify(context.Background(), []string{"false"}, nil); err == nil {
		t.Fatal("exit diverso da 0: atteso errore")
	}
}

func TestRunNotifyMissingBinaryReportsError(t *testing.T) {
	if err := runNotify(context.Background(), []string{"gmail-tui-no-such-binary-piano11"}, nil); err == nil {
		t.Fatal("eseguibile inesistente: atteso errore")
	}
}

func TestRunNotifyTimeoutKillsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep non disponibile su Windows")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep non nel PATH")
	}
	start := time.Now()
	err := runNotifyTimeout(context.Background(), []string{"sleep", "10"}, nil, 200*time.Millisecond)
	if err == nil {
		t.Fatal("timeout: atteso errore")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("timeout non applicato: %v", el)
	}
}

// Helper process: il test binary rieseguito con GMAIL_TUI_TEST_HELPER=1 scrive
// su file il valore di GMAIL_TUI_SUBJECT che ha ricevuto nell'ambiente. Nessuno
// script esterno e nessuna shell: prova che i dati arrivano SOLO via env.
func TestNotifyHelperProcess(t *testing.T) {
	if os.Getenv("GMAIL_TUI_TEST_HELPER") != "1" {
		t.Skip("solo come processo helper")
	}
	_ = os.WriteFile(os.Getenv("GMAIL_TUI_TEST_OUT"), []byte(os.Getenv("GMAIL_TUI_SUBJECT")), 0o600)
}

func TestRunNotifyPassesDataOnlyInEnv(t *testing.T) {
	out := filepath.Join(t.TempDir(), "subject.txt")
	refs := []msg.MessageRef{{From: "Mallory; rm -rf $HOME", Subject: "$(touch /tmp/pwned) `id`"}}
	env := append(notifyEnv(refs, 1), "GMAIL_TUI_TEST_HELPER=1", "GMAIL_TUI_TEST_OUT="+out)
	argv := []string{os.Args[0], "-test.run=^TestNotifyHelperProcess$"}
	if err := runNotify(context.Background(), argv, env); err != nil {
		t.Fatalf("helper: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("il helper non ha scritto il file: %v", err)
	}
	if string(data) != "$(touch /tmp/pwned) `id`" {
		t.Fatalf("oggetto ricevuto = %q: l'env deve arrivare intatto (solo sanitizzato)", data)
	}
	// argv non contiene i dati del messaggio: solo l'eseguibile e i flag fissi.
	for _, a := range argv {
		if strings.Contains(a, "Mallory") || strings.Contains(a, "pwned") {
			t.Fatalf("argv contiene dati del messaggio: %q", a)
		}
	}
}
