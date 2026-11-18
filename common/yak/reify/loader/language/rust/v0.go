package rust

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The decoder is deliberately strict. A symbol it cannot parse keeps the
// encoded linker name, so an unfamiliar mangling form never turns into a wrong
// function name.
const (
	maxOutputSize  = 1 << 16
	maxSteps       = 1 << 20
	maxBackrefs    = 64
	maxIdentLength = 1 << 12
)

type state struct {
	steps     int
	depth     int
	lifetimes int
}

type parser struct {
	src string
	pos int
	end int
	st  *state
}

// demangleV0 decodes the RFC 2603 scheme. The input includes the _R prefix.
func demangleV0(name string) (string, error) {
	if !strings.HasPrefix(name, "_R") {
		return "", fmt.Errorf("not a v0 symbol")
	}
	body := name[2:]
	// A toolchain may append .llvm.<hash>; it is not part of the name.
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		body = body[:dot]
	}
	if body == "" || body[0] >= '0' && body[0] <= '9' {
		return "", fmt.Errorf("unsupported v0 version")
	}
	p := &parser{src: body, end: len(body), st: &state{}}
	out, err := p.symbolName()
	if err != nil {
		return "", err
	}
	if p.pos != p.end {
		return "", fmt.Errorf("unparsed characters at %d", p.pos)
	}
	if len(out) > maxOutputSize {
		out = out[:maxOutputSize]
	}
	return out, nil
}

// symbolName parses <path> [<instantiating-crate>].
func (p *parser) symbolName() (string, error) {
	out, err := p.path(true)
	if err != nil {
		return "", err
	}
	if p.pos < p.end {
		if _, err := p.path(false); err != nil {
			return "", err
		}
	}
	return out, nil
}

func (p *parser) path(needsSeparator bool) (string, error) {
	if err := p.step(); err != nil {
		return "", err
	}
	if p.pos >= p.end {
		return "", p.errorf("expected path")
	}
	switch c := p.src[p.pos]; c {
	case 'C':
		p.pos++
		_, name, err := p.identifier()
		return name, err
	case 'M', 'X':
		p.pos++
		if err := p.implPath(); err != nil {
			return "", err
		}
		inner, err := p.typ()
		if err != nil {
			return "", err
		}
		if c == 'M' {
			return "<" + inner + ">", nil
		}
		trait, err := p.path(false)
		if err != nil {
			return "", err
		}
		return "<" + inner + " as " + trait + ">", nil
	case 'Y':
		p.pos++
		inner, err := p.typ()
		if err != nil {
			return "", err
		}
		trait, err := p.path(false)
		if err != nil {
			return "", err
		}
		return "<" + inner + " as " + trait + ">", nil
	case 'N':
		p.pos++
		if p.pos >= p.end || !isNamespace(p.src[p.pos]) {
			return "", p.errorf("expected namespace")
		}
		namespace := p.src[p.pos]
		p.pos++
		parent, err := p.path(needsSeparator)
		if err != nil {
			return "", err
		}
		dis, name, err := p.identifier()
		if err != nil {
			return "", err
		}
		if namespace >= 'A' && namespace <= 'Z' {
			label := string(namespace)
			switch namespace {
			case 'C':
				label = "closure"
			case 'S':
				label = "shim"
			}
			out := parent + "::{" + label
			if name != "" {
				out += ":" + name
			}
			return out + "#" + strconv.FormatUint(dis, 10) + "}", nil
		}
		if name == "" {
			// An implementation namespace may carry an empty identifier. rustc
			// prints nothing for it, not a trailing separator.
			return parent, nil
		}
		return parent + "::" + name, nil
	case 'I':
		p.pos++
		base, err := p.path(needsSeparator)
		if err != nil {
			return "", err
		}
		if needsSeparator {
			base += "::"
		}
		args, err := p.genericArgs()
		if err != nil {
			return "", err
		}
		if err := p.expect('E'); err != nil {
			return "", err
		}
		return base + "<" + args + ">", nil
	case 'B':
		return backref(p, func(sub *parser) (string, error) { return sub.path(needsSeparator) })
	default:
		return "", p.errorf("unrecognized path letter %q", c)
	}
}

// implPath parses [<disambiguator>] <path> without printing it.
func (p *parser) implPath() error {
	if _, err := p.disambiguator(); err != nil {
		return err
	}
	_, err := p.path(false)
	return err
}

