package asyncexecmanager

type Request struct {
	// ID may be supplied by the caller (e.g. its tool call id). Empty generates
	// a unique id. Duplicate ids are rejected, never silently reused.
	ID       string
	Metadata Metadata
	Resource Resource
}
