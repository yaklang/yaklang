package ssa

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type SideEffectKind string

const (
	NormalSideEffect  SideEffectKind = "NormalSideEffect"
	PointerSideEffect SideEffectKind = "PointerSideEffect"
)

func SwitchSideEffectKind(kind string) SideEffectKind {
	switch kind {
	case string(NormalSideEffect):
		return NormalSideEffect
	case string(PointerSideEffect):
		return PointerSideEffect
	}
	return NormalSideEffect
}

func (s *FunctionType) SetFreeValue(fv map[*Variable]*Parameter) {
	s.FreeValue = lo.MapToSlice(fv, func(name *Variable, para *Parameter) *Parameter { return para })
}

// FunctionSideEffect is a side-effect in a closure
type FunctionSideEffect struct {
	Name        string
	VerboseName string
	Modify      int64
	// only call-side Scope > this Scope-level, this side-effect can be create
	// Scope *Scope
	Variable *Variable

	forceCreate bool
	Kind        SideEffectKind

	*parameterMemberInner
}

// FunctionSideEffectSite is the scope-free form of one recorded side effect.
// It carries the same fields the AOT compiler needs to find the capture cell
// and the value to store, but holds no *Variable, so it can outlive the maps
// that Finish releases.
type FunctionSideEffectSite struct {
	Name         string
	VerboseName  string
	Modify       int64
	Kind         SideEffectKind
	VariableName string
}

func (f *Function) AddForceSideEffect(variable *Variable, value Value, index int, kind SideEffectKind) {
	defer f.markPersistenceDirty()
	if variable.IsMemberCall() {
		if para, ok := ToParameter(variable.object); ok {
			f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
				Name:                 variable.GetName(),
				VerboseName:          getMemberVerboseName(variable.object, variable.key),
				Modify:               value.GetId(),
				Variable:             variable,
				forceCreate:          false,
				Kind:                 kind,
				parameterMemberInner: newParameterMember(para, variable.key),
			})
		} else if param, ok := ToParameterMember(variable.object); ok {
			f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
				Name:                 variable.GetName(),
				VerboseName:          getMemberVerboseName(variable.object, variable.key),
				Modify:               value.GetId(),
				Variable:             variable,
				forceCreate:          false,
				Kind:                 kind,
				parameterMemberInner: newMoreParameterMember(param, variable.key),
			})
		}
	} else {
		f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
			Name:        variable.GetName(),
			VerboseName: variable.GetName(),
			Modify:      value.GetId(),
			Variable:    variable,
			forceCreate: true,
			Kind:        kind,
			parameterMemberInner: &parameterMemberInner{
				MemberCallKind:        ParameterCall,
				MemberCallObjectIndex: index,
			},
		})
	}
}

func (f *Function) AddSideEffect(variable *Variable, v Value) {
	defer f.markPersistenceDirty()
	var bind *Variable = variable

	// f.builder is nil for functions rebuilt from DB (compile-unit spill +
	// lazy reload) or extern functions; the parent-scope walk is compiler-only
	// bookkeeping, so just keep the original variable as bind.
	if f.builder != nil {
		// FunctionBuilder.parentBuilder stays valid after the parent's
		// Function.builder is cleared by a release; follow that link.
		for p := f.builder.parentBuilder; p != nil; p = p.parentBuilder {
			parentScope := p.CurrentBlock.ScopeTable
			if find := ReadVariableFromScopeAndParent(parentScope, variable.GetName()); find != nil {
				bind = find
				break
			}
		}
	}

	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        variable.GetName(),
		VerboseName: variable.GetName(),
		Modify:      v.GetId(),
		Variable:    bind,
		Kind:        NormalSideEffect,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind: NoMemberCall,
		},
	})
	if f.Type != nil {
		f.Type.resetStringCache()
	}
}

