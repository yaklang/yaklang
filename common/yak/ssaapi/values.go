package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Value struct {
	runtimeCtx *omap.OrderedMap[string, *Value]
	EffectOn   Values
	DependOn   Values

	node ssa.Value
	// cache
	disasmLine string
	users      Values
	operands   Values
}

func ValueCompare(v1, v2 *Value) bool {
	return v1.node == v2.node
}

func NewValue(n ssa.Value) *Value {
	return &Value{
		runtimeCtx: omap.NewEmptyOrderedMap[string, *Value](),
		node:       n,
	}
}

func (v *Value) GetId() int {
	if v.node == nil {
		return -1
	}
	return v.node.GetId()
}

func (v *Value) NewError(tag, msg string) {
	v.node.NewError(ssa.Error, ssa.ErrorTag(tag), msg)
}

func (v *Value) NewWarn(tag, msg string) {
	v.node.NewError(ssa.Warn, ssa.ErrorTag(tag), msg)
}

func (v *Value) String() string      { return ssa.LineDisasm(v.node) }
func (v *Value) ShortString() string { return ssa.LineShortDisasm(v.node) }
func (i *Value) StringWithSource() string {
	if i.disasmLine == "" {
		i.disasmLine = fmt.Sprintf("[%-6s] %s\t%s", i.node.GetOpcode(), ssa.LineDisasm(i.node), i.node.GetRange())
	}
	return i.disasmLine
}

func (i *Value) GetName() string { return i.node.GetName() }

func (i *Value) GetVerboseName() string {
	var name string
	if name = i.node.GetName(); name != "" {
		if i.IsPhi() {
			return "[phi]: " + name
		}
		return name
	} else if name = i.node.GetVerboseName(); name != "" {
		return fmt.Sprintf(`t%d: %v=%v`, i.GetId(), name, i.ShortString())
	}
	return fmt.Sprintf(`t%d: %v`, i.GetId(), i.ShortString())
}

func (i *Value) Show()           { fmt.Println(i) }
func (i *Value) ShowWithSource() { fmt.Println(i.StringWithSource()) }

func (v *Value) Compare(other *Value) bool { return ValueCompare(v, other) }

func (v *Value) GetType() *Type {
	if n, ok := v.node.(ssa.Typed); ok {
		return NewType(n.GetType())
	}
	return Any
}

func (v *Value) GetTypeKind() ssa.TypeKind {
	if n, ok := v.node.(ssa.Typed); ok {
		return n.GetType().GetTypeKind()
	}
	return ssa.AnyTypeKind
}

func (v *Value) GetRange() *ssa.Range {
	return v.node.GetRange()
}

func (i *Value) HasOperands() bool {
	return i.node.HasValues()
}

func (i *Value) GetOperands() Values {
	if i.operands == nil {
		i.operands = lo.Map(ssa.GetValues(i.node), func(v ssa.Value, _ int) *Value { return NewValue(v) })
	}
	return i.operands
}

func (i *Value) GetOperand(index int) *Value {
	opts := i.GetOperands()
	if index >= len(opts) {
		return nil
	}
	return opts[index]
}

func (i *Value) HasUsers() bool {
	return i.node.HasUsers()
}

func (i *Value) GetUsers() Values {
	if i.users == nil {
		i.users = lo.FilterMap(i.node.GetUsers(),
			func(v ssa.User, _ int) (*Value, bool) {
				if value, ok := ssa.ToValue(v); ok {
					return NewValue(value), true
				}
				return nil, false
			},
		)
	}
	return i.users
}

func (i *Value) GetUser(index int) *Value {
	users := i.GetUsers()
	if index >= len(users) {
		return nil
	}
	return users[index]
}

func (v *Value) ShowUseDefChain() {
	showUseDefChain(v)
}

// for function

func (v *Value) GetReturn() Values {
	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, r := range f.Return {
			ret = append(ret, NewValue(r))
		}
	}
	return ret
}

func (v *Value) GetParameter(i int) *Value {
	if f, ok := ssa.ToFunction(v.node); ok {
		if i < len(f.Param) {
			return NewValue(f.Param[i])
		}
	}
	return nil
}

