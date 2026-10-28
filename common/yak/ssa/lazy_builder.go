package ssa

import (
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"go.uber.org/atomic"
)

// lazyTask is one closure queued on a Function/Blueprint LazyBuilder.
type lazyTask func()

// LazyBuilder 是一个并发安全、内存安全的延迟执行器
type LazyBuilder struct {
	_lazybuild_name string
	tasks           []lazyTask
	mu              sync.RWMutex
	build           atomic.Bool
}

// NewLazyBuilder 创建一个新的 LazyBuilder 实例
func NewLazyBuilder(name string) *LazyBuilder {
	lz := &LazyBuilder{
		_lazybuild_name: name + "||" + uuid.NewString(),
		tasks:           make([]lazyTask, 0),
	}
	return lz
}

// Add 添加一个延迟执行的任务。
// work 是要执行的函数，ctx 是要传递给该函数的上下文数据。
func (l *LazyBuilder) AddLazyBuilder(work func(), async ...bool) {
	if l == nil {
		log.Errorf("LazyBuilder is nil")
		return
	}
	// Always queue. NEVER execute immediately even if Build() has already
	// been called: AddGlobalVariable is invoked from buildGenDecl during
	// PreHandler, and executing valueFunc() there compiles large global
	// literals (gbk2utf8/gbk2unicode are 21000-entry maps) synchronously,
	// adding 11-17s per file. Instead Build() is non-idempotent and runs
	// queued tasks when called later (deferred build), when the resident
	// cache is small after batch flushes.
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tasks = append(l.tasks, lazyTask(work))
}

