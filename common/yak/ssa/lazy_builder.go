package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type LazyBuildType int

const (
	_FunctionSign LazyBuildType = iota + 1
	_FunctionBody
	_Class
)

type buildItem struct {
	build   func()
	isBuild bool
	typ     LazyBuildType
}

type lazyBuilder struct {
	items []*buildItem
}

/*
	LazyBuilder -- Add:

just call AddLazyBuilder function, this function will be create in PreHandlerTime and build in BuildTime
*/
func (l *lazyBuilder) addLazyBuilderEx(Builder func(), typ LazyBuildType) {
	l.items = append(l.items, &buildItem{
		build:   Builder,
		isBuild: false,
		typ:     typ,
	})
}
func (l *lazyBuilder) AddFunctionSignBuilder(builder func()) {
	l.addLazyBuilderEx(builder, _FunctionSign)
}
func (l *lazyBuilder) AddFunctionBodyBuilder(builder func()) {
	l.addLazyBuilderEx(builder, _FunctionBody)
}
func (l *lazyBuilder) AddClassBuilder(builder func()) {
	l.addLazyBuilderEx(builder, _Class)
}

func (l *lazyBuilder) buildEx(buildItem func(item *buildItem)) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic in LazyBuild: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	for _, item := range l.items {
		if !item.isBuild {
			buildItem(item)
		}
	}
}
func (l *lazyBuilder) BuildFunctionSign() {
	l.buildEx(func(item *buildItem) {
		if item.typ == _FunctionSign {
			item.isBuild = true
			item.build()
		}
	})
}
func (n *lazyBuilder) Build() {
	n.buildEx(func(item *buildItem) {
		item.isBuild = true
		item.build()
	})
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
	buildFunction := func(build func(*Function)) {
		for _, functions := range p.Funcs.GetMap() {
			for _, function := range functions {
				build(function)
			}
		}
	}
	buildFunction(func(function *Function) {
		function.BuildFunctionSign()
	})
	for _, blueprint := range p.Blueprint.GetMap() {
		blueprint.Build()
		blueprint.BuildConstructorAndDestructor()
	}
	buildFunction(func(function *Function) {
		function.Build()
	})
	for _, f := range p.fixImportCallback {
		f()
	}
}

func (c *Blueprint) BuildConstructorAndDestructor() {
	for _, methods := range c.MagicMethod {
		methods.Build()
	}
	for _, m := range c.NormalMethod {
		m.Build()
	}
	for _, function := range c.StaticMethod {
		function.Build()
	}
}
