package ssaapi

import (
	"fmt"

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
	EffectOn      Values // this value effect current value     [effectOn -> self]
	DependOn      Values // this value depend on current value  [self -> dependOn]

	node ssa.Value
	// cache
	disasmLine string
	users      Values
	operands   Values

	// for debug
	syntaxFlowName []string

	// for syntaxflow vm
	Predecessors []*PredecessorValue
	DescInfo     map[string]string
	// value from database
	auditNode *ssadb.AuditNode
}

type PredecessorValue struct {
	Node *Value
	Info *sfvm.AnalysisContext
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
	v1 := v1raw.node
	v2 := v2raw.node
	if v1 == nil || v2 == nil {
		return v1 == v2
	}

	same := func(v1, v2 ssa.Value) bool {
		for _, v := range v1.GetPointer() {
			if v.GetId() == v2.GetId() {
				return true
			}
		}
		return v1.GetId() == v2.GetId()
	}
	res := same(v1, v2) || same(v2, v1)
	if v1Ref := v1.GetReference(); v1Ref != nil {
		res = res || same(v1Ref, v2) || same(v2, v1Ref)
	}
	if v2Ref := v2.GetReference(); v2Ref != nil {
		res = res || same(v1, v2Ref) || same(v2Ref, v1)
	}
	return res
}

func (v *Value) GetProgramName() string {
	if v.IsNil() {
		return ""
	}
	if prog := v.node.GetProgram(); prog != nil {
		return prog.GetProgramName()
	}
	if prog := v.ParentProgram; prog != nil {
		if prog.Program != nil {
			return prog.Program.GetProgramName()
		}
	}
	return ""
}

func (v *Value) IsNil() bool {
	return v == nil || v.node == nil
}

func (v *Value) GetId() int64 {
	if v.IsNil() {
		return -1
	}
	return v.node.GetId()
}

func (v *Value) GetSSAValue() ssa.Value {
	if v.IsNil() {
		return nil
	}
	return v.node
}

func (v *Value) NewError(tag, msg string) {
	if v.IsNil() {
		return
	}
	v.node.NewError(ssa.Error, ssa.ErrorTag(tag), msg)
}

func (v *Value) NewWarn(tag, msg string) {
	if v.IsNil() {
		return
	}
	v.node.NewError(ssa.Warn, ssa.ErrorTag(tag), msg)
}

func (v *Value) String() string {
	if v.IsNil() {
		return ""
	}
	return ssa.LineDisasm(v.node)
}

func (v *Value) ShortString() string {
	if v.IsNil() {
		return ""
	}
	return ssa.LineShortDisasm(v.node)
}

func (v *Value) StringWithRange() string {
	if v.IsNil() {
		return ""
	}

	if v.disasmLine == "" {
		v.disasmLine = fmt.Sprintf("[%-6s] %s\t%s", ssa.SSAOpcode2Name[v.node.GetOpcode()], ssa.LineDisasm(v.node), v.node.GetRange())
	}
	return v.disasmLine
}

func (v *Value) StringWithSourceCode(msg ...string) string {
	if v.IsNil() {
		return ""
	}

	if v.disasmLine == "" {
		v.disasmLine = fmt.Sprintf("[%-6s] %s\t%s", ssa.SSAOpcode2Name[v.node.GetOpcode()], ssa.LineDisasm(v.node), v.node.GetRange())
		v.disasmLine += "\n" + v.GetRange().GetTextContextWithPrompt(2, msg...)
	}
	return v.disasmLine
}

func (v *Value) GetName() string {
	if v.IsNil() {
		return ""
	}
	return v.node.GetName()
}

func (v *Value) GetVerboseName() string {
	if v.IsNil() {
		return ""
	}
	var name string
	if name = v.node.GetName(); name != "" {
		if v.IsPhi() {
			return "[phi]: " + name
		}
		return name
	} else if name = v.node.GetVerboseName(); name != "" {
		return fmt.Sprintf(`t%d: %v=%v`, v.GetId(), name, v.ShortString())
	}
	return fmt.Sprintf(`t%d: %v`, v.GetId(), v.ShortString())
}

func (i *Value) Show() *Value               { fmt.Println(i); return i }
func (i *Value) ShowWithRange() *Value      { fmt.Println(i.StringWithRange()); return i }
func (i *Value) ShowWithSourceCode() *Value { fmt.Println(i.StringWithSourceCode()); return i }

func (v *Value) Compare(other *Value) bool { return ValueCompare(v, other) }

func (v *Value) GetType() *Type {
	if v.IsNil() {
		return Any
	}
	if n, ok := v.node.(ssa.Typed); ok {
		return NewType(n.GetType())
	}
	return Any
}

