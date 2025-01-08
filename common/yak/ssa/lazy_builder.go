package ssa

import (
	"github.com/yaklang/yaklang/common/log"
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
		}
	}
}

func (p *Program) LazyBuild() {
	for _, blueprint := range p.Blueprint.GetMap() {
		blueprint.Build()
	}
	for _, fun := range p.Funcs.GetMap() {
		fun.Build()
	}
	for _, f := range p.fixImportCallback {
		f()
	}
	for _, blueprint := range p.Blueprint.GetMap() {
		blueprint.BuildConstructorAndDestructor()
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
