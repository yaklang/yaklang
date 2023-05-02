package spec

import (
	"fmt"
	"reflect"

	"github.com/davecgh/go-spew/spew"
)

// -----------------------------------------------------------------------------

type undefinedType struct {
	undefined bool
}

func (s *undefinedType) String() string {
	return "undefined"
}

var undefinedBytes = []byte("\"```undefined```\"")

func (p undefinedType) Error() string {
	return "undefined"
}

func (p undefinedType) MarshalJSON() ([]byte, error) {
	return undefinedBytes, nil
}

var (
	// Undefined is `undefined` in yak.
	Undefined interface{} = &undefinedType{undefined: true}
)

func IsUndefinedType(val interface{}) bool {
	v, ok := val.(*undefinedType)
	return ok && v.undefined
}

// -----------------------------------------------------------------------------

// A Chan represents chan class in yak.
type Chan struct {
	Data reflect.Value
}

// A DataIndex represents a compound data and its index. such as map[index], slice[index], object.member.
type DataIndex struct {
	Data        interface{}
	Index       interface{}
	DataVerbose string
}

// -----------------------------------------------------------------------------

// GoTyper is required by `yak type` spec.
type GoTyper interface {
	GoType() reflect.Type
}

type GoTypeAdaptor struct {
	Type reflect.Type
}

func (g *GoTypeAdaptor) GoType() reflect.Type {
	return g.Type
}

func GoTypeFunc(r reflect.Type) GoTyper {
	return &GoTypeAdaptor{Type: r}
}

// A Type represents a yak builtin type.
type Type struct {
	t reflect.Type
}

// NewType returns a yak builtin type object.
func NewType(t reflect.Type) *Type {

	return &Type{t: t}
}

// StructOf returns a yak builtin type object.
func StructOf(ptr interface{}) *Type {

	return &Type{t: reflect.TypeOf(ptr).Elem()}
}

// GoType returns the underlying go type. required by `yak type` spec.
func (p *Type) GoType() reflect.Type {

	return p.t
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p *Type) NewInstance(args ...interface{}) interface{} {

	ret := reflect.New(p.t)
	if len(args) > 0 {
		v := reflect.ValueOf(args[0]).Convert(p.t)
		ret.Set(v)
	}
	return ret.Interface()
}

// Call returns `T(a)`.
func (p *Type) Call(a interface{}) interface{} {

	if a == nil {
		return reflect.Zero(p.t).Interface()
	}
	return reflect.ValueOf(a).Convert(p.t).Interface()
}

func (p *Type) String() string {

	return p.t.String()
}

// TySliceOf represents the `[]T` type.
func TySliceOf(elem reflect.Type) *Type {

	return &Type{t: reflect.SliceOf(elem)}
}

// TyMapOf represents the `map[key]elem` type.
func TyMapOf(key, elem reflect.Type) *Type {

	return &Type{t: reflect.MapOf(key, elem)}
}

// TyPtrTo represents the `*T` type.
func TyPtrTo(elem reflect.Type) *Type {

	return &Type{t: reflect.PtrTo(elem)}
}

// -----------------------------------------------------------------------------

// A TypeEx represents a yak builtin type with a cast function.
type TypeEx struct {
	t    reflect.Type
	Call interface{}
}

// NewTypeEx returns a yak builtin type object with a cast function.
func NewTypeEx(t reflect.Type, cast interface{}) *TypeEx {

	return &TypeEx{t: t, Call: cast}
}

// StructOfEx returns a yak builtin type object with a cast function.
func StructOfEx(ptr interface{}, cast interface{}) *TypeEx {

	return &TypeEx{t: reflect.TypeOf(ptr).Elem(), Call: cast}
}

// GoType returns the underlying go type. required by `yak type` spec.
func (p *TypeEx) GoType() reflect.Type {

	return p.t
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p *TypeEx) NewInstance(args ...interface{}) interface{} {

	ret := reflect.New(p.t)
	if len(args) > 0 {
		panic(fmt.Sprintf("type `%v` doesn't support initializing with a constructor", p.t))
	}
	return ret.Interface()
}

