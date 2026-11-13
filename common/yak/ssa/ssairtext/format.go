package ssairtext

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

var (
	ssairAssignment = regexp.MustCompile(`^\t(?:<([^>]+)>\s+)?(t\d+)\s+=\s+(.*)$`)
	ssairLoad       = regexp.MustCompile(`load:(\d+)`)
)

// Format renders a function with one type on every assigned value and an
// aligned '=' column. Block names use '_' instead of '-'.
func Format(function *ssa.Function) string {
	if function == nil {
		return ""
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
	blockNames := make([]string, 0, len(function.Blocks))
	for _, id := range function.Blocks {
		block, ok := function.GetBasicBlockByID(id)
		if !ok || block == nil {
			continue
		}
		blockNames = append(blockNames, block.GetName())
	}
	sort.Slice(blockNames, func(i, j int) bool { return len(blockNames[i]) > len(blockNames[j]) })
	replaceNames := func(line string) string {
		for _, name := range blockNames {
			line = strings.ReplaceAll(line, name, strings.ReplaceAll(name, "-", "_"))
		}
		return line
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
		line = strings.ReplaceAll(replaceNames(line), "type-case", "type_cast")
		line = ssairLoad.ReplaceAllString(line, "load(t$1)")
		match := ssairAssignment.FindStringSubmatch(line)
		if match == nil {
			items = append(items, formatted{raw: line})
			continue
		}
		name := match[2]
		typeName := match[1]
		if typeName == "" {
			if value, ok := function.GetValueById(parseID(name)); ok && value != nil && value.GetType() != nil {
				typeName = value.GetType().String()
			}
		}
		if typeName == "" {
			typeName = "any"
		}
		left := fmt.Sprintf("<%s> %s", typeName, name)
		if len(left) > width {
			width = len(left)
		}
		items = append(items, formatted{left: left, right: match[3], assign: true})
	}
	var output strings.Builder
	output.WriteString(prefix.String())
	for _, item := range items {
		if !item.assign {
			if item.raw == "type: () -> null" {
				item.raw = "type: () -> void"
			}
			output.WriteString(item.raw)
			output.WriteByte('\n')
			continue
		}
		fmt.Fprintf(&output, "\t%-*s = %s\n", width, item.left, item.right)
	}
	return renumberIDs(output.String())
}

func renumberIDs(text string) string {
	index := regexp.MustCompile(`t(\d+)|\((\d+)\)`)
	next := int64(1)
	ids := make(map[int64]int64)
	return index.ReplaceAllStringFunc(text, func(match string) string {
		sub := index.FindStringSubmatch(match)
		var raw int64
		if sub[1] != "" {
			fmt.Sscanf(sub[1], "%d", &raw)
		} else {
			fmt.Sscanf(sub[2], "%d", &raw)
		}
		mapped, ok := ids[raw]
		if !ok {
			mapped = next
			next++
			ids[raw] = mapped
		}
		if sub[1] != "" {
			return fmt.Sprintf("t%d", mapped)
		}
		return fmt.Sprintf("(%d)", mapped)
	})
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
	var id int64
	_, _ = fmt.Sscanf(name, "t%d", &id)
	return id
}

// FormatProgram renders every non-extern function, prefixed by extern
// declarations. The result is stable: parsing it back and formatting again
// yields the same text.
func FormatProgram(program *ssa.Program) string {
	if program == nil {
		return ""
	}
	var output strings.Builder
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
		fmt.Fprintf(&output, "extern %s\n", name)
	}
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		output.WriteString(Format(function))
		return true
	})
	return output.String()
}
