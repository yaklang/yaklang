package ssadb

// MatchMode defines how to match (by name, key, both, or const type).
type MatchMode int

const (
	NameMatch MatchMode = 1
	KeyMatch  MatchMode = 1 << 1
	BothMatch MatchMode = NameMatch | KeyMatch
	ConstType MatchMode = 1 << 2
)

// CompareMode defines how to compare name/pattern (exact, glob, or regexp).
type CompareMode int

const (
	ExactCompare CompareMode = iota
	GlobCompare
	RegexpCompare
)
