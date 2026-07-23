package preprocess

type macroCollector struct {
	project   *CPreprocessProject
	env       *MacroEnvironment
	depth     int
	seenFiles map[string]bool
}

// buildMacroTables collects macros from all project headers and the entry translation unit.
// Macro expansion is include-agnostic: headers do not need to be #included to contribute macros.
// Header contribution is cached on the project; only the TU is rescanned per call.
func (p *CPreprocessProject) buildMacroTables(entryPath, src string) MacroTables {
	out := p.getHeaderMacroTables().Clone()
	out.MergeFrom(p.collectMacrosFromSource(entryPath, src))
	return out
}

// collectMacroEnvironment is kept for manual/integration checks.
func (p *CPreprocessProject) collectMacroEnvironment(entryPath, src string) MacroTables {
	return p.buildMacroTables(entryPath, src)
}

func (p *CPreprocessProject) collectMacrosFromSource(filePath, src string) MacroTables {
	mc := &macroCollector{
		project:   p,
		env:       NewMacroEnvironment(nil),
		seenFiles: make(map[string]bool),
	}
	mc.scanSource(filePath, src, p.config.Defines)
	return mc.env.Flatten()
}

func (mc *macroCollector) scanSource(filePath, src string, defs map[string]string) {
	if mc.depth >= mc.project.config.MaxIncludeDepth {
		return
	}
	norm := normalizeSlash(filePath)
	if mc.seenFiles[norm] {
		return
	}
	mc.seenFiles[norm] = true
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
		if _, _, ok := ParseIncludePath(line); ok {
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
	baseTables := p.buildMacroTables(entryPath, src)

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
