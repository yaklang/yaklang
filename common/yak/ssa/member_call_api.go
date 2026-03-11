package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

type memberCallReadVisitKey struct {
	objectID     int64
	keyID        int64
	wantFunction bool
}

// ReadMemberCallMethodOrValue read member call method or value depends on type
func (b *FunctionBuilder) ReadMemberCallMethodOrValue(object, key Value) Value {
	res := checkCanMemberCallExist(object, key, false)
	if res.exist && res.typ.GetTypeKind() == FunctionTypeKind {
		return b.readMemberCallValueEx(object, key, true)
	}
	return b.readMemberCallValueEx(object, key, false)
}

// read member call variable, want method
func (b *FunctionBuilder) ReadMemberCallMethod(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, true)
}

// read member call variable, want variable
func (b *FunctionBuilder) ReadMemberCallValue(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, false)
}

func (b *FunctionBuilder) ReadMemberCallValueByName(object Value, key string) Value {
	name := fmt.Sprintf("#%d.%s", object.GetId(), key)
	if ret := b.PeekValueInThisFunction(name); ret != nil {
		return ret
	}
	return nil
}

func (b *FunctionBuilder) readMemberCallValueEx(object, key Value, wantFunction bool) Value {
	return b.readMemberCallValueExWithVisited(object, key, wantFunction, nil)
}

func (b *FunctionBuilder) readMemberCallValueExWithVisited(object, key Value, wantFunction bool, visited map[memberCallReadVisitKey]struct{}) Value {
	if res := b.CheckMemberCallNilValue(object, key, "readMemberCallVariableEx"); res != nil {
		return res
	}

	// to extern lib
	if extern, ok := ToExternLib(object); ok {
		return b.TryBuildExternLibValue(extern, key)
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// Phi value: read member from each edge and merge as Phi.
	if phi, ok := ToPhi(objectt); ok {
		if visited == nil {
			visited = make(map[memberCallReadVisitKey]struct{}, 8)
		}
		vk := memberCallReadVisitKey{
			objectID:     objectt.GetId(),
			keyID:        key.GetId(),
			wantFunction: wantFunction,
		}
		if _, ok := visited[vk]; ok {
			return b.getFieldValue(objectt, key, wantFunction)
		}
		visited[vk] = struct{}{}
		defer delete(visited, vk)

		res := checkCanMemberCallExist(objectt, key, wantFunction)
		if ret := b.PeekValueInThisFunction(res.name); ret != nil {
			return ret
		}

		edgeValues := make(Values, 0, len(phi.Edge))
		for _, edgeID := range phi.Edge {
			edgeValue, ok := objectt.GetValueById(edgeID)
			if !ok || edgeValue == nil {
				continue
			}
			edgeRes := checkCanMemberCallExist(edgeValue, key, wantFunction)
			if !edgeRes.exist {
				continue
			}
			memberValue := b.readMemberCallValueExWithVisited(edgeValue, key, wantFunction, visited)
			if memberValue != nil {
				edgeValues = append(edgeValues, memberValue)
			}
		}
		if len(edgeValues) > 1 {
			dedupeKey := func(v Value) string {
				if utils.IsNil(v) {
					return "<nil>"
				}
				switch vv := v.(type) {
				case *ConstInst:
					return "const:" + vv.String()
				case *Undefined:
					return fmt.Sprintf("undef:%d:%s:%s", vv.Kind, vv.GetVerboseName(), vv.GetType())
				default:
					return fmt.Sprintf("id:%d", v.GetId())
				}
			}
			seen := make(map[string]struct{}, len(edgeValues))
			deduped := make(Values, 0, len(edgeValues))
			for _, edgeValue := range edgeValues {
				s := dedupeKey(edgeValue)
				if _, ok := seen[s]; ok {
					continue
				}
				seen[s] = struct{}{}
				deduped = append(deduped, edgeValue)
			}
			edgeValues = deduped
		}
		if len(edgeValues) == 0 {
			return b.getFieldValue(objectt, key, wantFunction)
		}
		if len(edgeValues) == 1 {
			return edgeValues[0]
		}
		return b.EmitPhi(res.name, edgeValues)
	}

	// normal member call
	return b.getFieldValue(objectt, key, wantFunction)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value, cross ...bool) *Variable {
	iscross := false
	if len(cross) > 0 {
		iscross = cross[0]
	}

	createVariable := func(name string) *Variable {
		if iscross {
			return b.CreateVariableCross(name)
		} else {
			return b.CreateVariable(name)
		}
	}

	if utils.IsNil(object) || utils.IsNil(key) {
		log.Errorf("CreateMemberCallVariable: object or key is nil")
		return nil
	}
	// check
	if object.GetId() == -1 || key.GetId() == -1 {
		log.Infof("CreateMemberCallVariable: %v, %v", object.GetName(), key)
	}
	// extern lib
	if extern, ok := ToExternLib(object); ok {
		name := getExternLibMemberCall(object, key)
		ret := createVariable(name)
		ret.SetMemberCall(extern, key)
		return ret
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(objectt, key)
	name := res.name
	if objectt.GetOpcode() != SSAOpcodeParameter {
		// if member not exist, create undefine member in object position
		b.checkAndCreateDefaultMember(res, objectt, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := createVariable(name)
	ret.SetMemberCall(objectt, key)
	return ret
}