func (f *FunctionBuilder) CheckMemberSideEffect(variable *Variable, v Value) {
	defer f.Function.markPersistenceDirty()
	var bind *Variable = variable

	if f.builder == nil {
		return
	}
	// FunctionBuilder.parentBuilder stays valid after this function's
	// Function.builder is cleared by a release; follow that link.
	for p := f.parentBuilder; p != nil; p = p.parentBuilder {
		parentScope := p.CurrentBlock.ScopeTable
		if find := ReadVariableFromScopeAndParent(parentScope, variable.GetName()); find != nil {
			bind = find
			break
		} else if obj := variable.object; obj != nil {
			if find := ReadVariableFromScopeAndParent(parentScope, obj.GetName()); find != nil {
				bind = find
				break
			}
		}
	}

	if variable.IsMemberCall() {
		// if name is member call, it's modify parameter field
		if para, ok := ToParameter(variable.object); ok {
			sideEffect := &FunctionSideEffect{
				Name:                 variable.GetName(),
				VerboseName:          getMemberVerboseName(variable.object, variable.key),
				Modify:               v.GetId(),
				Variable:             bind,
				forceCreate:          false,
				Kind:                 NormalSideEffect,
				parameterMemberInner: newParameterMember(para, variable.key),
			}
			f.SideEffects = append(f.SideEffects, sideEffect)

			if f.MarkedThisObject != nil &&
				para.GetDefault() != nil &&
				f.MarkedThisObject.GetId() == para.GetDefault().GetId() {
				f.SetMethod(true, para.GetType())
			}
			return
		}

		if paramMember, ok := ToParameterMember(variable.object); ok {
			f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
				Name:                 variable.GetName(),
				VerboseName:          getMemberVerboseName(variable.object, variable.key),
				Modify:               v.GetId(),
				Variable:             bind,
				forceCreate:          false,
				Kind:                 NormalSideEffect,
				parameterMemberInner: newMoreParameterMember(paramMember, variable.key),
			})
		}
	}
}

func (s *FunctionType) SetSideEffect(se []*FunctionSideEffect) {
	s.SideEffects = se
	s.resetStringCache()
}

// getActualKeyFromCall 获取调用时的实际 key 值
// 如果 key 是一个 Parameter，则从调用参数中获取实际值
func getActualKeyFromCall(c *Call, key Value) Value {
	if param, ok := ToParameter(key); ok && !param.IsFreeValue {
		// 从 c.Args 中获取实际的参数值
		if param.FormalParameterIndex < len(c.Args) {
			if argVal, ok := c.GetValueById(c.Args[param.FormalParameterIndex]); ok && argVal != nil {
				return argVal
			}
		}
	}
	return key
}

// computeActualVerboseName 计算调用时的实际 VerboseName
// 如果 VerboseName 中包含参数名，则替换为实际的参数值
func computeActualVerboseName(c *Call, se *FunctionSideEffect) string {
	// 如果没有 MemberCallKey，直接返回原始 VerboseName
	if se.MemberCallKey <= 0 {
		return se.VerboseName
	}

	key, ok := c.GetValueById(se.MemberCallKey)
	if !ok || key == nil {
		return se.VerboseName
	}

	// 获取实际的 key 值
	actualKey := getActualKeyFromCall(c, key)
	if actualKey == key {
		// key 没有变化，返回原始 VerboseName
		return se.VerboseName
	}

	// 构造新的 VerboseName
	// 原始 VerboseName 格式: "objectName.keyName"
	// 新的 VerboseName 格式: "objectName.actualKeyValue"
	actualKeyStr := GetKeyString(actualKey)
	if actualKeyStr == "" {
		return se.VerboseName
	}

	// 找到最后一个 "." 并替换后面的部分
	if idx := strings.LastIndex(se.VerboseName, "."); idx != -1 {
		return se.VerboseName[:idx+1] + actualKeyStr
	}

	return se.VerboseName
}

func isCallerVisibleMemberSideEffectKind(kind ParameterMemberCallKind) bool {
	switch kind {
	case ParameterMemberCall, CallMemberCall:
		return true
	default:
		return false
	}
}

func shouldBindSideEffectInCurrentScope(se *FunctionSideEffect, lexicalVar, bindVariable *Variable) bool {
	if isCallerVisibleMemberSideEffectKind(se.MemberCallKind) {
		return true
	}
	if lexicalVar == nil || bindVariable == nil {
		return true
	}
	return lexicalVar.GetCaptured() == bindVariable.GetCaptured()
}

