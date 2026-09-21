// Derived from BurntSushi/toml v1.3.2 (MIT). See LICENSE and source map.
package locktoml

import (
	"fmt"
)

type ParseError struct {
	Message  string   // Short technical message.
	Usage    string   // Longer message with usage guidance; may be blank.
	Position Position // Position of the error
	LastKey  string   // Last parsed key, may be blank.

	// Line the error occurred.
	//
	// Deprecated: use [Position].
	Line int

	err   error
	input string
}

// Position of an error.
type Position struct {
	Line  int // Line number, starting at 1.
	Start int // Start of error, as byte offset starting at 0.
	Len   int // Lenght in bytes.
}

func (pe ParseError) Error() string {
	msg := pe.Message
	if msg == "" { // Error from errorf()
		msg = pe.err.Error()
	}

	if pe.LastKey == "" {
		return fmt.Sprintf("toml: line %d: %s", pe.Position.Line, msg)
	}
	return fmt.Sprintf("toml: line %d (last key %q): %s",
		pe.Position.Line, pe.LastKey, msg)
}

type (
	errLexControl       struct{ r rune }
	errLexEscape        struct{ r rune }
	errLexUTF8          struct{ b byte }
	errLexInvalidNum    struct{ v string }
	errLexInvalidDate   struct{ v string }
	errLexInlineTableNL struct{}
	errLexStringNL      struct{}
	errParseRange       struct {
		i    interface{} // int or float
		size string      // "int64", "uint16", etc.
	}
	errParseDuration struct{ d string }
)

func (e errLexControl) Error() string {
	return fmt.Sprintf("TOML files cannot contain control characters: '0x%02x'", e.r)
}

func (e errLexEscape) Error() string        { return fmt.Sprintf(`invalid escape in string '\%c'`, e.r) }
func (e errLexUTF8) Error() string          { return fmt.Sprintf("invalid UTF-8 byte: 0x%02x", e.b) }
func (e errLexInvalidNum) Error() string    { return fmt.Sprintf("invalid number: %q", e.v) }
func (e errLexInvalidDate) Error() string   { return fmt.Sprintf("invalid date: %q", e.v) }
func (e errLexInlineTableNL) Error() string { return "newlines not allowed within inline tables" }
func (e errLexStringNL) Error() string      { return "strings cannot contain newlines" }
func (e errParseRange) Error() string       { return fmt.Sprintf("%v is out of range for %s", e.i, e.size) }
