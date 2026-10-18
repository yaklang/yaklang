// Package textsectlayout derives runtime.moduledata.textsectmap's offset from
// the toolchain that will be linked, then checks it against the Go versions
// whose layout has been verified. An unknown version or a disagreement fails
// closed: callers must refuse to build instead of guessing.
package textsectlayout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

const (
	// amd64 pointer, uintptr, and map header. Slice headers are three words.
	wordSize  = 8
	sliceSize = wordSize * 3
	// A real textsectmap sits after minpc/text and well before the end of
	// moduledata. Values outside this window are a parser bug, not a layout.
	minSliceOffset = 0xc0
	maxSliceOffset = 0x200
)

// ErrUnknownVersion means the toolchain is not in the verified table.
var ErrUnknownVersion = errors.New("unverified Go moduledata layout")

// ErrMismatch means the derived offset disagrees with the verified table.
var ErrMismatch = errors.New("textsectmap offset mismatch")

// ErrProbe means the struct layout could not be derived.
var ErrProbe = errors.New("textsectmap probe failed")

// Result is a derived and table-checked textsectmap location.
type Result struct {
	Version string
	Major   int
	Minor   int
	Patch   int
	Offset  int
}

// Check reads GOROOT's runtime.moduledata and accepts it only when the derived
// textsectmap offset matches the verified table for that Go release.
func Check(goroot string) (Result, error) {
	if runtime.GOARCH != "amd64" {
		return Result{}, fmt.Errorf("%w: GOARCH %s is not amd64", ErrProbe, runtime.GOARCH)
	}
	version, major, minor, patch, err := ReadVersion(goroot)
	if err != nil {
		return Result{}, err
	}
	offset, err := ProbeFile(filepath.Join(goroot, "src", "runtime", "symtab.go"))
	if err != nil {
		return Result{}, err
	}
	if err := Validate(version, major, minor, offset); err != nil {
		return Result{}, err
	}
	return Result{Version: version, Major: major, Minor: minor, Patch: patch, Offset: offset}, nil
}

// ExpectedOffset reports the verified textsectmap offset for a Go 1.minor
// release. ok is false when that release has not been checked by hand.
func ExpectedOffset(major, minor int) (offset int, ok bool) {
	if major != 1 {
		return 0, false
	}
	switch minor {
	case 26:
		return 0x150, true
	case 27:
		return 0x168, true
	default:
		return 0, false
	}
}

// Validate fails closed unless version is in the table and probed matches it.
func Validate(version string, major, minor, probed int) error {
	want, ok := ExpectedOffset(major, minor)
	if !ok {
		return fmt.Errorf("%w: Go %s is not in the ssa2llvm moduledata layout table (known: Go 1.26 -> 0x150, Go 1.27 -> 0x168); 需人工验证 moduledata 布局 and add that release before building", ErrUnknownVersion, version)
	}
	if probed != want {
		return fmt.Errorf("%w: Go %s probed runtime.moduledata.textsectmap at %#x, layout table requires %#x", ErrMismatch, version, probed, want)
	}
	return nil
}

var versionRE = regexp.MustCompile(`go(\d+)\.(\d+)(?:\.(\d+))?`)

// ReadVersion reads the first line of GOROOT/VERSION.
func ReadVersion(goroot string) (version string, major, minor, patch int, err error) {
	if strings.TrimSpace(goroot) == "" {
		return "", 0, 0, 0, fmt.Errorf("%w: empty GOROOT", ErrProbe)
	}
	data, err := os.ReadFile(filepath.Join(goroot, "VERSION"))
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("%w: read VERSION: %v", ErrProbe, err)
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	match := versionRE.FindStringSubmatch(line)
	if match == nil {
		return "", 0, 0, 0, fmt.Errorf("%w: cannot parse %q as a Go version", ErrProbe, line)
	}
	major, _ = strconv.Atoi(match[1])
	minor, _ = strconv.Atoi(match[2])
	if match[3] != "" {
		patch, _ = strconv.Atoi(match[3])
	}
	return match[0], major, minor, patch, nil
}

// ProbeFile derives textsectmap's byte offset from a runtime/symtab.go source.
// It understands only the pointer-sized types that appear before that field
// on amd64 and returns ErrProbe for anything else.
func ProbeFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("%w: read symtab: %v", ErrProbe, err)
	}
	return ProbeSource(string(data))
}

