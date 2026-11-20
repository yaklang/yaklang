package preprocess

import "strings"

type tuExpandCtx struct {
	tu           *tuProcessor
	env          *MacroEnvironment
	cond         *ConditionalStack
	localTables  MacroTables
	expandEnv    *macroEnv
	commentState *macroScanState
	fromPath     string
	included     map[string]bool
	includeDepth int
	collectOnly  int
	typeAcc      headerStmtAcc
	// pendingExpand holds a macro invocation split over several source lines.
	pendingExpand []string
}

func (tu *tuProcessor) run(src string) string {
	// tu.base is built per translation unit by PreprocessTU and nothing else
	// holds it, so the TU can mutate it directly. Cloning it here copied the
	// whole macro table (three map passes) only to be mutated immediately.
	localTables := tu.base
	env := &MacroEnvironment{tables: localTables}
	cond := NewConditionalStack(env, tu.defs)
	var commentState macroScanState

	ctx := &tuExpandCtx{
		tu:           tu,
		env:          env,
		cond:         cond,
		localTables:  localTables,
		expandEnv:    newMacroEnvFromTables(localTables),
		commentState: &commentState,
		fromPath:     tu.entryPath,
		included:     map[string]bool{tu.entryPath: true},
	}
	outLines := ctx.expandSource(src, tu.entryPath)
	out := strings.Join(outLines, "\n")
	// Single collapse at end (logical lines already joined; macro bodies may reintroduce \\n).
	out = CollapsePreprocessorContinuations(out)
	// Translation phase 6: join string literals that macro expansion placed side by side.
	return ConcatAdjacentStringLiterals(out)
}

func (ctx *tuExpandCtx) expandSource(src, filePath string) []string {
	prevFrom := ctx.fromPath
	ctx.fromPath = normalizeSlash(filePath)
	defer func() { ctx.fromPath = prevFrom }()

	var outLines []string
	for _, line := range JoinLogicalLines(src) {
		// A line inside a block comment is comment text, not a directive, and
		// nothing in it can expand. Consume it for the scan state so the
		// closing */ is seen, then keep it verbatim.
		if ctx.commentState.inBlockComment {
			advanceMacroScanState(line, ctx.commentState)
			outLines = append(outLines, line)
			continue
		}
		if handleCondDirective(ctx.cond, line) {
			continue
		}
		if !ctx.cond.Active() {
			continue
		}

		if incPath, system, ok := ParseIncludePath(line); ok {
			outLines = append(outLines, ctx.ingestInclude(incPath, system)...)
			continue
		}

		switch DirectiveName(line) {
		case "define":
			if applyDefineLineIncremental(line, &ctx.localTables, ctx.expandEnv) {
				continue
			}
			if ctx.collectOnly > 0 {
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			if macro := ppFirstIdent(DirectiveRest(line)); macro != "" {
				undefineMacroIncremental(macro, &ctx.localTables, ctx.expandEnv)
			}
			if ctx.collectOnly > 0 {
				continue
			}
			outLines = append(outLines, line)
		default:
			if ctx.collectOnly > 0 {
				outLines = append(outLines, ctx.takeHeaderTypeLines(line)...)
				continue
			}
			if len(ctx.pendingExpand) > 0 {
				ctx.pendingExpand = append(ctx.pendingExpand, line)
				joined := strings.Join(ctx.pendingExpand, "\n")
				if macroParenBalance(joined) <= 0 {
					expanded := ctx.expandEnv.expandSourceWithState(joined, ctx.commentState)
					outLines = append(outLines, strings.Split(expanded, "\n")...)
					ctx.pendingExpand = nil
				}
				continue
			}
			if !ctx.expandEnv.lineMayNeedExpand(line) {
				outLines = append(outLines, line)
				continue
			}
			if macroParenBalance(line) > 0 {
				// A macro invocation may span lines (TEST_CASE("x" * tag(...))).
				// Join until the parentheses balance, then expand once.
				ctx.pendingExpand = []string{line}
				continue
			}
			expanded := ctx.expandEnv.expandSourceWithState(line, ctx.commentState)
			outLines = append(outLines, expanded)
		}
	}
	if len(ctx.pendingExpand) > 0 {
		// Unbalanced to EOF: keep the original text instead of dropping code.
		outLines = append(outLines, ctx.pendingExpand...)
		ctx.pendingExpand = nil
	}
	if ctx.collectOnly > 0 {
		if _, lines, keep := ctx.typeAcc.flush(); keep {
			outLines = append(outLines, ctx.expandHeaderTypeLines(lines)...)
		}
	}
	return outLines
}

func (ctx *tuExpandCtx) takeHeaderTypeLines(line string) []string {
	done, lines, keep := ctx.typeAcc.feed(line)
	if !done || !keep {
		return nil
	}
	return ctx.expandHeaderTypeLines(lines)
}

func (ctx *tuExpandCtx) expandHeaderTypeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if ctx.expandEnv.lineMayNeedExpand(line) {
			line = ctx.expandEnv.expandSourceWithState(line, ctx.commentState)
		}
		out = append(out, line)
	}
	return out
}

// ingestInclude follows #include for macros and types only; library functions are not imported.
func (ctx *tuExpandCtx) ingestInclude(incPath string, system bool) []string {
	if ctx.tu == nil || ctx.tu.project == nil || ctx.tu.project.resolver == nil {
		return nil
	}
	max := ctx.tu.project.config.MaxIncludeDepth
	if max <= 0 {
		max = 64
	}
	if ctx.includeDepth >= max {
		return nil
	}
	h, ok := ctx.tu.project.resolver.ResolveHeader(incPath, system, ctx.fromPath)
	if !ok {
		return nil
	}
	key := h.Path
	if ctx.included[key] {
		return nil
	}
	ctx.included[key] = true

	savedCond := ctx.cond
	savedAcc := ctx.typeAcc
	ctx.cond = NewConditionalStack(ctx.env, ctx.tu.defs)
	ctx.typeAcc = headerStmtAcc{}
	ctx.includeDepth++
	if ctx.tu.project.config.InlineProjectHeaders {
		// C++ headers define classes and member functions; keep the header body
		// instead of extracting type declarations only.
		//
		// Project header macros are pre-collected without following includes, so
		// an include guard (SHAPE_HPP) is already defined when the header body is
		// expanded. Undefine the macros this header owns first; its own #define
		// directives then run in include order and the guard behaves normally.
		tables := ctx.tu.project.collectMacrosFromSource(h.Path, string(h.Content))
		for name := range tables.Object {
			undefineMacroIncremental(name, &ctx.localTables, ctx.expandEnv)
		}
		for name := range tables.Function {
			undefineMacroIncremental(name, &ctx.localTables, ctx.expandEnv)
		}
		lines := ctx.expandSource(string(h.Content), h.Path)
		ctx.includeDepth--
		ctx.cond = savedCond
		ctx.typeAcc = savedAcc
		return lines
	}
	ctx.collectOnly++
	lines := ctx.expandSource(string(h.Content), h.Path)
	ctx.collectOnly--
	ctx.includeDepth--
	ctx.cond = savedCond
	ctx.typeAcc = savedAcc
	return lines
}
