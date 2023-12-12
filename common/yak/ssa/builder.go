package ssa

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
	CurrentBlock       *BasicBlock // current block to build
	CurrentPos         *Position   // current position in source code
	CurrentScope       *Scope
	scopeId            int
	parentScope        *Scope      // parent symbol block for build FreeValue
	parentCurrentBlock *BasicBlock // parent build subFunction position

	ExternInstance map[string]any
	ExternLib      map[string]map[string]any

	parentBuilder *FunctionBuilder
	cmap          map[string]struct{}
	lmap          map[string]struct{}
}

func NewBuilder(f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		Function:      f,
		target:        &target{},
		labels:        make(map[string]*BasicBlock),
		subFuncBuild:  make([]func(), 0),
		deferExpr:     make([]*Call, 0),
		CurrentBlock:  nil,
		CurrentPos:    nil,
		CurrentScope:  NewScope(0, nil, f),
		scopeId:       0,
		parentBuilder: parent,
		cmap:          make(map[string]struct{}),
		lmap:          make(map[string]struct{}),
	}
	if parent != nil {
		b.ExternInstance = parent.ExternInstance
		b.ExternLib = parent.ExternLib
	}

	b.ScopeStart()
	b.Function.SetScope(b.CurrentScope)
	b.CurrentBlock = f.EnterBlock
	f.builder = b
	return b
}

// current block is finish?
func (b *FunctionBuilder) IsBlockFinish() bool {
	return b.CurrentBlock.finish
}

// new function
func (b *FunctionBuilder) NewFunc(name string) (*Function, *Scope) {
	f := b.Package.NewFunctionWithParent(name, b.Function)
	f.SetPosition(b.CurrentPos)
	return f, b.CurrentScope
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
	b.ScopeEnd()
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
	// re-calculate return type
	for _, ret := range b.Return {
		recoverRange := b.SetCurrent(ret)
		ret.calcType(b)
		recoverRange()
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
func (b *FunctionBuilder) PushFunction(newFunc *Function, scope *Scope, block *BasicBlock) *FunctionBuilder {
	build := NewBuilder(newFunc, b)
	build.parentScope = scope
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
