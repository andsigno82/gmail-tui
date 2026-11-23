package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTrace(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trace-x.jsonl")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Piano 16: `gmail-tui trace <file>` stampa il riepilogo e esce 0.
func TestTraceStampaRiepilogo(t *testing.T) {
	p := writeTrace(t, `{"t":1,"k":"meta","os":"darwin","arch":"arm64"}
{"t":2000,"k":"store","op":"Connect","dur_us":1500}
`)
	var out, errw bytes.Buffer
	if code := Trace(&out, &errw, []string{p}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errw.String())
	}
	if !strings.Contains(out.String(), "Connect") || !strings.Contains(out.String(), "darwin/arm64") {
		t.Errorf("riepilogo inatteso:\n%s", out.String())
	}
}

func TestTraceErrori(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Trace(&out, &errw, nil); code != 1 || !strings.Contains(errw.String(), "uso") {
		t.Errorf("senza argomenti: exit %d, stderr %q", code, errw.String())
	}
	errw.Reset()
	if code := Trace(&out, &errw, []string{filepath.Join(t.TempDir(), "manca.jsonl")}); code != 1 {
		t.Errorf("file mancante: exit %d", code)
	}
	errw.Reset()
	if code := Trace(&out, &errw, []string{writeTrace(t, "")}); code != 1 || errw.Len() == 0 {
		t.Errorf("file vuoto: exit %d, stderr %q", code, errw.String())
	}
}