func (p *TypeEx) String() string {

	return p.t.String()
}

// -----------------------------------------------------------------------------

// AutoConvert converts a value to specified type automatically.
func AutoConvert(v reflect.Value, t reflect.Type) reflect.Value {

	tkind := t.Kind()
	if tkind == reflect.Interface {
		return v
	}

	kind := v.Kind()
	if kind == tkind || convertible(kind, tkind) {
		return v.Convert(t)
	}
	panic(fmt.Sprintf("Can't convert `%v` to `%v` automatically", v.Type(), t))
}

func convertible(kind, tkind reflect.Kind) bool {

	if tkind >= reflect.Int && tkind <= reflect.Uintptr {
		return kind >= reflect.Int && kind <= reflect.Uintptr
	}
	if tkind == reflect.Float64 || tkind == reflect.Float32 {
		return kind >= reflect.Int && kind <= reflect.Float64
	}
	return false
}

// -----------------------------------------------------------------------------

// A Module represents a yak module to be imported.
type Module struct {
	Exports map[string]interface{}
}

// Disable disables some export names.
func (p Module) Disable(fnNames ...string) {

	for _, fnName := range fnNames {
		p.Exports[fnName] = dummyN
	}
}

// GoModuleName returns name of a yak module.
func GoModuleName(table map[string]interface{}) (name string, ok bool) {

	name, ok = table["_name"].(string)
	return
}

func fetchInitSafe(table map[string]interface{}) (initSafe func(mod Module), ok bool) {

	vinitSafe, ok := table["_initSafe"]
	if !ok {
		return
	}
	initSafe, ok = vinitSafe.(func(mod Module))
	if !ok {
		panic("invalid prototype of initSafe: must be `func initSafe(mod yak.Module)`")
	}
	delete(table, "_initSafe")
	return
}

// -----------------------------------------------------------------------------

func dummy1(a interface{}) interface{} {
	panic("function not implemented")
}

func dummy2(a, b interface{}) interface{} {
	panic("function not implemented")
}

func dummy3(a, i, j interface{}) interface{} {
	panic("function not implemented")
}

func dummyN(a ...interface{}) interface{} {
	panic("function not implemented")
}

func dummyGet(a interface{}, b interface{}, c ...interface{}) interface{} {
	panic("function not implemented")
}

func dummySetIndex(m, index, val interface{}) {
	panic("function not implemented")
}

func chanInHandler(ch, val interface{}, try bool) interface{} {
	t := reflect.TypeOf(ch)
	if t.Kind() != reflect.Chan || t.ChanDir()&reflect.SendDir == 0 {
		panic("cannot write this channel")
	}

	v := reflect.ValueOf(ch)
	v.Send(reflect.ValueOf(val))
	return nil
}

func chanOutHandler(ch interface{}, try bool) interface{} {
	t := reflect.TypeOf(ch)
	if t.Kind() != reflect.Chan || t.ChanDir()&reflect.RecvDir == 0 {
		panic("unreadable channel")
	}

	v := reflect.ValueOf(ch)
	x, ok := v.Recv()
	if !ok {
		return Undefined
	}
	return x.Interface()
}

func makeChan(typ reflect.Type, cap ...int) interface{} {
	if len(cap) > 1 {
		panic(fmt.Sprintf("channel buffer should be one integer~ no [%v] ", cap))
	}

	if typ.Kind() != reflect.Chan {
		panic(fmt.Sprintf("make chan cannot recognize type: %v", typ))
	}

	if len(cap) <= 0 {
		return reflect.MakeChan(typ, 0).Interface()
	}
	return reflect.MakeChan(typ, cap[0]).Interface()
}