// Build executes all tasks queued since the last Build call. Unlike the
// old idempotent Build (which ran only once), this allows AddGlobalVariable
// to register lazy builders across multiple compile batches: each batch's
// LoadGlobalVariable calls Build(), executing that batch's registrations.
// This also lets valueFunc() run during deferred build when the resident
// cache is small, instead of during PreHandler where it would compile
// large global literals (21000-entry maps) synchronously and stall.
func (l *LazyBuilder) Build() {
	if l == nil {
		return
	}

	// Mark as built so future AddLazyBuilder calls never try to execute
	// immediately (they still queue, and next Build runs them).
	l.build.Store(true)

	l.mu.Lock()
	tasksToRun := l.tasks
	l.tasks = nil
	l.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("lazy builder panic: name=%s panic=%v", l._lazybuild_name, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	for _, task := range tasksToRun {
		if task != nil {
			task()
		}
	}
}

func (l *LazyBuilder) hasPending() bool {
	if l == nil {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.tasks) > 0
}

func (p *Program) drainLazyBuilders() {
	if p == nil {
		return
	}
	for {
		progressed := false
		if p.Blueprint != nil {
			for _, key := range p.Blueprint.Keys() {
				bp, ok := p.Blueprint.Get(key)
				if !ok || bp == nil || !bp.hasPending() {
					continue
				}
				p.runLazyBuilder(bp.LazyBuilder, bp.Range)
				progressed = true
			}
		}
		p.forEachFunction(func(fun *Function) {
			if fun == nil || !fun.hasPending() {
				return
			}
			p.runLazyBuilder(fun.LazyBuilder, fun.GetRange())
			progressed = true
		})
		if !progressed {
			break
		}
	}
	if p.Blueprint != nil {
		for _, key := range p.Blueprint.Keys() {
			bp, ok := p.Blueprint.Get(key)
			if !ok || bp == nil {
				continue
			}
			bp.BuildConstructorAndDestructor()
		}
	}
}

func (p *Program) LazyBuild() {
	p.drainLazyBuilders()
	for _, f := range p.fixImportCallback {
		f()
	}
	function := p.GetFunction(string(MainFunctionName), "")
	if function != nil {
		function.Finish()
	}
	// Missing imports each own a function named @virtual so they do not
	// share one CurrentBlock. Finish every one of them, not only the
	// canonical map entry.
	hasVirtual := false
	p.Funcs.ForEach(func(_ string, fun *Function) bool {
		if fun != nil && fun.GetName() == string(VirtualFunctionName) {
			hasVirtual = true
			fun.Finish()
		}
		return true
	})
	initFunction := p.GetFunction(string(InitFunctionName), "")
	if initFunction != nil {
		initFunction.Finish()
	}
	if function == nil && !hasVirtual && initFunction == nil {
		// Library/placeholder programs may legitimately contain no entry functions.
		// Treat this as "nothing to finish" instead of an error log which is noisy in
		// contexts like SyntaxFlow rule verification and language-server analysis.
		if p.ProgramKind != Application {
			return
		}
		log.Errorf("main function is not found and virtual function is not found")
		return
	}
}

// LazyBuildForUnits drains lazy closures on each unit's library Program
// (functions + blueprints) and then, for application-owned units, the upstream
// libraries the unit depends on. It is the single-pass path for frontends that
// share one application scope (no dependency DAG available); the DAG scheduler
// calls DrainUnscheduledApplicationUpstreams instead so it only picks up
// libraries it did not schedule itself.
//
// A compile unit is that library: it already owns the package's methods and
// classes, so there is no per-task unit tag.
func (prog *Program) LazyBuildForUnits(unitKeys []string) {
	if prog == nil || len(unitKeys) == 0 {
		return
	}
	seen := make(map[*Program]struct{})
	for _, key := range unitKeys {
		lib := prog.ProgramForCompileUnit(key)
		if lib == nil {
			continue
		}
		if _, ok := seen[lib]; ok {
			continue
		}
		seen[lib] = struct{}{}
		drainProgramLazy(lib)
		drainApplicationUpstreams(key, lib, seen)
	}
}

// DrainUnscheduledApplicationUpstreams drains upstream libraries reachable from
// the given application-owned unit keys but not owned by this batch. The DAG
// scheduler builds every library it scheduled, so draining those again would be
// wasted work and could pull later batches early; the unscheduled remainder is
// what the single-pass path used to cover for application-owned units.
func (prog *Program) DrainUnscheduledApplicationUpstreams(unitKeys []string) {
	if prog == nil || len(unitKeys) == 0 {
		return
	}
	seen := make(map[*Program]struct{}, len(unitKeys))
	for _, key := range unitKeys {
		if lib := prog.ProgramForCompileUnit(key); lib != nil {
			seen[lib] = struct{}{}
		}
	}
	for _, key := range unitKeys {
		lib := prog.ProgramForCompileUnit(key)
		if lib == nil {
			continue
		}
		drainApplicationUpstreams(key, lib, seen)
	}
}

// drainApplicationUpstreams drains the upstream libraries of an
// application-owned compile unit. Library-owned units keep their dependencies
// behind the scheduler: expanding them here would defeat unit isolation.
func drainApplicationUpstreams(unitKey string, lib *Program, seen map[*Program]struct{}) {
	if lib == nil || lib.UpStream == nil || libraryNameFromCompileUnitKey(unitKey) != "" {
		return
	}
	lib.UpStream.ForEach(func(_ string, child *Program) bool {
		if child == nil {
			return true
		}
		if _, ok := seen[child]; ok {
			return true
		}
		seen[child] = struct{}{}
		drainProgramLazy(child)
		return true
	})
}

// LazyBuildCurrentProgram drains only this program's declarations. A scheduler
// that already owns the dependency graph must not implicitly build all upstream
// libraries when it schedules an application-owned unit.
func (p *Program) LazyBuildCurrentProgram() {
	drainProgramLazy(p)
}

func drainProgramLazy(p *Program) {
	if p == nil {
		return
	}
	p.drainLazyBuilders()
	for _, f := range p.fixImportCallback {
		f()
	}
}

func (p *Program) forEachFunction(fn func(*Function)) {
	if p == nil || fn == nil || p.Funcs == nil {
		return
	}
	visited := make(map[*Function]struct{})
	var stack []*Function
	for _, key := range p.Funcs.Keys() {
		fun, ok := p.Funcs.Get(key)
		if !ok || fun == nil {
			continue
		}
		stack = append(stack, fun)
	}
	if p.Blueprint != nil {
		for _, key := range p.Blueprint.Keys() {
			bp, ok := p.Blueprint.Get(key)
			if !ok || bp == nil {
				continue
			}
			if bp.MagicMethod != nil {
				bp.MagicMethod.ForEach(func(_ BlueprintMagicMethodKind, value Value) bool {
					if fun, ok := ToFunction(value); ok && fun != nil {
						stack = append(stack, fun)
					}
					return true
				})
			}
			for _, fun := range bp.NormalMethod {
				if fun != nil {
					stack = append(stack, fun)
				}
			}
			for _, fun := range bp.StaticMethod {
				if fun != nil {
					stack = append(stack, fun)
				}
			}
		}
	}
	for len(stack) > 0 {
		fun := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if fun == nil {
			continue
		}
		if _, ok := visited[fun]; ok {
			continue
		}
		visited[fun] = struct{}{}
		fn(fun)
		for _, childID := range fun.ChildFuncs {
			childValue, ok := fun.GetValueById(childID)
			if !ok || childValue == nil {
				continue
			}
			if childFunc, ok := ToFunction(childValue); ok && childFunc != nil {
				stack = append(stack, childFunc)
			}
		}
	}
}

func (c *Blueprint) BuildConstructorAndDestructor() {
	if c.MagicMethod != nil {
		c.MagicMethod.ForEach(func(_ BlueprintMagicMethodKind, value Value) bool {
			if function, b := ToFunction(value); b {
				function.Build()
			}
			return true
		})
	}
	for _, m := range c.NormalMethod {
		m.Build()
	}
	for _, function := range c.StaticMethod {
		function.Build()
	}
}
