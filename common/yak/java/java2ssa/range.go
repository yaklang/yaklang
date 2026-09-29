package java2ssa

import "github.com/yaklang/yaklang/common/yak/ssa"

func (y *singleFileBuilder) SetRange(token ssa.CanStartStopToken) func() {
	if y == nil || y.FunctionBuilder == nil {
		return func() {}
	}
	// Lazy method builds may temporarily switch y.FunctionBuilder while nested
	// ranges are active. Restore the range on the builder that owned it at entry,
	// even if the frontend's current builder has since changed.
	owner := y.FunctionBuilder
	editor := owner.GetEditor()
	if editor == nil {
		return func() {}
	}
	r := ssa.GetRange(editor, token)
	ssa.SlimRangeToken(token)
	if r == nil {
		return func() {}
	}
	backup := owner.CurrentRange
	owner.CurrentRange = r

	// fix template language range
	prog := owner.GetProgram().GetApplication()
	if prog == nil {
		return func() {}
	}
	if t := prog.TryGetTemplate(editor.GetFilename()); t != nil {
		m := t.GetRangeMap()
		line := owner.CurrentRange.GetStart().GetLine()
		if tr, ok := m[line]; ok && tr != nil {
			backup = tr
			owner.CurrentRange = tr
		}
	}

	return func() {
		owner.CurrentRange = backup
	}
}