func (p *parser) identifier() (uint64, string, error) {
	dis, err := p.disambiguator()
	if err != nil {
		return 0, "", err
	}
	name, _, err := p.undisambiguated()
	return dis, name, err
}

// disambiguator parses an optional "s" <base-62-number>.
func (p *parser) disambiguator() (uint64, error) {
	if p.pos >= p.end || p.src[p.pos] != 's' {
		return 0, nil
	}
	p.pos++
	n, err := p.base62()
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// undisambiguated parses ["u"] <decimal-number> ["_"] <bytes>. The "u" marker
// selects the Rust variant of punycode.
func (p *parser) undisambiguated() (string, bool, error) {
	punycode := false
	if p.pos < p.end && p.src[p.pos] == 'u' {
		p.pos++
		punycode = true
	}
	length, err := p.decimal()
	if err != nil {
		return "", false, err
	}
	if length > maxIdentLength {
		return "", false, p.errorf("identifier too long")
	}
	if p.pos < p.end && p.src[p.pos] == '_' {
		p.pos++
	}
	if p.end-p.pos < length {
		return "", false, p.errorf("truncated identifier")
	}
	name := p.src[p.pos : p.pos+length]
	p.pos += length
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_') {
			return "", false, p.errorf("invalid identifier byte %q", c)
		}
	}
	if punycode {
		expanded, err := expandPunycode(name)
		if err != nil {
			return "", false, err
		}
		name = expanded
	}
	return name, punycode, nil
}

func (p *parser) typ() (string, error) {
	if err := p.step(); err != nil {
		return "", err
	}
	if p.pos >= p.end {
		return "", p.errorf("expected type")
	}
	c := p.src[p.pos]
	if c >= 'a' && c <= 'z' {
		return p.basicType()
	}
	switch c {
	case 'C', 'M', 'X', 'Y', 'N', 'I':
		return p.path(false)
	case 'A', 'S':
		p.pos++
		element, err := p.typ()
		if err != nil {
			return "", err
		}
		if c == 'S' {
			return "[" + element + "]", nil
		}
		length, err := p.constValue()
		if err != nil {
			return "", err
		}
		return "[" + element + "; " + length + "]", nil
	case 'T':
		p.pos++
		var elements []string
		for p.pos < p.end && p.src[p.pos] != 'E' {
			element, err := p.typ()
			if err != nil {
				return "", err
			}
			elements = append(elements, element)
		}
		if err := p.expect('E'); err != nil {
			return "", err
		}
		if len(elements) == 1 {
			return "(" + elements[0] + ",)", nil
		}
		return "(" + strings.Join(elements, ", ") + ")", nil
	case 'R', 'Q':
		p.pos++
		out := "&"
		if lifetime, err := p.optionalLifetime(); err != nil {
			return "", err
		} else if lifetime != "" {
			out += lifetime + " "
		}
		if c == 'Q' {
			out += "mut "
		}
		inner, err := p.typ()
		if err != nil {
			return "", err
		}
		return out + inner, nil
	case 'P', 'O':
		p.pos++
		prefix := "*const "
		if c == 'O' {
			prefix = "*mut "
		}
		inner, err := p.typ()
		if err != nil {
			return "", err
		}
		return prefix + inner, nil
	case 'F':
		p.pos++
		saved := p.st.lifetimes
		sig, err := p.fnSig()
		p.st.lifetimes = saved
		return sig, err
	case 'D':
		p.pos++
		saved := p.st.lifetimes
		out, err := p.dynType()
		p.st.lifetimes = saved
		return out, err
	case 'B':
		return backref(p, func(sub *parser) (string, error) { return sub.typ() })
	default:
		return "", p.errorf("unrecognized type letter %q", c)
	}
}

func (p *parser) basicType() (string, error) {
	if p.pos >= p.end {
		return "", p.errorf("expected basic type")
	}
	name, ok := basicTypes[p.src[p.pos]]
	if !ok {
		return "", p.errorf("unrecognized basic type %q", p.src[p.pos])
	}
	p.pos++
	return name, nil
}

