package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsDefaultsAndMissingPassword(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	code := Doctor(&out, filepath.Join(dir, "config.toml"), func(string) string { return "" })
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
	code := Doctor(&out, path, func(string) string { return "" })
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
	})
	if code != 0 {
		t.Fatalf("tutto ok atteso exit 0, avuto %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "theme: dracula ok") {
		t.Errorf("attesa riga theme ok, avuto:\n%s", out.String())
	}
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
