package app

// TraceScreen e la vista corrente come stringa COSTANTE per la traccia dei tempi
// (Piano 16, internal/trace/tracetea): mai dati dell'utente. Il wrapper del
// modello la legge dopo ogni Update per attribuire i tempi alla vista.
func (m Model) TraceScreen() string {
	switch m.view {
	case viewInbox:
		return "inbox"
	case viewThread:
		return "thread"
	case viewSearch:
		return "search"
	case viewComposer:
		return "composer"
	case viewWizard:
		return "wizard"
	}
	return ""
}
