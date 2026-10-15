package builtinmetadata

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SourceString is [metadata byte offset, source byte offset, source byte length].
// It replaces a JSON string value with "" in the template. Source hashes bind
// every range to the exact script, so documentation need not be stored twice.
type SourceString [3]int

func NewEntry(script string, metadata Metadata, source string) (Entry, error) {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Script: script}
	previous := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		start := i
		i++
		for raw[i] != '"' {
			if raw[i] == '\\' {
				i++
			}
			i++
		}
		end := i + 1
		if end < len(raw) && raw[end] == ':' {
			continue
		} // Keep object keys readable.
		var value string
		if err := json.Unmarshal(raw[start:end], &value); err != nil {
			return Entry{}, err
		}
		// Below this size the reference itself costs as much as a literal.
		if len(value) < 24 {
			continue
		}
		offset := strings.Index(source, value)
		if offset < 0 {
			continue
		} // Computed/escaped values stay literal.
		e.Metadata = append(e.Metadata, raw[previous:start]...)
		e.SourceStrings = append(e.SourceStrings, SourceString{len(e.Metadata), offset, len(value)})
		e.Metadata = append(e.Metadata, '"', '"')
		previous = end
	}
	e.Metadata = append(e.Metadata, raw[previous:]...)
	return e, nil
}

func (e Entry) validate(size int64) error {
	previous := 0
	for _, ref := range e.SourceStrings {
		position, offset, length := ref[0], ref[1], ref[2]
		if position < previous || position > len(e.Metadata)-2 || string(e.Metadata[position:position+2]) != `""` || offset < 0 || length <= 0 || int64(offset) > size || int64(length) > size-int64(offset) {
			return fmt.Errorf("invalid source string reference: %s", e.Script)
		}
		previous = position + 2
	}
	return nil
}

func (e Entry) resolve(source string) (Metadata, error) {
	if err := e.validate(int64(len(source))); err != nil {
		return Metadata{}, err
	}
	raw := e.Metadata
	if len(e.SourceStrings) > 0 {
		raw = make([]byte, 0, len(e.Metadata))
		previous := 0
		for _, ref := range e.SourceStrings {
			raw = append(raw, e.Metadata[previous:ref[0]]...)
			value, _ := json.Marshal(source[ref[1] : ref[1]+ref[2]])
			raw = append(raw, value...)
			previous = ref[0] + 2
		}
		raw = append(raw, e.Metadata[previous:]...)
	}
	var metadata Metadata
	err := json.Unmarshal(raw, &metadata)
	return metadata, err
}
