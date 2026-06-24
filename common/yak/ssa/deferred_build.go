package ssa

import (
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type DeferredBuildKind string

const (
	DeferredBuildKindFunction  DeferredBuildKind = "function"
	DeferredBuildKindBlueprint DeferredBuildKind = "blueprint"
	DeferredBuildKindFile      DeferredBuildKind = "file"
	DeferredBuildKindHelper    DeferredBuildKind = "helper"
)

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

func (prog *Program) registerDeferredBuildTask(task *deferredBuildTask) *deferredBuildTask {
	if prog == nil || task == nil {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	if app.deferredBuilds == nil {
		app.deferredBuilds = omap.NewEmptyOrderedMap[string, *deferredBuildTask]()
	}
	if existing, ok := app.deferredBuilds.Get(task.id); ok {
		return existing
	}
	app.deferredBuilds.Set(task.id, task)
	app.deferredBuildTotal++
	return task
}

func (prog *Program) RegisterDeferredBuild(kind DeferredBuildKind, name string, work func()) {
	prog.registerDeferredBuildTask(newDeferredBuildTask(kind, name, work))
}

func (prog *Program) RegisterDeferredFunction(name string, fun *Function) {
	if fun == nil {
		return
	}
	prog.RegisterDeferredBuild(DeferredBuildKindFunction, name, func() {
		fun.Build()
	})
}

func (prog *Program) RegisterDeferredBlueprint(name string, blueprint *Blueprint) {
	if blueprint == nil {
		return
	}
	prog.RegisterDeferredBuild(DeferredBuildKindBlueprint, name, func() {
		blueprint.Build()
	})
}

func (prog *Program) RegisterFileBuild(name string, editor *memedit.MemEditor, builder *FunctionBuilder, work func(*FunctionBuilder)) {
	if builder == nil {
		return
	}
	prog.RegisterDeferredBuild(DeferredBuildKindFile, name, func() {
		buildWithEditor(prog, editor, builder, work)
	})
}

func (prog *Program) RunDeferredBuilds() {
	_ = prog.RunDeferredBuildsWithCallback(nil)
}

func (prog *Program) DeferredBuildCount() int {
	if prog == nil {
		return 0
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	return app.deferredBuildTotal
}

func (prog *Program) RunDeferredBuildsWithCallback(afterEach func(index int, total int) bool) bool {
	if prog == nil {
		return true
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	if app.deferredBuilds == nil {
		return true
	}
	total := app.deferredBuilds.Len()
	for index := 0; index < app.deferredBuilds.Len(); index++ {
		task, ok := app.deferredBuilds.GetByIndex(index)
		if !ok || task == nil {
			continue
		}
		task.Build()
		task.release()
		if afterEach != nil {
			if app.deferredBuilds.Len() > total {
				total = app.deferredBuilds.Len()
			}
			if !afterEach(index+1, total) {
				return false
			}
		}
	}
	app.releaseDeferredBuildTasks()
	return true
}

func (prog *Program) releaseDeferredBuildTasks() {
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.deferredBuilds = nil
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
