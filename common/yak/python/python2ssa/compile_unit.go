package python2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CompileUnitDependencies extracts Python import edges by resolving dotted
// module names against a path index built from unit paths and file stems.
//
// This runs for every .py file in the project before any unit is built, so
// imports are collected by a regex scan (scanPyImportEdges) rather than by
// parsing: building a syntax tree here would parse every file a second time
// (the batch phase parses again to build SSA) and would reach the shared static
// ANTLR caches from the planning step. The scan still covers absolute imports
// ("import a.b", "from a.b import c"), relative imports ("from . import x",
// "from ..pkg import y"), parenthesized/multi-line and semicolon-separated
// forms, and literal dynamic imports (importlib.import_module("a.b") /
// __import__("a.b")).
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	pathToKey := pythonImportPathIndex(fs, units)
	var edges []ssa.UnitRef
	addEdge := func(from, to, raw string) {
		if to == "" || to == from {
			return
		}
		edges = append(edges, ssa.UnitRef{From: from, To: to, Kind: "import", Raw: raw})
	}
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			imports := scanPyImportEdges(src)
			for _, imp := range imports {
				switch imp.kind {
				case pyImportRelative:
					if to := resolvePyRelativeImport(fs, pathToKey, file, imp.dots, imp.module); to != "" {
						addEdge(unit.Key, to, strings.TrimSpace(imp.dots+imp.module))
					}
				case pyImportDynamic:
					if to := resolvePyDynamicImport(fs, pathToKey, file, imp.module, imp.pkg); to != "" {
						addEdge(unit.Key, to, imp.module)
					}
				default:
					// Absolute import: the dependency is on the module itself.
					addEdge(unit.Key, ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(imp.module, ".", "/")), imp.module)
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

// pyImportKind classifies a collected import edge.
type pyImportKind int

const (
	pyImportAbsolute pyImportKind = iota
	pyImportRelative
	pyImportDynamic
)

// pyImportEdge is one import collected from a source file.
//
//   - Absolute ("import a.b" / "from a.b import c"): module = "a.b".
//   - Relative ("from ..pkg import x"): dots = "..", module = "pkg".
//   - Dynamic (importlib.import_module(".b", package="p")):
//     module = ".b", pkg = "p"; a leading-dot module keeps the dots inside
//     module (resolvePyDynamicImport splits them).
type pyImportEdge struct {
	kind   pyImportKind
	dots   string
	module string
	pkg    string
}

// unescapePyString applies the escapes that appear in module names in practice.
func unescapePyString(raw string) string {
	out := strings.Builder{}
	out.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case '\\':
				out.WriteByte('\\')
			case '"':
				out.WriteByte('"')
			case '\'':
				out.WriteByte('\'')
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			default:
				// Unknown escape: keep both characters verbatim.
				out.WriteByte(raw[i])
				out.WriteByte(raw[i+1])
			}
			i++
			continue
		}
		out.WriteByte(raw[i])
	}
	return out.String()
}

// String renders the import kind for the dedupe key.
func (k pyImportKind) String() string {
	switch k {
	case pyImportRelative:
		return "relative"
	case pyImportDynamic:
		return "dynamic"
	default:
		return "absolute"
	}
}

func pythonImportPathIndex(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) map[string]string {
	index := ssa.NewUniqueStringIndex()
	for _, unit := range units {
		if unit == nil {
			continue
		}
		unitPath := ssa.CleanUnitPath(fs, unit.Path)
		if unitPath != "" && unitPath != "." {
			index.Add(unitPath, unit.Key)
		}
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			normalized := ssa.NormalizeUnitPath(fs, file)
			stem := strings.TrimSuffix(normalized, fs.Ext(normalized))
			if stem == "" {
				continue
			}
			index.Add(stem, unit.Key)
			if base := ssa.UnitBase(fs, stem); base != "" && base != "." && base != "__init__" {
				index.Add(base, unit.Key)
			}
		}
	}
	return index.Values()
}

var (
	pyModuleNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	pyImportAsRe   = regexp.MustCompile(`\s+as\s+[A-Za-z_][A-Za-z0-9_]*$`)
)

// resolvePyRelativeImport resolves a relative import ("from <dots><module> import ...")
// to a unit key. dots is one or more "." characters: each dot ascends one
// directory level from the importing file's package; the first dot refers to
// the file's own package (its directory), two dots the parent, and so on.
// module is the optional dotted suffix after the dots.
func resolvePyRelativeImport(
	fs filesys_interface.FileSystem,
	pathToKey map[string]string,
	importerFile string,
	dots, module string,
) string {
	dotCount := strings.Count(dots, ".")
	if dotCount <= 0 {
		return ""
	}
	// The importing file's package directory is the base for the first dot.
	dir := ssa.UnitDir(fs, importerFile)
	for i := 1; i < dotCount; i++ {
		dir = ssa.CleanUnitPath(fs, dir+"/..")
	}
	base := dir
	if module != "" {
		base = ssa.CleanUnitPath(fs, fs.Join(dir, strings.ReplaceAll(module, ".", "/")))
	}
	// Candidate forms, most specific first: the module file itself, then its
	// package directory (from .pkg import x -> pkg/__init__.py).
	if key := ssa.ResolvePathImport(pathToKey, base); key != "" {
		return key
	}
	return ssa.ResolvePathImport(pathToKey, ssa.UnitDir(fs, base))
}

