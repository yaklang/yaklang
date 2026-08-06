package ssaapi

// DedupeProgramsCoveredByOverlay drops standalone programs that are already
// covered as a lower layer of another program's overlay.
func DedupeProgramsCoveredByOverlay(programs []*Program) []*Program {
	if len(programs) <= 1 {
		return programs
	}
	covered := make(map[string]struct{})
	for _, prog := range programs {
		if prog == nil {
			continue
		}
		overlay := prog.GetOverlay()
		if overlay == nil || !overlay.IsTopLayerProgram(prog) {
			continue
		}
		for _, name := range overlay.ProgramNames() {
			if name == "" || name == prog.GetProgramName() {
				continue
			}
			covered[name] = struct{}{}
		}
		if base := prog.GetBaseProgramName(); base != "" && base != prog.GetProgramName() {
			covered[base] = struct{}{}
		}
	}
	out := make([]*Program, 0, len(programs))
	for _, prog := range programs {
		if prog == nil {
			continue
		}
		if _, ok := covered[prog.GetProgramName()]; ok {
			log.Infof("SyntaxFlow: skip program %s (covered by overlay of another program)", prog.GetProgramName())
			continue
		}
		out = append(out, prog)
	}
	return out
}

// AssembleSyntaxFlowQueryTargets maps each program to its SyntaxFlowQueryInstance
// (overlay when this program is the top layer, otherwise the program itself).
func AssembleSyntaxFlowQueryTargets(programs []*Program) []SyntaxFlowQueryInstance {
	out := make([]SyntaxFlowQueryInstance, 0, len(programs))
	for _, prog := range programs {
		if prog == nil {
			continue
		}
		if target := prog.AsSyntaxFlowQueryInstance(); target != nil {
			out = append(out, target)
		}
	}
	return out
}

// PrepareSyntaxFlowQueryTargets dedupes overlay-covered programs and assembles
// SyntaxFlowQueryInstance entrypoints. Upper layers should use the returned
// targets and not branch on GetOverlay().
func PrepareSyntaxFlowQueryTargets(programs []*Program) ([]*Program, []SyntaxFlowQueryInstance) {
	programs = DedupeProgramsCoveredByOverlay(programs)
	return programs, AssembleSyntaxFlowQueryTargets(programs)
}
