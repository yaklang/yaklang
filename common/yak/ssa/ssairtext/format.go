package ssairtext

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func isRE2Space(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

func isRE2Word(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// parseAssignmentLine parses a line matching `^\t(?:<([^>]+)>\s+)?(t\d+)\s+=\s+(.*)$`.
func parseAssignmentLine(line string) (typeName, name, right string, ok bool) {
	if len(line) < 6 || line[0] != '\t' {
		return "", "", "", false
	}
	pos := 1
	if line[pos] == '<' {
		closeIdx := strings.IndexByte(line[pos+1:], '>')
		if closeIdx <= 0 {
			return "", "", "", false
		}
		typeName = line[pos+1 : pos+1+closeIdx]
		pos = pos + 1 + closeIdx + 1
		wsStart := pos
		for pos < len(line) && isRE2Space(line[pos]) {
			pos++
		}
		if pos == wsStart {
			return "", "", "", false
		}
	}
	if pos >= len(line) || line[pos] != 't' {
		return "", "", "", false
	}
	nameStart := pos
	pos++
	if pos >= len(line) || line[pos] < '0' || line[pos] > '9' {
		return "", "", "", false
	}
	for pos < len(line) && line[pos] >= '0' && line[pos] <= '9' {
		pos++
	}
	name = line[nameStart:pos]
	ws1 := pos
	for pos < len(line) && isRE2Space(line[pos]) {
		pos++
	}
	if pos == ws1 || pos >= len(line) || line[pos] != '=' {
		return "", "", "", false
	}
	pos++
	ws2 := pos
	for pos < len(line) && isRE2Space(line[pos]) {
		pos++
	}
	if pos == ws2 {
		return "", "", "", false
	}
	return typeName, name, line[pos:], true
}

// Format renders a function with one type on every assigned value and an
// aligned '=' column. Block names use '_' instead of '-'.
func Format(function *ssa.Function) string {
	if function == nil {
		return ""
	}
	var output strings.Builder
	_ = formatInto(&output, function)
	return output.String()
}

func formatInto(dst *strings.Builder, function *ssa.Function) error {
	if function == nil {
		return nil
	}
	var prefix strings.Builder
	if boundary := function.GetVerboseName(); strings.HasPrefix(boundary, "boundary:") {
		fmt.Fprintf(&prefix, "; %s\n", boundary)
	}
	raw := function.DisAsm(ssa.DisAsmDefault)
	if newline := strings.IndexByte(raw, '\n'); newline >= 0 {
		raw = formatHeader(function) + raw[newline:]
	} else {
		raw = formatHeader(function)
	}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	var blockNames []string
	for _, id := range function.Blocks {
		block, ok := function.GetBasicBlockByID(id)
		if !ok || block == nil {
			continue
		}
		if name := block.GetName(); strings.ContainsRune(name, '-') {
			blockNames = append(blockNames, name)
		}
	}
	var blockReplacer *strings.Replacer
	if len(blockNames) > 0 {
		sort.Slice(blockNames, func(i, j int) bool { return len(blockNames[i]) > len(blockNames[j]) })
		pairs := make([]string, 0, len(blockNames)*2)
		for _, name := range blockNames {
			pairs = append(pairs, name, strings.ReplaceAll(name, "-", "_"))
		}
		blockReplacer = strings.NewReplacer(pairs...)
	}

	type formatted struct {
		left   string
		right  string
		raw    string
		assign bool
	}
	items := make([]formatted, 0, len(lines))
	width := 0
	for _, line := range lines {
		if blockReplacer != nil {
			line = blockReplacer.Replace(line)
		}
		if strings.Contains(line, "type-case") {
			line = strings.ReplaceAll(line, "type-case", "type_cast")
		}
		typeName, name, right, matched := parseAssignmentLine(line)
		if !matched {
			items = append(items, formatted{raw: line})
			continue
		}
		if typeName == "" {
			if value, ok := function.GetValueById(parseID(name)); ok && value != nil && value.GetType() != nil {
				typeName = value.GetType().String()
			}
		}
		if typeName == "" {
			typeName = "any"
		}
		left := "<" + typeName + "> " + name
		if len(left) > width {
			width = len(left)
		}
		items = append(items, formatted{left: left, right: right, assign: true})
	}
	var preRenumber strings.Builder
	preRenumber.Grow(len(raw) + prefix.Len() + 64)
	preRenumber.WriteString(prefix.String())
	for _, item := range items {
		if !item.assign {
			if item.raw == "type: () -> null" {
				item.raw = "type: () -> void"
			}
			preRenumber.WriteString(item.raw)
			preRenumber.WriteByte('\n')
			continue
		}
		preRenumber.WriteByte('\t')
		preRenumber.WriteString(item.left)
		for pad := width - len(item.left); pad > 0; pad-- {
			preRenumber.WriteByte(' ')
		}
		preRenumber.WriteString(" = ")
		preRenumber.WriteString(item.right)
		preRenumber.WriteByte('\n')
	}
	renumberIDsInto(dst, preRenumber.String())
	return nil
}

func renumberIDs(text string) string {
	var out strings.Builder
	renumberIDsInto(&out, text)
	return out.String()
}

func renumberIDsInto(out *strings.Builder, text string) {
	// Match `\bt(\d+)\b` or `\((\d+)\)` in a single pass so type names such as
	// int32 (where 't' is preceded by word char 'n') are left untouched.
	out.Grow(len(text))
	next := int64(1)
	ids := make(map[int64]int64)
	var numBuf [24]byte
	lastCopied := 0
	for i := 0; i < len(text); {
		switch text[i] {
		case 't':
			if (i == 0 || !isRE2Word(text[i-1])) && i+1 < len(text) && text[i+1] >= '0' && text[i+1] <= '9' {
				j := i + 1
				for j < len(text) && text[j] >= '0' && text[j] <= '9' {
					j++
				}
				if j == len(text) || !isRE2Word(text[j]) {
					if raw, err := strconv.ParseInt(text[i+1:j], 10, 64); err == nil {
						mapped, ok := ids[raw]
						if !ok {
							mapped = next
							next++
							ids[raw] = mapped
						}
						out.WriteString(text[lastCopied:i])
						out.WriteByte('t')
						out.Write(strconv.AppendInt(numBuf[:0], mapped, 10))
						lastCopied = j
						i = j
						continue
					}
				}
			}
			i++
		case '(':
			if i+1 < len(text) && text[i+1] >= '0' && text[i+1] <= '9' {
				j := i + 1
				for j < len(text) && text[j] >= '0' && text[j] <= '9' {
					j++
				}
				if j < len(text) && text[j] == ')' {
					if raw, err := strconv.ParseInt(text[i+1:j], 10, 64); err == nil {
						mapped, ok := ids[raw]
						if !ok {
							mapped = next
							next++
							ids[raw] = mapped
						}
						out.WriteString(text[lastCopied:i])
						out.WriteByte('(')
						out.Write(strconv.AppendInt(numBuf[:0], mapped, 10))
						out.WriteByte(')')
						lastCopied = j + 1
						i = j + 1
						continue
					}
				}
			}
			i++
		default:
			i++
		}
	}
	if lastCopied < len(text) {
		out.WriteString(text[lastCopied:])
	}
}

func formatHeader(function *ssa.Function) string {
	if function == nil {
		return ""
	}
	params := make([]string, 0, len(function.Params))
	for _, id := range function.Params {
		value, ok := function.GetValueById(id)
		if !ok || value == nil || !value.HasUsers() {
			continue
		}
		param, ok := ssa.ToParameter(value)
		if !ok || param == nil {
			continue
		}
		typeName := "any"
		if value.GetType() != nil && value.GetType().String() != "" {
			typeName = value.GetType().String()
		}
		params = append(params, fmt.Sprintf("<%s> (%d) %s", typeName, value.GetId(), value.GetName()))
	}
	if len(params) == 0 {
		return function.GetName()
	}
	return function.GetName() + " " + strings.Join(params, ", ")
}

func parseID(name string) int64 {
	if !strings.HasPrefix(name, "t") {
		return 0
	}
	id, err := strconv.ParseInt(name[1:], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// FormatTo writes formatted SSAIR text for function directly into w.
func FormatTo(w io.Writer, function *ssa.Function) error {
	if function == nil {
		return nil
	}
	var output strings.Builder
	_ = formatInto(&output, function)
	_, err := io.WriteString(w, output.String())
	return err
}

// FormatProgram renders every non-extern function, prefixed by extern
// declarations. The result is stable: parsing it back and formatting again
// yields the same text.
func FormatProgram(program *ssa.Program) string {
	if program == nil {
		return ""
	}
	var output strings.Builder
	_ = FormatProgramTo(&output, program)
	return output.String()
}

// FormatProgramTo streams formatted SSAIR text for program directly into w.
func FormatProgramTo(w io.Writer, program *ssa.Program) error {
	if program == nil {
		return nil
	}
	var externs []string
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil {
			return true
		}
		if function.IsExtern() {
			externs = append(externs, name)
		}
		return true
	})
	sort.Strings(externs)
	for _, name := range externs {
		if _, err := fmt.Fprintf(w, "extern %s\n", name); err != nil {
			return err
		}
	}
	var writeErr error
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		if _, err := io.WriteString(w, Format(function)); err != nil {
			writeErr = err
			return false
		}
		return true
	})
	return writeErr
}
