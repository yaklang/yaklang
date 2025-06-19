package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

type lazyBuilder struct {
	_build  []func()
	isBuild bool
}

/*
	LazyBuilder -- Add:

just call AddLazyBuilder function, this function will be create in PreHandlerTime and build in BuildTime
*/
func (l *lazyBuilder) AddLazyBuilder(Builder func(), async ...bool) {
	l._build = append(l._build, Builder)
}

func (n *lazyBuilder) Build() {
	if len(n._build) == 0 || n.isBuild {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic in LazyBuild: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	n.isBuild = true
	for _, f := range n._build {
		f()
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
			log.Errorf("ast[%s] is not found in ast map", ast.GetText())
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
