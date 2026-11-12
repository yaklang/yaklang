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
	return CollapsePreprocessorContinuations(out)
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
