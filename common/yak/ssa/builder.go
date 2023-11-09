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
	prog.buildOnce.Do(b.Build)
}

// Function builder API
type FunctionBuilder struct {
	*Function

	// build sub-function
	subFuncBuild []func()

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call
	deferExpr []*Call // defer function, reverse  for-range
	// unsealed block
	unsealedBlock []*BasicBlock

	// for build
	CurrentBlock       *BasicBlock       // current block to build
	CurrentPos         *Position         // current position in source code
	blockSymbolTable   *blockSymbolTable //  blockId -> variable -> variableId
	blockId            int
	parentSymbolBlock  *blockSymbolTable // parent symbol block for build FreeValue
	parentCurrentBlock *BasicBlock       // parent build subFunction position

	ExternInstance map[string]any
	ExternLib      map[string]map[string]any

	parentBuilder *FunctionBuilder
	cmap          map[string]struct{}
	lmap          map[string]struct{}
}

func NewBuilder(f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		Function:     f,
		target:       &target{},
		labels:       make(map[string]*BasicBlock),
		subFuncBuild: make([]func(), 0),
		deferExpr:    make([]*Call, 0),
		CurrentBlock: nil,
		CurrentPos:   nil,
		blockSymbolTable: &blockSymbolTable{
			symbol:  nil,
			blockId: "main",
		},
		blockId:       0,
		parentBuilder: parent,
		cmap:          make(map[string]struct{}),
		lmap:          make(map[string]struct{}),
	}
	if parent != nil {
		b.ExternInstance = parent.ExternInstance
		b.ExternLib = parent.ExternLib
	}

	b.PushBlockSymbolTable()
	b.CurrentBlock = f.EnterBlock
	f.builder = b
	return b
}

// new function
func (b *FunctionBuilder) NewFunc(name string) (*Function, *blockSymbolTable) {
	f := b.Package.NewFunctionWithParent(name, b.Function)
	f.SetPosition(b.CurrentPos)
	return f, b.blockSymbolTable
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	b.Param[len(b.Param)-1].SetType(NewSliceType(BasicTypes[Any]))
	b.hasEllipsis = true
}

// get parent function
func (b FunctionBuilder) GetParentBuilder() *FunctionBuilder {
	return b.parentBuilder
}

// add current function defer function
func (b *FunctionBuilder) AddDefer(call *Call) {
	b.deferExpr = append(b.deferExpr, call)
}

func (b *FunctionBuilder) AddUnsealedBlock(block *BasicBlock) {
	b.unsealedBlock = append(b.unsealedBlock, block)
}

// finish current function builder
func (b *FunctionBuilder) Finish() {
	// fmt.Println("finish func: ", b.Name)

	// sub-function
	for _, builder := range b.subFuncBuild {
		builder()
	}
	for _, block := range b.unsealedBlock {
		block.Sealed()
	}
	// set defer function
	if len(b.deferExpr) > 0 {
		b.CurrentBlock = b.NewBasicBlock("defer")
		for _, call := range b.deferExpr {
			b.EmitOnly(call)
		}
	}
	// function finish
	b.Function.Finish()
}

// handler position: set new position and return original position for backup
func (b *FunctionBuilder) SetPosition(pos *Position) *Position {
	backup := b.CurrentPos
	// if b.CurrentBlock.GetPosition() == nil {
	// 	b.CurrentBlock.SetPosition(pos)
	// }
	b.CurrentPos = pos
	return backup
}

// sub-function builder
func (b *FunctionBuilder) AddSubFunction(builder func()) {
	b.subFuncBuild = append(b.subFuncBuild, builder)
}

// function stack
func (b *FunctionBuilder) PushFunction(newFunc *Function, symbol *blockSymbolTable, block *BasicBlock) *FunctionBuilder {
	build := NewBuilder(newFunc, b)
	build.parentSymbolBlock = symbol
	build.parentCurrentBlock = block
	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	return b.parentBuilder
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
		// b.NewError(Error, SSATAG, "error target struct this position when build")
		return false
	} else {
		return true
	}
}

// for goto and label
func (b *FunctionBuilder) AddLabel(name string, block *BasicBlock) {
	b.labels[name] = block
}

func (b *FunctionBuilder) GetLabel(name string) *BasicBlock {
	if b, ok := b.labels[name]; ok {
		return b
	} else {
		return nil
	}
}

func (b *FunctionBuilder) DeleteLabel(name string) {
	delete(b.labels, name)
}

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
	symbol  map[string]string // variable -> variableId(variable-blockId)
	blockId string
	next    *blockSymbolTable
}

func (b *FunctionBuilder) NewBlockId() string {
	ret := fmt.Sprintf("block%d", b.blockId)
	b.blockId += 1
	return ret
}

// block symbol-table stack
func (b *FunctionBuilder) PushBlockSymbolTable() {
	b.blockSymbolTable = &blockSymbolTable{
		symbol:  make(map[string]string),
		blockId: b.NewBlockId(),
		next:    b.blockSymbolTable,
	}
}

func (b *FunctionBuilder) PopBlockSymbolTable() {
	b.blockSymbolTable = b.blockSymbolTable.next
}

// use block symbol table map variable -> variable+blockId
func (b *FunctionBuilder) MapBlockSymbolTable(text string) string {
	newText := text + b.blockSymbolTable.blockId
	b.blockSymbolTable.symbol[text] = newText
	return newText
}

func (b *FunctionBuilder) GetIdByBlockSymbolTable(id string) string {
	return GetIdByBlockSymbolTable(id, b.blockSymbolTable)
}

func GetIdByBlockSymbolTable(id string, symbolEnter *blockSymbolTable) string {
	for symbol := symbolEnter; symbol != nil && symbol.blockId != "main"; symbol = symbol.next {
		if v, ok := symbol.symbol[id]; ok {
			return v
		}
	}
	return id
}

func (b *FunctionBuilder) AddToCmap(key string) {
	b.cmap[key] = struct{}{}
}

func (b *FunctionBuilder) GetFromCmap(key string) bool {
	if _, ok := b.cmap[key]; ok {
		return true
	} else {
		return false
	}
}

func (b *FunctionBuilder) AddToLmap(key string) {
	b.lmap[key] = struct{}{}
}

func (b *FunctionBuilder) GetFromLmap(key string) bool {
	if _, ok := b.lmap[key]; ok {
		return true
	} else {
		return false
	}
}
