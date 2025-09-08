package ssa

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func saveTypeWithValue(value Value, typ Type) {
	// i know is ugle, just is, and i will fix this after remove init value in ssa/next.go
	if utils.IsNil(value) {
		return
	}
	prog := value.GetProgram()
	if utils.IsNil(prog) {
		return
	}
	application := prog.GetApplication()
	if utils.IsNil(application) {
		return
	}

	if cache := application.Cache; cache != nil && cache.HaveDatabaseBackend() {
		saveType(cache, typ)
	}
}

func saveType(cache *ProgramCache, typ Type) int64 {
	if utils.IsNil(typ) {
		return -1
	}
	if id := typ.GetId(); id > 0 {
		// log.Errorf("saveType: type %v already has id %d", typ, id)
		return id
	}
	cache.TypeCache.Set(typ)
	return typ.GetId()
}

func marshalType(typ Type, irType *ssadb.IrType) bool {
	if utils.IsNil(typ) || utils.IsNil(irType) {
		log.Errorf("BUG: marshalType called with nil type")
		return false
	}
	if irType.GetIdInt64() == -1 {
		log.Errorf("[BUG]: type id is -1: %s", typ.GetFullTypeNames())
		return false
	}

	type2IrType(typ, irType)
	if irType.Kind < 0 {
		log.Errorf("BUG: save type called with empty kind: %v", typ.GetFullTypeNames())
	}
	return true
}

func type2IrType(typ Type, ir *ssadb.IrType) {
	kind := (typ.GetTypeKind())
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
		var parentBlueprintIds []int64
		var interfaceBlueprintIds []int64
		param["name"] = t.Name
		param["fullTypeName"] = t.GetFullTypeNames()
		param["kind"] = t.Kind
		for _, blueprint := range t.ParentBlueprints {
			parentBlueprintIds = append(parentBlueprintIds, blueprint.GetId())
		}
		for _, blueprint := range t.InterfaceBlueprints {
			interfaceBlueprintIds = append(interfaceBlueprintIds, blueprint.GetId())
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
	ir.Kind = int(kind)
	ir.ExtraInformation = utils.UnsafeBytesToString(extra)
	ir.String = str
}

func GetTypeFromDB(cache *ProgramCache, id int64) Type {
	if id == -1 {
		return nil
	}

	irType := ssadb.GetIrTypeById(cache.DB, id)
	if utils.IsNil(irType) {
		log.Errorf("GetTypeFromDB: failed type is nil: id: %v", id)
		return nil
	}
	kind, str, extra := irType.Kind, irType.String, irType.ExtraInformation

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
		typ := NewObjectType()
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = TypeKind(kind)
		return typ
	case NumberTypeKind, StringTypeKind, ByteTypeKind, BytesTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		typ := NewBasicType(TypeKind(kind), getParamStr("name"))
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		return typ
	case ClassBluePrintTypeKind:
		typ := &Blueprint{
			LazyBuilder: NewLazyBuilder("Blueprint:" + getParamStr("name")),
		}
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = ValidBlueprintKind(getParamStr("kind"))
		parents, ok := params["parentBlueprints"].([]interface{})
		if ok {
			for _, typeId := range parents {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(cache, int64(utils.InterfaceToInt(typeId))))
				if isBlueprint {
					typ.ParentBlueprints = append(typ.ParentBlueprints, blueprint)
				}
			}
		}
		interfaces, ok := params["interfaceBlueprints"].([]interface{})
		if ok {
			for _, typeId := range interfaces {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(cache, int64(utils.InterfaceToInt(typeId))))
				if isBlueprint {
					typ.InterfaceBlueprints = append(typ.InterfaceBlueprints, blueprint)
				}
			}
		}
		containerId := utils.MapGetInt64Or(params, "container", -1)
		if containerId != -1 {
			if container, err := NewInstructionWithCover(cache.program, containerId, ToValue); err == nil {
				typ.InitializeWithContainer(container)
			}
		}
		return typ
	default:
	}

	return nil
}
