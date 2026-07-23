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
	env := NewMacroEnvironment(nil)
	env.tables = tu.base.Clone()
	cond := NewConditionalStack(env, tu.defs)
	localTables := tu.base.Clone()
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
	return CollapsePreprocessorContinuations(out)
}

func (ctx *tuExpandCtx) syncExpandEnv() {
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
				ctx.env.tables = ctx.localTables.Clone()
				ctx.syncExpandEnv()
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			macro := ppFirstIdent(DirectiveRest(line))
			if macro != "" {
				delete(ctx.localTables.Function, macro)
				delete(ctx.localTables.Object, macro)
				ctx.env.tables = ctx.localTables.Clone()
				ctx.syncExpandEnv()
			}
			outLines = append(outLines, line)
		default:
			// Reuse expandEnv; avoid per-line Flatten + exportToMacroTables.
			expanded := ctx.expandEnv.expandSourceWithState(line, ctx.commentState)
			outLines = append(outLines, CollapsePreprocessorContinuations(expanded))
		}
	}
	return outLines
}
