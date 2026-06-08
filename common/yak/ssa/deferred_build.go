package ssa

import "github.com/yaklang/yaklang/common/utils/memedit"

type deferredBuildTask struct {
	id string
	*LazyBuilder
}

func newDeferredBuildTask(kind DeferredBuildKind, name string, work func()) *deferredBuildTask {
	task := &deferredBuildTask{
		id:          string(kind) + ":" + name,
		LazyBuilder: NewLazyBuilder("deferred:" + string(kind) + ":" + name),
	}
	if work != nil {
		task.AddLazyBuilder(work)
	}
	return task
}

func (t *deferredBuildTask) release() {
	if t == nil {
		return
	}
	t.LazyBuilder = nil
}

func buildWithEditor(prog *Program, editor *memedit.MemEditor, builder *FunctionBuilder, work func(*FunctionBuilder)) {
	if prog == nil || builder == nil {
		return
	}

	app := prog.GetApplication()
	if app == nil {
		app = prog
	}

	originEditor := builder.GetEditor()
	builder.SetEditor(editor)
	if originEditor == nil && editor != nil {
		enter, ok := builder.GetBasicBlockByID(builder.EnterBlock)
		if ok && enter != nil && enter.GetRange() == nil {
			enter.SetRange(editor.GetFullRange())
		}
	}
	if originEditor != nil && editor != nil {
		originEditor.PushSourceCodeContext(editor.SourceCodeMd5())
	}
	if editor != nil {
		app.PushEditor(editor)
	}
	defer func() {
		builder.SetEditor(originEditor)
		if editor != nil {
			app.PopEditor(true)
		}
	}()

	if work != nil {
		work(builder)
	}
}
