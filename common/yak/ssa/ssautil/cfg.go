package ssautil

// BuildSyntaxBlock builds a syntax block using the provided scope and buildBody function.
/*
if this scope finish this program

* BuildBody should return true

* this function will return true
*/
func BuildSyntaxBlock[T comparable](
	global *ScopedVersionedTable[T],
	buildBody func(*ScopedVersionedTable[T]) *ScopedVersionedTable[T],
) *ScopedVersionedTable[T] {
	/*
		scope
			sub // build body
				--- body
			end // cover by body
	*/

	body := global.CreateSubScope()
	bodyEnd := buildBody(body)

	end := global.CreateSubScope()
	end.CoverBy(bodyEnd)
	return end
}

// IfStmt represents an if statement.
type IfStmt[T comparable] struct {
	global             *ScopedVersionedTable[T]
	lastConditionScope *ScopedVersionedTable[T]
	BodyScopes         []*ScopedVersionedTable[T]
	hasElse            bool
}

// NewIfStmt creates a new IfStmt with the given global scope.
/*
	IfStmt will handle if-stmt scope.
	API:
		* BuildItem(condition fun(scope), body func(scope)):
			build if item using the provided Condition and Body functions.
		* BuildElse(elseBody func(scope)):
			set the else function for the IfStmt.
		* BuildFinish(mergeHandler func(name string, t []T) T):
			build the IfStmt finish, using the provided mergeHandler function create Phi.
	IfStmt will build this scope when this method call
*/
func NewIfStmt[T comparable](global *ScopedVersionedTable[T]) *IfStmt[T] {
	// condition := global.CreateSubScope()
	return &IfStmt[T]{
		global:             global,
		lastConditionScope: global,
		BodyScopes:         make([]*ScopedVersionedTable[T], 0),
		hasElse:            false,
	}
}

// BuildItem build the if item using the provided Condition and Body functions.
func (i *IfStmt[T]) BuildItem(Condition func(*ScopedVersionedTable[T]), Body func(*ScopedVersionedTable[T]) *ScopedVersionedTable[T]) {
	if i.hasElse {
		panic("cannot add item after else")
	}

	// create new condition and body scope
	i.lastConditionScope = i.lastConditionScope.CreateSubScope()
	Condition(i.lastConditionScope)

	bodyScope := i.lastConditionScope.CreateSubScope()
	end := Body(bodyScope)
	if end != nil {
		i.BodyScopes = append(i.BodyScopes, end)
	}
}

// SetElse sets the else function for the IfStmt.
func (i *IfStmt[T]) BuildElse(elseBody func(*ScopedVersionedTable[T]) *ScopedVersionedTable[T]) {
	elseScope := i.lastConditionScope.CreateSubScope()
	end := elseBody(elseScope)
	if end != nil {
		i.BodyScopes = append(i.BodyScopes, end)
	}
	i.hasElse = true
}

// Build builds the IfStmt using the provided mergeHandler function.
func (i *IfStmt[T]) BuildFinish(
	mergeHandler func(name string, t []T) T,
) *ScopedVersionedTable[T] {
	/*
		global
			condition1 // condition
				body1 // body
				condition2 // condition
					body2 // body
					...
					else // else // same level with last body
		end // end scope
		// [phi] from all body and else
	*/

	endScope := i.global.CreateSubScope()

	Merge(
		i.global,   // base
		!i.hasElse, // has base
		func(name string, t []T) {
			ret := mergeHandler(name, t)
			endScope.CreateLexicalVariable(name, ret)
		},
		i.BodyScopes...,
	)

	return endScope
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
