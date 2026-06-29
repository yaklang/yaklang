package core

import (
	"os"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type StackItem struct {
	parent *StackItem
	value  values.JavaValue
}

func (s *StackItem) GetParent() *StackItem {
	return s.parent
}

func newStackItem(parent *StackItem, value values.JavaValue) *StackItem {
	return &StackItem{
		parent: parent,
		value:  value,
	}
}

type StackSimulationProxy struct {
	*StackSimulationImpl
	push func(values.JavaValue)
	pop  func() values.JavaValue
}

func (s *StackSimulationProxy) Push(value values.JavaValue) {
	s.push(value)
}

func (s *StackSimulationProxy) Pop() values.JavaValue {
	return s.pop()
}
func (s *StackSimulationProxy) PopN(n int) []values.JavaValue {
	vals := make([]values.JavaValue, n)
	for i := 0; i < n; i++ {
		vals[i] = s.Pop()
	}
	return vals
}

func NewStackSimulationProxy(stack *StackSimulationImpl, push func(values.JavaValue), pop func() values.JavaValue) *StackSimulationProxy {
	return &StackSimulationProxy{
		StackSimulationImpl: stack,
		push:                push,
		pop:                 pop,
	}
}

type StackSimulation interface {
	Size() int
	Pop() values.JavaValue
	PopN(n int) []values.JavaValue
	Push(values.JavaValue)
	Peek() values.JavaValue
	NewVar(val values.JavaValue) *values.JavaRef
	GetVarId() *utils.VariableId
	SetVarId(*utils.VariableId)
	AssignVar(slot int, val values.JavaValue) (*values.JavaRef, bool)
	AssignVarGuarded(slot int, val values.JavaValue, blockNullAdopt bool) (*values.JavaRef, bool)
	GetVar(slot int) *values.JavaRef
	SetVar(slot int, ref *values.JavaRef)
}

var _ StackSimulation = &StackSimulationImpl{}
var _ StackSimulation = &StackSimulationProxy{}

type StackSimulationImpl struct {
	stackEntry   *StackItem
	varTable     map[int]*values.JavaRef
	currentVarId *utils.VariableId
}

func (s *StackSimulationImpl) GetVarId() *utils.VariableId {
	return s.currentVarId
}

func (s *StackSimulationImpl) SetVarId(id *utils.VariableId) {
	s.currentVarId = id
}

func (s *StackSimulationImpl) GetVar(slot int) *values.JavaRef {
	return s.varTable[slot]
}

// SetVar overrides the slot's current variable version in the single global slot table. Used by the
// store-side reaching-definition repair to install the true dominating definition before AssignVar
// decides reuse-vs-mint, undoing DFS-order corruption of the slot table (Bug AI store side).
func (s *StackSimulationImpl) SetVar(slot int, ref *values.JavaRef) {
	s.varTable[slot] = ref
}

func (s *StackSimulationImpl) NewVar(val values.JavaValue) *values.JavaRef {
	s.currentVarId = s.currentVarId.Next()
	newRef := values.NewJavaRef(s.currentVarId, val, slotDeclType(val))
	//d.idToValue[d.currentVarId] = val
	return newRef
}

// slotDeclType returns the static type to declare a local slot with when val is stored into it. It
// equals val.Type() for every value EXCEPT a class literal (`Foo.class`): a JavaClassValue.Type()
// reports the *referenced* class (Foo) because that drives bare-type rendering (the `Foo.class`
// receiver and the `Foo.parseInt(...)` static-call qualifier), but as a stored rvalue the literal is
// a java.lang.Class instance, so the capturing local must be declared `Class`, not `Foo`. Declaring
// it `Foo` made later reads (`c.getName()`, `c.isPrimitive()`) fail to recompile with
// "cannot find symbol". Inline (single-use) class literals are unaffected because they fold back to
// `Foo.class.getName()` and never reach a declaration. Kill-switch: JDEC_NO_CLASSLIT_SLOT_TYPE=1.
func slotDeclType(val values.JavaValue) types.JavaType {
	if val == nil {
		return nil
	}
	if os.Getenv("JDEC_NO_CLASSLIT_SLOT_TYPE") == "" {
		if _, ok := values.UnpackSoltValue(val).(*values.JavaClassValue); ok {
			return types.NewJavaClass("java.lang.Class")
		}
	}
	return val.Type()
}

func (s *StackSimulationImpl) AssignVar(slot int, val values.JavaValue) (*values.JavaRef, bool) {
	return s.AssignVarGuarded(slot, val, false)
}

// AssignVarGuarded is AssignVar with an extra control: when blockNullAdopt is true the
// "null-initialized slot adopts the new concrete reference type" shortcut is suppressed and the
// store falls through to minting a fresh, block-scoped variable. The caller sets this when the
// slot's null initializer does NOT reach this store along the CFG (i.e. the null `T x = null`
// lives on a sibling/disjoint branch, e.g. a try-with-resources synthetic `primaryExc = null`),
// so adopting an unrelated type here would wrongly unify two distinct variables onto one slot.
func (s *StackSimulationImpl) AssignVarGuarded(slot int, val values.JavaValue, blockNullAdopt bool) (*values.JavaRef, bool) {
	typ := slotDeclType(val)
	ref, ok := s.varTable[slot]
	// Both the incoming value's type and the slot's current ref type must be present to compare
	// them; an upstream value occasionally carries a nil type (incomplete simulation), and
	// dereferencing it here panicked the whole method into a stub. Treat a missing type as
	// "different" so we fall through to allocating a fresh variable instead of crashing.
	if ok && typ != nil && ref.Type() != nil {
		ctx := &class_context.ClassContext{}
		if ref.Type().String(ctx) == typ.String(ctx) {
			return ref, false
		}
		// The slot already holds a variable of a different type. If that variable was only
		// null-initialized (an Object-typed `x = null` with no committed concrete type), reuse
		// it and adopt the new concrete reference type instead of splitting the slot into a
		// second, block-scoped variable. This keeps the ubiquitous
		// `T x = null; ...; x = v; ...; return x;` idiom as a single in-scope variable; the
		// split form left the reassigned variable block-scoped and read out of scope
		// ("cannot find symbol"). Only reference types may adopt a null (a primitive cannot),
		// so a primitive reassignment still falls through to the original split behavior.
		// blockNullAdopt suppresses this when the null initializer does not reach this store
		// (sibling-branch reuse of one slot for unrelated types, e.g. try-with-resources
		// primaryExc reusing the slot of an else-branch String).
		// A null-initialized slot may adopt a concrete reference type AT MOST ONCE. ResetVarType only
		// repoints the declared type and never clears Val, so IsNullInitialized stays true for the
		// lifetime of the ref; without this guard the same ref keeps adopting every later incompatible
		// store, collapsing two disjoint-live variables that merely reuse the JVM slot. The dominant
		// case is the old-javac try-with-resources desugaring: a synthetic `Throwable primaryExc = null`
		// (committed to Throwable in the synthetic catch) shares slot N with a later
		// `for (Map.Entry e : ...)` loop variable; adopting Map.Entry onto the already-Throwable ref
		// mistyped the declaration to `Map.Entry var = null`, so `var = <throwable>` and
		// `var.addSuppressed(..)` failed to recompile (commons-codec DaitchMokotoffSoundex.<clinit>).
		// Once committed, the incompatible store is a genuine slot reuse and falls through to minting a
		// fresh, block-scoped variable. Kill-switch: JDEC_NO_NULL_ADOPT_ONCE=1.
		if ref.IsNullInitialized() && !blockNullAdopt &&
			(os.Getenv("JDEC_NO_NULL_ADOPT_ONCE") != "" || !ref.NullTypeAdopted()) {
			if _, isPrim := typ.RawType().(*types.JavaPrimer); !isPrim {
				ref.ResetVarType(typ)
				if os.Getenv("JDEC_NO_NULL_ADOPT_ONCE") == "" {
					ref.MarkNullTypeAdopted()
				}
				return ref, false
			}
		}
		// A method parameter reassigned with a reference-typed value (`seq = str` where seq is a
		// CharSequence param and str is a String subtype) is the SAME variable being reassigned, not
		// a slot reused for a new local. Splitting it minted a fresh block-scoped ref whose
		// declaration sat inside the conditional that performed the reassignment, so a later read of
		// the slot referenced an out-of-scope name ("cannot find symbol", guava Ascii.truncate). Keep
		// the parameter as one variable (its broader declared type still accepts the subtype). Limit
		// to reference types on both sides: a parameter slot genuinely repurposed for a different
		// primitive category must still split. Kill-switch: JDEC_PARAM_REASSIGN_SPLIT=1.
		if ref.IsParam && !ref.IsThis && os.Getenv("JDEC_PARAM_REASSIGN_SPLIT") == "" {
			_, refPrim := ref.Type().RawType().(*types.JavaPrimer)
			_, valPrim := typ.RawType().(*types.JavaPrimer)
			if !refPrim && !valPrim {
				return ref, false
			}
		}
		// A primitive slot reassigned within the numeric int computational category
		// (byte/char/short/int — NOT boolean, which has no int<->boolean conversion) is the SAME
		// local widening, not a slot reused for a new variable. The JVM stores all of
		// byte/char/short/int through the int stack category and the same slot, so a value's static
		// element type (e.g. a byte from baload, an int from iconst) routinely disagrees with the
		// slot's declared type while denoting one variable. Splitting it minted a fresh block-scoped
		// ref whose declaration sat inside the branch performing the reassignment, so a later read of
		// the slot referenced an out-of-scope name (commons-codec Base16.decodeOctet:
		// `int r = -1; if (...) r = table[b]; if (r == -1) ...` read `r` outside its if). Keep one
		// variable and widen its declared type to int so every store assigns by implicit widening;
		// narrowing at byte/short/char-typed use sites is reintroduced by the call/return cast logic.
		// Same-slot int-category locals never have overlapping live ranges (the verifier would force
		// distinct slots), so merging is always safe to compile. Kill-switch:
		// JDEC_INTCAT_REASSIGN_SPLIT=1.
		if isIntCategoryNumeric(ref.Type()) && isIntCategoryNumeric(typ) && os.Getenv("JDEC_INTCAT_REASSIGN_SPLIT") == "" {
			if p, okp := ref.Type().RawType().(*types.JavaPrimer); okp && p.Name != types.JavaInteger {
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			}
			return ref, false
		}
	}
	newRef := s.NewVar(val)
	s.varTable[slot] = newRef
	return newRef, true
}

// isIntCategoryNumeric reports whether t is one of the numeric primitive types that share the JVM int
// computational category and hold each other's values by implicit widening on read: byte, char,
// short, int. boolean is deliberately excluded (Java forbids int<->boolean conversion); long, float
// and double are distinct categories with their own load/store opcodes and slot widths.
func isIntCategoryNumeric(t types.JavaType) bool {
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	if !ok {
		return false
	}
	switch p.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger:
		return true
	}
	return false
}

