package app

// Piano 11, Task 5: voce di sola lettura "Notifiche desktop" in fondo al menu `,`.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// La voce mostra lo stato del comando dalla config: non configurato.
func TestSettingsNotifyRowShowsNotConfigured(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	view := plainText(mm.settingsView())
	if !strings.Contains(view, "Notifiche desktop") {
		t.Fatal("voce \"Notifiche desktop\" assente nel menu")
	}
	if !strings.Contains(view, "non configurate") {
		t.Errorf("senza comando: atteso \"non configurate\" nel menu")
	}
}

// Con notify_command impostato la voce mostra "comando configurato".
func TestSettingsNotifyRowShowsConfigured(t *testing.T) {
	m, _ := settingsModel(t)
	m.SetNotify([]string{"notify-cmd"})
	mm, _ := step(m, pressKey(","))
	view := plainText(mm.settingsView())
	if !strings.Contains(view, "comando configurato") {
		t.Fatal("con comando: atteso \"comando configurato\" nel menu")
	}
}

// La voce è di sola lettura: le frecce non cambiano le bozze del menu.
func TestSettingsNotifyRowIsReadOnly(t *testing.T) {
	m, _ := settingsModel(t)
	m.SetNotify([]string{"notify-cmd"})
	mm, _ := step(m, pressKey(","))
	for i := 0; i < 5; i++ {
		mm, _ = step(mm, keyDown)
	}
	before := mm.menu
	mm, _ = step(mm, keyRight)
	mm, _ = step(mm, tea.KeyPressMsg{Code: tea.KeyLeft})
	if !mm.menu.open {
		t.Fatal("le frecce sulla voce notifiche non devono chiudere il menu")
	}
	if mm.menu.markRead != before.markRead || mm.menu.name.Value() != before.name.Value() {
		t.Errorf("le frecce sulla voce notifiche non devono cambiare le bozze")
	}
}
