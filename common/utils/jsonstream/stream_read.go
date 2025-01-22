// Reference: https://github.com/simon-engledew/jsoniter
package jsonstream

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	tokenLeftBracket  = json.Delim('[')
	tokenRightBracket = json.Delim(']')
	tokenLeftBrace    = json.Delim('{')
	tokenRightBrace   = json.Delim('}')
)

var (
	ErrorUnexpectRightBracket = utils.Error("invalid json: unexpected ]")
	ErrorUnexpectRightBrace   = utils.Error("invalid json: unexpected }")
	ErrorExpectRightBracket   = utils.Error("invalid json: expected ]")
	ErrorExpectRightBrace     = utils.Error("invalid json: expected }")
)

func Match(path []json.Token, target []json.Token) bool {
	if len(path) != len(target) {
		return false
	}
	for i, v := range target {
		if v != "*" && v != path[i] {
			return false
		}
	}
	return true
}

func Iterate(d *json.Decoder, callback func(path []json.Token) error) error {
	return iterate(d, make([]json.Token, 0, 16), callback)
}

func iterate(d *json.Decoder, path []json.Token, callback func(path []json.Token) error) error {
	offset := d.InputOffset()
	if len(path) > 0 {
		err := callback(path)
		if err != nil {
			return err
		}
		if d.InputOffset() != offset { // skip if decoder has read more tokens in callback
			return nil
		}
	}

	t, err := d.Token()
	if err != nil {
		return err
	}
	switch t {
	case tokenLeftBracket:
		return iterateArray(d, path, callback)
	case tokenLeftBrace:
		return iterateObject(d, path, callback)
	case tokenRightBracket:
		return ErrorUnexpectRightBracket
	case tokenRightBrace:
		return ErrorUnexpectRightBrace
	}

	return nil
}

func iterateArray(d *json.Decoder, path []json.Token, callback func(path []json.Token) error) error {
	index := 0
	for d.More() {
		if err := iterate(d, append(path, index), callback); err != nil {
			return err
		}
		index++
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t != tokenRightBracket {
		return ErrorExpectRightBracket
	}
	return nil
}

func iterateObject(d *json.Decoder, path []json.Token, callback func(path []json.Token) error) error {
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return err
		}
		if err := iterate(d, append(path, key), callback); err != nil {
			return err
		}
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t != tokenRightBrace {
		return ErrorExpectRightBrace
	}
	return nil
}
