package ssaapi

import (
	"fmt"
	"slices"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type Value struct {
	runtimeCtx    *omap.OrderedMap[ContextID, *Value]
	ParentProgram *Program
	EffectOn      *utils.SafeMapWithKey[string, *Value]
	DependOn      *utils.SafeMapWithKey[string, *Value]

	// 唯一标识符
	uuid string

	// inner data for ssa
	innerValue ssa.Value
	innerUser  ssa.User

	// cache
	disasmLine string

	users    Values
	operands Values

	// for debug
	syntaxFlowName []string

	// for syntaxflow vm
	Predecessors []*PredecessorValue
	PrevDataFlow []*Value
	DescInfo     map[string]string
	// value from database
	auditNode *ssadb.AuditNode
}

type PredecessorValue struct {
	Node *Value
	Info *sfvm.AnalysisContext
}

func (v *Value) getInstruction() ssa.Instruction {
	if utils.IsNil(v) {
		log.Errorf("ssaapi.Value: getInstruction on nil value")
		return nil
	}
	if v.innerValue != nil {
		return v.innerValue
	}
	if v.innerUser != nil {
		return v.innerUser
	}
	return nil
}

func (v *Value) hasDataFlow(target *Value) bool {
	if v == nil {
		return false
	}
	if v.PrevDataFlow == nil {
		return false
	}
	return slices.Contains(v.PrevDataFlow, target)
}

func (v *Value) hasDependOn(target *Value) bool {
	if v == nil {
		return false
	}
	if v.DependOn == nil {
		return false
	}
	_, b := v.DependOn.Get(target.GetUUID())
	return b
}

func (v *Value) setDependOn(target *Value) {
	if v == nil {
		return
	}
	if v.DependOn == nil {
		v.DependOn = utils.NewSafeMapWithKey[string, *Value]()
	}
	v.DependOn.Set(target.GetUUID(), target)
}

func (v *Value) deleteDependOn(target *Value) {
	if v == nil || v.DependOn == nil {
		return
	}
	v.DependOn.Delete(target.GetUUID())
}

func (v *Value) hasEffectOn(target *Value) bool {
	if v == nil {
		return false
	}
	if v.EffectOn == nil {
		return false
	}
	_, b := v.EffectOn.Get(target.GetUUID())
	return b
}

func (v *Value) setEffectOn(target *Value) {
	if v == nil {
		return
	}
	if v.EffectOn == nil {
		v.EffectOn = utils.NewSafeMapWithKey[string, *Value]()
	}
	v.EffectOn.Set(target.GetUUID(), target)
}

func (v *Value) deleteEffectOn(target *Value) {
	if v == nil || v.EffectOn == nil {
		return
	}
	v.EffectOn.Delete(target.GetUUID())
}

func (v *Value) GetDependOnCount() int {
	if v == nil || v.DependOn == nil {
		return 0
	}
	return v.DependOn.Count()
}

func (v *Value) EffectOnCount() int {
	if v == nil || v.EffectOn == nil {
		return 0
	}
	return v.EffectOn.Count()
}

func (v *Value) GetAuditNodeId() uint {
	if v.auditNode == nil {
		return 0
	}
	return v.auditNode.ID
}

func (v *Value) IsFromDataBase() bool {
	return v.auditNode != nil
}

func ValueContain(v1 *Value, v2 ...*Value) bool {
	for _, v := range v2 {
		if ValueCompare(v1, v) {
			return true
		}
	}
	return false
}

func ValueCompare(v1raw, v2raw *Value) bool {
	if v1raw == nil || v2raw == nil {
		return v1raw == v2raw
	}
	v1 := v1raw.getInstruction()
	v2 := v2raw.getInstruction()
	if v1 == nil || v2 == nil {
		return v1 == v2
	}

	same := func(v1, v2 ssa.Instruction) bool {
		if pointIF, ok := v1.(ssa.PointerIF); ok {
			for _, v := range pointIF.GetPointer() {
				if v.GetId() == v2.GetId() {
					return true
				}
			}
		}
		return v1.GetId() == v2.GetId()
	}
	res := same(v1, v2) || same(v2, v1)
	if pointIF, ok := v1.(ssa.PointerIF); ok {
		if v1Ref := pointIF.GetReference(); v1Ref != nil {
			res = res || same(v1Ref, v2) || same(v2, v1Ref)
		}
	}
	if pointIF, ok := v2.(ssa.PointerIF); ok {
		if v2Ref := pointIF.GetReference(); v2Ref != nil {
			res = res || same(v1, v2Ref) || same(v2Ref, v1)
		}
	}
	return res
}

func (v *Value) Hash() (string, bool) {
	if v.IsNil() {
		return "", false
	}
	return utils.CalcSha256(v.GetId()), true
}

func (v *Value) GetProgramName() string {
	if v.IsNil() {
		return ""
	}
	if prog := v.getInstruction().GetProgram(); prog != nil {
		return prog.GetProgramName()
	}
	if prog := v.ParentProgram; prog != nil {
		if prog.Program != nil {
			return prog.Program.GetProgramName()
		}
	}
	return ""
}

func (v *Value) IsNil(checkValue ...bool) bool {
	// return v == nil || v.getInstruction() == nil
	if utils.IsNil(v) {
		return true
	}

	if len(checkValue) > 0 && checkValue[0] {
		// check is value ?
		return v.innerValue == nil
	}
	return v.getInstruction() == nil
}

func (v *Value) GetId() int64 {
	if v.IsNil() {
		return -1
	}
	return v.getInstruction().GetId()
}

// GetUUID 返回Value的唯一标识符
func (v *Value) GetUUID() string {
	if v.IsNil() {
		return ""
	}
	return v.uuid
}

func (v *Value) GetSSAInst() ssa.Instruction {
	if v.IsNil() {
		return nil
	}
	return v.getInstruction()
}

func (v *Value) NewError(tag, msg string) {
	if v.IsNil() {
		return
	}
	v.getInstruction().NewError(ssa.Error, ssa.ErrorTag(tag), msg)
}

func (v *Value) NewWarn(tag, msg string) {
	if v.IsNil() {
		return
	}
	v.getInstruction().NewError(ssa.Warn, ssa.ErrorTag(tag), msg)
}

func (v *Value) String() string {
	if v.IsNil() {
		return ""
	}
	inst := v.getInstruction()
	return ssa.LineDisASM(inst)
}

func (v *Value) ShortString() string {
	if v.IsNil() {
		return ""
	}
	inst := v.getInstruction()
	return ssa.LineShortDisASM(inst)
}

func (v *Value) StringWithRange() string {
	if v.IsNil() {
		return ""
	}

	if v.disasmLine == "" {
		inst := v.getInstruction()
		v.disasmLine = fmt.Sprintf("[%-6s] %s\t%s", ssa.SSAOpcode2Name[inst.GetOpcode()], ssa.LineDisASM(inst), inst.GetRange())
	}
	return v.disasmLine
}

func (v *Value) StringWithSourceCode(msg ...string) string {
	if v.IsNil() {
		return ""
	}

	if v.disasmLine == "" {
		inst := v.getInstruction()
		if v.innerUser != nil {
			inst = v.innerUser
		}
		v.disasmLine = fmt.Sprintf("[%-6s] %s\t%s", ssa.SSAOpcode2Name[inst.GetOpcode()], ssa.LineDisASM(inst), inst.GetRange())
		v.disasmLine += "\n" + v.GetRange().GetTextContextWithPrompt(2, msg...)
	}
	return v.disasmLine
}

func (v *Value) GetName() string {
	if v.IsNil() {
		return ""
	}
	return v.getInstruction().GetName()
}

func (v *Value) GetVerboseName() string {
	if v.IsNil() {
		return ""
	}
	var name string
	if name = v.getInstruction().GetName(); name != "" {
		if v.IsPhi() {
			return "[phi]: " + name
		}
		return name
	} else if name = v.getInstruction().GetVerboseName(); name != "" {
		return fmt.Sprintf(`t%d: %v=%v`, v.GetId(), name, v.ShortString())
	}
	return fmt.Sprintf(`t%d: %v`, v.GetId(), v.ShortString())
}

func (v *Value) GetInnerValueVerboseName() string {
	if v.IsNil() {
		return ""
	}
	inner := v.innerValue
	if utils.IsNil(inner) {
		return ""
	}
	return inner.GetVerboseName()
}

func (i *Value) Show() *Value               { fmt.Println(i); return i }
func (i *Value) ShowWithRange() *Value      { fmt.Println(i.StringWithRange()); return i }
func (i *Value) ShowWithSourceCode() *Value { fmt.Println(i.StringWithSourceCode()); return i }

func (v *Value) Compare(other *Value) bool { return ValueCompare(v, other) }

func (v *Value) GetType() *Type {
	if v.IsNil() {
		return NewType(ssa.CreateAnyType())
	}
	inst := v.getInstruction()
	if n, ok := inst.(ssa.Typed); ok {
		return NewType(n.GetType())
	}
	return NewType(ssa.CreateAnyType())
}

func (v *Value) GetTypeKind() ssa.TypeKind {
	if v.IsNil() {
		return ssa.AnyTypeKind
	}
	inst := v.getInstruction()
	if n, ok := inst.(ssa.Typed); ok {
		if typ := n.GetType(); typ != nil {
			return typ.GetTypeKind()
		}
	}
	return ssa.AnyTypeKind
}

func (v *Value) GetRange() *memedit.Range {
	if v.IsNil() {
		return nil
	}
	inst := v.getInstruction()
	return inst.GetRange()
}

func (v *Value) HasOperands() bool {
	if v.IsNil() {
		return false
	}
	return v.innerUser.HasValues()
}

func (v *Value) GetOperands() Values {
	if v.IsNil() {
		return nil
	}
	if v.operands == nil {
		v.operands = lo.FilterMap(v.innerUser.GetValues(), func(ssaVal ssa.Value, _ int) (*Value, bool) {
			return v.NewValue(ssaVal), true
		})
	}
	return v.operands
}

func (v *Value) GetOperand(index int) *Value {
	if v.IsNil() {
		return nil
	}

	opts := v.GetOperands()
	if index >= len(opts) {
		return nil
	}
	return opts[index]
}

func (v *Value) HasUsers() bool {
	if v.IsNil(true) {
		return false
	}
	return v.innerValue.HasUsers()
}

func (v *Value) GetUsers() Values {
	if v.IsNil(true) {
		return nil
	}

	appendUser := func(node ssa.Value) {
		if node == nil {
			log.Warnf("ssaapi.Value appendUser: node is nil")
			return
		}
		v.users = append(v.users,
			lo.FilterMap(node.GetUsers(), func(user ssa.User, _ int) (*Value, bool) {
				return v.NewValue(user), true
			},
			)...)
	}

	if v.users == nil {
		appendUser(v.innerValue)
		for _, reference := range v.innerValue.GetPointer() {
			appendUser(reference)
		}
	}
	return v.users
}

func (v *Value) GetUser(index int) *Value {
	if v.IsNil() {
		return nil
	}
	users := v.GetUsers()
	if index >= len(users) {
		return nil
	}
	return users[index]
}

func (v *Value) GetPointer() Values {
	if v.IsNil() {
		return nil
	}

	pointerIF, ok := v.getInstruction().(ssa.PointerIF)
	if !ok {
		return nil
	}
	return lo.FilterMap(pointerIF.GetPointer(), func(item ssa.Value, index int) (*Value, bool) {
		return v.NewValue(item), true
	})
}

func (v *Value) GetReference() *Value {
	if v.IsNil() {
		return nil
	}

	pointerIF, ok := v.getInstruction().(ssa.PointerIF)
	if !ok {
		return nil
	}
	return v.NewValue(pointerIF.GetReference())
}

func (v *Value) GetMask() Values {
	if v.IsNil() {
		return nil
	}

	maskIF, ok := v.getInstruction().(ssa.Maskable)
	if !ok {
		return nil
	}
	return lo.FilterMap(maskIF.GetMask(), func(item ssa.Value, index int) (*Value, bool) {
		return v.NewValue(item), true
	})
}

func (v *Value) ShowUseDefChain() {
	if v.IsNil() {
		return
	}
	showUseDefChain(v)
}

// for variable
func (v *Value) GetVariable(name string) *ssa.Variable {
	if v.IsNil() {
		return nil
	}

	assignAble, ok := v.getInstruction().(ssa.AssignAble)
	if !ok {
		return nil
	}
	return assignAble.GetVariable(name)
}

func (v *Value) GetAllVariables() map[string]*ssa.Variable {
	if v.IsNil() {
		return nil
	}

	assignAble, ok := v.getInstruction().(ssa.AssignAble)
	if !ok {
		return nil
	}
	return assignAble.GetAllVariables()
}

// for function

func (v *Value) GetReturn() Values {
	if v.IsNil(true) {
		return nil
	}

	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.innerValue); ok {
		for _, r := range f.Return {
			r, ok := f.GetValueById(r)
			if !ok {
				continue
			}
			ret = append(ret, v.NewValue(r))
		}
	}
	return ret
}