func handleSideEffect(c *Call, funcTyp *FunctionType, buildPointer bool) {
	defer c.markPersistenceDirty()
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	// Functions rebuilt from DB have no builder; side effects for them were
	// already handled at compile time and cannot be re-emitted here.
	if builder == nil {
		return
	}

	for _, se := range funcTyp.SideEffects {
		if se.Kind == NormalSideEffect && buildPointer {
			continue
		}

		modify, ok := c.GetValueById(se.Modify)
		if modify == nil {
			modify, _ = se.GetActualParam(c)
		}
		if !ok || utils.IsNil(modify) {
			continue
		}
		if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
			log.Warnf("[ssa.handleSideEffect] skip side effect %s: modify value missing block scope", se.Name)
			continue
		}
		if p, ok := ToParameter(modify); ok && !p.IsFreeValue {
			if p.FormalParameterIndex < len(c.Args) {
				id := c.Args[p.FormalParameterIndex]
				if id > 0 && se.Kind == PointerSideEffect {
					if v, ok := c.GetValueById(id); ok && !utils.IsNil(v) {
						modify = v
						if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
							log.Warnf("[ssa.handleSideEffect] skip pointer side effect %s: resolved modify value missing block", se.Name)
							continue
						}
					} else {
						log.Warnf("[ssa.handleSideEffect] skip pointer side effect %s: actual argument missing", se.Name)
						continue
					}
				}
			}
		}
		if pm, ok := ToParameterMember(modify); ok && se.Kind == PointerSideEffect {
			if v, ok := pm.GetActualCallParam(c); ok && !utils.IsNil(v) {
				modify = v
				if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
					log.Warnf("[ssa.handleSideEffect] skip pointer member side effect %s: resolved modify value missing block", se.Name)
					continue
				}
			}
		}

		var variable *Variable
		if _, ok := ToFunction(modify); ok {
			continue
		}
		modifyScope := modify.GetBlock().ScopeTable

		switch se.MemberCallKind {
		case NoMemberCall:
			if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
				if modifyScope.IsSameOrSubScope(ret.GetScope()) {
					continue
				}
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterCall:
			v, exists := se.GetActualParam(c)
			if !exists || utils.IsNil(v) {
				log.Warnf("[ssa.handleSideEffect] skip parameter side effect %s: actual param missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = v
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Warnf("[ssa.handleSideEffect] skip parameter side effect %s: modify value missing block scope", se.Name)
				continue
			}
			if v.GetType() != nil && v.GetType().GetTypeKind() == PointerKind {
				se.Name = builder.GetOriginPointerName(v)
				if se.Kind == PointerSideEffect &&
					!utils.IsNil(modify) && modify.GetType() != nil && modify.GetType().GetTypeKind() != PointerKind {
					if deref := builder.GetOriginValue(v); !utils.IsNil(deref) && deref.GetType() != nil && deref.GetType().GetTypeKind() == PointerKind {
						se.Name = builder.GetOriginPointerName(deref)
					}
				}
			} else {
				if v.GetName() != "" {
					se.Name = v.GetName()
				} else {
					if lva := v.GetLastVariable(); lva != nil {
						se.Name = lva.GetName()
					}
				}
			}
			if v := currentScope.ReadVariable(se.Name); v != nil {
				se.Variable = v.(*Variable)
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterMemberCall:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Warnf("[ssa.handleSideEffect] skip parameter member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Warnf("[ssa.handleSideEffect] skip parameter member %s: modify value missing block scope", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Warnf("[ssa.handleSideEffect] skip parameter member %s: member key missing", se.Name)
				continue
			}
		case CallMemberCall:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Warnf("[ssa.handleSideEffect] skip call member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Warnf("[ssa.handleSideEffect] skip call member %s: modify value missing block scope", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Warnf("[ssa.handleSideEffect] skip call member %s: member key missing", se.Name)
				continue
			}
		default:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Warnf("[ssa.handleSideEffect] skip default member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Warnf("[ssa.handleSideEffect] skip default member %s: modify value missing block scope", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Warnf("[ssa.handleSideEffect] skip default member %s: member key missing", se.Name)
				continue
			}
		}

		if variable == nil {
			log.Warnf("[ssa.handleSideEffect] skip side effect %s: variable creation failed", se.Name)
			continue
		}
		if variable == nil {
			log.Debugf("[ssa.handleSideEffectBind] skip side effect %s: variable creation failed", se.Name)
			continue
		}
		if se.Kind == PointerSideEffect && se.MemberCallKind == CallMemberCall {
			// 直接把实参绑定到成员变量上可以获得确定值（$this->a++ => 2）。
			// 但若该实参已经属于另一个对象（例如 $_GET[1] 是 $_GET 的成员），
			// 复用同一个 Value 会覆盖它的对象归属与 verbose name，破坏它自身的
			// topdef 走向；这类值仍交给 EmitSideEffect 生成独立节点。
			targetObj, _ := variable.GetMemberCall()
			reusable := !utils.IsNil(modify)
			if reusable {
				if owner, ok := GetLatestObjectKeyPair(modify); ok && !utils.IsNil(owner.Object) &&
					(utils.IsNil(targetObj) || owner.Object.GetId() != targetObj.GetId()) {
					// 该值已经明确属于别的对象（$_GET[1] 之于 $_GET）；
					// 但构造器形参（Parameter-*）在调用点被复用是预期行为。
					if _, isParam := ToParameter(modify); !isParam {
						reusable = false
					}
				}
			}
			if reusable {
				builder.AssignVariable(variable, modify)
				continue
			}
		}
		if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
			// TODO: handle side effect in loop scope,
			// will replace value in scope and create new phi
			sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
			if v := ReadVariableFromScopeAndParent(currentScope, se.Name); v != nil {
				variable.SetCaptured(v)
			}

			builder.AssignVariable(variable, sideEffect)

			// 计算实际的 VerboseName（使用调用时的实际参数值）
			actualVerboseName := computeActualVerboseName(c, se)
			if strings.Contains(actualVerboseName, "this") {
				sideEffect.SetVerboseName(actualVerboseName)
			}
			sideEffectId := sideEffect.GetId()
			if sideEffectId == -1 {
				log.Warnf("[ssa.handleSideEffect] skip side effect %s: side effect id is 0", se.Name)
				continue
			}
		}
	}
}

