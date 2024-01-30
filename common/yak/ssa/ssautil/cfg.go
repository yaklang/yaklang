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

	endScope.Merge(
		!i.hasElse, // has base
		mergeHandler,
		i.BodyScopes...,
	)
	return endScope
}

// LoopStmt represents a loop statement.
type LoopStmt[T comparable] struct {
	MergeToEnd   []*ScopedVersionedTable[T] // break, merge phi in exit
	MergeToLatch []*ScopedVersionedTable[T] // continue, merge phi in latch

	ThirdBuilder func(*ScopedVersionedTable[T]) // third

	global    *ScopedVersionedTable[T]
	header    *ScopedVersionedTable[T]
	condition *ScopedVersionedTable[T]
	body      *ScopedVersionedTable[T]
}

// NoneBuilder is a helper function that does nothing.
// func NoneBuilder[T comparable](*ScopedVersionedTable[T])                                     {}
// func NoneBuilderReturnScope[T comparable](*ScopedVersionedTable[T]) *ScopedVersionedTable[T] {}

// NewLoopStmt creates a new LoopStmt with the given global scope.
func NewLoopStmt[T comparable](global *ScopedVersionedTable[T], NewPhi func(string) T) *LoopStmt[T] {
	l := &LoopStmt[T]{
		global: global,
	}
	l.header = l.global.CreateSubScope()
	l.condition = l.header.CreateSubScope()
	l.condition.SetSpin(NewPhi)
	l.body = l.condition.CreateSubScope()
	l.ThirdBuilder = nil
	return l
}

// SetFirst sets the first function for the LoopStmt.
func (l *LoopStmt[T]) SetFirst(f func(*ScopedVersionedTable[T])) {
	f(l.header)
}

// SetCondition sets the condition function for the LoopStmt.
func (l *LoopStmt[T]) SetCondition(f func(*ScopedVersionedTable[T])) {
	f(l.condition)
}

// SetThird sets the third function for the LoopStmt.
func (l *LoopStmt[T]) SetThird(f func(*ScopedVersionedTable[T])) {
	l.ThirdBuilder = f
}

// SetBody sets the body function for the LoopStmt.
func (l *LoopStmt[T]) SetBody(f func(*ScopedVersionedTable[T]) *ScopedVersionedTable[T]) {
	l.body = f(l.body)
}

func (l *LoopStmt[T]) Continue(from *ScopedVersionedTable[T]) {
	l.MergeToLatch = append(l.MergeToLatch, from)
}

func (l *LoopStmt[T]) Break(from *ScopedVersionedTable[T]) {
	l.MergeToEnd = append(l.MergeToEnd, from)
}

// Build builds the LoopStmt using the provided NewPhi and SpinHandler functions.
func (l *LoopStmt[T]) Build(SpinHandler func(name string, phi, v1, v2 T) T, merge func(name string, t []T) T) *ScopedVersionedTable[T] {

	/*
		global [i = 0]
			header [i] // first
				condition // condition [phi] from header and latch
					body [i] // body
						latch    // third [phi] from all continue and body
			exit // exit loop [phi]  from all break and global

		// in body
		* break to global scope
		* continue to latch scope
	*/

	// latch
	latch := l.body.CreateSubScope()
	latch.Merge(
		true,
		merge,
		l.MergeToLatch...,
	)
	// this `l.ThirdBuilder` only set in `l.SetThird`
	if l.ThirdBuilder != nil {
		// if not nil, mean, this `SetThird` is called before `SetBody`
		// call it
		l.ThirdBuilder(latch)
	}

	l.condition.Spin(l.header, latch, SpinHandler)

	// end
	end := l.global.CreateSubScope()
	l.header.CoverBy(l.condition)
	end.CoverBy(l.header)

	end.Merge(
		true,
		merge,
		l.MergeToEnd...,
	)

	return end
}