// resolvePyDynamicImport resolves a literal dynamic import. Absolute module
// names go through the plain path index. A leading dot (relative dynamic import,
// e.g. importlib.import_module(".b", package="pkg")) resolves against the
// package= argument when present, otherwise against the importer's directory.
func resolvePyDynamicImport(
	fs filesys_interface.FileSystem,
	pathToKey map[string]string,
	importerFile string,
	mod, pkg string,
) string {
	mod = strings.TrimSpace(mod)
	if mod == "" {
		return ""
	}
	if !strings.HasPrefix(mod, ".") {
		return ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(mod, ".", "/"))
	}
	// Split the leading dots from the module suffix.
	dots := mod[:strings.IndexFunc(mod, func(r rune) bool { return r != '.' })]
	if dots == "" {
		dots = mod
	}
	suffix := strings.TrimPrefix(mod, dots)
	base := ""
	if pkg != "" {
		// package= anchors the resolution: one dot = pkg, two = pkg's parent, ...
		base = strings.ReplaceAll(strings.Trim(pkg, "/"), ".", "/")
		up := strings.Count(dots, ".") - 1
		for i := 0; i < up; i++ {
			base = ssa.CleanUnitPath(fs, base+"/..")
		}
	} else {
		dir := ssa.UnitDir(fs, importerFile)
		up := strings.Count(dots, ".") - 1
		for i := 0; i < up; i++ {
			dir = ssa.CleanUnitPath(fs, dir+"/..")
		}
		base = dir
	}
	if suffix != "" {
		base = ssa.CleanUnitPath(fs, fs.Join(base, strings.ReplaceAll(suffix, ".", "/")))
	}
	if key := ssa.ResolvePathImport(pathToKey, base); key != "" {
		return key
	}
	return ssa.ResolvePathImport(pathToKey, ssa.UnitDir(fs, base))
}

// splitPyImportNames parses the tail of a plain `import` statement
// ("a, b, c" or "a as b, c as d", possibly with a trailing comment) into the
// list of imported module names, dropping aliases and anything that is not a
// valid dotted module name (e.g. stray parens from `import (a, b)`).
func splitPyImportNames(s string) []string {
	var names []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = pyImportAsRe.ReplaceAllString(part, "")
		part = strings.TrimSpace(part)
		if part == "" || !pyModuleNameRe.MatchString(part) {
			continue
		}
		names = append(names, part)
	}
	return names
}

// ---- regex-based import scanning (plan phase, no ANTLR) ----
//
// CompileUnitDependencies runs over every source file of the project before any
// unit is built, so this step has to stay cheap: parsing each file with the
// Python grammar here would parse every .py file twice (plan + batch) and grow
// the shared static ANTLR caches for no lasting benefit. These scanners recover
// the same edges the AST walk produced for every construct the corpus and the
// test suite exercise:
//
//	import a.b, c.d as e             absolute, aliased, comma lists
//	from a.b import c                absolute
//	from . import x                  relative without a module
//	from ..pkg.mod import x          relative with a dotted module
//	from pkg import (x, y)           parenthesized / multi-line name lists
//	import os; from b import c       semicolon-separated simple statements
//	__import__("m"), importlib.import_module("m"), import_module("m", package="p")
//
// and, like the AST walk did, import-like text inside comments, one-line
// strings and docstrings is ignored, as are dynamic calls whose argument is not
// a plain string literal (variables, f-strings) or whose callee is some other
// attribute (myobj.import_module).