func handleSideEffectBind(c *Call, funcTyp *FunctionType) {
	defer c.markPersistenceDirty()
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	// Functions rebuilt from DB have no builder; binding side effects for them
	// were already handled at compile time and cannot be re-emitted here.
	if builder == nil {
		return
	}

	for _, se := range funcTyp.SideEffects {
		if se.Kind == PointerSideEffect {
			continue
		}

		modify, ok := c.GetValueById(se.Modify)
		if !ok || utils.IsNil(modify) {
			continue
		}
		if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
			log.Debugf("[ssa.handleSideEffectBind] skip side effect %s: modify value missing block scope", se.Name)
			continue
		}
		var variable, bindVariable *Variable
		var bindScope, modifyScope ScopeIF
		if se.Variable != nil {
			bindScope = se.Variable.GetScope()
			bindVariable = se.Variable
		} else {
			bindScope = currentScope
		}
		modifyScope = modify.GetBlock().ScopeTable
		_ = modifyScope
		_ = bindScope

		// is object
		switch se.MemberCallKind {
		case NoMemberCall:
			if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
				if modifyScope.IsSameOrSubScope(ret.GetScope()) {
					continue
				}
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterCall:
			v, exists := se.GetActualParam(c)
			if !exists || utils.IsNil(v) {
				log.Debugf("[ssa.handleSideEffectBind] skip parameter side effect %s: actual param missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = v
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Debugf("[ssa.handleSideEffectBind] skip parameter side effect %s: modify value missing block", se.Name)
				continue
			}
			if v.GetType() != nil && v.GetType().GetTypeKind() == PointerKind {
				se.Name = builder.GetOriginPointerName(v)
			} else {
				if v.GetName() != "" {
					se.Name = v.GetName()
				} else {
					if lva := v.GetLastVariable(); lva != nil {
						se.Name = lva.GetName()
					}
				}
			}
			if v := currentScope.ReadVariable(se.Name); v != nil {
				se.Variable = v.(*Variable)
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterMemberCall:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Debugf("[ssa.handleSideEffectBind] skip parameter member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Debugf("[ssa.handleSideEffectBind] skip parameter member %s: modify value missing block", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Debugf("[ssa.handleSideEffectBind] skip parameter member %s: key missing", se.Name)
				continue
			}
		case CallMemberCall:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Debugf("[ssa.handleSideEffectBind] skip call member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Debugf("[ssa.handleSideEffectBind] skip call member %s: modify value missing block", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Debugf("[ssa.handleSideEffectBind] skip call member %s: key missing", se.Name)
				continue
			}
			if p, ok := ToParameter(modify); ok {
				if len(c.Args) > p.FormalParameterIndex {
					if arg, ok := c.GetValueById(c.Args[p.FormalParameterIndex]); ok && arg != nil {
						Point(modify, arg)
					}
				}
			}
		default:
			obj, ok := se.GetActualParam(c)
			if !ok {
				log.Debugf("[ssa.handleSideEffectBind] skip default member %s: object missing", se.Name)
				continue
			}
			if utils.IsNil(modify) {
				modify = obj
			}
			if modify.GetBlock() == nil || modify.GetBlock().ScopeTable == nil {
				log.Debugf("[ssa.handleSideEffectBind] skip default member %s: modify value missing block", se.Name)
				continue
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			// is object
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				// 使用实际的 key 值（如果 key 是 Parameter，则从调用参数中获取实际值）
				actualKey := getActualKeyFromCall(c, key)
				variable = builder.CreateMemberCallVariable(obj, actualKey)
			} else {
				log.Debugf("[ssa.handleSideEffectBind] skip default member %s: key missing", se.Name)
				continue
			}
		}

		if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
			if builder.SupportClosure {
				if parentValue, ok := builder.getParentFunctionVariable(se.Name); ok && se.Variable != nil {
					// the ret variable should be FreeValue
					noteCapturedByClosure(parentValue, se.Name)
					para := builder.BuildFreeValueByVariable(se.Variable)
					para.SetDefault(parentValue)
					para.SetType(parentValue.GetType())
					parentValue.AddOccultation(para)
				}
			}

			// 计算实际的 VerboseName（使用调用时的实际参数值）
			actualVerboseName := computeActualVerboseName(c, se)

			assignSideEffectInCurrentScope := func() {
				// TODO: handle side effect in loop scope,
				// will replace value in scope and create new phi
				sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
				if se.Variable != nil {
					variable.SetCaptured(se.Variable)
				}
				builder.AssignVariable(variable, sideEffect)
				sideEffect.SetVerboseName(actualVerboseName)
				if c.SideEffectValue == nil {
					c.SideEffectValue = make(map[string]int64)
				}
				c.SideEffectValue[actualVerboseName] = sideEffect.GetId()
			}

			captureSideEffectForOuterScope := func() {
				err := variable.Assign(sideEffect)
				if err != nil {
					log.Warnf("BUG: variable.Assign error: %v", err)
					return
				}
				if strings.Contains(actualVerboseName, "this") {
					sideEffect.SetVerboseName(actualVerboseName)
				}
				currentScope.SetCapturedSideEffect(actualVerboseName, variable, se.Variable)

				function.SideEffects = append(function.SideEffects, se)
				function.markPersistenceDirty()
			}

			applySideEffectByScope := func(find *Variable) {
				if shouldBindSideEffectInCurrentScope(se, find, bindVariable) {
					assignSideEffectInCurrentScope()
				} else {
					captureSideEffectForOuterScope()
				}
			}

			var GetScope func(ScopeIF, string, *FunctionBuilder) *Variable
			GetScope = func(scope ScopeIF, name string, builder *FunctionBuilder) *Variable {
				var ret *Variable
				if vairable := GetFristLocalVariableFromScopeAndParent(scope, name); vairable != nil {
					ret = vairable
				} else if vairable := GetFristVariableFromScopeAndParent(scope, name); vairable != nil {
					ret = vairable
				}
				if ret == nil {
					return nil
				}
				if _, ok := ToParameter(ret.GetValue()); ok {
					parentBuilder := builder.parentBuilder
					if parentBuilder != nil {
						parentScope := parentBuilder.CurrentBlock.ScopeTable
						return GetScope(parentScope, name, parentBuilder)
					}
				}

				return ret
			}

			if _, ok := modify.(*Parameter); ok {
				assignSideEffectInCurrentScope()
				continue
			}

			obj := se.parameterMemberInner
			if ret := GetScope(currentScope, se.Name, builder); ret != nil {
				applySideEffectByScope(ret)
				continue
			} else if ret := GetScope(currentScope, obj.ObjectName, builder); ret != nil {
				applySideEffectByScope(ret)
				continue
			} else if obj.ObjectName == "this" {
				assignSideEffectInCurrentScope()
				continue
			}

			if isCallerVisibleMemberSideEffectKind(obj.MemberCallKind) {
				assignSideEffectInCurrentScope()
				continue
			}

			// 处理跨闭包的side-effect
			if block := function.GetBlock(); block != nil {
				functionScope := block.ScopeTable
				if ret := GetScope(functionScope, se.Name, builder); ret != nil {
					applySideEffectByScope(ret)
					continue
				} else if obj := se.parameterMemberInner; obj.ObjectName != "" { // 处理object
					if ret := GetScope(functionScope, obj.ObjectName, builder); ret != nil {
						applySideEffectByScope(ret)
						continue
					} else {
						assignSideEffectInCurrentScope()
						continue
					}
				}
			}
		}
	}
}

