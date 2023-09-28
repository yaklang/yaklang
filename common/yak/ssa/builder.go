package ssa

import (
	"fmt"
)

type Builder interface {
	Build()
}

// build enter pointer
// front implement `Builder`
func (prog *Program) Build(b Builder) {
	prog.buildOnece.Do(b.Build)
}

// Function builder API
type FunctionBuilder struct {
	*Function

	// build sub-function
	subFuncBuild []func()

	target *target // for break and continue
	// defer function call
	deferexpr []*Call // defer funciton, reverse  for-range

	// for build
	currentDef   map[string]map[*BasicBlock]Value // currentDef[variableId][block]value
	CurrentBlock *BasicBlock                      // current block to build
	currtenPos   *Position                        // current position in source code
	symbolBlock  *blockSymbolTable                //  blockId -> variable -> variableId

	undefineHijack func(string) Value

	prev *FunctionBuilder
}

func NewBuilder(f *Function, next *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		Function:     f,
		target:       &target{},
		subFuncBuild: make([]func(), 0),
		deferexpr:    make([]*Call, 0),
		currentDef:   make(map[string]map[*BasicBlock]Value),
		CurrentBlock: nil,
		currtenPos:   nil,
		symbolBlock:  nil,
		prev:         next,
	}
	if next != nil {
		b.undefineHijack = next.undefineHijack
	}

	b.PushBlockSymbolTable()
	b.CurrentBlock = f.EnterBlock
	f.builder = b
	return b
}

// new function
func (b *FunctionBuilder) NewFunc() *Function {
	return b.Package.NewFunctionWithParent("", b.Function)
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	b.Param[len(b.Param)-1].typs = NewObjectType()
	b.hasEllipsis = true
}

// get parent function
func (b FunctionBuilder) GetParentBuilder() *FunctionBuilder {
	return b.parent.builder
}

// add current function defer function
func (b *FunctionBuilder) AddDefer(call *Call) {
	b.deferexpr = append(b.deferexpr, call)
}

// finish current function builder
func (b *FunctionBuilder) Finish() {
	// fmt.Println("finsh func: ", b.Name)

	// sub-function
	for _, builder := range b.subFuncBuild {
		builder()
	}
	// set defer function
	b.CurrentBlock = b.Blocks[len(b.Blocks)-1]
	for i := len(b.deferexpr) - 1; i >= 0; i-- {
		b.EmitCall(b.deferexpr[i])
	}
	// function finish
	b.Function.Finish()
}

// handler position: set new position and return original position for backup
func (b *FunctionBuilder) SetPosition(pos *Position) *Position {
	backup := b.currtenPos
	b.currtenPos = pos
	return backup
}

// sub-function builder
func (b *FunctionBuilder) AddSubFunction(builder func()) {
	b.subFuncBuild = append(b.subFuncBuild, builder)
}

// function stack
func (b *FunctionBuilder) PushFunction(newfunc *Function) *FunctionBuilder {
	build := NewBuilder(newfunc, b)
	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	return b.prev
}

// use in for/switch
type target struct {
	tail         *target // the stack
	_break       *BasicBlock
	_continue    *BasicBlock
	_fallthrough *BasicBlock
}

// target stack
func (b *FunctionBuilder) PushTarget(_break, _continue, _fallthrough *BasicBlock) {
	b.target = &target{
		tail:         b.target,
		_break:       _break,
		_continue:    _continue,
		_fallthrough: _fallthrough,
	}
}

func (b *FunctionBuilder) PopTarget() bool {
	b.target = b.target.tail
	if b.target == nil {
		b.NewError(Error, SSATAG, "error target struct this position when build")
		return false
	} else {
		return true
	}
}

// get target feild
func (b *FunctionBuilder) GetBreak() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._break != nil {
			return target._break
		}
	}
	return nil
}

func (b *FunctionBuilder) GetContinue() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._continue != nil {
			return target._continue
		}
	}
	return nil
}
func (b *FunctionBuilder) GetFallthrough() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._fallthrough != nil {
			return target._fallthrough
		}
	}
	return nil
}

type blockSymbolTable struct {
	symbol  map[string]string // variable -> variableId(variable-blockid)
	blockid string
	next    *blockSymbolTable
}

var (
	blockId int = 0
)

func NewBlockId() string {
	ret := fmt.Sprintf("block%d", blockId)
	blockId += 1
	return ret
}

// block symboltable stack
func (b *FunctionBuilder) PushBlockSymbolTable() {
	b.symbolBlock = &blockSymbolTable{
		symbol:  make(map[string]string),
		blockid: NewBlockId(),
		next:    b.symbolBlock,
	}
}

func (b *FunctionBuilder) PopBlockSymbolTable() {
	b.symbolBlock = b.symbolBlock.next
}

// use block symbol table map variable -> variable+blockid
func (b *FunctionBuilder) MapBlockSymbolTable(text string) string {
	newtext := text + b.symbolBlock.blockid
	b.symbolBlock.symbol[text] = newtext
	return newtext
}

func (b *FunctionBuilder) GetIdByBlockSymbolTable(id string) string {
	for block := b.symbolBlock; block != nil; block = block.next {
		if v, ok := block.symbol[id]; ok {
			return v
		}
	}

	return id
}
