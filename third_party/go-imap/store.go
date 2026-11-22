package imap

// StoreOptions contains options for the STORE command.
type StoreOptions struct {
	UnchangedSince uint64 // requires CONDSTORE
}

// StoreFlagsOp is a flag operation: set, add or delete.
type StoreFlagsOp int

const (
	StoreFlagsSet StoreFlagsOp = iota
	StoreFlagsAdd
	StoreFlagsDel
)

// StoreFlags alters message flags.
type StoreFlags struct {
	Op     StoreFlagsOp
	Silent bool
	Flags  []Flag
}

// StoreGmailLabels modifica le label Gmail X-GM-EXT-1 (Piano 8, Task 4, patch
// Gmail). Op: StoreFlagsSet sostituisce, StoreFlagsAdd aggiunge (+),
// StoreFlagsDel rimuove (-). Silent come in StoreFlags.
type StoreGmailLabels struct {
	Op     StoreFlagsOp
	Silent bool
	Labels []string
}