var basicTypes = map[byte]string{
	'a': "i8",
	'b': "bool",
	'c': "char",
	'd': "f64",
	'e': "str",
	'f': "f32",
	'h': "u8",
	'i': "isize",
	'j': "usize",
	'l': "i32",
	'm': "u32",
	'n': "i128",
	'o': "u128",
	'p': "_",
	's': "i16",
	't': "u16",
	'u': "()",
	'v': "...",
	'x': "i64",
	'y': "u64",
	'z': "!",
}

func (p *parser) genericArgs() (string, error) {
	var args []string
	for p.pos < p.end && p.src[p.pos] != 'E' {
		arg, err := p.genericArg()
		if err != nil {
			return "", err
		}
		args = append(args, arg)
	}
	return strings.Join(args, ", "), nil
}

func (p *parser) genericArg() (string, error) {
	if p.pos >= p.end {
		return "", p.errorf("expected generic argument")
	}
	switch p.src[p.pos] {
	case 'L':
		p.pos++
		n, err := p.base62()
		if err != nil {
			return "", err
		}
		return p.lifetime(n)
	case 'K':
		p.pos++
		return p.constValue()
	default:
		return p.typ()
	}
}

// binder parses an optional "G" <base-62-number> and prints for<'a, 'b>.
func (p *parser) binder() (string, error) {
	if p.pos >= p.end || p.src[p.pos] != 'G' {
		return "", nil
	}
	p.pos++
	n, err := p.base62()
	if err != nil {
		return "", err
	}
	count := n + 1
	if count > uint64(p.end) {
		return "", p.errorf("binder too large")
	}
	names := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		p.st.lifetimes++
		name, err := p.lifetime(1)
		if err != nil {
			return "", err
		}
		names = append(names, name)
	}
	return "for<" + strings.Join(names, ", ") + "> ", nil
}

func (p *parser) optionalLifetime() (string, error) {
	if p.pos >= p.end || p.src[p.pos] != 'L' {
		return "", nil
	}
	p.pos++
	n, err := p.base62()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	return p.lifetime(n)
}

func (p *parser) lifetime(n uint64) (string, error) {
	if n == 0 {
		return "'_", nil
	}
	if n > uint64(p.st.lifetimes) {
		return "", p.errorf("invalid lifetime")
	}
	depth := uint64(p.st.lifetimes) - n
	if depth < 26 {
		return "'" + string(rune('a'+depth)), nil
	}
	return "'z" + strconv.FormatUint(depth-25, 10), nil
}

func (p *parser) fnSig() (string, error) {
	binder, err := p.binder()
	if err != nil {
		return "", err
	}
	out := binder
	if p.pos < p.end && p.src[p.pos] == 'U' {
		p.pos++
		out += "unsafe "
	}
	if p.pos < p.end && p.src[p.pos] == 'K' {
		p.pos++
		if p.pos >= p.end {
			return "", p.errorf("truncated ABI")
		}
		if p.src[p.pos] == 'C' {
			p.pos++
			out += `extern "C" `
		} else {
			abi, punycode, err := p.undisambiguated()
			if err != nil {
				return "", err
			}
			if punycode {
				return "", p.errorf("punycode in ABI name")
			}
			out += `extern "` + strings.ReplaceAll(abi, "_", "-") + `" `
		}
	}
	out += "fn("
	var args []string
	for p.pos < p.end && p.src[p.pos] != 'E' {
		arg, err := p.typ()
		if err != nil {
			return "", err
		}
		args = append(args, arg)
	}
	if err := p.expect('E'); err != nil {
		return "", err
	}
	out += strings.Join(args, ", ") + ")"
	if p.pos < p.end && p.src[p.pos] == 'u' {
		p.pos++
		return out, nil
	}
	ret, err := p.typ()
	if err != nil {
		return "", err
	}
	return out + " -> " + ret, nil
}

func (p *parser) dynType() (string, error) {
	bounds, err := p.dynBounds()
	if err != nil {
		return "", err
	}
	out := "dyn " + bounds
	lifetime, err := p.optionalLifetime()
	if err != nil {
		return "", err
	}
	if lifetime != "" {
		out += " + " + lifetime
	}
	return out, nil
}

func (p *parser) dynBounds() (string, error) {
	binder, err := p.binder()
	if err != nil {
		return "", err
	}
	var traits []string
	for p.pos < p.end && p.src[p.pos] != 'E' {
		trait, err := p.dynTrait()
		if err != nil {
			return "", err
		}
		traits = append(traits, trait)
	}
	if err := p.expect('E'); err != nil {
		return "", err
	}
	return binder + strings.Join(traits, " + "), nil
}

