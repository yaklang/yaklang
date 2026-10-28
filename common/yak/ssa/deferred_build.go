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
	// Binding runs after every skeleton and before hierarchy and bodies.
	// Frontends use it for import and name links that cannot be resolved
	// while another worker still owns a package.
	DeferredBuildKindBinding DeferredBuildKind = "binding"
	// Hierarchy runs after binding and before bodies, so type relations exist
	// no matter which library is built first.
	DeferredBuildKindHierarchy DeferredBuildKind = "hierarchy"
)

type deferredBuildTask struct {
	id      string
	kind    DeferredBuildKind
	unitKey string
	*LazyBuilder
}

func newDeferredBuildTask(kind DeferredBuildKind, name string, unitKey string, work func()) *deferredBuildTask {
	task := &deferredBuildTask{
		id:          string(kind) + ":" + name,
		kind:        kind,
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
	app.deferredBuildsMu.Lock()
	defer app.deferredBuildsMu.Unlock()
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
	app.deferredBuildsMu.Lock()
	defer app.deferredBuildsMu.Unlock()
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
	index := 0
	for {
		app.deferredBuildsMu.Lock()
		if app.deferredBuilds == nil {
			app.deferredBuildsMu.Unlock()
			return true
		}
		total := app.deferredBuilds.Len()
		if index >= total {
			app.deferredBuildsMu.Unlock()
			break
		}
		task, ok := app.deferredBuilds.GetByIndex(index)
		app.deferredBuildsMu.Unlock()
		if !ok || task == nil {
			index++
			continue
		}
		task.Build()
		task.release()
		if afterEach != nil {
			app.deferredBuildsMu.Lock()
			if app.deferredBuilds != nil && app.deferredBuilds.Len() > total {
				total = app.deferredBuilds.Len()
			}
			app.deferredBuildsMu.Unlock()
			if !afterEach(index+1, total) {
				return false
			}
		}
		index++
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
	app.deferredBuildsMu.Lock()
	app.deferredBuilds = nil
	app.deferredBuildsMu.Unlock()
}

// RunDeferredBuildsForUnits 只执行归属给定编译单元的延迟构建任务，执行后从队列移除。
// 用于编译单元粒度流式编译：每个单元编译完即释放其体 AST，内存上界与项目总规模解耦。
// RunDeferredBuildsForUnits executes all deferred-build tasks whose unitKey
// matches any of the given unitKeys. afterEach is called after every task
// with (completed, total). afterUnit is called after the LAST task of a
// given unitKey completes — this is the true per-unit completion point.
// Both callbacks may be nil. If either returns false, execution stops.
func (prog *Program) RunDeferredBuildsForUnits(unitKeys []string, afterEach func(index int, total int) bool) bool {
	return prog.RunDeferredBuildsForUnitsWithUnitCallback(unitKeys, afterEach, nil)
}

// RunDeferredBuildsForUnitsWithUnitCallback is like RunDeferredBuildsForUnits
// but adds afterUnit: called when ALL deferred tasks for a specific unitKey
// have completed. This enables per-unit FlushCompileUnit: the caller can
// flush instructions belonging to a completed unit without waiting for the
// entire batch to finish. Task execution order is unchanged (same as
// RunDeferredBuildsForUnits). If tasks from different units interleave,
// afterUnit fires only when that unit's remaining task count reaches zero.
func (prog *Program) RunDeferredBuildsForUnitsWithUnitCallback(
	unitKeys []string,
	afterEach func(index int, total int) bool,
	afterUnit func(unitKey string) bool,
) bool {
	return prog.runDeferredBuildsForUnits(unitKeys, afterEach, afterUnit, nil)
}

// RunDeferredBuildsForUnitsOfKind runs the tasks of one kind that belong to
// unitKeys. The kind is the frontend's phase, not a language name.
func (prog *Program) RunDeferredBuildsForUnitsOfKind(
	unitKeys []string,
	kind DeferredBuildKind,
	afterEach func(index int, total int) bool,
	afterUnit func(unitKey string) bool,
) bool {
	return prog.runDeferredBuildsForUnits(unitKeys, afterEach, afterUnit, func(task *deferredBuildTask) bool {
		return task != nil && task.kind == kind
	})
}

func (prog *Program) runDeferredBuildsForUnits(
	unitKeys []string,
	afterEach func(index int, total int) bool,
	afterUnit func(unitKey string) bool,
	match func(*deferredBuildTask) bool,
) bool {
	if prog == nil || len(unitKeys) == 0 {
		return true
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
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

	app.deferredBuildsMu.Lock()
	if app.deferredBuilds == nil {
		app.deferredBuildsMu.Unlock()
		return true
	}

	// Count tasks per unit and total, in a single pass over the keys.
	total := 0
	unitRemaining := make(map[string]int, len(units))
	keys := app.deferredBuilds.Keys()
	queue := make([]string, 0, len(keys))
	queued := make(map[string]struct{}, len(keys))
	for _, id := range keys {
		task, ok := app.deferredBuilds.Get(id)
		if !ok || task == nil {
			continue
		}
		if _, match := units[task.unitKey]; !match {
			continue
		}
		if match != nil && !match(task) {
			continue
		}
		total++
		unitRemaining[task.unitKey]++
		queue = append(queue, id)
		queued[id] = struct{}{}
	}
	app.deferredBuildsMu.Unlock()

	completed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		app.deferredBuildsMu.Lock()
		task, ok := app.deferredBuilds.Get(id)
		app.deferredBuildsMu.Unlock()
		if !ok || task == nil {
			continue
		}
		if _, match := units[task.unitKey]; !match {
			continue
		}
		// Restore only this goroutine's registration context. A task may visit
		// another library or register more work, but must not overwrite a
		// concurrent worker's context. Restore even if the task panics.
		func() {
			previousUnit := app.CurrentCompileUnit()
			if task.unitKey != "" {
				app.BeginCompileUnit(task.unitKey)
			}
			defer app.BeginCompileUnit(previousUnit)
			task.Build()
		}()
		task.release()
		app.deferredBuildsMu.Lock()
		app.deferredBuilds.Delete(id)
		var newcomers []string
		for _, nid := range app.deferredBuilds.Keys() {
			if _, seen := queued[nid]; seen {
				continue
			}
			ntask, ok := app.deferredBuilds.Get(nid)
			if !ok || ntask == nil {
				continue
			}
			if _, match := units[ntask.unitKey]; !match {
				continue
			}
			if match != nil && !match(ntask) {
				continue
			}
			queued[nid] = struct{}{}
			newcomers = append(newcomers, nid)
			total++
			unitRemaining[ntask.unitKey]++
		}
		app.deferredBuildsMu.Unlock()
		queue = append(queue, newcomers...)
		completed++
		if afterEach != nil {
			if !afterEach(completed, total) {
				return false
			}
		}
		// Per-unit completion: decrement this unit's remaining count.
		// When it reaches zero, all tasks for this unit are done.
		unitKey := task.unitKey
		unitRemaining[unitKey]--
		if unitRemaining[unitKey] <= 0 && afterUnit != nil {
			if !afterUnit(unitKey) {
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
