package dockerhttp

import (
	"encoding/json"
	"net/url"
)

// Filters is a map of filter key → set of values, encoded the way the Engine
// API expects: filters={"label":["a=b"]} as a single query parameter.
type Filters map[string][]string

// Add appends a value for key.
func (f Filters) Add(key, value string) {
	f[key] = append(f[key], value)
}

// Encode returns the JSON encoding used in ?filters=...
func (f Filters) Encode() (string, error) {
	if len(f) == 0 {
		return "", nil
	}
	b, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// setFilters adds the filters query param if non-empty.
func setFilters(q url.Values, f Filters) error {
	if len(f) == 0 {
		return nil
	}
	enc, err := f.Encode()
	if err != nil {
		return err
	}
	q.Set("filters", enc)
	return nil
}