func (v *Value) GetTypeKind() ssa.TypeKind {
	if v.IsNil() {
		return ssa.AnyTypeKind
	}
	if n, ok := v.node.(ssa.Typed); ok {
		if typ := n.GetType(); typ != nil {
			return typ.GetTypeKind()
		}
	}
	return ssa.AnyTypeKind
}

func (v *Value) GetRange() memedit.RangeIf {
	if v.IsNil() {
		return nil
	}
	return v.node.GetRange()
}

func (v *Value) HasOperands() bool {
	if v.IsNil() {
		return false
	}
	return v.node.HasValues()
}

func (v *Value) GetOperands() Values {
	if v.IsNil() {
		return nil
	}
	if v.operands == nil {
		v.operands = lo.Map(v.node.GetValues(), func(ssaVal ssa.Value, _ int) *Value { return v.NewValue(ssaVal) })
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
	if v.IsNil() {
		return false
	}
	return v.node.HasUsers()
}

func (v *Value) GetUsers() Values {
	if v.IsNil() {
		return nil
	}

	appendUser := func(node ssa.Value) {
		v.users = append(v.users,
			lo.FilterMap(node.GetUsers(), func(ssaVal ssa.User, _ int) (*Value, bool) {
				if value, ok := ssa.ToValue(ssaVal); ok {
					return v.NewValue(value), true
				}
				return nil, false
			},
			)...)
	}

	if v.users == nil {
		appendUser(v.node)
		for _, reference := range v.node.GetPointer() {
			appendUser(reference)
		}
	}
	return v.users
}

func (v *Value) GetPointer() Values {
	if v.IsNil() {
		return nil
	}

	return lo.Map(v.node.GetPointer(), func(item ssa.Value, index int) *Value {
		return v.NewValue(item)
	})
}

func (v *Value) GetMask() Values {
	if v.IsNil() {
		return nil
	}

	return lo.Map(v.node.GetMask(), func(item ssa.Value, index int) *Value {
		return v.NewValue(item)
	})
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

	return v.node.GetVariable(name)
}

func (v *Value) GetAllVariables() map[string]*ssa.Variable {
	if v.IsNil() {
		return nil
	}

	return v.node.GetAllVariables()
}

// for function

func (v *Value) GetReturn() Values {
	if v.IsNil() {
		return nil
	}

	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, r := range f.Return {
			ret = append(ret, v.NewValue(r))
		}
	}
	return ret
}

func (v *Value) GetParameter(i int) *Value {
	if v.IsNil() {
		return nil
	}

	if f, ok := ssa.ToFunction(v.node); ok {
		if i < len(f.Params) {
			return v.NewValue(f.Params[i])
		}
	}
	return nil
}
func (v *Value) GetFreeValue(name string) *Value {
	if v.IsNil() {
		return nil
	}
	if variable := v.GetVariable(name); variable != nil {
		if f, ok := ssa.ToFunction(v.node); ok {
			if fv, ok := f.FreeValues[variable]; ok {
				return v.NewValue(fv)
			}
		}
	}
	return nil
}

func (v *Value) GetParameters() Values {
	if v.IsNil() {
		return nil
	}

	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, param := range f.Params {
			ret = append(ret, v.NewValue(param))
		}
	}
	return ret
}

func (v *Value) GetCallArgs() Values {
	if v.IsNil() {
		return nil
	}

	if f, ok := ssa.ToCall(v.node); ok {
		return lo.Map(f.Args, func(item ssa.Value, index int) *Value {
			return v.NewValue(item)
		})
	}
	return nil
}

func (v *Value) GetCallReturns() Values {
	if v.IsNil() {
		return nil
	}

	return v.GetUsers()
}

// for const instruction
func (v *Value) GetConstValue() any {
	if v.IsNil() {
		return nil
	}

	if v == nil || v.node == nil {
		return nil
	}
	if cInst, ok := ssa.ToConst(v.node); ok {
		return cInst.GetRawValue()
	} else {
		return nil
	}
}

