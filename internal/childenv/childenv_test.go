package childenv

import (
	"slices"
	"testing"
)

// TestFilteredDropsSecrets: la password App e le GMAIL_TUI_* non passano mai ai
// comandi esterni; le altre variabili sì (merge Piani 9 e 11).
func TestFilteredDropsSecrets(t *testing.T) {
	t.Setenv("GMAIL_APP_PASSWORD", "segreto")
	t.Setenv("GMAIL_TUI_FROM", "x")
	t.Setenv("GMAIL_TUI_TEST_KEEP_OTHER", "x")
	t.Setenv("CHILDENV_TEST_VISIBLE", "ok")
	env := Filtered()
	for _, kv := range env {
		if kv == "GMAIL_APP_PASSWORD=segreto" || kv == "GMAIL_TUI_FROM=x" || kv == "GMAIL_TUI_TEST_KEEP_OTHER=x" {
			t.Errorf("variabile riservata passata al figlio: %q", kv)
		}
	}
	if !slices.Contains(env, "CHILDENV_TEST_VISIBLE=ok") {
		t.Error("variabile ordinaria assente dall'ambiente filtrato")
	}
}
