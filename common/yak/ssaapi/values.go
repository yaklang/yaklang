package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Values []*Value

func (value Values) Ref(name string) Values {
	// return nil
	var ret Values
	for _, v := range value {
		if v.IsField() {
			if v.GetOperand(1).String() == name {
				ret = append(ret, v)
			}
		}
		v.GetUsers().ForEach(func(v *Value) {
			// get value.Name or value["name"]
			if v.IsField() {
				if v.GetOperand(1).String() == name {
					ret = append(ret, v)
				}
			}
		})
	}
	return getValuesWithUpdate(ret)
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

func (v *Value) FixUpdateValue() Values {
	return getValuesWithUpdateSingle(v)
}

func (v Values) GetUsers() Values {
	ret := make(Values, 0, len(v))
	v.ForEach(func(v *Value) {
		ret = append(ret, v.GetUsers()...)
	})
	return ret
}

func (v Values) GetDefs() Values {
	ret := make(Values, 0, len(v))
	v.ForEach(func(v *Value) {
		ret = append(ret, v.GetOperands()...)
	})
	return ret
}

type Value struct {
	runtimeCtx *omap.OrderedMap[string, *Value]
	EffectOn   Values
	DependOn   Values

	node ssa.InstructionNode
	// cache
	disasmLine string
	users      Values
	operands   Values
}

func ValueCompare(v1, v2 *Value) bool {
	return v1.node == v2.node
}

func NewValue(n ssa.InstructionNode) *Value {
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
	if n, ok := v.node.(ssa.TypedNode); ok {
		return NewType(n.GetType())
	}
	return Any
}

func (v *Value) GetTypeKind() ssa.TypeKind {
	if n, ok := v.node.(ssa.TypedNode); ok {
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

func (i *Value) GetDefs() Values {
	return i.GetCallReturns()
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

func (i *Value) GetFieldName() *Value {
	if p, ok := ssa.ToField(i.node); ok {
		return NewValue(p.Key)
	}
	return nil
}

func (i *Value) GetFieldValues() Values {
	if p, ok := ssa.ToField(i.node); ok {
		return lo.Map(p.GetUsers(), func(item ssa.User, index int) *Value {
			if p, ok := ssa.ToUpdate(item); ok {
				return NewValue(p.Value)
			}
			return NewValue(item)
		})
	}
	return nil
}

func (i *Value) GetFirstFieldValue() *Value {
	vals := i.GetFieldValues()
	if len(vals) > 0 {
		return vals[0]
	}
	return nil
}

func (i *Value) GetLatestFieldValue() *Value {
	vals := i.GetFieldValues()
	if len(vals) > 0 {
		return vals[len(vals)-1]
	}
	return nil
}

func (i *Value) HasUsers() bool {
	return i.node.HasUsers()
}

func (i *Value) GetUsers() Values {
	if i.users == nil {
		i.users = lo.Map(i.node.GetUsers(), func(v ssa.User, _ int) *Value { return NewValue(v) })
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

func (v *Value) GetMakeObjectFields() Values {
	if f, ok := ssa.ToMake(v.node); ok {
		return lo.Filter(lo.Map(f.GetUsers(), func(item ssa.User, index int) *Value {
			return NewValue(item)
		}), func(item *Value, index int) bool {
			return item.IsField()
		})
	}
	return nil
}

func (v *Value) GetCallReturns() Values {
	return v.GetUsers()
}

// for const instruction
func (v *Value) GetConstValue() any {
	if v == nil {
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

func (v *Value) IsExternLib() bool    { return v.node.GetOpcode() == ssa.OpExternLib }
func (v *Value) IsFunction() bool     { return v.node.GetOpcode() == ssa.OpFunction }
func (v *Value) IsBasicBlock() bool   { return v.node.GetOpcode() == ssa.OpBasicBlock }
func (v *Value) IsParameter() bool    { return v.node.GetOpcode() == ssa.OpParameter }
func (v *Value) IsPhi() bool          { return v.node.GetOpcode() == ssa.OpPhi }
func (v *Value) IsConstInst() bool    { return v.node.GetOpcode() == ssa.OpConstInst }
func (v *Value) IsUndefined() bool    { return v.node.GetOpcode() == ssa.OpUndefined }
func (v *Value) IsBinOp() bool        { return v.node.GetOpcode() == ssa.OpBinOp }
func (v *Value) IsUnOp() bool         { return v.node.GetOpcode() == ssa.OpUnOp }
func (v *Value) IsCall() bool         { return v.node.GetOpcode() == ssa.OpCall }
func (v *Value) IsReturn() bool       { return v.node.GetOpcode() == ssa.OpReturn }
func (v *Value) IsMake() bool         { return v.node.GetOpcode() == ssa.OpMake }
func (v *Value) IsField() bool        { return v.node.GetOpcode() == ssa.OpField }
func (v *Value) IsUpdate() bool       { return v.node.GetOpcode() == ssa.OpUpdate }
func (v *Value) IsNext() bool         { return v.node.GetOpcode() == ssa.OpNext }
func (v *Value) IsAssert() bool       { return v.node.GetOpcode() == ssa.OpAssert }
func (v *Value) IsTypeCast() bool     { return v.node.GetOpcode() == ssa.OpTypeCast }
func (v *Value) IsTypeValue() bool    { return v.node.GetOpcode() == ssa.OpTypeValue }
func (v *Value) IsErrorHandler() bool { return v.node.GetOpcode() == ssa.OpErrorHandler }
func (v *Value) IsPanic() bool        { return v.node.GetOpcode() == ssa.OpPanic }
func (v *Value) IsRecover() bool      { return v.node.GetOpcode() == ssa.OpRecover }
func (v *Value) IsJump() bool         { return v.node.GetOpcode() == ssa.OpJump }
func (v *Value) IsIf() bool           { return v.node.GetOpcode() == ssa.OpIf }
func (v *Value) IsLoop() bool         { return v.node.GetOpcode() == ssa.OpLoop }
func (v *Value) IsSwitch() bool       { return v.node.GetOpcode() == ssa.OpSwitch }

func GetBareNode(v *Value) ssa.InstructionNode {
	return v.node
}

// IsCalled desc any of 'Users' is Call or Make
func (v *Value) IsCalled() bool {
	return len(v.GetUsers().Filter(func(value *Value) bool {
		return value.IsCall()
	})) > 0
}

// GetCalledBy desc all of 'Users' is Call or Make
func (v *Value) GetCalledBy() Values {
	return v.GetUsers().Filter(func(value *Value) bool {
		return value.IsCall()
	})
}

// GetCallee desc any of 'Users' is Call
func (v *Value) GetCallee() *Value {
	if v.IsCall() {
		return v.GetOperand(0)
	}
	return nil
}