// ProbeSource derives textsectmap's byte offset from symtab.go contents.
func ProbeSource(src string) (int, error) {
	body, err := moduledataBody(src)
	if err != nil {
		return 0, err
	}
	offset := 0
	found := false
	for _, line := range fieldLines(body) {
		count, typ, names, err := parseField(line)
		if err != nil {
			return 0, err
		}
		for _, name := range names {
			if name == "textsectmap" {
				size, _, err := typeLayout(typ)
				if err != nil {
					return 0, err
				}
				if size != sliceSize || !strings.HasPrefix(typ, "[]") {
					return 0, fmt.Errorf("%w: textsectmap type %s is not an amd64 slice", ErrProbe, typ)
				}
				if offset < minSliceOffset || offset > maxSliceOffset || offset%wordSize != 0 {
					return 0, fmt.Errorf("%w: derived textsectmap offset %#x is outside the amd64 safety window [%#x,%#x]", ErrProbe, offset, minSliceOffset, maxSliceOffset)
				}
				found = true
				break
			}
		}
		if found {
			break
		}
		size, align, err := typeLayout(typ)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrProbe, err)
		}
		for i := 0; i < count; i++ {
			offset = alignUp(offset, align)
			offset += size
		}
	}
	if !found {
		return 0, fmt.Errorf("%w: runtime.moduledata has no textsectmap field", ErrProbe)
	}
	return offset, nil
}

func moduledataBody(src string) (string, error) {
	const marker = "type moduledata struct {"
	start := strings.Index(src, marker)
	if start < 0 {
		return "", fmt.Errorf("%w: type moduledata struct not found", ErrProbe)
	}
	i := start + len(marker)
	depth := 1
	for i < len(src) {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start+len(marker) : i], nil
			}
		}
		i++
	}
	return "", fmt.Errorf("%w: type moduledata struct is not closed", ErrProbe)
}

func fieldLines(body string) []string {
	body = stripBlockComments(body)
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if cut := strings.Index(line, "//"); cut >= 0 {
			line = line[:cut]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func stripBlockComments(s string) string {
	var b strings.Builder
	for {
		start := strings.Index(s, "/*")
		if start < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:start])
		end := strings.Index(s[start+2:], "*/")
		if end < 0 {
			break
		}
		s = s[start+2+end+2:]
	}
	return b.String()
}

func parseField(line string) (count int, typ string, names []string, err error) {
	tokens := strings.Fields(line)
	if len(tokens) == 0 {
		return 0, "", nil, fmt.Errorf("empty field")
	}
	if len(tokens) == 1 {
		return 1, tokens[0], []string{embeddedName(tokens[0])}, nil
	}
	typ = tokens[len(tokens)-1]
	for _, token := range tokens[:len(tokens)-1] {
		name := strings.TrimSuffix(token, ",")
		if name == "" || !isIdent(name) {
			return 0, "", nil, fmt.Errorf("bad field name %q in %q", token, line)
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return 0, "", nil, fmt.Errorf("field %q has no name", line)
	}
	return len(names), typ, names, nil
}

func embeddedName(typ string) string {
	if i := strings.LastIndex(typ, "."); i >= 0 {
		return typ[i+1:]
	}
	return typ
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
		if i > 0 && !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func typeLayout(typ string) (size, align int, err error) {
	switch typ {
	case "sys.NotInHeap", "NotInHeap":
		// Embedded zero-size marker. It does not move the next field.
		return 0, 1, nil
	case "uintptr", "uint64", "int64", "unsafe.Pointer":
		return wordSize, wordSize, nil
	case "string":
		return wordSize * 2, wordSize, nil
	case "uint32", "int32", "rune":
		return 4, 4, nil
	case "uint16", "int16":
		return 2, 2, nil
	case "uint8", "int8", "byte", "bool":
		return 1, 1, nil
	}
	switch {
	case strings.HasPrefix(typ, "*"):
		return wordSize, wordSize, nil
	case strings.HasPrefix(typ, "[]"):
		return sliceSize, wordSize, nil
	case strings.HasPrefix(typ, "map["):
		return wordSize, wordSize, nil
	default:
		return 0, 0, fmt.Errorf("unknown moduledata field type %q", typ)
	}
}

func alignUp(off, align int) int {
	if align <= 1 {
		return off
	}
	return (off + align - 1) &^ (align - 1)
}
