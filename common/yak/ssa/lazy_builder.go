package ssa

import (
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
)

// lazyTask is the unit of deferred work queued in a LazyBuilder.
type lazyTask func()

// LazyBuilder is a concurrency-safe, memory-safe deferred executor.
type LazyBuilder struct {
	_lazybuild_name string
	tasks           []lazyTask
	mu              sync.RWMutex
}

// NewLazyBuilder creates a new LazyBuilder instance.
func NewLazyBuilder(name string) *LazyBuilder {
	lz := &LazyBuilder{
		_lazybuild_name: name + "||" + uuid.NewString(),
		tasks:           make([]lazyTask, 0),
	}
	return lz
}

// AddLazyBuilder queues a task for deferred execution.
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

	// // Execute all queued tasks in order
	for _, task := range tasksToRun {
		if task != nil {
			task()
		}
	}
}

func (p *Program) LazyBuild() {
	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		p.runLazyBuilder(blueprint.LazyBuilder, blueprint.Range)
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

	for len(stack) > 0 {
		// Depth-first traversal of functions and their children so every LazyBuilder runs
		fun := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if fun == nil {
			continue
		}
		if _, ok := visited[fun]; ok {
			continue
		}
		visited[fun] = struct{}{}
		p.runLazyBuilder(fun.LazyBuilder, fun.GetRange())
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
	for _, f := range p.fixImportCallback {
		f()
	}
	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		blueprint.BuildConstructorAndDestructor()
	}
	function := p.GetFunction(string(MainFunctionName), "")
	if function != nil {
		function.Finish()
	}
	virtualFunction := p.GetFunction(string(VirtualFunctionName), "")
	if virtualFunction != nil {
		virtualFunction.Finish()
	}
	initFunction := p.GetFunction(string(InitFunctionName), "")
	if initFunction != nil {
		initFunction.Finish()
	}
	if function == nil && virtualFunction == nil && initFunction == nil {
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

func (c *Blueprint) BuildConstructorAndDestructor() {
	for _, value := range c.MagicMethod {
		if function, b := ToFunction(value); b {
			function.Build()
		}
	}
	for _, m := range c.NormalMethod {
		m.Build()
	}
	for _, function := range c.StaticMethod {
		function.Build()
	}
}
