package preprocess

import "strings"

type tuExpandCtx struct {
	tu           *tuProcessor
	env          *MacroEnvironment
	cond         *ConditionalStack
	localTables  MacroTables
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
		commentState: &commentState,
		fromPath:     tu.entryPath,
	}
	outLines := ctx.expandSource(src, tu.entryPath)
	out := strings.Join(outLines, "\n")
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

		if _, _, ok := ParseIncludePath(line); ok {
			continue
		}

		switch DirectiveName(line) {
		case "define":
			if ApplyDefineLine(line, &ctx.localTables, true) {
				ctx.env.tables = ctx.localTables.Clone()
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			macro := ppFirstIdent(DirectiveRest(line))
			if macro != "" {
				delete(ctx.localTables.Function, macro)
				delete(ctx.localTables.Object, macro)
				ctx.env.tables = ctx.localTables.Clone()
			}
			outLines = append(outLines, line)
		default:
			expanded := ExpandSourceWithTablesState(line, ctx.env.Flatten(), ctx.commentState)
			outLines = append(outLines, expanded)
		}
	}
	return outLines
}
