package python2ssa

import (
	"regexp"
	"strings"

	antlr "github.com/yaklang/antlr/v4"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CompileUnitDependencies extracts Python import edges by resolving dotted
// module names against a path index built from unit paths and file stems.
// Imports are collected by parsing each file with the Python grammar
// (extractPyImportsFromAST); files that fail to parse fall back to the legacy
// regex scan (fallbackRegexImports) so a syntax error never silently drops
// edges. It handles absolute imports ("import a.b", "from a.b import c"),
// relative imports ("from . import x", "from ..pkg import y"), and dynamic
// imports (importlib.import_module("a.b") / __import__("a.b") with a literal
// argument).
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
			imports, parsed := extractPyImportsFromAST(src)
			if !parsed {
				imports = fallbackRegexImports(src)
			}
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

// extractPyImportsFromAST parses src with the Python grammar and collects every
// import: import statements, from statements (including relative ones and
// parenthesized multi-line lists), and dynamic imports
// (__import__("m") / import_module("m") / importlib.import_module("m") /
// ... with a string-literal first argument). The second return is false when
// the source fails to parse, in which case the caller should fall back to the
// regex scan instead of dropping the file's edges.
func extractPyImportsFromAST(src string) ([]pyImportEdge, bool) {
	root, err := FrontendWithCache(src)
	if err != nil || root == nil {
		return nil, false
	}
	var imports []pyImportEdge
	seen := make(map[string]bool)
	add := func(edge pyImportEdge) {
		key := edge.kind.String() + "\x00" + edge.dots + "\x00" + edge.module + "\x00" + edge.pkg
		if seen[key] {
			return
		}
		seen[key] = true
		imports = append(imports, edge)
	}
	// import a.b / from .mod import x / importlib.import_module("m") / __import__("m")
	var walk func(node antlr.Tree)
	walk = func(node antlr.Tree) {
		if node == nil {
			return
		}
		switch ctx := node.(type) {
		case *pythonparser.Import_stmtContext:
			if names, ok := ctx.Dotted_as_names().(*pythonparser.Dotted_as_namesContext); ok && names != nil {
				for _, entry := range names.AllDotted_as_name() {
					if as, ok := entry.(*pythonparser.Dotted_as_nameContext); ok && as != nil && as.Dotted_name() != nil {
						add(pyImportEdge{kind: pyImportAbsolute, module: as.Dotted_name().GetText()})
					}
				}
			}
		case *pythonparser.From_stmtContext:
			// Relative prefix from the DOT/ELLIPSIS tokens (each ELLIPSIS is
			// three dots, matching resolvePyRelativeImport's dot counting),
			// then the optional dotted module name.
			prefix := strings.Repeat(".", len(ctx.AllDOT())) + strings.Repeat("...", len(ctx.AllELLIPSIS()))
			module := ""
			if dotted := ctx.Dotted_name(); dotted != nil {
				module = dotted.GetText()
			}
			if prefix != "" {
				add(pyImportEdge{kind: pyImportRelative, dots: prefix, module: module})
			} else if module != "" {
				add(pyImportEdge{kind: pyImportAbsolute, module: module})
			}
		case *pythonparser.ExprContext:
			collectPyDynamicImport(ctx, add)
		}
		for i := 0; i < node.GetChildCount(); i++ {
			walk(node.GetChild(i))
		}
	}
	walk(root)
	return imports, true
}

// collectPyDynamicImport matches an expr node that calls a dynamic import
// builtin — __import__("m"), import_module("m") or importlib.import_module("m")
// — with a string-literal first argument, and extracts the module name plus
// the optional package= keyword argument. The grammar is
// `expr: atom trailer*`, so the callee name is the atom text and each
// trailing `.name` becomes a trailer: importlib.import_module("m") parses as
// atom(importlib) + trailer(.import_module) + trailer((...)).
func collectPyDynamicImport(expr *pythonparser.ExprContext, add func(pyImportEdge)) {
	if expr == nil {
		return
	}
	trailers := expr.AllTrailer()
	if len(trailers) == 0 {
		return
	}
	// The call trailer is the last one holding Arguments().
	var call *pythonparser.TrailerContext
	for _, t := range trailers {
		if tc, ok := t.(*pythonparser.TrailerContext); ok && tc != nil && tc.Arguments() != nil {
			call = tc
		}
	}
	if call == nil {
		return
	}
	// Build the callee's dotted name from the atom and the attribute trailers
	// preceding the call: importlib + .import_module -> "importlib.import_module".
	atomCtx, ok := expr.Atom().(*pythonparser.AtomContext)
	if !ok || atomCtx == nil || atomCtx.Name() == nil {
		return
	}
	name := atomCtx.Name().GetText()
	for _, t := range trailers {
		tc, ok := t.(*pythonparser.TrailerContext)
		if !ok || tc == nil {
			break
		}
		if tc == call {
			// The call trailer itself may be attribute access:
			// `trailer: DOT name arguments?` — importlib.import_module("m")
			// parses as ONE trailer holding both name and arguments.
			if tc.Name() != nil {
				name += "." + tc.Name().GetText()
			}
			break
		}
		if tc.DOT() == nil || tc.Name() == nil {
			return
		}
		name += "." + tc.Name().GetText()
	}
	if name != "__import__" && name != "import_module" && name != "importlib.import_module" {
		return
	}
	args, ok := call.Arguments().(*pythonparser.ArgumentsContext)
	if !ok || args == nil || args.OPEN_BRACKET() != nil {
		return
	}
	// First positional argument: the module string literal. Keyword argument
	// detection uses ASSIGN (grammar: `argument: test (comp_for | ASSIGN test)?`).
	mod := ""
	pkg := ""
	if list, ok := args.Arglist().(*pythonparser.ArglistContext); ok && list != nil {
		for _, entry := range list.AllArgument() {
			arg, ok := entry.(*pythonparser.ArgumentContext)
			if !ok || arg == nil || arg.Comp_for() != nil {
				continue
			}
			tests := arg.AllTest()
			if arg.ASSIGN() == nil && len(tests) == 1 {
				if mod == "" {
					mod = pyStringLiteral(tests[0])
				}
				continue
			}
			// keyword argument: key is Test(0), value is Test(1)
			if arg.ASSIGN() != nil && len(tests) > 1 {
				if key, ok := tests[0].(*pythonparser.TestContext); ok && key != nil && key.GetText() == "package" {
					pkg = pyStringLiteral(tests[1])
				}
			}
		}
	}
	if mod == "" {
		return
	}
	add(pyImportEdge{kind: pyImportDynamic, module: mod, pkg: pkg})
}

// pyStringLiteral extracts the value of a test node that is a plain string
// literal (possibly concatenated via the expr arithmetic chain), returning ""
// otherwise.
func pyStringLiteral(test pythonparser.ITestContext) string {
	ctx, ok := test.(*pythonparser.TestContext)
	if !ok || ctx == nil {
		return ""
	}
	// A single string atom: STRING+ under `atom`.
	if logical := ctx.Logical_test(0); logical != nil {
		if logicalCtx, ok := logical.(*pythonparser.Logical_testContext); ok && logicalCtx != nil {
			if comp := logicalCtx.Comparison(); comp != nil {
				if expr := comp.Expr(); expr != nil {
					if exprCtx, ok := expr.(*pythonparser.ExprContext); ok {
						return pyExprStringLiteral(exprCtx)
					}
				}
			}
		}
	}
	return ""
}

// pyExprStringLiteral renders an expr that consists purely of string literals
// concatenated with `+` (or adjacent atoms), returning "" for anything else.
func pyExprStringLiteral(expr *pythonparser.ExprContext) string {
	if expr == nil {
		return ""
	}
	atomCtx, ok := expr.Atom().(*pythonparser.AtomContext)
	if !ok || atomCtx == nil {
		return ""
	}
	literal := pyAtomStringLiteral(atomCtx)
	if literal == "" {
		return ""
	}
	// Only string atoms may carry trailers that still denote a literal? No —
	// any trailer (call/subscript) makes it dynamic, so stop at the first one.
	if len(expr.AllTrailer()) > 0 {
		return ""
	}
	// Sibling exprs joined by `+`: "a" + "b" -> literal only when both sides are.
	parts := []string{literal}
	for _, sibling := range expr.AllExpr() {
		other, ok := sibling.(*pythonparser.ExprContext)
		if !ok {
			return ""
		}
		if other.GetText() != "+" {
			continue
		}
		part := pyExprStringLiteral(other)
		if part == "" {
			return ""
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "")
}

// pyAtomStringLiteral unquotes a string-literal atom (the grammar allows
// adjacent STRING+ tokens), returning "" for non-string atoms and for
// prefixed literals (f"..." / b"..." etc.), which are not static module names.
func pyAtomStringLiteral(atom *pythonparser.AtomContext) string {
	if atom == nil || len(atom.AllSTRING()) == 0 {
		return ""
	}
	head := atom.GetText()
	idx := strings.IndexAny(head, "\"'")
	if idx > 0 {
		switch strings.ToLower(head[:idx]) {
		case "f", "b", "rb", "br", "fr", "rf", "u", "r", "ur":
			return ""
		}
	}
	joined := ""
	for _, s := range atom.AllSTRING() {
		joined += pyUnquote(s.GetText())
	}
	return joined
}

// pyUnquote strips surrounding quotes from a Python string token.
func pyUnquote(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 {
		switch raw[0] {
		case '"', '\'':
			if raw[len(raw)-1] == raw[0] {
				return unescapePyString(raw[1 : len(raw)-1])
			}
		}
	}
	return ""
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

// Legacy regex fallback for files that fail to parse (syntax errors, exotic
// constructs): the pre-AST scanners, kept verbatim so a bad file degrades to
// the old behavior instead of silently losing edges.
var (
	fallbackPyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import\b|import[ \t]+([^#\n]+))`)
	fallbackPyRelRe    = regexp.MustCompile(`(?m)^\s*from\s+(\.+)([A-Za-z_][A-Za-z0-9_.]*)?\s+import\b`)
	fallbackPyDynRe    = regexp.MustCompile(`(?:__import__|importlib\s*\.\s*import_module|import_module)\s*\(\s*["']([^"'\n]+)["'](?:\s*,[^)]*?package\s*=\s*["']([^"'\n]*)["'])?`)
	pyModuleNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	pyImportAsRe       = regexp.MustCompile(`\s+as\s+[A-Za-z_][A-Za-z0-9_]*$`)
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

// fallbackRegexImports scans src with the legacy regexes and returns the same
// pyImportEdge shapes the AST extraction produces.
func fallbackRegexImports(src string) []pyImportEdge {
	var imports []pyImportEdge
	add := func(edge pyImportEdge) {
		imports = append(imports, edge)
	}
	for _, match := range fallbackPyImportRe.FindAllStringSubmatch(src, -1) {
		if mod := match[1]; mod != "" {
			add(pyImportEdge{kind: pyImportAbsolute, module: mod})
			continue
		}
		for _, mod := range splitPyImportNames(match[2]) {
			add(pyImportEdge{kind: pyImportAbsolute, module: mod})
		}
	}
	for _, match := range fallbackPyRelRe.FindAllStringSubmatch(src, -1) {
		add(pyImportEdge{kind: pyImportRelative, dots: match[1], module: match[2]})
	}
	for _, match := range fallbackPyDynRe.FindAllStringSubmatch(src, -1) {
		add(pyImportEdge{kind: pyImportDynamic, module: match[1], pkg: strings.TrimSpace(match[2])})
	}
	return imports
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