func (v *Value) GetParameter(i int) *Value {
	if v.IsNil(true) {
		return nil
	}

	if f, ok := ssa.ToFunction(v.innerValue); ok {
		if i < len(f.Params) {
			param, ok := f.GetValueById(f.Params[i])
			if !ok {
				return nil
			}
			return v.NewValue(param)
		}
	}
	return nil
}
func (v *Value) GetFreeValue(name string) *Value {
	if v.IsNil(true) {
		return nil
	}
	if variable := v.GetVariable(name); variable != nil {
		if f, ok := ssa.ToFunction(v.innerValue); ok {
			if fv, ok := f.FreeValues[variable]; ok {
				fv, ok := f.GetValueById(fv)
				if !ok {
					return nil
				}
				return v.NewValue(fv)
			}
		}
	}
	return nil
}

func (v *Value) GetParameters() Values {
	if v.IsNil(true) {
		return nil
	}

	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.innerValue); ok {
		for _, param := range f.Params {
			param, ok := f.GetValueById(param)
			if !ok {
				continue
			}
			ret = append(ret, v.NewValue(param))
		}
	}
	return ret
}

func (v *Value) GetCallArgs() Values {
	if v.IsNil(true) {
		return nil
	}

	if f, ok := ssa.ToCall(v.innerValue); ok {
		return lo.FilterMap(f.Args, func(itemId int64, index int) (*Value, bool) {
			item, ok := f.GetValueById(itemId)
			if !ok {
				return nil, false
			}
			return v.NewValue(item), true
		})
	}
	return nil
}

