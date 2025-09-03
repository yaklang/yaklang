package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// lazyTask 将任务逻辑和任务数据分离开
type lazyTask func()

// type lazyTask struct {
// 	work func(context interface{})
// 	ctx  interface{}
// }

// LazyBuilder 是一个并发安全、内存安全的延迟执行器
type LazyBuilder struct {
	tasks []lazyTask
	mu    sync.Mutex
	build bool
}

// NewLazyBuilder 创建一个新的 LazyBuilder 实例
func NewLazyBuilder() *LazyBuilder {
	return &LazyBuilder{}
}

// Add 添加一个延迟执行的任务。
// work 是要执行的函数，ctx 是要传递给该函数的上下文数据。
func (l *LazyBuilder) AddLazyBuilder(work func(), async ...bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tasks = append(l.tasks, lazyTask(work))
}

// Build 执行所有已添加的任务，该方法在整个生命周期中只会有效执行一次。
func (l *LazyBuilder) Build() {
	if l.build {
		return // 已经构建过，直接返回
	}
	// 在 once.Do 内部，我们是线程安全的。
	// 先将任务列表转移到局部变量，然后立即清空原始列表，
	// 这样可以尽快释放引用，并防止在 Build 执行期间有新的 Add 调用。
	l.build = true

	l.mu.Lock()
	tasksToRun := l.tasks
	l.tasks = nil // 【关键】立即清空，释放对闭包和上下文的引用
	l.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// 依次执行所有任务
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
			log.Errorf("ast[%v] is not found in ast map", p.GetProgramName())
			return
		}
		delete(p.astMap, hash)

		if len(p.astMap) == 0 {
			p.Application.ProcessInfof("program %s all ast visit done", p.Name)
			p.Application.ProcessInfof("program %s build Instruction", p.Name)
			p.LazyBuild() // build instruction
			p.Application.ProcessInfof("program %s save Instruction(%d) to database", p.Name, p.Cache.CountInstruction())
			// will cause instruction not save bug
			// p.Cache.SaveToDatabase() // save instruction
			builder := p.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			builder.SyntaxIncludingStack = nil
		}
	}
}

func (p *Program) LazyBuild() {
	for _, key := range p.Blueprint.Keys() {
		blueprint, ok := p.Blueprint.Get(key)
		_ = ok
		blueprint.Build()
	}
	for _, key := range p.Funcs.Keys() {
		fun, ok := p.Funcs.Get(key)
		_ = ok
		fun.Build()
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
