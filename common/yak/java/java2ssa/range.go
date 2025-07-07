package java2ssa

import "github.com/yaklang/yaklang/common/yak/ssa"

func (y *singleFileBuilder) SetRange(token ssa.CanStartStopToken) func() {
	if y == nil {
		return func() {}
	}
	editor := y.GetEditor()
	if editor == nil {
		return func() {}
	}
	r := ssa.GetRange(editor, token)
	if r == nil {
		return func() {}
	}
	backup := y.CurrentRange
	y.CurrentRange = r

	// fix template language range
	prog := y.GetProgram().GetApplication()
	if prog == nil {
		return func() {}
	}
	if t := prog.TryGetTemplate(editor.GetFilename()); t != nil {
		m := t.GetRangeMap()
		line := y.CurrentRange.GetStart().GetLine()
		if tr, ok := m[line]; ok && tr != nil {
			backup = tr
			y.CurrentRange = tr
		}
	}

	return func() {
		y.CurrentRange = backup
	}
}
