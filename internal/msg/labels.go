// Label utente (Piano 10, review punto 3 / Task 7): l'unica definizione di che cosa
// è una label utente. La usano app (annulla del cestino, pannello label), imap
// (X-GM-LABELS, cache) e mock, così nessun punto filtra le label a modo suo.
package msg

import "strings"

// systemNames sono le etichette di sistema di Gmail che X-GM-LABELS può dare senza
// backslash.
var systemNames = map[string]bool{"Important": true, "Starred": true, "Chats": true}

// IsUserLabel dice se name è una label utente, cioè un'etichetta che l'utente può
// applicare e rimuovere: non è vuota, non è un attributo di sistema (\Inbox, \Sent…),
// non è un'etichetta di sistema senza backslash, e non è INBOX né una cartella
// [Gmail]/…
func IsUserLabel(name string) bool {
	if name == "" || strings.HasPrefix(name, `\`) || systemNames[name] {
		return false
	}
	up := strings.ToUpper(name)
	return up != "INBOX" && !strings.HasPrefix(up, "[GMAIL]/")
}

// UserLabels restituisce le sole label utente di labels, nell'ordine dato.
func UserLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		if IsUserLabel(l) {
			out = append(out, l)
		}
	}
	return out
}
