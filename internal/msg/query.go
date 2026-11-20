package msg

// Query è una ricerca: campi vuoti = filtro non attivo.
type Query struct {
	Text          string // testo libero (subject, from, corpo)
	From, To      string
	Label         string
	Unread        bool
	Starred       bool
	HasAttachment bool
}

// IsEmpty true se nessun filtro attivo.
func (q Query) IsEmpty() bool { return q == Query{} }
