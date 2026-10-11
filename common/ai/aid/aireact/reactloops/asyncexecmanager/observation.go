package asyncexecmanager

import "encoding/json"

// Observation belongs to the resource, not the manager. Output limits and
// intermediate result semantics are implemented by each inspector. The manager
// does not infer structured results from log text or duplicate output streams.
type Observation struct {
	Stdout        string          `json:"stdout"`
	Stderr        string          `json:"stderr"`
	Result        json.RawMessage `json:"result,omitempty"`
	PartialResult json.RawMessage `json:"partial_result,omitempty"`
	Truncated     bool            `json:"truncated"`
}
