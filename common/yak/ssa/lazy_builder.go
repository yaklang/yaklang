package ssa

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"go.uber.org/atomic"
)

const lazyBuildOtherFile = "__Other__" // 无法按文件归类的 LazyBuild 耗时（如 fixImportCallback）

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
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// // 依次执行所有任务
	for _, task := range tasksToRun {
		if task != nil {
			task()
		}
	}
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

// getFuncFilename 从 Function 的 Range/Editor 获取文件名，用于 LazyBuild 按文件计时
func getFuncFilename(fun *Function) string {
	if fun == nil {
		return ""
	}
	rng := fun.GetRange()
	if rng == nil || rng.GetEditor() == nil {
		return ""
	}
	return filepath.Base(rng.GetEditor().GetFilename())
}

// getBlueprintFilename 从 Blueprint 的方法中获取代表文件名
func getBlueprintFilename(c *Blueprint) string {
	if c == nil {
		return ""
	}
	for _, m := range c.NormalMethod {
		if f := getFuncFilename(m); f != "" {
			return f
		}
	}
	for _, m := range c.StaticMethod {
		if f := getFuncFilename(m); f != "" {
			return f
		}
	}
	for _, v := range c.MagicMethod {
		if m, ok := ToFunction(v); ok {
			if f := getFuncFilename(m); f != "" {
				return f
			}
		}
	}
	return ""
}

func (p *Program) LazyBuild() {
	trackByFile := p.OnLazyBuildCompleteByFile != nil || (p.Application != nil && p.Application.OnLazyBuildCompleteByFile != nil)
	byFile := make(map[string]time.Duration)

	addDuration := func(file string, d time.Duration) {
		if file == "" {
			file = lazyBuildOtherFile
		}
		byFile[file] += d
	}

	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		start := time.Now()
		blueprint.Build()
		if trackByFile {
			addDuration(getBlueprintFilename(blueprint), time.Since(start))
		}
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
		start := time.Now()
		fun.Build()
		if trackByFile {
			addDuration(getFuncFilename(fun), time.Since(start))
		}
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
	start := time.Now()
	for _, f := range p.fixImportCallback {
		f()
	}
	if trackByFile {
		addDuration(lazyBuildOtherFile, time.Since(start))
	}
	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		start := time.Now()
		blueprint.BuildConstructorAndDestructor()
		if trackByFile {
			addDuration(getBlueprintFilename(blueprint), time.Since(start))
		}
	}
	function := p.GetFunction(string(MainFunctionName), "")
	if function != nil {
		start := time.Now()
		function.Finish()
		if trackByFile {
			addDuration(getFuncFilename(function), time.Since(start))
		}
	}
	virtualFunction := p.GetFunction(string(VirtualFunctionName), "")
	if virtualFunction != nil {
		start := time.Now()
		virtualFunction.Finish()
		if trackByFile {
			addDuration(getFuncFilename(virtualFunction), time.Since(start))
		}
	}
	if function == nil && virtualFunction == nil {
		log.Errorf("main function is not found and virtual function is not found")
		return
	}

	cbByFile := p.OnLazyBuildCompleteByFile
	if cbByFile == nil && p.Application != nil {
		cbByFile = p.Application.OnLazyBuildCompleteByFile
	}
	if cbByFile != nil && len(byFile) > 0 {
		cbByFile(byFile)
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