func (v *Value) GetCallReturns() Values {
	if v.IsNil(true) {
		return nil
	}

	return v.GetUsers()
}

// for const instruction
func (v *Value) GetConstValue() any {
	if v.IsNil(true) {
		return nil
	}

	if cInst, ok := ssa.ToConstInst(v.innerValue); ok {
		return cInst.GetRawValue()
	} else {
		return nil
	}
}

func (v *Value) GetConst() *ssa.ConstInst {
	if v.IsNil(true) {
		return nil
	}

	if cInst, ok := ssa.ToConstInst(v.innerValue); ok {
		return cInst
	} else {
		return nil
	}
}

func (v *Value) IsModifySelf() bool {
	if v.IsNil() {
		return false
	}

	if !v.IsCall() {
		return false
	}
	ft, ok := ssa.ToFunctionType(GetBareType(v.GetOperand(0).GetType()))
	return ok && ft.IsModifySelf
}

func (v *Value) GetSelf() *Value {
	if v.IsNil() {
		return nil
	}

	if v.IsModifySelf() {
		return v.GetOperand(1)
	}
	return v
}

// // Instruction Opcode
func (v *Value) getOpcode() ssa.Opcode {
	if v.IsNil() {
		return ssa.SSAOpcodeUnKnow
	}

	return v.getInstruction().GetOpcode()
}