func (f *FunctionBuilder) SwitchFreevalueInSideEffectFromScope(name string, se *SideEffect, scope ScopeIF) *SideEffect {
	vs := make(Values, 0)
	if scope == nil {
		return se
	}
	if seValue, ok := f.GetValueById(se.Value); ok && seValue != nil {
		if phi, ok := ToPhi(seValue); ok {
			for i, id := range phi.Edge {
				edgeValue, ok := f.GetValueById(id)
				if !ok || edgeValue == nil {
					continue
				}
				vs = append(vs, edgeValue)
				if p, ok := ToParameter(edgeValue); ok && p.IsFreeValue {
					if value := scope.ReadValue(name); value != nil {
						vs[i] = value
					}
				}
			}
			phit := &Phi{
				anValue:            phi.anValue,
				CFGEntryBasicBlock: phi.CFGEntryBasicBlock,
				Edge:               vs.GetIds(),
			}

			if callSite, ok := f.GetValueById(se.CallSite); ok && callSite != nil {
				if call, ok := callSite.(*Call); ok {
					sideEffect := f.EmitSideEffect(name, call, phit)
					return sideEffect
				}
			}
		}
	}
	return se
}

// isParameterCalledInFunction checks if a parameter at the given index is called
// (used as Method in a Call instruction) inside the function.
// This is used to determine if a function argument will actually be invoked.
//
// For example:
//
//	execute = (fn) => { fn() }  // parameter fn IS called, returns true
//	store = (fn) => { arr.push(fn) }  // parameter fn is NOT called, returns false
func isParameterCalledInFunction(calleeFunc *Function, paramIndex int) bool {
	if calleeFunc == nil {
		return false
	}

	// Get the parameter at the given index
	if paramIndex >= len(calleeFunc.Params) {
		return false
	}

	paramId := calleeFunc.Params[paramIndex]
	param, ok := calleeFunc.GetValueById(paramId)
	if !ok || utils.IsNil(param) {
		return false
	}

	// Check if any user of this parameter is a Call instruction
	// where this parameter is the Method (i.e., the parameter is being called)
	for _, user := range param.GetUsers() {
		if call, isCall := ToCall(user); isCall {
			// Check if this parameter is the Method of the call
			if call.Method == paramId {
				return true
			}
		}
	}

	return false
}

