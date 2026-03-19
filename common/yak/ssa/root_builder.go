package ssa

import "github.com/yaklang/yaklang/common/utils/memedit"

type TopLevelBuilder struct {
	name    string
	program *Program
	editor  *memedit.MemEditor
	builder *FunctionBuilder
	*LazyBuilder
}

func NewTopLevelBuilder(name string, prog *Program, editor *memedit.MemEditor, builder *FunctionBuilder, work func(*FunctionBuilder)) *TopLevelBuilder {
	node := &TopLevelBuilder{
		name:        name,
		program:     prog,
		editor:      editor,
		builder:     builder,
		LazyBuilder: NewLazyBuilder("RootBuild:" + string(RootBuildKindTopLevel) + ":" + name),
	}
	node.AddLazyBuilder(func() {
		if node == nil || node.program == nil || node.builder == nil {
			return
		}

		app := node.program.GetApplication()
		if app == nil {
			app = node.program
		}

		originEditor := node.builder.GetEditor()
		node.builder.SetEditor(node.editor)
		if originEditor == nil && node.editor != nil {
			enter, ok := node.builder.GetBasicBlockByID(node.builder.EnterBlock)
			if ok && enter != nil && enter.GetRange() == nil {
				enter.SetRange(node.editor.GetFullRange())
			}
		}
		if originEditor != nil && node.editor != nil {
			originEditor.PushSourceCodeContext(node.editor.SourceCodeMd5())
		}
		if node.editor != nil {
			app.PushEditor(node.editor)
		}
		defer func() {
			node.builder.SetEditor(originEditor)
			if node.editor != nil {
				app.PopEditor(true)
			}
		}()

		if work != nil {
			work(node.builder)
		}
	})
	return node
}

func (t *TopLevelBuilder) ID() string {
	if t == nil {
		return ""
	}
	return string(RootBuildKindTopLevel) + ":" + t.name
}
