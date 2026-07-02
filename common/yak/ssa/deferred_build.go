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
	id      string
	unitKey string
	*LazyBuilder
}

func newDeferredBuildTask(kind DeferredBuildKind, name string, unitKey string, work func()) *deferredBuildTask {
	task := &deferredBuildTask{
		id:          string(kind) + ":" + name,
		unitKey:     unitKey,
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
	prog.registerDeferredBuildTask(newDeferredBuildTask(kind, name, prog.CurrentCompileUnit(), work))
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

// RunDeferredBuildsForUnits 只执行归属给定编译单元的延迟构建任务，执行后从队列移除。
// 用于编译单元粒度流式编译：每个单元编译完即释放其体 AST，内存上界与项目总规模解耦。
func (prog *Program) RunDeferredBuildsForUnits(unitKeys []string, afterEach func(index int, total int) bool) bool {
	if prog == nil || len(unitKeys) == 0 {
		return true
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	if app.deferredBuilds == nil {
		return true
	}
	units := make(map[string]struct{}, len(unitKeys))
	for _, unitKey := range unitKeys {
		if unitKey != "" {
			units[unitKey] = struct{}{}
		}
	}
	if len(units) == 0 {
		return true
	}

	// 先统计匹配单元的任务总数，供 afterEach 进度回调使用。
	total := 0
	keys := app.deferredBuilds.Keys()
	for _, id := range keys {
		task, ok := app.deferredBuilds.Get(id)
		if !ok || task == nil {
			continue
		}
		if _, match := units[task.unitKey]; match {
			total++
		}
	}

	completed := 0
	for _, id := range keys {
		task, ok := app.deferredBuilds.Get(id)
		if !ok || task == nil {
			continue
		}
		if _, match := units[task.unitKey]; !match {
			continue
		}
		// 执行 task 期间恢复其所属编译单元上下文，task 内通过 CurrentCompileUnit()
		// 取到的 unitKey 与注册时一致；执行完恢复原值。
		previousUnit := app.currentCompileUnit
		if task.unitKey != "" {
			app.currentCompileUnit = task.unitKey
		}
		task.Build()
		app.currentCompileUnit = previousUnit
		task.release()
		app.deferredBuilds.Delete(id)
		completed++
		if afterEach != nil {
			if !afterEach(completed, total) {
				return false
			}
		}
	}
	return true
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
