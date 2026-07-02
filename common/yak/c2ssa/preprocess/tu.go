package preprocess

import (
	"strings"
)

type macroCollector struct {
	project      *CPreprocessProject
	env          *MacroEnvironment
	depth        int
	seenIncludes map[string]bool
}

func (p *CPreprocessProject) collectMacroEnvironment(entryPath, src string) MacroTables {
	mc := &macroCollector{
		project:      p,
		env:          NewMacroEnvironment(nil),
		seenIncludes: make(map[string]bool),
	}
	mc.processSource(entryPath, src, p.config.Defines)
	return mc.env.Flatten()
}

func (mc *macroCollector) processSource(filePath, src string, defs map[string]string) {
	if mc.depth >= mc.project.config.MaxIncludeDepth {
		return
	}
	norm := normalizeSlash(filePath)
	if mc.seenIncludes[norm] {
		return
	}
	mc.seenIncludes[norm] = true
	mc.depth++

	localEnv := NewMacroEnvironment(mc.env)
	cond := NewConditionalStackWithGlobal(localEnv, mc.env, defs)

	for _, line := range JoinLogicalLines(src) {
		if handleCondDirective(cond, line) {
			continue
		}
		if !cond.Active() {
			continue
		}

		if incPath, system, ok := ParseIncludePath(line); ok {
			if stored, found := mc.project.resolver.Resolve(incPath, system, filePath); found {
				if content, ok := mc.project.ReadHeader(stored); ok {
					mc.processSource(stored, string(content), defs)
					localEnv.MergeFrom(mc.env)
				}
			}
			continue
		}

		switch DirectiveName(line) {
		case "define":
			localEnv.ApplyDefineLine(line)
		case "undef":
			macro := ppFirstIdent(DirectiveRest(line))
			if macro != "" {
				localEnv.ApplyUndef(macro)
			}
		}
	}

	mc.env.MergeFrom(localEnv)
	mc.depth--
}

func handleCondDirective(cond *ConditionalStack, line string) bool {
	name := DirectiveName(line)
	switch name {
	case "if", "ifdef", "ifndef", "elif", "else", "endif":
		cond.HandleDirective(line)
		return true
	default:
		return false
	}
}

func (p *CPreprocessProject) PreprocessTU(entryPath, src string) (string, error) {
	baseTables := p.collectMacroEnvironment(entryPath, src)

	tu := &tuProcessor{
		project:   p,
		entryPath: normalizeSlash(entryPath),
		base:      baseTables,
		defs:      p.config.Defines,
	}
	return tu.run(src), nil
}

type tuProcessor struct {
	project   *CPreprocessProject
	entryPath string
	base      MacroTables
	defs      map[string]string
}

func (tu *tuProcessor) run(src string) string {
	env := NewMacroEnvironment(nil)
	env.tables = tu.base.Clone()
	cond := NewConditionalStack(env, tu.defs)
	localTables := tu.base.Clone()

	var outLines []string
	var commentState macroScanState
	for _, line := range JoinLogicalLines(src) {
		if handleCondDirective(cond, line) {
			continue
		}
		if !cond.Active() {
			continue
		}

		if incPath, system, ok := ParseIncludePath(line); ok {
			outLines = append(outLines, line)
			tu.mergeIncludeMacros(incPath, system, &localTables, env)
			continue
		}

		switch DirectiveName(line) {
		case "define":
			if ApplyDefineLine(line, &localTables, true) {
				env.tables = localTables.Clone()
				continue
			}
			outLines = append(outLines, line)
		case "undef":
			macro := ppFirstIdent(DirectiveRest(line))
			if macro != "" {
				delete(localTables.Function, macro)
				delete(localTables.Object, macro)
				env.tables = localTables.Clone()
			}
			outLines = append(outLines, line)
		default:
			expanded := ExpandSourceWithTablesState(line, env.Flatten(), &commentState)
			outLines = append(outLines, expanded)
		}
	}

	out := strings.Join(outLines, "\n")
	return CollapsePreprocessorContinuations(out)
}

func (tu *tuProcessor) mergeIncludeMacros(incPath string, system bool, localTables *MacroTables, env *MacroEnvironment) {
	if stored, found := tu.project.resolver.Resolve(incPath, system, tu.entryPath); found {
		if content, ok := tu.project.ReadHeader(stored); ok {
			extra := tu.project.collectMacroEnvironment(stored, string(content))
			localTables.MergeFrom(extra)
			env.tables = localTables.Clone()
		}
	}
}