func (v *Value) GetParameters() Values {
	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, v := range f.Param {
			ret = append(ret, NewValue(v))
		}
	}
	return ret
}

func (v *Value) GetCallArgs() Values {
	if f, ok := ssa.ToCall(v.node); ok {
		return lo.Map(f.Args, func(item ssa.Value, index int) *Value {
			return NewValue(item)
		})
	}
	return nil
}

func (v *Value) GetCallReturns() Values {
	return v.GetUsers()
}

// for const instruction
func (v *Value) GetConstValue() any {
	if v == nil || v.node == nil {
		return nil
	}
	if v.IsConstInst() {
		return v.node.(*ssa.ConstInst).GetRawValue()
	} else {
		return nil
	}
}

func (v *Value) GetConst() *ssa.Const {
	if v.IsConstInst() {
		return v.node.(*ssa.ConstInst).Const
	} else {
		return nil
	}
}

func (v *Value) GetOpcode() ssa.Opcode {
	return v.node.GetOpcode()
}

func (v *Value) IsModifySelf() bool {
	if !v.IsCall() {
		return false
	}
	ft, ok := ssa.ToFunctionType(GetBareType(v.GetOperand(0).GetType()))
	return ok && ft.IsModifySelf
}

func (v *Value) GetSelf() *Value {
	if v.IsModifySelf() {
		return v.GetOperand(1)
	}
	return v
}

// // Instruction Opcode
func (v *Value) getOpcode() ssa.Opcode {
	if v.node == nil {
		return ssa.OpUnknown
	}
	return v.node.GetOpcode()
}

// IsExternLib desc if the value is extern lib
//
// extern-lib is a special value that is used to represent the external library
/*
	code := `a = fmt.Println`
	fmt := prog.Ref("fmt") // extern-lib
	fmt.GetOperands() // Values // [Function-Println]
*/
func (v *Value) IsExternLib() bool    { return v.getOpcode() == ssa.OpExternLib }
func (v *Value) IsFunction() bool     { return v.getOpcode() == ssa.OpFunction }
func (v *Value) IsBasicBlock() bool   { return v.getOpcode() == ssa.OpBasicBlock }
func (v *Value) IsParameter() bool    { return v.getOpcode() == ssa.OpParameter }
func (v *Value) IsPhi() bool          { return v.getOpcode() == ssa.OpPhi }
func (v *Value) IsConstInst() bool    { return v.getOpcode() == ssa.OpConstInst }
func (v *Value) IsUndefined() bool    { return v.getOpcode() == ssa.OpUndefined }
func (v *Value) IsBinOp() bool        { return v.getOpcode() == ssa.OpBinOp }
func (v *Value) IsUnOp() bool         { return v.getOpcode() == ssa.OpUnOp }
func (v *Value) IsCall() bool         { return v.getOpcode() == ssa.OpCall }
func (v *Value) IsReturn() bool       { return v.getOpcode() == ssa.OpReturn }
func (v *Value) IsMake() bool         { return v.getOpcode() == ssa.OpMake }
func (v *Value) IsNext() bool         { return v.getOpcode() == ssa.OpNext }
func (v *Value) IsAssert() bool       { return v.getOpcode() == ssa.OpAssert }
func (v *Value) IsTypeCast() bool     { return v.getOpcode() == ssa.OpTypeCast }
func (v *Value) IsTypeValue() bool    { return v.getOpcode() == ssa.OpTypeValue }
func (v *Value) IsErrorHandler() bool { return v.getOpcode() == ssa.OpErrorHandler }
func (v *Value) IsPanic() bool        { return v.getOpcode() == ssa.OpPanic }
func (v *Value) IsRecover() bool      { return v.getOpcode() == ssa.OpRecover }
func (v *Value) IsJump() bool         { return v.getOpcode() == ssa.OpJump }
func (v *Value) IsIf() bool           { return v.getOpcode() == ssa.OpIf }
func (v *Value) IsLoop() bool         { return v.getOpcode() == ssa.OpLoop }
func (v *Value) IsSwitch() bool       { return v.getOpcode() == ssa.OpSwitch }