// IsExternLib desc if the value is extern lib
//
// extern-lib is a special value that is used to represent the external library
/*
	code := `a = fmt.Println`
	fmt := prog.Ref("fmt") // extern-lib
	fmt.GetOperands() // Values // [Function-Println]
*/
func (v *Value) IsExternLib() bool       { return v.getOpcode() == ssa.SSAOpcodeExternLib }
func (v *Value) IsFunction() bool        { return v.getOpcode() == ssa.SSAOpcodeFunction }
func (v *Value) IsBasicBlock() bool      { return v.getOpcode() == ssa.SSAOpcodeBasicBlock }
func (v *Value) IsParameter() bool       { return v.getOpcode() == ssa.SSAOpcodeParameter }
func (v *Value) IsSideEffect() bool      { return v.getOpcode() == ssa.SSAOpcodeSideEffect }
func (v *Value) IsParameterMember() bool { return v.getOpcode() == ssa.SSAOpcodeParameterMember }
func (v *Value) IsPhi() bool             { return v.getOpcode() == ssa.SSAOpcodePhi }
func (v *Value) IsConstInst() bool       { return v.getOpcode() == ssa.SSAOpcodeConstInst }
func (v *Value) IsUndefined() bool       { return v.getOpcode() == ssa.SSAOpcodeUndefined }
func (v *Value) IsBinOp() bool           { return v.getOpcode() == ssa.SSAOpcodeBinOp }
func (v *Value) IsUnOp() bool            { return v.getOpcode() == ssa.SSAOpcodeUnOp }
func (v *Value) IsCall() bool            { return v.getOpcode() == ssa.SSAOpcodeCall }
func (v *Value) IsReturn() bool          { return v.getOpcode() == ssa.SSAOpcodeReturn }
func (v *Value) IsMake() bool            { return v.getOpcode() == ssa.SSAOpcodeMake }
func (v *Value) IsNext() bool            { return v.getOpcode() == ssa.SSAOpcodeNext }
func (v *Value) IsAssert() bool          { return v.getOpcode() == ssa.SSAOpcodeAssert }
func (v *Value) IsTypeCast() bool        { return v.getOpcode() == ssa.SSAOpcodeTypeCast }
func (v *Value) IsTypeValue() bool       { return v.getOpcode() == ssa.SSAOpcodeTypeValue }
func (v *Value) IsErrorHandler() bool    { return v.getOpcode() == ssa.SSAOpcodeErrorHandler }
func (v *Value) IsPanic() bool           { return v.getOpcode() == ssa.SSAOpcodePanic }
func (v *Value) IsRecover() bool         { return v.getOpcode() == ssa.SSAOpcodeRecover }
func (v *Value) IsJump() bool            { return v.getOpcode() == ssa.SSAOpcodeJump }
func (v *Value) IsIf() bool              { return v.getOpcode() == ssa.SSAOpcodeIf }
func (v *Value) IsLoop() bool            { return v.getOpcode() == ssa.SSAOpcodeLoop }
func (v *Value) IsSwitch() bool          { return v.getOpcode() == ssa.SSAOpcodeSwitch }
func (v *Value) IsLazy() bool {
	if v.IsNil() {
		return false
	}
	return v.getInstruction().IsLazy()
}

