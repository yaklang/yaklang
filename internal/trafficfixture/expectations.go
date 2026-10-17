package trafficfixture

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
)

// FrozenExpectations is the sealed inventory supplied with the baseline ZIP.
// Partial and unmapped statuses describe oracle scope, not execution success.
type FrozenExpectations struct {
	SchemaVersion int          `json:"schema_version"`
	SourceSHA     string       `json:"source_sha"`
	Cases         []FrozenCase `json:"cases"`
}
type Frame struct {
	Number      int    `json:"number"`
	LengthBytes int    `json:"length_bytes"`
	SHA256      string `json:"sha256"`
}
type FrozenCase struct {
	ID    string `json:"case_id"`
	Input struct {
		File         string `json:"file"`
		OriginalPath string `json:"original_path"`
		SHA256       string `json:"sha256"`
		Bytes        int64  `json:"bytes"`
	} `json:"input"`
	ExpectedStatus string        `json:"expected_status"`
	Expectations   []Expectation `json:"engine_expectations"`
	Facts          struct {
		PacketCount    int    `json:"packet_count"`
		LinkType       string `json:"link_type"`
		Representative *Frame `json:"representative_frame"`
	} `json:"capture_facts"`
	Missing []string `json:"missing"`
}
type Expectation struct {
	Kind               string          `json:"kind"`
	Protocol           string          `json:"protocol_name"`
	Frame              *Frame          `json:"selected_frame"`
	Selection          json.RawMessage `json:"input_selection"`
	Layer              string          `json:"layer"`
	Rule               string          `json:"rule_identifier"`
	Entry              string          `json:"entry_identifier"`
	PayloadConstraints json.RawMessage `json:"expected_payload_constraints"`
	Expected           struct {
		RequiredFields  []string                   `json:"required_field_names"`
		Values          map[string]json.RawMessage `json:"field_values"`
		FailureClass    string                     `json:"failure_class"`
		ErrorContains   string                     `json:"error_contains"`
		EventCounts     map[string]int             `json:"event_counts"`
		PacketCount     int                        `json:"packet_count"`
		FirstLayerError string                     `json:"first_layer_error_contains"`
	} `json:"expected"`
}

func Expectations() (*FrozenExpectations, error) {
	data, err := ReadBatch("baseline-2fb090d177c3", "validation/expected.json")
	if err != nil {
		return nil, err
	}
	var result FrozenExpectations
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AllExpectations reads the answers from every versioned batch that declares
// them. Supplemental reference-only batches do not invent semantic answers.
func AllExpectations() ([]*FrozenExpectations, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	c.expectationsOnce.Do(func() { c.expectations, c.expectationsErr = readAllExpectations(c) })
	if c.expectationsErr != nil {
		return nil, c.expectationsErr
	}
	return cloneExpectations(c.expectations), nil
}

// Archives and aliases are the immutable, hash-verified snapshot loaded by
// load(). Validate each answer inventory once, but never share its mutable
// projections with test callers.
func readAllExpectations(c *corpus) ([]*FrozenExpectations, error) {
	var results []*FrozenExpectations
	ids := map[string]bool{}
	for _, b := range c.index.Batches {
		if _, ok := b.Members["validation/expected.json"]; !ok {
			for name := range b.Members {
				if isCapture(name) {
					return nil, fmt.Errorf("batch %s: captures require an answer inventory", b.ID)
				}
			}
			continue
		}
		raw, err := readMember(b, "validation/expected.json")
		if err != nil {
			return nil, err
		}
		var answers FrozenExpectations
		if err := json.Unmarshal(raw, &answers); err != nil {
			return nil, err
		}
		source, sourceErr := hex.DecodeString(answers.SourceSHA)
		if answers.SchemaVersion != 1 || sourceErr != nil || len(source) != 20 || len(answers.Cases) == 0 {
			return nil, fmt.Errorf("batch %s: invalid answer inventory", b.ID)
		}
		paths := map[string]bool{}
		members := map[string]bool{}
		for _, item := range answers.Cases {
			loc, ok := c.aliases[item.Input.OriginalPath]
			if item.ID == "" || ids[item.ID] || paths[item.Input.OriginalPath] || !ok || loc.batch.ID != b.ID || loc.member != item.Input.File || b.Members[loc.member].SHA256 != item.Input.SHA256 || b.Members[loc.member].Bytes != item.Input.Bytes {
				return nil, fmt.Errorf("batch %s: answer is not bound to unique indexed input: %s", b.ID, item.ID)
			}
			switch item.ExpectedStatus {
			case "partial", "unmapped", "complete":
			default:
				return nil, fmt.Errorf("batch %s: unknown oracle scope for %s", b.ID, item.ID)
			}
			for _, e := range item.Expectations {
				switch e.Kind {
				case "selected_input_parse", "selected_input_rejection", "whole_capture_event_counts", "classification_only", "capture_structure", "source_manifest_constraints":
				default:
					return nil, fmt.Errorf("batch %s: unsupported assertion kind %q for %s", b.ID, e.Kind, item.ID)
				}
			}
			members[loc.member] = true
			ids[item.ID] = true
			paths[item.Input.OriginalPath] = true
		}
		for alias, member := range b.Aliases {
			if isCapture(member) && !paths[alias] {
				return nil, fmt.Errorf("batch %s: capture missing from answer inventory: %s", b.ID, alias)
			}
		}
		for member := range b.Members {
			if isCapture(member) && !members[member] {
				return nil, fmt.Errorf("batch %s: capture member has no executable input alias: %s", b.ID, member)
			}
		}
		results = append(results, &answers)
	}
	return results, nil
}

func isCapture(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".pcap", ".pcapng", ".cap":
		return true
	}
	return false
}

func cloneExpectations(source []*FrozenExpectations) []*FrozenExpectations {
	out := make([]*FrozenExpectations, len(source))
	for i, answers := range source {
		owned := *answers
		owned.Cases = make([]FrozenCase, len(answers.Cases))
		for j, item := range answers.Cases {
			copy := item
			copy.Missing = slices.Clone(item.Missing)
			if item.Facts.Representative != nil {
				frame := *item.Facts.Representative
				copy.Facts.Representative = &frame
			}
			if item.Expectations != nil {
				copy.Expectations = make([]Expectation, len(item.Expectations))
			}
			for k, expected := range item.Expectations {
				e := expected
				if expected.Frame != nil {
					frame := *expected.Frame
					e.Frame = &frame
				}
				e.Selection = slices.Clone(expected.Selection)
				e.PayloadConstraints = slices.Clone(expected.PayloadConstraints)
				e.Expected.RequiredFields = slices.Clone(expected.Expected.RequiredFields)
				if expected.Expected.Values != nil {
					e.Expected.Values = make(map[string]json.RawMessage, len(expected.Expected.Values))
					for key, value := range expected.Expected.Values {
						e.Expected.Values[key] = slices.Clone(value)
					}
				}
				if expected.Expected.EventCounts != nil {
					e.Expected.EventCounts = make(map[string]int, len(expected.Expected.EventCounts))
					for key, value := range expected.Expected.EventCounts {
						e.Expected.EventCounts[key] = value
					}
				}
				copy.Expectations[k] = e
			}
			owned.Cases[j] = copy
		}
		out[i] = &owned
	}
	return out
}
