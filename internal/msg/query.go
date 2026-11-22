package msg

import "strings"

// Query è una ricerca: campi vuoti = filtro non attivo.
type Query struct {
	Text          string // testo libero (subject, from, corpo)
	From, To      string
	Label         string
	Unread        bool
	Starred       bool
	HasAttachment bool

	// Raw (Piano 8, Task 5): sintassi di ricerca Gmail completa, passata
	// verbatim a X-GM-RAW dal backend Gmail. Vuota = non attiva.
	Raw string
}

// IsEmpty true se nessun filtro attivo.
func (q Query) IsEmpty() bool { return q == Query{} }

// ParseQuery traduce una query testuale stile Gmail (usata da
// `gmail-tui fetch --query`) in Query. Token riconosciuti (chiave:valore):
//
//	from:x          mittente (l'ultimo vince)
//	to:x            destinatario (l'ultimo vince)
//	label:x         label/cartella (l'ultima vince)
//	is:unread       solo non letti
//	is:starred      solo con stella
//	has:attachment  solo con allegati
//
// Tutto il resto — INCLUSE le coppie chiave:valore con chiave (o valore "is"/
// "has") non riconosciuta, comportamento permissivo deliberato — resta testo
// libero: i token di testo si accumulano in Text separati da spazio.
// La stringa vuota produce Query{} (nessun filtro).
func ParseQuery(s string) Query {
	var q Query
	var text []string
	for _, tok := range strings.Fields(s) {
		key, val, hasValue := strings.Cut(tok, ":")
		if !hasValue {
			text = append(text, tok)
			continue
		}
		switch key {
		case "from":
			q.From = val
		case "to":
			q.To = val
		case "label":
			q.Label = val
		case "is":
			switch val {
			case "unread":
				q.Unread = true
			case "starred":
				q.Starred = true
			default:
				text = append(text, tok) // is:<valore ignoto> → testo
			}
		case "has":
			if val == "attachment" {
				q.HasAttachment = true
			} else {
				text = append(text, tok) // has:<valore ignoto> → testo
			}
		default:
			text = append(text, tok) // chiave ignota → testo (permissivo)
		}
	}
	q.Text = strings.Join(text, " ")
	return q
}
