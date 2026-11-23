package golang

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Instance is one function name from the Go function table split into the
// parts a binary tool needs. The compiler encodes generic type arguments,
// receivers and closures into the symbol name, so the name itself carries the
// source-level identity.
//
// Name is the readable form a consumer should display. Raw is the exact
// compiler string, which is also what the pclntab stores. A Go generic loses
// the compiler's go.shape. prefix in Name:
//
//	Raw:  main.(*Box[go.shape.int]).Get
//	Name: main.(*Box[int]).Get
//
// Family is a grouping key, not a display name: every depth-zero
// type-argument group becomes [T], so instances of one generic share a family.
type Instance struct {
	// Name is the source-like display name.
	Name string `json:"name"`
	// Raw is the exact name in the function table.
	Raw string `json:"raw"`
	// Package is the import path, for example "common/utils/omap". It is empty
	// for compiler-owned names such as "type:.eq" or "go:buildid".
	Package string `json:"package,omitempty"`
	// Receiver is the source-like receiver type, for example "*OrderedMap[int]"
	// or "OrderedMap[int]" for a value receiver.
	Receiver string `json:"receiver,omitempty"`
	// Function is the declared function or method name without type arguments
	// and without a closure suffix.
	Function string `json:"function,omitempty"`
	// Closure is the ".funcN" suffix chain of a function literal, empty for a
	// declared function.
	Closure string `json:"closure,omitempty"`
	// Family is the name with every depth-zero type-argument list replaced by
	// "[T]". Two instances of one generic function share it.
	Family string `json:"family,omitempty"`
	// TypeArgs holds source-like type arguments of the receiver and the
	// function. A hashed shape is rendered as shape#<8 hex digits>.
	TypeArgs []string `json:"type_args,omitempty"`
	// Generic reports that the name carries at least one type-argument list.
	Generic bool `json:"generic,omitempty"`
	// Synthetic marks a compiler-owned name: a type: or go: prefix, or a -fm
	// method value wrapper. Such a name has no Go source declaration.
	Synthetic bool `json:"synthetic,omitempty"`
}

// ParseInstance splits one function name from the Go function table. Names the
// compiler owns keep their text in Name and Raw and set Synthetic.
func ParseInstance(name string) Instance {
	instance := Instance{Name: name, Raw: name, Family: name}
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
		instance.Synthetic = true
		return instance
	}
	instance.Synthetic = strings.HasSuffix(name, "-fm")
	groups := depthZeroGroups(name)
	instance.Generic = len(groups) > 0
	if instance.Generic {
		instance.Name = replaceGroupsWithDisplay(name, groups)
		for _, group := range groups {
			instance.TypeArgs = append(instance.TypeArgs, displayTypeArgs(name[group.start+1:group.end])...)
		}
		instance.Family = replaceGroups(name, groups, "[T]")
	}

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
		instance.Receiver = trimReceiver(replaceGroupsWithDisplay(raw, depthZeroGroups(raw)))
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

// DisplayOf returns the source-like display name of one pclntab name.
func DisplayOf(name string) string { return ParseInstance(name).Name }

// Identity returns the language-neutral decomposition attached to a Symbol.
// The caller owns the slice in the result.
func (i Instance) Identity() *symbol.Identity {
	return &symbol.Identity{
		Package:   i.Package,
		Receiver:  i.Receiver,
		Function:  i.Function,
		Closure:   i.Closure,
		Family:    i.Family,
		TypeArgs:  append([]string(nil), i.TypeArgs...),
		Generic:   i.Generic,
		Synthetic: i.Synthetic,
	}
}

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

// replaceGroupsWithDisplay keeps the original text outside the groups and
// rewrites every group as a display type-argument list.
func replaceGroupsWithDisplay(name string, groups []bracketGroup) string {
	if len(groups) == 0 {
		return name
	}
	var builder strings.Builder
	builder.Grow(len(name))
	last := 0
	for _, group := range groups {
		builder.WriteString(name[last:group.start])
		builder.WriteByte('[')
		builder.WriteString(strings.Join(displayTypeArgs(name[group.start+1:group.end]), ","))
		builder.WriteByte(']')
		last = group.end + 1
	}
	builder.WriteString(name[last:])
	return builder.String()
}

func stripGroups(text string) string {
	return replaceGroups(text, depthZeroGroups(text), "")
}

func trimReceiver(text string) string {
	return strings.TrimSuffix(strings.TrimPrefix(text, "("), ")")
}

// displayTypeArgs returns one display string per top-level type argument.
func displayTypeArgs(list string) []string {
	if list == "" {
		return nil
	}
	args := splitTypeArgs(list)
	for index := range args {
		args[index] = displayTypeArg(args[index])
	}
	return args
}

// displayTypeArg removes the compiler's shape prefix. A shape without a type
// string is a 64-digit hash; the first eight digits identify it without
// printing the whole hash into every symbol name.
func displayTypeArg(arg string) string {
	arg = strings.TrimSpace(arg)
	arg = strings.ReplaceAll(arg, "go.shape.", "")
	if IsShapeHash(arg) {
		return "shape#" + arg[:8]
	}
	return arg
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
