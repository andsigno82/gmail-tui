// Package contacts è la rubrica locale per l'autocompletamento di A/Cc (Piano 9,
// Task 8): indirizzi dei messaggi caricati (mittenti, destinatari) e di quelli
// inviati, ordinati per frequenza e poi per recenza. Persistenza su file JSON a
// 0600 con scrittura atomica; path "" = solo memoria (demo, test).
package contacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Suggestion è un contatto proposto nel composer.
type Suggestion struct {
	Addr  string    `json:"addr"`
	Name  string    `json:"name,omitempty"`
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

// Book è la rubrica. Non è sicura per uso concorrente: la radice la usa solo dal
// loop UI (le scritture su disco sono brevi e atomiche).
type Book struct {
	path    string
	entries map[string]*Suggestion // chiave: indirizzo in minuscolo
}

// Open apre la rubrica nel file dato (creato al primo Observe). Un file
// illeggibile o non JSON è un errore: non si sostituisce in silenzio con una
// rubrica vuota. path "" = solo memoria.
func Open(path string) (*Book, error) {
	b := &Book{path: path, entries: map[string]*Suggestion{}}
	if path == "" {
		return b, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("rubrica: lettura %s: %w", path, err)
	}
	var list []Suggestion
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("rubrica: %s non valido: %w", path, err)
	}
	for i := range list {
		s := list[i]
		b.entries[strings.ToLower(s.Addr)] = &s
	}
	return b, nil
}

// Observe registra i contatti people ("Nome <addr>" o "addr"); le voci non
// valide sono ignorate. bump: incrementa il conteggio e aggiorna la recenza
// (destinatari di un messaggio aperto, invii); senza bump un contatto nuovo
// entra con Count 0 (mittenti dalla lista, che si rilegge a ogni refresh).
// Con path la rubrica viene riscritta se qualcosa è cambiato.
func (b *Book) Observe(now time.Time, bump bool, people ...string) error {
	changed := false
	for _, p := range people {
		a, err := mail.ParseAddress(p)
		if err != nil {
			continue
		}
		key := strings.ToLower(a.Address)
		e, ok := b.entries[key]
		if !ok {
			e = &Suggestion{Addr: a.Address}
			b.entries[key] = e
			e.Last = now
			changed = true
		}
		if a.Name != "" && e.Name != a.Name {
			e.Name = a.Name
			changed = true
		}
		if bump {
			e.Count++
			e.Last = now
			changed = true
		}
	}
	if !changed || b.path == "" {
		return nil
	}
	return b.save()
}

// Suggest restituisce al più limit contatti la cui query è sottostringa (case
// insensitive) di indirizzo o nome. Query vuota: nessun suggerimento.
func (b *Book) Suggest(query string, limit int) []Suggestion {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return nil
	}
	var out []Suggestion
	for _, e := range b.entries {
		if strings.Contains(strings.ToLower(e.Addr), q) || strings.Contains(strings.ToLower(e.Name), q) {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Addr < out[j].Addr
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// save riscrive il file in modo atomico: temporaneo nella stessa directory
// (0600), poi rename.
func (b *Book) save() error {
	list := make([]Suggestion, 0, len(b.entries))
	for _, e := range b.entries {
		list = append(list, *e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("rubrica: codifica: %w", err)
	}
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("rubrica: directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".contacts-*.json")
	if err != nil {
		return fmt.Errorf("rubrica: file temporaneo: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op dopo il rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("rubrica: permessi: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("rubrica: scrittura: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("rubrica: chiusura: %w", err)
	}
	if err := os.Rename(tmp.Name(), b.path); err != nil {
		return fmt.Errorf("rubrica: rinomina: %w", err)
	}
	return nil
}