func (v *Value) GetConst() *ssa.Const {
	if v.IsNil() {
		return nil
	}

	if cInst, ok := ssa.ToConst(v.node); ok {
		return cInst.Const
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
	if v != nil && v.node != nil {
		return v.node.IsLazy()
	}
	return false
}

// // MemberCall : Object

// IsObject desc if the value is object
func (v *Value) IsObject() bool {
	if v.IsNil() {
		return false
	}

	return v.node.IsObject()
}

func (v *Value) IsExtern() bool {
	if v.IsNil() {
		return false
	}

	return v.node.IsExtern()
}

func (v *Value) IsFreeValue() bool {
	if v.IsNil() {
		return false
	}

	if f, ok := ssa.ToFreeValue(v.node); ok && f.IsFreeValue {
		return true
	}
	return false
}

// GetMember get member of object by key
func (v *Value) GetMember(value *Value) *Value {
	if v.IsNil() {
		return nil
	}

	// TODO: key is string or int
	key := value.node.String()
	node := v.node
	for name, member := range node.GetAllMember() {
		if name.String() == key {
			return v.NewValue(member)
		}
	}
	return nil
}

// GetAllMember get all member of object
func (v *Value) GetAllMember() Values {
	if v.IsNil() {
		return nil
	}

	all := v.node.GetAllMember()
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
	all := v.node.GetAllMember()
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

	f, ok := ssa.ToFunctionType(v.node.GetType())
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
	if v.IsNil() {
		return false
	}
	return v.node.IsMember()
}

// GetObject get object of member
func (v *Value) GetObject() *Value {
	if v.IsNil() {
		return nil
	}

	return v.NewValue(v.node.GetObject())
}

// GetKey get key of member
func (v *Value) GetKey() *Value {
	if v.IsNil() {
		return nil
	}

	return v.NewValue(v.node.GetKey())
}

// GetBareNode get ssa.Value from ssaapi.Value
// only use this function in golang
func GetBareNode(v *Value) ssa.Value {
	if v.IsNil() {
		return nil
	}

	return v.node
}

func GetValues(v *Value) Values {
	if v.IsNil() {
		return nil
	}

	return lo.Map(v.node.GetValues(), func(item ssa.Value, _ int) *Value { return v.NewValue(item) })
}

func GetFreeValue(v *Value) *ssa.Parameter {
	if v.IsNil() {
		return nil
	}

	if f, ok := ssa.ToFreeValue(v.node); ok && f.IsFreeValue {
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

// GetCalledBy desc all of 'Users' is Call
func (v *Value) GetCalledBy() Values {
	if v.IsNil() {
		return nil
	}
	vs := make(Values, 0)
	addCall := func(node ssa.Value) {
		if node == nil {
			return
		}
		nodeId := node.GetId()
		for _, user := range node.GetUsers() {
			call, ok := ssa.ToCall(user)
			if !ok {
				continue
			}
			if call == nil || call.Method == nil {
				continue
			}
			if call.Method.GetId() == nodeId {
				vs = append(vs, v.NewValue(call))
			}
		}
	}

	handler := func(node ssa.Value) {
		if node == nil {
			return
		}
		// self
		addCall(node)
		for _, pointer := range node.GetPointer() {
			addCall(pointer)
		}
	}
	// handler self
	handler(v.node)
	if v.IsFunction() {
		// function's reference, like parent-class same name function
		// this function over-write parent-class function
		handler(v.node.GetReference())
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
			for _, edges := range edges {
				p := v.NewValueFromAuditNode(uint(edges.ToNode))
				if p != nil {
					preds = append(preds, &PredecessorValue{
						Node: p,
						Info: &sfvm.AnalysisContext{
							Step:  int(edges.AnalysisStep),
							Label: edges.AnalysisLabel,
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
	if len(v.DependOn) == 0 {
		if auditNode := v.auditNode; auditNode != nil {
			nodeIds := ssadb.GetDependEdgeOnByFromNodeId(auditNode.ID)
			var dependOn Values
			for _, id := range nodeIds {
				d := v.NewValueFromAuditNode(id)
				if d != nil {
					dependOn = append(dependOn, d)
				}
			}
			v.DependOn = dependOn
		}
	}
	return v.DependOn
}

func (v *Value) GetEffectOn() Values {
	if len(v.EffectOn) == 0 {
		if auditNode := v.auditNode; auditNode != nil {
			nodeIds := ssadb.GetEffectOnEdgeByFromNodeId(auditNode.ID)
			var effectOn Values
			for _, id := range nodeIds {
				e := v.NewValueFromAuditNode(id)
				if e != nil {
					effectOn = append(effectOn, e)
				}
			}
			v.EffectOn = effectOn
		}
	}
	return v.EffectOn
}

func (v *Value) AnalyzeDepth() int {
	return v.GetDepth()
}

type Values []*Value

func (value Values) Ref(name string) Values {
	ret := make(Values, 0, len(value))
	for _, v := range value {
		v.GetAllMember().ForEach(func(v *Value) {
			if v.GetKey().node != nil {
				if v.GetKey().node.String() == name {
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

func (v Values) NewValue(ssaVal ssa.Value) *Value {
	if len(v) > 0 {
		return v[0].ParentProgram.NewValue(ssaVal)
	}
	return &Value{
		runtimeCtx: omap.NewEmptyOrderedMap[ContextID, *Value](),
		node:       ssa.NewUndefined(""),
	}
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

func (v Values) DotGraph() string {
	vg := NewValueGraph(v...)
	return vg.Dot()
}
