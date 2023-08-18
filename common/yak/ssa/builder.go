package ssa

import "fmt"

type builder struct {
	*Function

	target *target // for break and continue
	// defer function call
	deferexpr []*Call // defer funciton, reverse  for-range

	// for build
	currentDef   map[string]map[*BasicBlock]Value // currentDef[variableId][block]value
	currentBlock *BasicBlock                      // current block to build
	currtenPos   *Position
	symbolBlock  *blockSymbolTable //  blockId -> variable -> variableId

	prev *builder
}

func NewBuilder(f *Function, next *builder) *builder {
	b := &builder{
		Function:     f,
		target:       &target{},
		deferexpr:    make([]*Call, 0),
		currentDef:   make(map[string]map[*BasicBlock]Value),
		currentBlock: nil,
		currtenPos:   nil,
		symbolBlock:  NewBlockSymbolTable("func-scope", nil),
		prev:         next,
	}
	b.currentBlock = f.EnterBlock
	f.builder = b
	return b
}

// use in for/switch
type target struct {
	tail         *target // the stack
	_break       *BasicBlock
	_continue    *BasicBlock
	_fallthrough *BasicBlock
}

type blockSymbolTable struct {
	symbol  map[string]string // variable -> variableId(variable-blockid)
	blockid string
	next    *blockSymbolTable
}

func NewBlockSymbolTable(id string, next *blockSymbolTable) *blockSymbolTable {
	return &blockSymbolTable{
		symbol:  make(map[string]string),
		blockid: id,
		next:    next,
	}
}

var (
	blockId int = 0
)

func NewBlockId() string {
	ret := fmt.Sprintf("block%d", blockId)
	blockId += 1
	return ret
}

func (b *builder) finish() {
	// set defer function
	b.currentBlock = b.Blocks[len(b.Blocks)-1]
	for i := len(b.deferexpr) - 1; i >= 0; i-- {
		b.emitCall(b.deferexpr[i])
	}
	b.Finish()
}

func (pkg *Package) build() {
	main := pkg.NewFunction("yak-main")
	b := NewBuilder(main, nil)
	b.build(pkg.ast)
	b.finish()
}

func (pkg *Package) Build() { pkg.buildOnece.Do(pkg.build) }

func (prog *Program) Build() {
	for _, pkg := range prog.Packages {
		pkg.Build()
	}
}
