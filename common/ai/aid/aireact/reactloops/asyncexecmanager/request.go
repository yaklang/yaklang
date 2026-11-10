package asyncexecmanager

type Request struct {
	// ID may be supplied by the caller (e.g. its tool call id). Empty generates
	// a unique id. Duplicate retained ids are rejected. Once the total-record
	// limit evicts a terminal record, its id may be used again.
	ID       string
	Metadata Metadata
	Resource Resource
}
