package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"go.uber.org/atomic"
)

// lazyTask 将任务逻辑和任务数据分离开
type lazyTask func()

// LazyBuilder 是一个并发安全、内存安全的延迟执行器
type LazyBuilder struct {
	name  string
	tasks []lazyTask
	mu    sync.RWMutex
	build atomic.Bool
}

// NewLazyBuilder 创建一个新的 LazyBuilder 实例
func NewLazyBuilder(name string) *LazyBuilder {
	lz := &LazyBuilder{
		name:  name,
		tasks: make([]lazyTask, 0),
	}
	return lz
}

// AddLazyBuilder 添加延迟执行的任务
func (l *LazyBuilder) AddLazyBuilder(work func(), _ ...bool) {
	if l == nil {
		log.Errorf("LazyBuilder is nil")
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tasks = append(l.tasks, lazyTask(work))
}

// HasBeenBuilt 返回是否已完成构建（用于跳过重复追踪）
func (l *LazyBuilder) HasBeenBuilt() bool {
	if l == nil {
		return true
	}
	return l.build.Load()
}

// Build 执行所有已添加的任务，该方法在整个生命周期中只会有效执行一次。
// 返回值 hadTasks：是否曾通过 AddLazyBuilder 添加过任务（用于诊断从未添加任务的 case）
func (l *LazyBuilder) Build() (hadTasks bool) {
	if l == nil {
		return false
	}

	if l.build.Load() {
		return false // 已经构建过，直接返回，不计入 hadTasks
	}

	l.build.Store(true)

	l.mu.Lock()
	tasksToRun := l.tasks
	l.tasks = nil // 【关键】立即清空，释放对闭包和上下文的引用
	l.mu.Unlock()

	hadTasks = len(tasksToRun) > 0

	defer func() {
		if r := recover(); r != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	runTasks := func() error {
		for _, task := range tasksToRun {
			if task != nil {
				task()
			}
		}
		return nil
	}
	if hadTasks {
		_ = TrackBuildWithOptions(diagnostics.GetCurrentRecorder(), l.name, runTasks,
			WithTrackKind(TrackKindBuild),
			WithTrackDepthEnabled(true))
	}
	return hadTasks
}

type ASTIF interface {
	GetText() string
}

// VisitAst 遍历 AST，preHandler 时标记 hash，否则删除；全部完成后触发 LazyBuild
func (p *Program) VisitAst(ast ASTIF) {
	hash := utils.CalcSha256(ast.GetText())

	if p.PreHandler() {
		p.astMap[hash] = struct{}{}
	} else {
		if _, ok := p.astMap[hash]; !ok {
			log.Warnf("ast[%v] is not found in ast map", p.GetProgramName())
			return
		}
		delete(p.astMap, hash)

		if len(p.astMap) == 0 {
			p.Application.ProcessInfof("program %s all ast visit done", p.Name)
			p.Application.ProcessInfof("program %s build Instruction", p.Name)
			p.LazyBuild()
			p.Application.ProcessInfof("program %s build Instruction(%d)", p.Name, p.Cache.CountInstruction())
			builder := p.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			builder.SyntaxIncludingStack = nil
		}
	}
}

func buildFuncID(p *Program, fun *Function) string {
	id := fun.GetName()
	if id == "" {
		id = "anonymous"
	}
	if pkg := p.GetProgramName(); pkg != "" {
		id = pkg + "/" + id
	}
	return id
}

func (p *Program) LazyBuild() {
	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		blueprint.Build()
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
		fun.Build()
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
	if function == nil && virtualFunction == nil {
		log.Errorf("main function is not found and virtual function is not found")
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