// dynTrait parses a path and its optional associated-type bindings. An
// instantiated trait keeps its argument list open until the bindings close it.
func (p *parser) dynTrait() (string, error) {
	base, open, err := p.dynTraitPath()
	if err != nil {
		return "", err
	}
	var bindings []string
	for p.pos < p.end && p.src[p.pos] == 'p' {
		p.pos++
		name, _, err := p.undisambiguated()
		if err != nil {
			return "", err
		}
		value, err := p.typ()
		if err != nil {
			return "", err
		}
		bindings = append(bindings, name+" = "+value)
	}
	switch {
	case open && len(bindings) > 0:
		return base + ", " + strings.Join(bindings, ", ") + ">", nil
	case open:
		return base + ">", nil
	case len(bindings) > 0:
		return base + "<" + strings.Join(bindings, ", ") + ">", nil
	default:
		return base, nil
	}
}

func (p *parser) dynTraitPath() (string, bool, error) {
	switch {
	case p.pos >= p.end:
		return "", false, p.errorf("expected trait path")
	case p.src[p.pos] == 'I':
		p.pos++
		base, err := p.path(false)
		if err != nil {
			return "", false, err
		}
		args, err := p.genericArgs()
		if err != nil {
			return "", false, err
		}
		if err := p.expect('E'); err != nil {
			return "", false, err
		}
		return base + "<" + args, true, nil
	case p.src[p.pos] == 'B':
		result, err := backref(p, func(sub *parser) (traitResult, error) {
			base, open, err := sub.dynTraitPath()
			return traitResult{base: base, open: open}, err
		})
		return result.base, result.open, err
	default:
		base, err := p.path(false)
		return base, false, err
	}
}

type traitResult struct {
	base string
	open bool
}

func (p *parser) constValue() (string, error) {
	if p.pos >= p.end {
		return "", p.errorf("expected constant")
	}
	switch p.src[p.pos] {
	case 'B':
		return backref(p, func(sub *parser) (string, error) { return sub.constValue() })
	case 'p':
		p.pos++
		return "_", nil
	case 'e':
		p.pos++
		return p.constString()
	case 'R', 'Q':
		tag := p.src[p.pos]
		p.pos++
		if tag == 'R' && p.pos < p.end && p.src[p.pos] == 'e' {
			p.pos++
			return p.constString()
		}
		inner, err := p.constValue()
		if err != nil {
			return "", err
		}
		if tag == 'Q' {
			return "&mut " + inner, nil
		}
		return "&" + inner, nil
	case 'A', 'T':
		tag := p.src[p.pos]
		p.pos++
		var items []string
		for p.pos < p.end && p.src[p.pos] != 'E' {
			item, err := p.constValue()
			if err != nil {
				return "", err
			}
			items = append(items, item)
		}
		if err := p.expect('E'); err != nil {
			return "", err
		}
		if tag == 'A' {
			return "[" + strings.Join(items, ", ") + "]", nil
		}
		if len(items) == 1 {
			return "(" + items[0] + ",)", nil
		}
		return "(" + strings.Join(items, ", ") + ")", nil
	case 'V':
		p.pos++
		path, err := p.path(true)
		if err != nil {
			return "", err
		}
		if p.pos >= p.end {
			return "", p.errorf("truncated constant struct")
		}
		switch p.src[p.pos] {
		case 'U':
			p.pos++
			return path, nil
		case 'T':
			p.pos++
			var values []string
			for p.pos < p.end && p.src[p.pos] != 'E' {
				value, err := p.constValue()
				if err != nil {
					return "", err
				}
				values = append(values, value)
			}
			if err := p.expect('E'); err != nil {
				return "", err
			}
			return path + "(" + strings.Join(values, ", ") + ")", nil
		case 'S':
			p.pos++
			var fields []string
			for p.pos < p.end && p.src[p.pos] != 'E' {
				if _, err := p.disambiguator(); err != nil {
					return "", err
				}
				name, _, err := p.undisambiguated()
				if err != nil {
					return "", err
				}
				if err := p.expect(':'); err != nil {
					return "", err
				}
				value, err := p.constValue()
				if err != nil {
					return "", err
				}
				fields = append(fields, name+": "+value)
			}
			if err := p.expect('E'); err != nil {
				return "", err
			}
			return path + " { " + strings.Join(fields, ", ") + " }", nil
		default:
			return "", p.errorf("unrecognized constant struct form")
		}
	}
	typ := p.src[p.pos]
	kind, ok := constKinds[typ]
	if !ok {
		return "", p.errorf("unrecognized constant type %q", typ)
	}
	p.pos++
	negative := false
	if kind == signedConst && p.pos < p.end && p.src[p.pos] == 'n' {
		p.pos++
		negative = true
	}
	raw, value, digits, err := p.hexNibbles()
	if err != nil {
		return "", err
	}
	switch kind {
	case signedConst, unsignedConst:
		if negative {
			raw = "-" + raw
		}
		if digits > 16 {
			return "0x" + strings.TrimLeft(raw, "0"), nil
		}
		if negative {
			return "-" + strconv.FormatUint(value, 10), nil
		}
		return strconv.FormatUint(value, 10), nil
	case boolConst:
		if digits > 1 {
			return "", p.errorf("boolean constant too large")
		}
		if value == 0 {
			return "false", nil
		}
		if value == 1 {
			return "true", nil
		}
		return "", p.errorf("invalid boolean constant")
	case charConst:
		if digits > 6 {
			return "", p.errorf("character constant too large")
		}
		return quoteRune(rune(value)), nil
	}
	return "", p.errorf("unrecognized constant kind")
}

