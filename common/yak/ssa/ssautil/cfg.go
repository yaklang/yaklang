package ssautil

// BuildSyntaxBlock builds a syntax block using the provided scope and buildBody function.
func BuildSyntaxBlock[T comparable](
	scope *ScopedVersionedTable[T],
	buildBody func(*ScopedVersionedTable[T]),
) {
	/*
		scope [a=1; b=1]
			sub [a=2; b:=2]
		- cover
	*/
	sub := scope.CreateSubScope()
	buildBody(sub)
	scope.CoverByChild()
}

// IfStmtItem represents an item in an IfStmt.
type IfStmtItem[T comparable] struct {
	Condition func(*ScopedVersionedTable[T])
	Body      func(*ScopedVersionedTable[T])
}

// NewIfStmtItem creates a new IfStmtItem with the given condition and body functions.
func NewIfStmtItem[T comparable](c func(*ScopedVersionedTable[T]), b func(*ScopedVersionedTable[T])) *IfStmtItem[T] {
	return &IfStmtItem[T]{
		Condition: c,
		Body:      b,
	}
}

// IfStmt represents an if statement.
type IfStmt[T comparable] struct {
	global         *ScopedVersionedTable[T]
	conditionScope *ScopedVersionedTable[T]
	hasElse        bool
}

// NewIfStmt creates a new IfStmt with the given global scope.
func NewIfStmt[T comparable](global *ScopedVersionedTable[T]) *IfStmt[T] {
	condition := global.CreateSubScope()
	return &IfStmt[T]{
		global:         global,
		conditionScope: condition,
		hasElse:        false,
	}
}

// AddItem adds an item to the IfStmt.
func (i *IfStmt[T]) BuildItem(Condition func(*ScopedVersionedTable[T]), Body func(*ScopedVersionedTable[T])) {
	if i.hasElse {
		panic("cannot add item after else")
	}
	Condition(i.conditionScope)
	sub := i.conditionScope.CreateSubScope()
	Body(sub)
}

// SetElse sets the else function for the IfStmt.
func (i *IfStmt[T]) BuildElse(elseBody func(*ScopedVersionedTable[T])) {
	sub := i.conditionScope.CreateSubScope()
	elseBody(sub)
	i.hasElse = true
}

// Build builds the IfStmt using the provided mergeHandler function.
func (i *IfStmt[T]) BuildFinish(
	mergeHandler func(name string, t []T) T,
) {
	/*
		scope
			condition
				item-0
				item-1
				...
			- merge
		- cover
	*/

	i.conditionScope.Merge(!i.hasElse, mergeHandler)
	i.global.CoverByChild()
}

// LoopStmt represents a loop statement.
type LoopStmt[T comparable] struct {
	global    *ScopedVersionedTable[T]
	First     func(*ScopedVersionedTable[T])
	Condition func(*ScopedVersionedTable[T])
	Third     func(*ScopedVersionedTable[T])
	Body      func(*ScopedVersionedTable[T])
	NewPhi    func() T
}

// NoneBuilder is a helper function that does nothing.
func NoneBuilder[T comparable](*ScopedVersionedTable[T]) {}

// NewLoopStmt creates a new LoopStmt with the given global scope.
func NewLoopStmt[T comparable](global *ScopedVersionedTable[T], NewPhi func() T) *LoopStmt[T] {
	return &LoopStmt[T]{
		global:    global,
		First:     NoneBuilder[T],
		Condition: NoneBuilder[T],
		Third:     NoneBuilder[T],
		Body:      NoneBuilder[T],
		NewPhi:    NewPhi,
	}
}

// SetFirst sets the first function for the LoopStmt.
func (l *LoopStmt[T]) SetFirst(f func(*ScopedVersionedTable[T])) *LoopStmt[T] {
	l.First = f
	return l
}

// SetCondition sets the condition function for the LoopStmt.
func (l *LoopStmt[T]) SetCondition(f func(*ScopedVersionedTable[T])) *LoopStmt[T] {
	l.Condition = f
	return l
}

// SetThird sets the third function for the LoopStmt.
func (l *LoopStmt[T]) SetThird(f func(*ScopedVersionedTable[T])) *LoopStmt[T] {
	l.Third = f
	return l
}

// SetBody sets the body function for the LoopStmt.
func (l *LoopStmt[T]) SetBody(f func(*ScopedVersionedTable[T])) *LoopStmt[T] {
	l.Body = f
	return l
}

// Build builds the LoopStmt using the provided NewPhi and SpinHandler functions.
func (l *LoopStmt[T]) Build(SpinHandler func(name string, phi T, origin T, last T) T) {

	/*
		global [i = 0]
			header [i] // first
				condition // condition
					body [i] // body + third
				- cover
				- spin
			- cover
		- cover
	*/

	header := l.global.CreateSubScope()

	l.First(header)

	condition := header.CreateSubScope()
	condition.SetSpin(l.NewPhi)
	l.Condition(condition)

	body := condition.CreateSubScope()
	l.Body(body)
	l.Third(body)
	condition.CoverByChild()

	// finish phi
	condition.Spin(SpinHandler)

	header.CoverByChild()
	// finish
	l.global.CoverByChild()
}