// handleArgumentFunctionSideEffect handles side effects from function arguments.
// When a function with side effects is passed as an argument and that parameter
// is actually called inside the callee function, the side effects should be
// propagated to the call site.
//
// Example:
//
//	var x = 111111
//	var modifier = () => { x = 222222 }  // modifier has side-effect on x
//	var execute = (fn) => { fn() }        // fn is called inside execute
//	execute(modifier)                      // side-effect should be propagated here
//	var a = x                              // a should track to 222222
//
// This function checks:
// 1. If an argument is a function with side effects
// 2. If the corresponding parameter is actually called inside the callee
// 3. If both conditions are met, create SideEffect instructions at the call site
// splitMemberVerboseName splits `params.file_count` into its object name and
// field. A side effect created for a captured object member already carries the
// object variable and the field in its verbose name.
func splitMemberVerboseName(se *FunctionSideEffect) (objName, field string) {
	if se == nil || se.Variable == nil {
		return "", ""
	}
	objName = se.Variable.GetName()
	// Only `obj.field` names qualify. An already-materialized member such as
	// `#242.lang` is not an object name: binding it again would create the
	// double member `#242.lang.lang`.
	if objName == "" || strings.ContainsAny(objName, ".#") {
		return "", ""
	}
	if !strings.HasPrefix(se.VerboseName, objName+".") {
		return "", ""
	}
	field = strings.TrimPrefix(se.VerboseName, objName+".")
	if field == "" || strings.Contains(field, ".") {
		return "", ""
	}
	return objName, field
}

