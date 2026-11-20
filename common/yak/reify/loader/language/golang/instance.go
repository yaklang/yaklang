package golang

import "strings"

// Instance is one function name from the Go function table split into the parts
// a binary audit needs. The compiler renders a generic instantiation by writing
// the type arguments into the symbol name, so the name itself carries the
// package, the receiver, the type arguments and the family.
//
// The split follows debug/gosym: bracket contents are masked before the dot and
// slash searches, so a package path or a type argument that contains a dot does
// not move the receiver boundary. Unlike gosym, the result is available without
// a decoded table entry and the family key is explicit.
type Instance struct {
	// Name is the full name from the function table.
	Name string `json:"name"`
	// Package is the import path, for example "common/utils/omap". It is empty
	// for compiler-owned names such as "type:.eq" or "go:buildid".
	Package string `json:"package,omitempty"`
	// Receiver is the type text of a method, for example "*OrderedMap[T]" or
	// "OrderedMap[T]" for a value receiver. The shape of the instantiation is
	// kept as the compiler wrote it.
	Receiver string `json:"receiver,omitempty"`
	// Function is the declared function or method name without type arguments
	// and without a closure suffix.
	Function string `json:"function,omitempty"`
	// Closure is the ".funcN" suffix chain of a function literal, empty for a
	// declared function.
	Closure string `json:"closure,omitempty"`
	// Family is the name with every depth-zero type-argument list replaced by
	// "[T]". Two instances of one generic function share a family.
	Family string `json:"family,omitempty"`
	// TypeArgs holds the depth-zero type arguments of the receiver and the
	// function. A non-generic function has none.
	TypeArgs []string `json:"type_args,omitempty"`
	// Generic reports that the name carries at least one type-argument list.
	Generic bool `json:"generic,omitempty"`
}

// ParseInstance splits one function name from the Go function table. Names the
// compiler owns keep their text in Family and leave Package empty.
func ParseInstance(name string) Instance {
	instance := Instance{Name: name, Family: name}
	if name == "" {
		return instance
	}
	masked, ok := maskBracketContents(name)
	if !ok {
		// An imbalanced name cannot be split safely. Keep it as one opaque
		// family so a malformed table never invents a package.
		return instance
	}
	// The compiler reserves these prefixes for names it generates. They have no
	// import path and their brackets are type syntax, not type arguments.
	if strings.HasPrefix(masked, "type:") || strings.HasPrefix(masked, "go:") {
		return instance
	}
	groups := depthZeroGroups(name)
	instance.Generic = len(groups) > 0
	for _, group := range groups {
		instance.TypeArgs = append(instance.TypeArgs, splitTypeArgs(name[group.start+1:group.end])...)
	}
	instance.Family = replaceGroups(name, groups, "[T]")

	decl, closure := splitClosureMasked(masked)
	instance.Closure = closure
	declText := name[:len(decl)]
	pathEnd := strings.LastIndex(decl, "/")
	if pathEnd < 0 {
		pathEnd = 0
	}
	firstDot := strings.Index(decl[pathEnd:], ".")
	lastDot := strings.LastIndex(decl[pathEnd:], ".")
	if firstDot < 0 {
		// No dot after the last slash: no package part can be named.
		instance.Function = stripGroups(declText)
		return instance
	}
	if firstDot != lastDot && pathEnd+firstDot+1 <= pathEnd+lastDot {
		// Two distinct dots name a method receiver. As in debug/gosym, a module
		// path segment with a dot ("gopkg.in/yaml.v2") is read as a receiver.
		instance.Package = name[:pathEnd+firstDot]
		raw := name[pathEnd+firstDot+1 : pathEnd+lastDot]
		raw = replaceGroups(raw, depthZeroGroups(raw), "[T]")
		instance.Receiver = strings.TrimSuffix(strings.TrimPrefix(raw, "("), ")")
		instance.Function = stripGroups(name[pathEnd+lastDot+1 : len(decl)])
		return instance
	}
	instance.Package = name[:pathEnd+firstDot]
	instance.Function = stripGroups(name[pathEnd+firstDot+1 : len(decl)])
	return instance
}

