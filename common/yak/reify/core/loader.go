package core

import "context"

// ProbeSize is the number of leading bytes available to format matching.
const ProbeSize = 64

type LoaderInfo struct {
	Kind        Format `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CheckResult is the metadata produced by Loader.Check. It validates a full
// header without building an artifact or decoding instructions.
type CheckResult struct {
	Kind        InputKind    `json:"kind"`
	Format      Format       `json:"format"`
	Target      Target       `json:"target"`
	ObjectType  string       `json:"object_type,omitempty"`
	Sections    int          `json:"sections"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Loader interprets one file format. Kind aligns a loader with a Format; OS and
// architecture are properties of the loaded target, not loader identities.
// Match selects a candidate from a read-only prefix; Check validates the full
// header; Load builds the artifact.
type Loader interface {
	Kind() Format
	Name() string
	Description() string
	Match(prefix []byte) bool
	Check(*ByteReader) (*CheckResult, error)
	Load(context.Context, *ByteReader) (*Artifact, error)
}
