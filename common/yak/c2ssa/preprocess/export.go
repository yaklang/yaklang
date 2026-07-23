package preprocess

import "strings"

// MacroTables holds function-like and object-like macros for expansion.
type MacroTables struct {
	Function map[string]FunctionMacro
	Object   map[string]string
}

// FunctionMacro describes a function-like #define.
type FunctionMacro struct {
	Params   []string
	Variadic bool
	Body     string
}

func NewMacroTables() MacroTables {
	t := newMacroTables()
	return macroTablesToExport(t)
}

// ScanMacroTablesFromSource collects function/object macros without modifying src.
func ScanMacroTablesFromSource(src string) MacroTables {
	return MacroTablesFromInternal(scanMacroTablesFromSource(src))
}

func (m MacroTables) Clone() MacroTables {
	return macroTablesToExport(cloneMacroTables(exportToMacroTables(m)))
}

func (m *MacroTables) MergeFrom(other MacroTables) {
	if m.Function == nil {
		m.Function = make(map[string]FunctionMacro)
	}
	if m.Object == nil {
		m.Object = make(map[string]string)
	}
	for k, v := range other.Function {
		m.Function[k] = v
	}
	for k, v := range other.Object {
		m.Object[k] = v
	}
}

func (m MacroTables) ToInternal() macroTables {
	return exportToMacroTables(m)
}

func MacroTablesFromInternal(t macroTables) MacroTables {
	return macroTablesToExport(t)
}

func exportToMacroTables(m MacroTables) macroTables {
	out := newMacroTables()
	for k, v := range m.Function {
		out.function[k] = functionMacro{
			params:   v.Params,
			variadic: v.Variadic,
			body:     v.Body,
		}
	}
	for k, v := range m.Object {
		out.object[k] = v
	}
	return out
}

func macroTablesToExport(t macroTables) MacroTables {
	out := MacroTables{
		Function: make(map[string]FunctionMacro, len(t.function)),
		Object:   make(map[string]string, len(t.object)),
	}
	for k, v := range t.function {
		out.Function[k] = FunctionMacro{
			Params:   v.params,
			Variadic: v.variadic,
			Body:     v.body,
		}
	}
	for k, v := range t.object {
		out.Object[k] = v
	}
	return out
}

// ExpandSourceWithTables expands macros in src using tables (no local #define collection).
func ExpandSourceWithTables(src string, tables MacroTables) string {
	var st macroScanState
	return ExpandSourceWithTablesState(src, tables, &st)
}

// ExpandSourceWithTablesState expands macros while preserving block-comment state across calls.
func ExpandSourceWithTablesState(src string, tables MacroTables, st *macroScanState) string {
	env := newMacroEnvFromTables(tables)
	expanded := env.expandSourceWithState(src, st)
	return CollapsePreprocessorContinuations(expanded)
}

// ExpandAndStripDefines collects local #define/#undef, merges base tables, expands, strips defines.
func ExpandAndStripDefines(src string, base MacroTables) (string, MacroTables) {
	collected := collectFunctionMacros(src, exportToMacroTables(base))
	env := &macroEnv{
		tables:   collected.tables,
		maxDepth: maxMacroExpandDepth,
	}
	expanded := env.expandSource(collected.output)
	expanded = CollapsePreprocessorContinuations(expanded)
	return expanded, MacroTablesFromInternal(collected.tables)
}

// CollapsePreprocessorContinuations removes backslash-newline sequences.
func CollapsePreprocessorContinuations(src string) string {
	return collapsePreprocessorContinuations(src)
}

// JoinLogicalLines merges physical lines connected by trailing backslash continuations.
func JoinLogicalLines(src string) []string {
	return joinLogicalLines(src)
}

// ApplyDefineLine applies a #define/#undef line to tables; returns true if a define was consumed.
func ApplyDefineLine(line string, tables *MacroTables, strip bool) bool {
	internal := exportToMacroTables(*tables)
	removed := applyDirectiveToTables(line, internal, strip)
	*tables = MacroTablesFromInternal(internal)
	return removed
}

// ParseIncludePath extracts the path from #include "..." or #include <...>.
func ParseIncludePath(line string) (path string, system bool, ok bool) {
	trimmed := trimDirectiveLine(line)
	if trimmed == "" {
		return "", false, false
	}
	if len(trimmed) < 7 || !strings.HasPrefix(trimmed, "include") {
		return "", false, false
	}
	if len(trimmed) > 7 && trimmed[7] != ' ' && trimmed[7] != '\t' {
		return "", false, false
	}
	rest := trimSpace(trimmed[7:])
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.Index(rest[1:], `"`)
		if end >= 0 {
			return rest[1 : 1+end], false, true
		}
	}
	if len(rest) >= 2 && rest[0] == '<' {
		end := strings.Index(rest, ">")
		if end > 1 {
			return rest[1:end], true, true
		}
	}
	return "", false, false
}

// DirectiveName returns the first token of a # directive line (e.g. "if", "define", "include").
func DirectiveName(line string) string {
	trimmed := trimDirectiveLine(line)
	if trimmed == "" {
		return ""
	}
	parts := splitDirectiveFields(trimmed)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// DirectiveRest returns tokens after the directive name joined as a string.
func DirectiveRest(line string) string {
	trimmed := trimDirectiveLine(line)
	if trimmed == "" {
		return ""
	}
	parts := splitDirectiveFields(trimmed)
	if len(parts) <= 1 {
		return ""
	}
	return joinDirectiveRest(trimmed, parts[0])
}

func trimDirectiveLine(line string) string {
	trimmed := trimSpace(line)
	if trimmed == "" || trimmed[0] != '#' {
		return ""
	}
	return trimSpace(trimmed[1:])
}

func splitDirectiveFields(directive string) []string {
	var parts []string
	var cur string
	inQuote := byte(0)
	for i := 0; i < len(directive); i++ {
		c := directive[i]
		if inQuote != 0 {
			cur += string(c)
			if c == inQuote && (i == 0 || directive[i-1] != '\\') {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '<':
			inQuote = c
			if c == '<' {
				// angle path ends with >
			}
			cur += string(c)
		case '>':
			cur += string(c)
		case ' ', '\t':
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		default:
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

func joinDirectiveRest(directive, first string) string {
	idx := indexDirectiveField(directive, first)
	if idx < 0 || idx+len(first) >= len(directive) {
		return ""
	}
	return trimSpace(directive[idx+len(first):])
}

func indexDirectiveField(directive, field string) int {
	// find field as whole token at start
	if len(directive) >= len(field) && directive[:len(field)] == field {
		if len(directive) == len(field) || directive[len(field)] == ' ' || directive[len(field)] == '\t' {
			return 0
		}
	}
	return -1
}

func trimSpace(s string) string {
	s = strings.TrimRight(s, "\r")
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}