// FamilyOf returns the family key of one name. It is the part of ParseInstance
// that a caller needs when it does not want the other fields.
func FamilyOf(name string) string { return ParseInstance(name).Family }

// PackageOf returns the import path of one name, or an empty string for a
// compiler-owned name.
func PackageOf(name string) string { return ParseInstance(name).Package }

// maskBracketContents replaces every byte inside a bracket group with '_' while
// keeping the brackets and the byte indices. Dots and slashes inside a type
// argument then cannot be mistaken for a package or a receiver boundary. An
// imbalanced name is reported so the caller can leave it unsplit.
func maskBracketContents(name string) (string, bool) {
	depth := 0
	var builder strings.Builder
	builder.Grow(len(name))
	for index := 0; index < len(name); index++ {
		c := name[index]
		switch c {
		case '[':
			depth++
			builder.WriteByte('[')
		case ']':
			depth--
			if depth < 0 {
				return name, false
			}
			builder.WriteByte(']')
		default:
			if depth > 0 && c != '[' && c != ']' {
				builder.WriteByte('_')
				continue
			}
			builder.WriteByte(c)
		}
	}
	if depth != 0 {
		return name, false
	}
	return builder.String(), true
}

type bracketGroup struct{ start, end int }

// depthZeroGroups returns every bracket group that is not nested in another
// bracket group. Go writes one such group per instantiated type and leaves
// array and slice brackets inside it.
func depthZeroGroups(name string) []bracketGroup {
	var groups []bracketGroup
	depth, start := 0, -1
	for index := 0; index < len(name); index++ {
		switch name[index] {
		case '[':
			if depth == 0 {
				start = index
			}
			depth++
		case ']':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 {
				groups = append(groups, bracketGroup{start: start, end: index})
				start = -1
			}
		}
	}
	return groups
}

func replaceGroups(name string, groups []bracketGroup, replacement string) string {
	if len(groups) == 0 {
		return name
	}
	var builder strings.Builder
	builder.Grow(len(name))
	last := 0
	for _, group := range groups {
		builder.WriteString(name[last:group.start])
		builder.WriteString(replacement)
		last = group.end + 1
	}
	builder.WriteString(name[last:])
	return builder.String()
}

func stripGroups(text string) string {
	return replaceGroups(text, depthZeroGroups(text), "")
}

// splitClosureMasked separates the ".funcN" chain the compiler appends to
// function literals from the declared name. The chain can repeat for nested
// closures: ".func1.2". The first depth-zero suffix starts the chain.
func splitClosureMasked(masked string) (string, string) {
	depth := 0
	for index := 0; index < len(masked); index++ {
		switch masked[index] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 && strings.HasPrefix(masked[index:], ".func") && closureNumber(masked[index+5:]) {
				return masked[:index], masked[index:]
			}
		}
	}
	return masked, ""
}

// closureNumber reports whether text starts with the digits of a closure
// suffix. The compiler uses ".func1" and, for nested literals, ".2".
func closureNumber(text string) bool {
	index := 0
	for index < len(text) && text[index] >= '0' && text[index] <= '9' {
		index++
	}
	if index == 0 {
		return false
	}
	return index == len(text) || text[index] == '.'
}

// splitTypeArgs splits one argument list on commas that are not nested in
// brackets, parentheses or braces.
func splitTypeArgs(list string) []string {
	if list == "" {
		return nil
	}
	var args []string
	depth, start := 0, 0
	for index := 0; index < len(list); index++ {
		switch list[index] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(list[start:index]))
				start = index + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(list[start:]))
	return args
}

// ShapeName removes the compiler's shape prefix from one type argument.
// Symbol names use "go.shape.string" where a reader expects "string".
func ShapeName(arg string) string {
	return strings.TrimPrefix(arg, "go.shape.")
}

// IsShapeHash reports whether one type argument is a hashed shape. The compiler
// hashes a shape when no type string names it, for example a struct with
// unexported fields from another package.
func IsShapeHash(arg string) bool {
	text := ShapeName(arg)
	if len(text) != 64 {
		return false
	}
	for index := 0; index < len(text); index++ {
		c := text[index]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