var (
	ChanOut  = chanOutHandler
	ChanIn   = chanInHandler
	MakeChan = makeChan
	ChanOf   = func(i interface{}) interface{} {
		t, ok := i.(GoTyper)
		if !ok {
			panic(fmt.Sprintf("unknown type: %v", spew.Sdump(i)))
		}
		typeChanOf := reflect.ChanOf(3, t.GoType())
		return GoTypeFunc(typeChanOf)
	}
)

var (
	fnDummy1    = reflect.ValueOf(dummy1).Pointer()
	fnDummy2    = reflect.ValueOf(dummy2).Pointer()
	GetEx       = dummy2
	SetIndex    = dummySetIndex
	MapInit     = dummyN
	MapFrom     = dummyN
	Map         = dummy2
	Slice       = dummy1
	SliceFrom   = dummyN
	SliceFromTy = dummyN
	StructInit  = dummyN
	SubSlice    = dummy3
	Xor         = dummy2
	Lshr        = dummy2
	Rshr        = dummy2
	BitAnd      = dummy2
	BitOr       = dummy2
	AndNot      = dummy2
	EQ          = dummy2
	GetVar      = dummy2
	Get         = dummyGet
	Add         = dummy2
	Sub         = dummy2
	Mul         = dummy2
	Quo         = dummy2
	Mod         = dummy2
	Inc         = dummy1
	Dec         = dummy1
)

// -----------------------------------------------------------------------------

// Fntable is function table required by tpl.Interpreter engine.
var Fntable = map[string]interface{}{
	"$neg":  dummy1,
	"$elem": dummy1,
	"$mul":  dummy2,
	"$quo":  dummy2,
	"$mod":  dummy2,
	"$add":  dummy2,
	"$sub":  dummy2,

	"$xor":    dummy2,
	"$lshr":   dummy2,
	"$rshr":   dummy2,
	"$bitand": dummy2,
	"$bitor":  dummy2,
	"$bitnot": dummy1,
	"$andnot": dummy2,

	"$lt":  dummy2,
	"$gt":  dummy2,
	"$le":  dummy2,
	"$ge":  dummy2,
	"$eq":  dummy2,
	"$ne":  dummy2,
	"$and": dummy2,
	"$or":  dummy2,
	"$not": dummy1,
}

// -----------------------------------------------------------------------------

// SafeMode is the init mode of yak.
var SafeMode bool

var goMods []string

// GoModuleList returns yak modules implemented by Go.
func GoModuleList() []string {
	return goMods
}

// Import imports a yak module implemented by Go.
func Import(mod string, table map[string]interface{}) {

	if SafeMode {
		if initSafe, ok := fetchInitSafe(table); ok {
			initSafe(Module{Exports: table})
		}
	}

	if mod != "" {
		if _, ok := Fntable[mod]; ok {
			panic("module to import exists already: " + mod)
		}
		Fntable[mod] = table
		goMods = append(goMods, mod)
		return
	}

	for name, fn := range table {
		if name == "_name" {
			continue
		}
		if fn, ok := Fntable[name]; ok {
			if v := reflect.ValueOf(fn); v.Kind() == reflect.Func {
				p := v.Pointer()
				if p != fnDummy1 && p != fnDummy2 {
					panic("symbol to import exists already: " + name)
				}
			} else {
				panic("symbol to import exists already: " + name)
			}
		}
		Fntable[name] = fn
	}
}

// -----------------------------------------------------------------------------

var (
	// DumpStack indicates to dump stack when error.
	DumpStack = false

	// AutoCall is reserved for internal use.
	AutoCall = make(map[reflect.Type]bool)

	// DontTyNormalize is reserved for internal use.
	DontTyNormalize = make(map[reflect.Type]bool)
)

// SetAutoCall is reserved for internal use.
func SetAutoCall(t reflect.Type) {
	AutoCall[t] = true
}

// SetDontTyNormalize is reserved for internal use.
func SetDontTyNormalize(t reflect.Type) {
	DontTyNormalize[t] = true
}

// SetDumpStack set to dump stack or not.
func SetDumpStack(dump bool) {
	DumpStack = dump
}

// -----------------------------------------------------------------------------
