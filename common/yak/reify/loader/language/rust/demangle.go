// Package rust implements the Rust language layer: detection, symbol recovery,
// the string model and both symbol manglings in use today.
//
// The decoder is written for this package. A symbol that does not match the
// grammar keeps its encoded linker name, so an unsupported construct never
// produces a wrong display name.
package rust

import (
	"errors"
	"strings"
)

const (
	// maxNameLength bounds the input taken from an untrusted symbol table.
	maxNameLength = 64 << 10
	// maxDisplayLength bounds one decoded name. Generic instantiation can grow
	// a name well beyond its encoded form.
	maxDisplayLength = 1 << 16
)

var errNotRust = errors.New("not a Rust mangled name")

// IsMangled reports whether a name is structurally a Rust symbol. It is a
// filter: a name that passes can still fail to decode.
func IsMangled(name string) bool {
	name = linkName(name)
	if strings.HasPrefix(name, "_R") {
		return true
	}
	if !strings.HasPrefix(name, "_ZN") {
		return false
	}
	if pos := strings.LastIndex(name, "E."); pos >= 0 {
		name = name[:pos+1]
	}
	if len(name) <= legacyHashLength+4 || name[len(name)-1] != 'E' {
		return false
	}
	start := len(name) - legacyHashLength - 4
	return name[start:start+3] == "17h" && isHex(name[start+3:len(name)-1])
}

// Demangle decodes a v0 or legacy Rust symbol. It accepts only names that pass
// IsMangled, so a C++ name cannot be reported as Rust.
func Demangle(name string) (string, bool) {
	if len(name) > maxNameLength || !IsMangled(name) {
		return "", false
	}
	link := linkName(name)
	var (
		out string
		err error
	)
	if strings.HasPrefix(link, "_R") {
		out, err = demangleV0(link)
	} else {
		out, err = demangleLegacy(link)
	}
	if err != nil || out == "" || len(out) > maxDisplayLength {
		return "", false
	}
	return out, true
}

// linkName removes the leading underscore a Mach-O symbol table adds. The
// caller keeps the exact original name in Symbol.Mangled.
func linkName(name string) string {
	if strings.HasPrefix(name, "__R") || strings.HasPrefix(name, "__ZN") {
		return name[1:]
	}
	return name
}