// // MemberCall : Object

// IsObject desc if the value is object
func (v *Value) IsObject() bool {
	if v.IsNil() {
		return false
	}

	objectIF, ok := v.getInstruction().(ssa.MemberCall)
	if !ok {
		return false
	}
	return objectIF.IsObject()
}

func (v *Value) IsExtern() bool {
	if v.IsNil() {
		return false
	}

	return v.getInstruction().IsExtern()
}

func (v *Value) IsFreeValue() bool {
	if v.IsNil() {
		return false
	}

	if f, ok := ssa.ToFreeValue(v.innerValue); ok && f.IsFreeValue {
		return true
	}
	return false
}

// GetMember get member of object by key
func (v *Value) GetMember(value *Value) []*Value {
	var ret []*Value
	if v.IsNil() {
		return nil
	}

	// TODO: key is string or int
	key := value.innerValue.String()
	node := v.innerValue
	for name, member := range node.GetAllMember() {
		if name.String() == key {
			ret = append(ret, v.NewValue(member))
		}
	}
	return ret
}

// GetAllMember get all member of object
func (v *Value) GetAllMember() Values {
	if v.IsNil() {
		return nil
	}

	all := v.innerValue.GetAllMember()
	ret := make(Values, 0, len(all))
	for _, value := range all {
		ret = append(ret, v.NewValue(value))
	}
	return ret
}

