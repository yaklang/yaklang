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
}

func (tu *tuProcessor) run(src string) string {
	// One clone of the base tables; env and expandEnv share / derive from it.
	localTables := tu.base.Clone()
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
	return CollapsePreprocessorContinuations(out)
}

func (ctx *tuExpandCtx) syncExpandEnv() {
	// Rebuild the expander's internal tables from the (possibly mutated) local
	// exported tables. exportToMacroTables produces a fresh internal map pair, so
	// expandEnv.tables and localTables hold independent map instances after this
	// call; subsequent #define/#undef must update localTables and call this again.
	ctx.expandEnv.setTables(exportToMacroTables(ctx.localTables))
}

func (ctx *tuExpandCtx) expandSource(src, filePath string) []string {
	prevFrom := ctx.fromPath
	ctx.fromPath = normalizeSlash(filePath)
	defer func() { ctx.fromPath = prevFrom }()

	var outLines []string
	for _, line := range JoinLogicalLines(src) {
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
			if ApplyDefineLine(line, &ctx.localTables, true) {
				// localTables mutated; resync the expander's internal tables.
				ctx.syncExpandEnv()
				continue
			}
			if ctx.collectOnly > 0 {
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			macro := ppFirstIdent(DirectiveRest(line))
			if macro != "" {
				delete(ctx.localTables.Function, macro)
				delete(ctx.localTables.Object, macro)
				ctx.syncExpandEnv()
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
			if !ctx.expandEnv.lineMayNeedExpand(line) {
				outLines = append(outLines, line)
				continue
			}
			expanded := ctx.expandEnv.expandSourceWithState(line, ctx.commentState)
			outLines = append(outLines, expanded)
		}
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
	ctx.collectOnly++
	lines := ctx.expandSource(string(h.Content), h.Path)
	ctx.collectOnly--
	ctx.includeDepth--
	ctx.cond = savedCond
	ctx.typeAcc = savedAcc
	return lines
}