// hexNibbles reads {<hex-digit>} "_" and returns the raw digits and value.
func (p *parser) hexNibbles() (raw string, value uint64, digits int, err error) {
	start := p.pos
	overflow := false
	for p.pos < p.end && p.src[p.pos] != '_' {
		digit, ok := hexDigit(p.src[p.pos])
		if !ok {
			return "", 0, 0, p.errorf("expected hex digit or _")
		}
		if value > (math.MaxUint64-uint64(digit))/16 {
			overflow = true
		}
		value = value*16 + uint64(digit)
		digits++
		p.pos++
	}
	if digits == 0 {
		return "", 0, 0, p.errorf("empty constant")
	}
	raw = p.src[start:p.pos]
	if err := p.expect('_'); err != nil {
		return "", 0, 0, err
	}
	if overflow {
		return raw, value, 17, nil
	}
	return raw, value, digits, nil
}

// constString reads a hex-encoded string literal and quotes it.
func (p *parser) constString() (string, error) {
	raw, _, _, err := p.hexNibbles()
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		high, _ := hexDigit(raw[i])
		low, _ := hexDigit(raw[i+1])
		bytes = append(bytes, byte(high<<4|low))
	}
	return quoteString(string(bytes)), nil
}

func quoteString(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}

const (
	invalidConst = iota
	signedConst
	unsignedConst
	boolConst
	charConst
)

var constKinds = map[byte]int{
	'a': signedConst, 's': signedConst, 'l': signedConst, 'x': signedConst, 'n': signedConst, 'i': signedConst,
	'h': unsignedConst, 't': unsignedConst, 'm': unsignedConst, 'y': unsignedConst, 'o': unsignedConst, 'j': unsignedConst,
	'b': boolConst,
	'c': charConst,
}

func quoteRune(r rune) string {
	switch r {
	case '\t':
		return `'\t'`
	case '\r':
		return `'\r'`
	case '\n':
		return `'\n'`
	case '\\':
		return `'\\'`
	case '\'':
		return `'\''`
	}
	if r >= ' ' && r <= '~' {
		return "'" + string(r) + "'"
	}
	return fmt.Sprintf(`'\u{%x}'`, r)
}

func (p *parser) base62() (uint64, error) {
	if p.pos < p.end && p.src[p.pos] == '_' {
		p.pos++
		return 0, nil
	}
	var value uint64
	for p.pos < p.end {
		c := p.src[p.pos]
		p.pos++
		if c == '_' {
			return value + 1, nil
		}
		digit, ok := base62Digit(c)
		if !ok {
			return 0, p.errorf("invalid base-62 digit %q", c)
		}
		if value > (math.MaxUint64-digit)/62 {
			return 0, p.errorf("base-62 overflow")
		}
		value = value*62 + digit
	}
	return 0, p.errorf("unterminated base-62 number")
}