func propagateOpaqueCallbackSideEffects(c *Call, builder *FunctionBuilder, currentScope ScopeIF) {
	for _, argId := range c.Args {
		argValue, ok := c.GetValueById(argId)
		if !ok || utils.IsNil(argValue) {
			continue
		}
		argFunc, isFunc := ToFunction(argValue)
		if !isFunc || argFunc.Type == nil {
			continue
		}
		// Only closures carry captures a Go callback can write through.
		// A plain named function passed as a callback has no free values, so
		// its side effects belong to its own call sites, not to this one.
		if len(argFunc.FreeValues) == 0 {
			continue
		}
		recoverBuilder := builder.SetCurrent(c)
		for _, se := range argFunc.Type.SideEffects {
			if se == nil || se.Kind != NormalSideEffect {
				continue
			}
			modify, ok := c.GetValueById(se.Modify)
			if !ok || utils.IsNil(modify) {
				modify, ok = argFunc.GetValueById(se.Modify)
				if !ok || utils.IsNil(modify) {
					continue
				}
			}
			// Only the closure's own writes belong to this call site. A write the
			// closure inherited from a callee inside its body already reached the
			// closure's own member/variable cell, and reporting it here would
			// overwrite an unrelated variable of the same name in the caller. Such
			// entries point at a SideEffect value rather than at a real value the
			// closure computed (a local write, a call result, or a literal).
			if modify.IsSideEffect() {
				continue
			}
			varName := se.Name
			if se.Variable != nil && se.Variable.GetName() != "" {
				varName = se.Variable.GetName()
			}
			sideEffect := builder.EmitSideEffect(se.Name, c, modify)
			if sideEffect == nil {
				continue
			}
			sideEffect.SetVerboseName(se.VerboseName)
			// Bind the write to the object's member, not to the object itself.
			// An object-named side effect (`[params]`) makes later reads of any
			// field resolve to this value, so `params.language` would read the
			// count. A member variable keeps the write on its own field.
			if objName, field := splitMemberVerboseName(se); objName != "" && field != "" {
				if objVariable := ReadVariableFromScopeAndParent(currentScope, objName); objVariable != nil {
					if objValue := objVariable.GetValue(); !utils.IsNil(objValue) {
						key := builder.EmitConstInst(field)
						if memberVariable := builder.CreateMemberCallVariable(objValue, key); memberVariable != nil {
							builder.AssignVariable(memberVariable, sideEffect)
							recoverBuilder()
							continue
						}
					}
				}
			}
			variable := builder.CreateVariableForce(varName)
			if variable == nil {
				recoverBuilder()
				continue
			}
			if v := ReadVariableFromScopeAndParent(currentScope, se.Name); v != nil {
				variable.SetCaptured(v)
			}
			builder.AssignVariable(variable, sideEffect)
		}
		recoverBuilder()
	}
}