// GetAllMember get member keys and values
func (v *Value) GetMembers() [][]*Value {
	if v.IsNil() {
		return nil
	}
	all := v.innerValue.GetAllMember()
	ret := make([][]*Value, 0, len(all))
	for key, value := range all {
		ret = append(ret, []*Value{v.NewValue(key), v.NewValue(value)})
	}
	return ret
}

// // MemberCall : member

func (v *Value) IsMethod() bool {
	if v.IsNil() {
		return false
	}

	f, ok := ssa.ToFunctionType(v.innerValue.GetType())
	if !ok {
		return false
	}
	return f.IsMethod
}

func (v *Value) GetFunctionObjectType() ssa.Type {
	if v.IsNil() {
		return nil
	}

	ft, ok := ssa.ToFunctionType(GetBareType(v.GetType()))
	if !ok {
		return nil
	}
	return ft.ObjectType
}

// IsMember desc if the value is member of some object
func (v *Value) IsMember() bool {
	if v.IsNil() || v.innerValue == nil {
		return false
	}
	return v.innerValue.IsMember()
}

// GetObject get object of member
func (v *Value) GetObject() *Value {
	if v.IsNil() || v.innerValue == nil {
		return nil
	}
	obj := v.innerValue.GetObject()
	if obj == nil {
		return nil
	}

	return v.NewValue(obj)
}

// GetKey get key of member
func (v *Value) GetKey() *Value {
	if v.IsNil() || v.innerValue == nil {
		return nil
	}

	if v.innerValue.GetKey() == nil {
		return nil
	}
	return v.NewValue(v.innerValue.GetKey())
}

func GetValues(v *Value) Values {
	if v.IsNil() || v.innerValue == nil {
		return nil
	}

	return lo.Map(v.innerValue.GetValues(), func(item ssa.Value, _ int) *Value { return v.NewValue(item) })
}

func GetFreeValue(v *Value) *ssa.Parameter {
	if v.IsNil() || v.innerValue == nil {
		return nil
	}

	if f, ok := ssa.ToFreeValue(v.innerValue); ok && f.IsFreeValue {
		return f
	}
	return nil
}

// IsCalled desc any of 'Users' is Call
func (v *Value) IsCalled() bool {
	if v.IsNil() {
		return false
	}

	return len(v.GetUsers().Filter(func(value *Value) bool {
		called := value.IsCall()
		return called
	})) > 0
}