// decimal parses an identifier length. A leading zero is always the complete
// number: an empty identifier is written as a single 0, and the next length
// follows immediately, so 0 followed by 11 means "" and then 11 bytes.
func (p *parser) decimal() (int, error) {
	if p.pos >= p.end || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
		return 0, p.errorf("expected decimal number")
	}
	value := int(p.src[p.pos] - '0')
	p.pos++
	if value == 0 {
		return 0, nil
	}
	for p.pos < p.end && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		value = value*10 + int(p.src[p.pos]-'0')
		if value > maxIdentLength {
			return 0, p.errorf("decimal number too large")
		}
		p.pos++
	}
	return value, nil
}

// backref resolves "B" <base-62-number>. The number is an absolute offset into
// the name after _R, biased by one, and always points backwards.
func backref[T any](p *parser, parse func(*parser) (T, error)) (T, error) {
	var zero T
	backoff := p.pos
	p.pos++
	index, err := p.base62()
	if err != nil {
		return zero, err
	}
	if index >= uint64(backoff) {
		return zero, p.errorf("invalid backref %d", index)
	}
	if p.st.depth >= maxBackrefs {
		return zero, p.errorf("backref nesting too deep")
	}
	p.st.depth++
	defer func() { p.st.depth-- }()
	sub := &parser{src: p.src, pos: int(index), end: backoff, st: p.st}
	return parse(sub)
}

func (p *parser) step() error {
	p.st.steps++
	if p.st.steps > maxSteps {
		return p.errorf("symbol too expensive to decode")
	}
	return nil
}

func (p *parser) expect(c byte) error {
	if p.pos >= p.end || p.src[p.pos] != c {
		return p.errorf("expected %q", c)
	}
	p.pos++
	return nil
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("%s at %d", fmt.Sprintf(format, args...), p.pos)
}

func isNamespace(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func base62Digit(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0'), true
	case c >= 'a' && c <= 'z':
		return uint64(c-'a') + 10, true
	case c >= 'A' && c <= 'Z':
		return uint64(c-'A') + 36, true
	}
	return 0, false
}

func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	}
	return 0, false
}

// expandPunycode implements the Rust variant of bootstring decoding (RFC 3492).
// Rust prefixes a punycode identifier with "u" and separates the basic code
// points from the encoded ones with "_".
func expandPunycode(s string) (string, error) {
	const (
		base        = 36
		tMin        = 1
		tMax        = 26
		skew        = 38
		damp        = 700
		initialBias = 72
		initialN    = 128
	)
	var output []rune
	encoded := s
	if idx := strings.LastIndexByte(s, '_'); idx >= 0 {
		output = []rune(s[:idx])
		encoded = s[idx+1:]
	}
	n, i, bias := int64(initialN), int64(0), int64(initialBias)
	for pos := 0; pos < len(encoded); {
		old := i
		w := int64(1)
		for k := int64(base); ; k += base {
			if pos >= len(encoded) {
				return "", fmt.Errorf("unterminated punycode")
			}
			digit, ok := punycodeDigit(encoded[pos])
			pos++
			if !ok {
				return "", fmt.Errorf("invalid punycode digit")
			}
			if digit > math.MaxInt32 || i > math.MaxInt32-int64(digit)*w {
				return "", fmt.Errorf("punycode overflow")
			}
			i += int64(digit) * w
			var t int64
			switch {
			case k <= bias:
				t = tMin
			case k >= bias+tMax:
				t = tMax
			default:
				t = k - bias
			}
			if int64(digit) < t {
				break
			}
			w *= base - t
		}
		count := int64(len(output)) + 1
		delta := i - old
		if old == 0 {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / count
		k := int64(0)
		for delta > (base-tMin)*tMax/2 {
			delta /= base - tMin
			k += base
		}
		bias = k + (base-tMin+1)*delta/(delta+skew)
		n += i / count
		if n > utf8.MaxRune || !utf8.ValidRune(rune(n)) {
			return "", fmt.Errorf("punycode code point out of range")
		}
		i %= count
		output = append(output, 0)
		copy(output[i+1:], output[i:])
		output[i] = rune(n)
		i++
	}
	return string(output), nil
}

func punycodeDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c-'0') + 26, true
	case c >= 'A' && c <= 'Z':
		return int(c - 'A'), true
	case c >= 'a' && c <= 'z':
		return int(c - 'a'), true
	}
	return 0, false
}