func handleArgumentFunctionSideEffect(c *Call, calleeFuncTyp *FunctionType) {
	defer c.markPersistenceDirty()
	function := c.GetFunc()
	if function == nil {
		return
	}
	builder := function.builder
	if builder == nil {
		return
	}

	currentScope := c.GetBlock().ScopeTable
	if currentScope == nil {
		return
	}

	// A callee with no SSA type is an extern/yaklib function. When one of its
	// arguments is a closure, the Go side may invoke it (`filesys.onFileStat`),
	// so the closure's captured writes have to surface at this call site.
	if calleeFuncTyp == nil {
		propagateOpaqueCallbackSideEffects(c, builder, currentScope)
		return
	}

	// Get the callee function (not just its type) to analyze parameter usage
	method, ok := c.GetValueById(c.Method)
	if !ok || utils.IsNil(method) {
		return
	}
	calleeFunc, isCalleeFunc := ToFunction(method)
	if !isCalleeFunc {
		// filesys.onFileStat(func...) has no SSA body, but the callback runs
		// and its captured writes must reach the caller.
		propagateOpaqueCallbackSideEffects(c, builder, currentScope)
		return
	}
	if len(calleeFunc.Blocks) == 0 {
		// A yaklib/Go-implemented callee has a type but no SSA body, so
		// isParameterCalledInFunction cannot observe the indirect call. The Go
		// side may still invoke the closure (filesys.onFileStat), so the
		// captured writes have to surface at this call site.
		propagateOpaqueCallbackSideEffects(c, builder, currentScope)
		return
	}

	// Check each argument to see if it's a function with side effects
	for i, argId := range c.Args {
		argValue, ok := c.GetValueById(argId)
		if !ok || utils.IsNil(argValue) {
			continue
		}

		// Check if the argument is a function
		argFunc, isFunc := ToFunction(argValue)
		if !isFunc {
			continue
		}

		// Get the function type of the argument function
		argFuncTyp := argFunc.Type
		if argFuncTyp == nil || len(argFuncTyp.SideEffects) == 0 {
			continue
		}

		// CRITICAL: Check if the parameter is actually called inside the callee function
		// Only propagate side effects if the parameter function is invoked
		if !isParameterCalledInFunction(calleeFunc, i) {
			// The parameter function is not called inside the callee,
			// so its side effects should not be propagated
			continue
		}

		// Parameter is called, propagate side effects from the argument function
		recoverBuilder := builder.SetCurrent(c)
		for _, se := range argFuncTyp.SideEffects {
			if se.Kind != NormalSideEffect {
				continue
			}

			modify, ok := c.GetValueById(se.Modify)
			if !ok || utils.IsNil(modify) {
				// Try to get modify value from the argument function's context
				modify, ok = argFunc.GetValueById(se.Modify)
				if !ok || utils.IsNil(modify) {
					continue
				}
			}

			// The write targets a member of a captured free variable
			// (`params.file_count` inside a callback that the holder received). The
			// caller reads the member cell `#<objectID>.<field>`, so resolve the
			// caller's object through the call binding and bind the effect there.
			// Binding a plain variable named after the object would both miss that
			// read and replace the object with the field's value, which makes every
			// other field read return this one.
			if se.MemberCallKind == FreeValueMemberCall {
				if obj, ok := se.GetActualParam(c); ok && !utils.IsNil(obj) {
					if key, keyOK := c.GetValueById(se.MemberCallKey); keyOK && key != nil {
						if memberVariable := builder.CreateMemberCallVariable(obj, key); memberVariable != nil {
							if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
								builder.AssignVariable(memberVariable, sideEffect)
								sideEffect.SetVerboseName(se.VerboseName)
								if c.SideEffectValue == nil {
									c.SideEffectValue = make(map[string]int64)
								}
								c.SideEffectValue[se.VerboseName] = sideEffect.GetId()
								continue
							}
						}
					}
				}
			}

			// Create side effect at call site. A member write inside the closure
			// (`m.n++`) is recorded under the field name (`#18.n`); the variable it
			// updates in the caller is the captured object (`m`). Bind the side
			// effect to that variable so a later read of `m.n` sees the update.
			varName := se.Name
			if se.Variable != nil && se.Variable.GetName() != "" {
				varName = se.Variable.GetName()
			}
			variable := builder.CreateVariableForce(varName)
			if variable == nil {
				continue
			}

			if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
				if v := ReadVariableFromScopeAndParent(currentScope, se.Name); v != nil {
					variable.SetCaptured(v)
				}
				builder.AssignVariable(variable, sideEffect)
				sideEffect.SetVerboseName(se.VerboseName)
				if c.SideEffectValue == nil {
					c.SideEffectValue = make(map[string]int64)
				}
				c.SideEffectValue[se.VerboseName] = sideEffect.GetId()
			}
		}
		recoverBuilder()
	}
}
