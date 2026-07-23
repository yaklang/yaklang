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
	}
	outLines := ctx.expandSource(src, tu.entryPath)
	out := strings.Join(outLines, "\n")
	// Single collapse at end (logical lines already joined; macro bodies may reintroduce \\n).
	return CollapsePreprocessorContinuations(out)
}

func (ctx *tuExpandCtx) syncExpandEnv() {
	// Rebuild internal tables once per define/undef; maps stay shared on MacroTables side.
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

		if _, _, ok := ParseIncludePath(line); ok {
			continue
		}

		switch DirectiveName(line) {
		case "define":
			if ApplyDefineLine(line, &ctx.localTables, true) {
				// localTables mutated in place; env.tables aliases the same maps.
				ctx.syncExpandEnv()
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
			outLines = append(outLines, line)
		default:
			if !ctx.expandEnv.lineMayNeedExpand(line) {
				outLines = append(outLines, line)
				continue
			}
			expanded := ctx.expandEnv.expandSourceWithState(line, ctx.commentState)
			outLines = append(outLines, expanded)
		}
	}
	return outLines
}
