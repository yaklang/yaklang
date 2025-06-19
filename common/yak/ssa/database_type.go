package ssa

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveTypeToDB(typ Type, progName string) int {
	if typ == nil {
		return -1
	}
	kind := int(typ.GetTypeKind())

	str := typ.String()

	param := make(map[string]any)
	switch t := typ.(type) {
	case *FunctionType:
		param["name"] = t.Name
		param["fullTypeName"] = t.GetFullTypeNames()
	case *ObjectType:
		param["name"] = t.Name
		param["fullTypeName"] = t.GetFullTypeNames()
	case *BasicType:
		param["name"] = t.name
		param["kind"] = t.Kind
		param["fullTypeName"] = t.GetFullTypeNames()
	case *Blueprint:
		var parentBlueprintIds []int
		var interfaceBlueprintIds []int
		param["name"] = t.Name
		param["fullTypeName"] = t.GetFullTypeNames()
		param["kind"] = t.Kind
		for _, blueprint := range t.ParentBlueprints {
			parentBlueprintIds = append(parentBlueprintIds, SaveTypeToDB(blueprint, progName))
		}
		for _, blueprint := range t.InterfaceBlueprints {
			interfaceBlueprintIds = append(interfaceBlueprintIds, SaveTypeToDB(blueprint, progName))
		}
		param["parentBlueprints"] = parentBlueprintIds
		param["interfaceBlueprints"] = interfaceBlueprintIds
		container := t.Container()
		if utils.IsNil(container) {
			log.Infof("SaveTypeToDB: container is nil, type: %+v", t)
			param["container"] = -1
		} else {
			param["container"] = container.GetId()
		}
	default:
		param["fullTypeName"] = t.GetFullTypeNames()
	}
	extra, err := json.Marshal(param)
	if err != nil {
		log.Errorf("SaveTypeToDB: %v: param: %v", err, param)
	}

	return ssadb.SaveType(kind, str, utils.UnsafeBytesToString(extra), progName)
}

func GetTypeFromDB(id int) Type {
	if id == -1 {
		return nil
	}
	kind, str, extra, err := ssadb.GetType(id)
	if err != nil {
		log.Errorf("GetTypeFromDB: %v: id: %v", err, id)
		return nil
	}

	_ = str
	_ = extra
	params := make(map[string]any)
	if err := json.Unmarshal(utils.UnsafeStringToBytes(extra), &params); err != nil {
		log.Errorf("GetTypeFromDB: %v: extra: %v", err, extra)
	}
	getParamStr := func(name string) string {
		v, ok := params[name]
		if !ok {
			return ""
		}
		str, ok := v.(string)
		if !ok {
			return ""
		}
		return str
	}

	switch TypeKind(kind) {
	case FunctionTypeKind:
		typ := &FunctionType{}
		if raw, ok := params["return_value"].(string); ok {
			_ = raw
			// typ.ReturnValue = lo.FilterMap(strings.Split(raw, ","), func(s string, _ int) (*Return, bool) {
			// 	id, err := strconv.ParseInt(s, 10, 64)
			// 	if err != nil {
			// 		return nil, false
			// 	}
			// 	r, err := NewInstructionFromLazy(id, ToReturn)
			// 	if err != nil {
			// 		return nil, false
			// 	}
			// 	return r, true
			// })
		}
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		return typ
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, TupleTypeKind, StructTypeKind:
		typ := &ObjectType{}
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = TypeKind(kind)
		return typ
	case NumberTypeKind, StringTypeKind, ByteTypeKind, BytesTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		typ := &BasicType{}
		typ.name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = TypeKind(kind)
		return typ
	case ClassBluePrintTypeKind:
		typ := &Blueprint{}
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = ValidBlueprintKind(getParamStr("kind"))
		parents, ok := params["parentBlueprints"].([]interface{})
		if ok {
			for _, typeId := range parents {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(utils.InterfaceToInt(typeId)))
				if isBlueprint {
					typ.ParentBlueprints = append(typ.ParentBlueprints, blueprint)
				}
			}
		}
		interfaces, ok := params["interfaceBlueprints"].([]interface{})
		if ok {
			for _, typeId := range interfaces {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(utils.InterfaceToInt(typeId)))
				if isBlueprint {
					typ.InterfaceBlueprints = append(typ.InterfaceBlueprints, blueprint)
				}
			}
		}
		containerId := utils.MapGetInt64Or(params, "container", -1)
		if containerId != -1 {
			if container, err := NewInstructionFromLazy(containerId, ToMake); err == nil {
				typ.InitializeWithContainer(container)
			}
		}
		return typ
	default:
	}

	return nil
}
