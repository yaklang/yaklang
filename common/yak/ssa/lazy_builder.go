package ssa

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"go.uber.org/atomic"
)

// lazyTask 将任务逻辑和任务数据分离开
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

	for _, task := range tasksToRun {
		if task != nil {
			task()
		}
	}
	return hadTasks
}

type ASTIF interface {
	GetText() string
}

/*
LazyBuilder -- Build:
when build, each program should call visitAST
  - when preHandler: mark ast hash to prog.astMap
  - when not preHandler: delete ast hash from prog.astMap

when all ast visit done, build instruction and save to database

note: need defer func\visit stmt finish\...
*/
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
			p.LazyBuild() // build instruction
			p.Application.ProcessInfof("program %s build Instruction(%d)", p.Name, p.Cache.CountInstruction())
			// will cause instruction not save bug
			// p.Cache.SaveToDatabase() // save instruction
			builder := p.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			builder.SyntaxIncludingStack = nil
		}
	}
}

// makeLazyBuildID 生成程序级 LazyBuild 的 ID：package:file:funcName
// @main 是程序入口，不绑定到具体文件，用 "-" 表示程序作用域
func makeLazyBuildID(p *Program) string {
	return makeBuildIDWithFile(p, "-", string(MainFunctionName))
}

// makeFunctionBuildID 生成 Function Build 的 ID：package:file:funcName
// 优先从 fun.GetRange() 获取定义文件，避免 LazyBuild 时 GetCurrentEditor 指向最后一个文件
func makeFunctionBuildID(p *Program, fun *Function) string {
	funcName := fun.GetName()
	if funcName == "" {
		funcName = string(MainFunctionName)
	}
	file := ""
	if r := fun.GetRange(); r != nil {
		if e := r.GetEditor(); e != nil {
			file = filepath.Base(e.GetFilename())
		}
	}
	return makeBuildIDWithFile(p, file, funcName)
}

func makeBuildID(p *Program, funcName string) string {
	return makeBuildIDWithFile(p, "", funcName)
}

func makeBuildIDWithFile(p *Program, file string, funcName string) string {
	pkg := p.PkgName
	if pkg == "" {
		pkg = p.Name
	}
	if file == "" {
		if e := p.GetCurrentEditor(); e != nil {
			file = filepath.Base(e.GetFilename())
		}
	}
	if file == "" {
		file = p.Name
	}
	return fmt.Sprintf("%s:%s:%s", pkg, file, funcName)
}

func getBuildTreeTracker(p *Program) diagnostics.BuildTreeTracker {
	if p == nil {
		return nil
	}
	app := p.Application
	if app == nil {
		app = p
	}
	return app.BuildTreeTracker
}

// buildFunctionWithPerfTracking 构建函数并在 BuildTreeTracker 中记录（若启用）。
// 用于 LazyBuild 主循环及按需构建（如 call 时构建 callee），确保所有函数都出现在性能树中。
func buildFunctionWithPerfTracking(fun *Function) (hadTasks bool) {
	if fun == nil {
		return false
	}
	p := fun.GetProgram()
	tracker := getBuildTreeTracker(p)
	start := time.Now()
	if tracker != nil {
		tracker.PushLazyBuild(makeFunctionBuildID(p, fun))
		defer func() {
			tracker.PopLazyBuild(time.Since(start), hadTasks)
		}()
	}
	hadTasks = fun.Build()
	return hadTasks
}

func (p *Program) LazyBuild() {
	start := time.Now()
	tracker := getBuildTreeTracker(p)
	if tracker != nil {
		id := makeLazyBuildID(p)
		tracker.PushLazyBuild(id)
		defer func() {
			tracker.PopLazyBuildProgramLevel(time.Since(start), true)
		}()
	}

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
		fun := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if fun == nil {
			continue
		}
		if _, ok := visited[fun]; ok {
			continue
		}
		visited[fun] = struct{}{}
		// 已构建过：正常情况，不重复显示（不加入性能树）
		if tracker != nil && fun.LazyBuilder != nil && fun.LazyBuilder.HasBeenBuilt() {
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
			continue
		}
		_ = buildFunctionWithPerfTracking(fun)
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
