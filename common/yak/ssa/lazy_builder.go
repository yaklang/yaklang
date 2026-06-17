package ssa

import (
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"go.uber.org/atomic"
)

// lazyTask 将任务逻辑和任务数据分离开
type lazyTask struct {
	work    func()
	unitKey string
}

// LazyBuilder 是一个并发安全、内存安全的延迟执行器
type LazyBuilder struct {
	_lazybuild_name  string
	tasks            []lazyTask
	mu               sync.RWMutex
	build            atomic.Bool
	unitProvider     func() string
	unitTaskObserver func(string, *LazyBuilder)
	unitTaskRunner   func(string, func())
}

// NewLazyBuilder 创建一个新的 LazyBuilder 实例
func NewLazyBuilder(name string) *LazyBuilder {
	lz := &LazyBuilder{
		_lazybuild_name: name + "||" + uuid.NewString(),
		tasks:           make([]lazyTask, 0),
	}
	return lz
}

func (l *LazyBuilder) SetUnitProvider(provider func() string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.unitProvider = provider
}

func (l *LazyBuilder) SetUnitTaskObserver(observer func(string, *LazyBuilder)) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.unitTaskObserver = observer
}

func (l *LazyBuilder) SetUnitTaskRunner(runner func(string, func())) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.unitTaskRunner = runner
}

func (l *LazyBuilder) currentUnitKey() string {
	if l == nil || l.unitProvider == nil {
		return ""
	}
	return l.unitProvider()
}

// Add 添加一个延迟执行的任务。
// work 是要执行的函数，ctx 是要传递给该函数的上下文数据。
func (l *LazyBuilder) AddLazyBuilder(work func(), async ...bool) {
	if l == nil {
		log.Errorf("LazyBuilder is nil")
		return
	}
	unitKey := l.currentUnitKey()
	l.mu.Lock()
	l.tasks = append(l.tasks, lazyTask{work: work, unitKey: unitKey})
	observer := l.unitTaskObserver
	l.mu.Unlock()
	if observer != nil {
		observer(unitKey, l)
	}
}

// Build 执行所有已添加的任务，该方法在整个生命周期中只会有效执行一次。
func (l *LazyBuilder) Build() {
	if l == nil {
		// log.Errorf("LazyBuilder is nil")
		return
	}

	if l.build.Load() {
		// log.Errorf("LazyBuilder is nil or already built")
		return // 已经构建过，直接返回
	}

	l.build.Store(true)

	l.mu.Lock()
	defer l.mu.Unlock()

	tasksToRun := l.tasks
	l.tasks = nil // 【关键】立即清空，释放对闭包和上下文的引用
	_ = tasksToRun

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("lazy builder panic: name=%s panic=%v", l._lazybuild_name, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// // 依次执行所有任务
	for _, task := range tasksToRun {
		if task.work != nil {
			task.work()
		}
	}
}

func (l *LazyBuilder) BuildForUnits(units map[string]struct{}) bool {
	if l == nil || len(units) == 0 {
		return false
	}
	if l.build.Load() {
		return false
	}

	l.mu.Lock()
	var tasksToRun []lazyTask
	remaining := make([]lazyTask, 0, len(l.tasks))
	for _, task := range l.tasks {
		if _, ok := units[task.unitKey]; ok {
			tasksToRun = append(tasksToRun, task)
			continue
		}
		remaining = append(remaining, task)
	}
	if len(tasksToRun) == 0 {
		l.mu.Unlock()
		return false
	}
	l.tasks = remaining
	runner := l.unitTaskRunner
	l.mu.Unlock()

	panicked := false
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			log.Errorf("lazy builder panic: name=%s panic=%v", l._lazybuild_name, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		// If we panicked, mark as built so caller won't retry forever
		if panicked {
			l.build.Store(true)
		}
	}()

	for _, task := range tasksToRun {
		if task.work != nil {
			if runner != nil {
				runner(task.unitKey, task.work)
			} else {
				task.work()
			}
		}
	}
	return true
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
		// 深度优先遍历函数与其子函数，确保所有 LazyBuilder 均被执行
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

func (p *Program) LazyBuildForUnits(unitKeys []string) {
	if p == nil || len(unitKeys) == 0 {
		return
	}
	units := make(map[string]struct{}, len(unitKeys))
	unitOrder := make([]string, 0, len(unitKeys))
	for _, unitKey := range unitKeys {
		if unitKey == "" {
			continue
		}
		if _, ok := units[unitKey]; ok {
			continue
		}
		units[unitKey] = struct{}{}
		unitOrder = append(unitOrder, unitKey)
	}
	if len(units) == 0 {
		return
	}
	p.lazyBuildForUnits(unitOrder, units, make(map[*Program]struct{}))
}

func (p *Program) lazyBuildForUnits(unitOrder []string, units map[string]struct{}, visitedPrograms map[*Program]struct{}) {
	if p == nil || len(units) == 0 {
		return
	}
	if _, ok := visitedPrograms[p]; ok {
		return
	}
	visitedPrograms[p] = struct{}{}

	if builders, indexed := p.lazyBuildersForUnitSet(unitOrder, units); indexed {
		for {
			if len(builders) == 0 {
				break
			}
			built := false
			for _, builder := range builders {
				if p.runLazyBuilderForUnits(builder, nil, units) {
					built = true
				}
			}
			if !built {
				break
			}
			builders, _ = p.lazyBuildersForUnitSet(unitOrder, units)
		}
	} else {
		for _, key := range p.Blueprint.Keys() {
			blueprint, ok := p.Blueprint.Get(key)
			if !ok || blueprint == nil {
				continue
			}
			p.runLazyBuilderForUnits(blueprint.LazyBuilder, blueprint.Range, units)
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
			fun := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if fun == nil {
				continue
			}
			if _, ok := visited[fun]; ok {
				continue
			}
			visited[fun] = struct{}{}
			p.runLazyBuilderForUnits(fun.LazyBuilder, fun.GetRange(), units)
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
	children := make([]*Program, 0, p.UpStream.Len())
	p.UpStream.ForEach(func(_ string, child *Program) bool {
		if child != nil {
			children = append(children, child)
		}
		return true
	})
	for _, child := range children {
		child.lazyBuildForUnits(unitOrder, units, visitedPrograms)
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