var (
	// pyTrimSpaceRe strips whitespace around a captured dotted receiver.
	pyTrimSpaceRe = strings.NewReplacer(" ", "", "\t", "")

	// pyFromImportRe captures the module part of a from-statement. Alternation
	// order matters: the relative forms come first so "from .. import x" is not
	// swallowed by the absolute pattern, and the dotted-suffix form comes before
	// the bare-dots form so "from .pkg import x" keeps its module.
	pyFromImportRe = regexp.MustCompile(`^from[ \t]+(\.+[A-Za-z_][A-Za-z0-9_.]*|\.+|[A-Za-z_][A-Za-z0-9_.]*)[ \t]+import\b`)
	// pyPlainImportRe captures the tail of an import statement; the tail is split
	// into module names by splitPyImportNames.
	pyPlainImportRe = regexp.MustCompile(`^import[ \t]+(.*)$`)
	// pyDynamicImportRe captures an optional dotted receiver, the callee name and
	// the string-literal argument, plus an optional package= literal. Go's regexp
	// has no lookbehind, so a leading non-word/non-dot guard is matched
	// explicitly and the receiver is validated in code.
	pyDynamicImportRe = regexp.MustCompile(
		`(?:^|[^\w.])((?:[A-Za-z_][A-Za-z0-9_]*[ \t]*\.[ \t]*)?)(?:__import__|import_module)` +
			`[ \t]*\([ \t]*('[^'\n]*'|"[^"\n]*")` +
			`(?:[^)\n]*?package[ \t]*=[ \t]*('[^'\n]*'|"[^"\n]*"))?`)
)

// scanPyImportEdges collects the import edges of a single source file without
// building a syntax tree.
func scanPyImportEdges(src string) []pyImportEdge {
	var edges []pyImportEdge
	seen := make(map[string]bool)
	add := func(edge pyImportEdge) {
		key := edge.kind.String() + "\x00" + edge.dots + "\x00" + edge.module + "\x00" + edge.pkg
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, edge)
	}

	// Statements are matched against text where comments and string bodies are
	// blanked, so only real import statements remain.
	code := pyBlankNonCode(src, true)
	for _, line := range strings.Split(code, "\n") {
		// A line may hold several simple statements: "import os; from b import c".
		for _, stmt := range strings.Split(line, ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if m := pyFromImportRe.FindStringSubmatch(stmt); m != nil {
				target := m[1]
				if strings.HasPrefix(target, ".") {
					dots := strings.TrimLeft(target, ".")
					add(pyImportEdge{
						kind:   pyImportRelative,
						dots:   target[:len(target)-len(dots)],
						module: dots,
					})
				} else {
					add(pyImportEdge{kind: pyImportAbsolute, module: target})
				}
				continue
			}
			if m := pyPlainImportRe.FindStringSubmatch(stmt); m != nil {
				for _, mod := range splitPyImportNames(m[1]) {
					add(pyImportEdge{kind: pyImportAbsolute, module: mod})
				}
			}
		}
	}

	// Dynamic calls need their string literal, so scan text that keeps one-line
	// strings but still blanks comments and docstrings.
	dyn := pyBlankNonCode(src, false)
	for _, m := range pyDynamicImportRe.FindAllStringSubmatch(dyn, -1) {
		switch receiver := pyTrimSpaceRe.Replace(m[1]); receiver {
		case "", "importlib.":
		default:
			continue
		}
		mod := unescapePyString(strings.Trim(m[2], "\"'"))
		if mod == "" {
			continue
		}
		pkg := ""
		if m[3] != "" {
			pkg = unescapePyString(strings.Trim(m[3], "\"'"))
		}
		add(pyImportEdge{kind: pyImportDynamic, module: mod, pkg: pkg})
	}
	return edges
}

// pyBlankNonCode copies src and replaces comment text with spaces. When
// blankStrings is set, one-line string bodies are blanked too. Triple-quoted
// bodies are always blanked, because a docstring can hold import-looking lines
// at column zero. Delimiters are kept and every scan advances past them, so
// quotes inside comments (common in license headers) cannot cascade.
func pyBlankNonCode(src string, blankStrings bool) string {
	out := []byte(src)
	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	for i := 0; i < len(out); {
		c := out[i]
		switch {
		case c == '#':
			end := i
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end
		case c == '"' || c == '\'':
			triple := i+2 < len(out) && out[i+1] == c && out[i+2] == c
			closerLen, body := 1, i+1
			if triple {
				closerLen, body = 3, i+3
			}
			end := pyStringBodyEnd(out, body, c, triple)
			if triple || blankStrings {
				blank(body, end)
			}
			if end >= len(out) {
				return string(out)
			}
			i = end + closerLen
		default:
			i++
		}
	}
	return string(out)
}

// pyStringBodyEnd returns the index of the closing delimiter of a string
// literal whose content starts at body, honouring backslash escapes. A
// single-line literal also ends at end of line, so an unterminated quote cannot
// swallow the rest of the file. Byte comparisons only: this runs per character.
func pyStringBodyEnd(out []byte, body int, quote byte, triple bool) int {
	for i := body; i < len(out); i++ {
		if out[i] == '\\' {
			i++
			continue
		}
		if out[i] == '\n' && !triple {
			return i
		}
		if out[i] != quote {
			continue
		}
		if !triple {
			return i
		}
		if i+2 < len(out) && out[i+1] == quote && out[i+2] == quote {
			return i
		}
	}
	return len(out)
}