func (v *Value) GetCalledBy() Values {
	if v.IsNil() {
		return nil
	}
	return v.getCallByEx(make(map[int64]struct{}))
}
func (v *Value) getCallByEx(tmp map[int64]struct{}) Values {
	if v.IsNil() {
		return nil
	}
	vs := make(Values, 0)
	_, ok := tmp[v.GetId()]
	if ok {
		return vs
	}
	tmp[v.GetId()] = struct{}{}
	check := func(val ssa.Value, id int64) {
		switch ret := val.(type) {
		case *ssa.Phi, *ssa.SideEffect:
			vs = append(vs, v.NewValue(ret).getCallByEx(tmp)...)
		case *ssa.Call:
			call := ret
			if call == nil || call.Method <= 0 {
				return
			}
			method, ok := call.GetValueById(call.Method)
			if !ok || method == nil {
				return
			}
			if method.GetId() == id {
				vs = append(vs, v.NewValue(call))
				return
			}
			function, ok := ssa.ToFunction(method)
			if !ok {
				reference := method.GetReference()
				if refFunction, b := ssa.ToFunction(reference); b {
					function = refFunction
				} else {
					return
				}
			}
			for index, valueId := range call.ArgMember {
				if len(function.ParameterMembers) <= index {
					break
				}
				value, ok := call.GetValueById(valueId)
				if !ok || value == nil {
					continue
				}
				if value.GetId() == id {
					paramMember, ok := call.GetValueById(function.ParameterMembers[index])
					if !ok {
						continue
					}
					vs = append(vs, v.NewValue(paramMember).getCallByEx(tmp)...)
					return
				}
			}
			for index, arg := range call.Args {
				if len(function.Params) <= index {
					break
				}
				if arg == id {
					param, ok := call.GetValueById(function.Params[index])
					if !ok {
						continue
					}
					vv := v.NewValue(param)
					vs = append(vs, vv.getCallByEx(tmp)...)
					return
				}
			}
			searchBindVariable := func(name string) {
				for _, valueId := range function.FreeValues {
					value, ok := call.GetValueById(valueId)
					if !ok || value == nil {
						continue
					}
					if value.GetName() == name {
						vs = append(vs, v.NewValue(value).getCallByEx(tmp)...)
						return
					}
				}
			}
			for name, valueId := range call.Binding {
				if valueId == id {
					searchBindVariable(name)
					return
				}
			}
		default:
			log.Errorf("unknown type %T", ret)
		}
	}
	checkUser := func(user ssa.User) (ssa.Value, bool) {
		call, isCall := ssa.ToCall(user)
		if isCall {
			return call, true
		}
		phi, isPhi := ssa.ToPhi(user)
		if isPhi {
			return phi, true
		}
		effect, isSideEffect := ssa.ToSideEffect(user)
		if isSideEffect {
			return effect, true
		}
		return nil, false
	}
	addCall := func(node ssa.Value) {
		nodeId := node.GetId()
		for _, user := range node.GetUsers() {
			value, ok := checkUser(user)
			if !ok {
				continue
			}
			check(value, nodeId)
		}
	}
	handler := func(node ssa.Value) {
		if node == nil {
			return
		}
		addCall(node)
		for _, pointer := range node.GetPointer() {
			addCall(pointer)
		}
	}
	handler(v.innerValue)
	if v.IsFunction() {
		/*
			function's reference, like parent-class same name function
			this function over-write parent-class function

			weakLanguagePoint use this eg: $a()
		*/
		handler(v.innerValue.GetReference())
	}
	return vs
}

// GetCallee desc any of 'Users' is Call
// return the first Call, if `risk.New()`, GetCallee will return `risk.New`.
func (v *Value) GetCallee() *Value {
	if v.IsNil() {
		return nil
	}

	if v.IsCall() {
		return v.GetOperand(0)
	}
	return nil
}

func (v *Value) GetPredecessors() []*PredecessorValue {
	if len(v.Predecessors) == 0 {
		if auditNode := v.auditNode; auditNode != nil {
			edges := ssadb.GetPredecessorEdgeByFromID(auditNode.ID)
			var preds []*PredecessorValue
			for _, edge := range edges {
				p := v.NewValueFromAuditNode(uint(edge.ToNode))
				if p != nil {
					preds = append(preds, &PredecessorValue{
						Node: p,
						Info: &sfvm.AnalysisContext{
							Step:  int(edge.AnalysisStep),
							Label: edge.AnalysisLabel,
						},
					})
				}
			}
			v.Predecessors = preds
		}
	}
	return v.Predecessors
}

func (v *Value) GetDependOn() Values {
	if v.DependOn == nil || v.DependOn.Count() == 0 {
		if auditNode := v.auditNode; auditNode != nil {
			nodeIds := ssadb.GetDependEdgeOnByFromNodeId(auditNode.ID)
			if v.DependOn == nil {
				v.DependOn = utils.NewSafeMapWithKey[string, *Value]()
			}
			for _, id := range nodeIds {
				d := v.NewValueFromAuditNode(id)
				if d != nil {
					v.setDependOn(d)
				}
			}
		}
	}
	return v.safeMapToValues(v.DependOn)
}

func (v *Value) GetDataFlow() Values {
	if len(v.PrevDataFlow) == 0 {
		// load from db
		if auditNode := v.auditNode; auditNode != nil {
			nodeIds := ssadb.GetDataFlowEdgeByToNodeId(auditNode.ID)
			for _, id := range nodeIds {
				d := v.NewValueFromAuditNode(id)
				if d != nil {
					v.PrevDataFlow = append(v.PrevDataFlow, d)
				}
			}
		}
	}
	return v.PrevDataFlow
}