// // MemberCall : Object

// IsObject desc if the value is object
func (v *Value) IsObject() bool { return v.node.IsObject() }

// GetMember get member of object by key
func (v *Value) GetMember(key Value) *Value {
	node := v.node
	if ret, ok := node.GetMember(key.node); ok {
		return NewValue(ret)
	}
	return nil
}

// GetAllMember get all member of object
func (v *Value) GetAllMember() Values {
	all := v.node.GetAllMember()
	ret := make(Values, 0, len(all))
	for _, value := range all {
		ret = append(ret, NewValue(value))
	}
	return ret
}

// // MemberCall : member

// IsMember desc if the value is member of some object
func (v *Value) IsMember() bool { return v.node.IsMember() }

// GetObject get object of member
func (v *Value) GetObject() *Value { return NewValue(v.node.GetObject()) }

// GetKey get key of member
func (v *Value) GetKey() *Value { return NewValue(v.node.GetKey()) }

// GetBareNode get ssa.Value from ssaapi.Value
// only use this function in golang
func GetBareNode(v *Value) ssa.Value {
	return v.node
}

// IsCalled desc any of 'Users' is Call
func (v *Value) IsCalled() bool {
	return len(v.GetUsers().Filter(func(value *Value) bool {
		return value.IsCall()
	})) > 0
}

// GetCalledBy desc all of 'Users' is Call
func (v *Value) GetCalledBy() Values {
	return v.GetUsers().Filter(func(value *Value) bool {
		return value.IsCall() && ValueCompare(value.GetCallee(), v)
	})
}

// GetCallee desc any of 'Users' is Call
func (v *Value) GetCallee() *Value {
	if v.IsCall() {
		return v.GetOperand(0)
	}
	return nil
}

type Values []*Value

func (value Values) Ref(name string) Values {
	ret := make(Values, 0, len(value))
	for _, v := range value {
		v.GetAllMember().ForEach(func(v *Value) {
			if v.GetKey().String() == name {
				ret = append(ret, v)
			}
		})
	}
	return ret
}

func (v Values) StringEx(flag int) string {
	ret := ""
	ret += fmt.Sprintf("Values: %d\n", len(v))
	for i, v := range v {
		switch flag {
		case 0:
			ret += fmt.Sprintf("\t%d: %5s: %s\n", i, v.node.GetOpcode(), v)
		case 1:
			ret += fmt.Sprintf("\t%d: %s\n", i, v.StringWithSource())
		}
	}
	return ret
}

func (v Values) String() string { return v.StringEx(0) }
func (v Values) Show(b ...bool) Values {
	if len(b) > 0 && !b[0] {
		return v
	}
	fmt.Println(v.StringEx(0))
	return v
}
func (v Values) ShowWithSource(b ...bool) Values {
	if len(b) > 0 && !b[0] {
		return v
	}
	fmt.Println(v.StringEx(1))
	return v
}

func (v Values) Get(i int) *Value {
	if i < len(v) {
		return v[i]
	}
	return NewValue(ssa.NewUndefined(""))
}

func (v Values) ForEach(f func(*Value)) Values {
	for _, v := range v {
		f(v)
	}
	return v
}

func (v Values) Flat(f func(*Value) Values) Values {
	var newVals Values
	for _, subValue := range v {
		if ret := f(subValue); len(ret) > 0 {
			newVals = append(newVals, ret...)
		}
	}
	return newVals
}

func (v Values) Filter(f func(*Value) bool) Values {
	ret := make(Values, 0, len(v))
	v.ForEach(func(v *Value) {
		if f(v) {
			ret = append(ret, v)
		}
	})
	return ret
}

func (v Values) GetUsers() Values {
	ret := make(Values, 0, len(v))
	v.ForEach(func(v *Value) {
		ret = append(ret, v.GetUsers()...)
	})
	return ret
}

func (v Values) GetOperands() Values {
	ret := make(Values, 0, len(v))
	v.ForEach(func(v *Value) {
		ret = append(ret, v.GetOperands()...)
	})
	return ret
}
