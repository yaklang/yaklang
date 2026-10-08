package stream_parser

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// JSONValue preserves member order and numeric spelling alongside typed values.
// It is read-only parse metadata; no embedded content is evaluated.
type JSONValue struct {
	Kind        string
	Raw         string
	StringValue string
	NumberValue string
	BoolValue   bool
	Keys        []string
	Members     map[string]*JSONValue
	Elements    []*JSONValue
}

// decodeJSONText is an explicitly bounded syntax helper for text-bearing rules.
// Duplicate keys are rejected rather than silently replacing earlier values.
func decodeJSONText(text string, depthLimit int) (*JSONValue, error) {
	if len(text) > 1<<20 || !utf8.ValidString(text) {
		return nil, fmt.Errorf("json: input exceeds limit or is not UTF-8")
	}
	if depthLimit < 1 || depthLimit > 128 {
		return nil, fmt.Errorf("json: depth limit must be between 1 and 128")
	}
	if !jsonPairedSurrogates(text) {
		return nil, fmt.Errorf("json: incomplete or unpaired Unicode escape")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	values := 0
	var read func(int) (*JSONValue, error)
	read = func(depth int) (*JSONValue, error) {
		values++
		if depth > depthLimit || values > 100000 {
			return nil, fmt.Errorf("json: structure exceeds depth or value limit")
		}
		start := int(decoder.InputOffset())
		// Token() consumes a pending member colon/array comma and whitespace.
		for start < len(text) && strings.ContainsRune(" \t\r\n:,", rune(text[start])) {
			start++
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("json: %w", err)
		}
		value := &JSONValue{}
		switch token := token.(type) {
		case json.Delim:
			switch token {
			case '{':
				value.Kind, value.Members = "object", make(map[string]*JSONValue)
				for decoder.More() {
					keyToken, err := decoder.Token()
					if err != nil {
						return nil, fmt.Errorf("json: %w", err)
					}
					key, ok := keyToken.(string)
					if !ok {
						return nil, fmt.Errorf("json: object key must be a string")
					}
					if _, duplicate := value.Members[key]; duplicate {
						return nil, fmt.Errorf("json: duplicate object key")
					}
					child, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					value.Keys = append(value.Keys, key)
					value.Members[key] = child
				}
			case '[':
				value.Kind = "array"
				for decoder.More() {
					child, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					value.Elements = append(value.Elements, child)
				}
			default:
				return nil, fmt.Errorf("json: unexpected closing delimiter")
			}
			end, err := decoder.Token()
			if err != nil || (token == '{' && end != json.Delim('}')) || (token == '[' && end != json.Delim(']')) {
				return nil, fmt.Errorf("json: incomplete container: %v", err)
			}
		case string:
			value.Kind, value.StringValue = "string", token
		case json.Number:
			value.Kind, value.NumberValue = "number", token.String()
		case bool:
			value.Kind, value.BoolValue = "boolean", token
		case nil:
			value.Kind = "null"
		default:
			return nil, fmt.Errorf("json: unexpected value type")
		}
		value.Raw = text[start:int(decoder.InputOffset())]
		return value, nil
	}
	value, err := read(1)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("json: trailing data after document")
	}
	return value, nil
}

// Go's JSON decoder substitutes U+FFFD for lone surrogate escapes. Reject that
// non-interoperable form so typed metadata cannot silently change source text.
// The decoder still performs all other string and document syntax checks.
func jsonPairedSurrogates(text string) bool {
	inString := false
	for i := 0; i < len(text); i++ {
		if text[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || text[i] != '\\' {
			continue
		}
		i++
		if i >= len(text) {
			return false
		}
		if text[i] != 'u' {
			continue
		}
		if i+4 >= len(text) {
			return false
		}
		code, err := strconv.ParseUint(text[i+1:i+5], 16, 16)
		if err != nil || (code >= 0xdc00 && code <= 0xdfff) {
			return false
		}
		i += 4
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(text) || text[i+1:i+3] != `\u` {
				return false
			}
			low, err := strconv.ParseUint(text[i+3:i+7], 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
