package composer

import (
	"strings"
	"testing"
)

// Piano 9 (decisione del coordinatore): il composer ha il campo Ccn, mostrato
// insieme a Cc con il toggle ctrl+e; nel Draft solo se visibile.

// TestCcnShownWithCcToggle: ctrl+e mostra Cc e Ccn, la navigazione li include.
func TestCcnShownWithCcToggle(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	m, _ = step(m, pressKey("ctrl+e"))
	if !m.ccVisible {
		t.Fatal("ctrl+e: Cc/Ccn non visibili")
	}
	if !strings.Contains(m.View(), "Ccn:") {
		t.Error("View senza etichetta Ccn con Cc visibile")
	}
	m.focus = FieldTo
	m, _ = step(m, pressKey("tab"))
	if m.focus != FieldCc {
		t.Errorf("tab da A: focus = %v, voglio Cc", m.focus)
	}
	m, _ = step(m, pressKey("tab"))
	if m.focus != FieldBcc {
		t.Errorf("tab da Cc: focus = %v, voglio Ccn", m.focus)
	}
	m, _ = step(m, pressKey("tab"))
	if m.focus != FieldSubject {
		t.Errorf("tab da Ccn: focus = %v, voglio Oggetto", m.focus)
	}
}

// TestCcnHiddenLeavesFocusOnTo: ctrl+e che nasconde Cc/Ccn porta il focus su A.
func TestCcnHiddenLeavesFocusOnTo(t *testing.T) {
	m := newModel(t, ComposerConfig{Cc: "c@x.dev", Bcc: "b@x.dev"})
	m, _ = step(m, pressKey("ctrl+e")) // nasconde
	m, _ = step(m, pressKey("ctrl+e")) // mostra
	m.focus = FieldBcc
	m.focusField(FieldBcc)
	m, _ = step(m, pressKey("ctrl+e")) // nasconde con focus su Ccn
	if m.focus != FieldTo {
		t.Errorf("focus = %v dopo nascondi, voglio A", m.focus)
	}
}

// TestDraftCarriesBccWhenHiddenToo: Ccn nel Draft se compilato, visibile o no.
func TestDraftCarriesBccWhenHiddenToo(t *testing.T) {
	// Un Ccn precompilato rende Cc/Ccn visibili (ccVisible in New): la bozza lo porta.
	m := newModel(t, ComposerConfig{To: "a@x.dev", Bcc: "b@x.dev"})
	if d := m.Draft(); len(d.Bcc) != 1 || d.Bcc[0] != "b@x.dev" {
		t.Errorf("Draft.Bcc = %v con Ccn visibile, voglio [b@x.dev]", d.Bcc)
	}
	// Nascosto con ctrl+e: il Ccn compilato resta nella bozza (Piano 9, review D).
	m, _ = step(m, pressKey("ctrl+e"))
	if d := m.Draft(); len(d.Bcc) != 1 || d.Bcc[0] != "b@x.dev" {
		t.Errorf("Draft.Bcc = %v con Ccn nascosto, voglio [b@x.dev]", d.Bcc)
	}
}

// TestEmptyMeansNoRecipientNoSubjectNoAttachmentNoBody: bozza vuota (D9-3).
func TestEmptyMeansNoRecipientNoSubjectNoAttachmentNoBody(t *testing.T) {
	if !newModel(t, ComposerConfig{}).Empty() {
		t.Error("composer vuoto: Empty() = false")
	}
	if newModel(t, ComposerConfig{To: "a@x.dev"}).Empty() {
		t.Error("solo A compilato: Empty() = true, voglio false (D9-3)")
	}
	if newModel(t, ComposerConfig{Bcc: "b@x.dev"}).Empty() {
		t.Error("solo Ccn compilato: Empty() = true")
	}
	if newModel(t, ComposerConfig{Attachments: []string{"/tmp/x"}}).Empty() {
		t.Error("solo allegato: Empty() = true")
	}
}