func (v *Value) GetEffectOn() Values {
	if v.EffectOn == nil || v.EffectOn.Count() == 0 {
		if auditNode := v.auditNode; auditNode != nil {
			nodeIds := ssadb.GetEffectOnEdgeByFromNodeId(auditNode.ID)
			if v.EffectOn == nil {
				v.EffectOn = utils.NewSafeMapWithKey[string, *Value]()
			}
			for _, id := range nodeIds {
				e := v.NewValueFromAuditNode(id)
				if e != nil {
					v.EffectOn.Set(e.GetUUID(), e)
				}
			}
		}
	}
	return v.safeMapToValues(v.EffectOn)
}

func (v *Value) safeMapToValues(safeMap *utils.SafeMapWithKey[string, *Value]) Values {
	if safeMap == nil {
		return Values{}
	}

	var result Values
	safeMap.ForEach(func(key string, value *Value) bool {
		result = append(result, value)
		return true
	})
	return result
}

func (v *Value) ForEachDependOn(f func(value *Value)) {
	if v == nil || v.DependOn == nil {
		return
	}
	v.DependOn.ForEach(func(key string, value *Value) bool {
		f(value)
		return true
	})
}

func (v *Value) ForEachEffectOn(f func(value *Value)) {
	if v == nil || v.EffectOn == nil {
		return
	}
	v.EffectOn.ForEach(func(key string, value *Value) bool {
		f(value)
		return true
	})
}

func (v *Value) AnalyzeDepth() int {
	return v.GetDepth()
}

type Values []*Value

func (value Values) Hash() (string, bool) {
	var retIds []int64
	haveNil := false
	value.ForEach(func(value *Value) {
		id := value.GetId()
		if id == -1 {
			haveNil = true
			return
		}
		retIds = append(retIds, value.GetId())
	})
	if haveNil {
		return "", false
	}
	return utils.CalcSha256(retIds), true
}

func (value Values) Ref(name string) Values {
	ret := make(Values, 0, len(value))
	for _, v := range value {
		v.GetAllMember().ForEach(func(v *Value) {
			if v.GetKey().innerValue != nil {
				if v.GetKey().innerValue.String() == name {
					ret = append(ret, v)
				}
			}
		})
	}
	return ret
}

func (v Values) StringEx(flag int) string {
	if len(v) <= 0 {
		return "Values: 0"
	}

	ret := ""
	ret += fmt.Sprintf("Values: %d\n", len(v))
	for i, v := range v {
		switch flag {
		case 0:
			ret += fmt.Sprintf("\t%d (t%d): %s\n", i, v.GetId(), v)
		case 1:
			ret += fmt.Sprintf("\t%d (t%d): %s\n", i, v.GetId(), v.StringWithRange())
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
	return v.NewValue(ssa.NewUndefined(""))
}

func (v Values) GetBySyntaxFlowName(name string) Values {
	return lo.Filter(v, func(v *Value, i int) bool {
		if utils.StringSliceContain(v.syntaxFlowName, name) {
			return true
		}
		return false
	})
}

func (v Values) NewValue(ssaVal ssa.Instruction) *Value {
	if len(v) > 0 {
		return v[0].NewValue(ssaVal)
	}
	return nil
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
func (v *Value) ShowDot() {
	dotStr := v.DotGraph()
	fmt.Println(dotStr)
	// dot.ShowDotGraphToAsciiArt(dotStr)
}

func (vs Values) ShowDot() Values {
	dotStr := vs.DotGraph()
	fmt.Println(dotStr)
	// dot.ShowDotGraphToAsciiArt(dotStr)
	return vs
}

func (v *Value) DotGraph() string {
	dotGraph := v.NewDotGraph()
	return dotGraph.String()
}

func (v *Value) NewDotGraph() *DotGraph {
	dotGraph := NewDotGraph()
	dotGraph.createNode(v, true)
	v.GenerateGraph(dotGraph)
	return dotGraph
}
func (vs Values) NewDotGraph() *DotGraph {
	dotGraph := NewDotGraph()
	for _, v := range vs {
		v.GenerateGraph(dotGraph)
	}
	return dotGraph
}
func (vs Values) DotGraph() string {
	dotGraph := vs.NewDotGraph()
	return dotGraph.String()
}