func NewEmptyStackEntry() *StackItem {
	return newStackItem(nil, nil)
}
func NewStackSimulation(entry *StackItem, varTable map[int]*values.JavaRef, generator *utils.VariableId) *StackSimulationImpl {
	sim := &StackSimulationImpl{
		stackEntry:   entry,
		varTable:     varTable,
		currentVarId: generator,
	}
	// maps.Copy(sim.varTable, varTable)
	return sim
}

func (s *StackSimulationImpl) Size() int {
	size := 0
	for entry := s.stackEntry; entry.parent != nil; entry = entry.GetParent() {
		size++
	}
	return size
}
func (s *StackSimulationImpl) Push(value values.JavaValue) {
	s.stackEntry = newStackItem(s.stackEntry, value)
}

func (s *StackSimulationImpl) Peek() values.JavaValue {
	if s.stackEntry.parent == nil {
		// Stack underflow: an incomplete simulation reached an opcode that expects an operand
		// that was never pushed. Return an empty-slot placeholder instead of panicking, so the
		// method degrades cleanly to a marked stub (the dumper detects the placeholder) and the
		// decompiler keeps its panic-free contract over the whole class.
		return values.NewSlotValue(nil, nil)
	}
	return s.stackEntry.value
}
func (s *StackSimulationImpl) Pop() values.JavaValue {
	if s.stackEntry.parent == nil {
		// Stack underflow (see Peek): hand back an empty-slot placeholder rather than panicking.
		return values.NewSlotValue(nil, nil)
	}
	val := s.stackEntry.value
	s.stackEntry = s.stackEntry.GetParent()
	return val
}
func (s *StackSimulationImpl) PopN(n int) []values.JavaValue {
	vals := make([]values.JavaValue, n)
	for i := 0; i < n; i++ {
		vals[i] = s.Pop()
	}
	return vals
}
