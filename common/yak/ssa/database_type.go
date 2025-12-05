package ssa

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/sync/singleflight"
)

var (
	typeFromDBSingleFlight singleflight.Group
	typeFromDBOnce         sync.Once
	// 跟踪正在加载的类型，防止循环依赖
	loadingTypes sync.Map // map[string]bool // key: "programName_typeID"
	// 限制递归深度，防止栈溢出
	maxRecursionDepth = 10
)

func saveTypeWithValue(value Value, typ Type) {
	// i know is ugle, just is, and i will fix this after remove init value in ssa/next.go
	if utils.IsNil(value) {
		log.Debugf("value is nil ")
		return
	}
	prog := value.GetProgram()
	if utils.IsNil(prog) {
		log.Debug("no program")
		return
	}
	application := prog.GetApplication()
	if utils.IsNil(application) {
		log.Debug("no application ")
		return
	}

	if cache := application.Cache; cache != nil {
		cache.TypeCache.Set(typ)
		saveType(typ)
	} else {
		log.Debug("cache is nil ")
	}
}

func saveType(typ Type) int64 {
	if utils.IsNil(typ) {
		return -1
	}
	if id := typ.GetId(); id > 0 {
		// log.Errorf("saveType: type %v already has id %d", typ, id)
		return id
	}
	// cache.TypeCache.Set(typ)
	return typ.GetId()
}

func marshalType(typ Type, irType *ssadb.IrType) bool {
	if utils.IsNil(typ) || utils.IsNil(irType) {
		log.Errorf("BUG: marshalType called with nil type")
		return false
	}
	if irType.GetIdInt64() < 0 {
		log.Errorf("[BUG]: type id is invalid: %d, type: %s", irType.GetIdInt64(), typ.GetFullTypeNames())
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
	ir.TypeId = uint64(typ.GetId())
	ir.Kind = int(kind)
	ir.ExtraInformation = utils.UnsafeBytesToString(extra)
	ir.String = str
}

// getTypeFromDBInternal 向后兼容的包装函数
func getTypeFromDBInternal(cache *ProgramCache, id int64) Type {
	return getTypeFromDBInternalWithDepth(cache, id, 0)
}

// getTypeFromDBInternalWithDepth 从数据库恢复类型，支持递归深度限制
func getTypeFromDBInternalWithDepth(cache *ProgramCache, id int64, depth int) Type {
	if id <= 0 {
		if id == 0 {
			log.Warnf("GetTypeFromDB: called with id=0, likely a serialization bug")
		}
		return nil
	}

	irType := ssadb.GetIrTypeById(cache.DB, cache.program.GetProgramName(), id)
	if utils.IsNil(irType) {
		log.Warnf("GetTypeFromDB: failed type is nil: id: %v", id)
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

	var typ Type
	switch TypeKind(kind) {
	case FunctionTypeKind:
		typ = &FunctionType{
			baseType: NewBaseType(),
		}
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
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, TupleTypeKind, StructTypeKind:
		typ = NewObjectType()
		if objTyp, ok := ToObjectType(typ); ok {
			objTyp.Name = getParamStr("name")
			objTyp.Kind = TypeKind(kind)
		}
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
	case NumberTypeKind, StringTypeKind, ByteTypeKind, BytesTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		typ = NewBasicType(TypeKind(kind), getParamStr("name"))
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
	case ClassBluePrintTypeKind:
		typ = &Blueprint{
			LazyBuilder: NewLazyBuilder("Blueprint:" + getParamStr("name")),
		}
		if blueprint, ok := ToClassBluePrintType(typ); ok {
			blueprint.Name = getParamStr("name")
			blueprint.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
			blueprint.Kind = ValidBlueprintKind(getParamStr("kind"))
			parents, ok := params["parentBlueprints"].([]interface{})
			if ok {
				for _, typeId := range parents {
					parentID := int64(utils.InterfaceToInt(typeId))
					// 使用带深度参数的版本，防止递归死锁
					parentBlueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDBWithDepth(cache, parentID, depth))
					if isBlueprint {
						blueprint.ParentBlueprints = append(blueprint.ParentBlueprints, parentBlueprint)
					}
				}
			}
			interfaces, ok := params["interfaceBlueprints"].([]interface{})
			if ok {
				for _, typeId := range interfaces {
					interfaceID := int64(utils.InterfaceToInt(typeId))
					// 使用带深度参数的版本，防止递归死锁
					interfaceBlueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDBWithDepth(cache, interfaceID, depth))
					if isBlueprint {
						blueprint.InterfaceBlueprints = append(blueprint.InterfaceBlueprints, interfaceBlueprint)
					}
				}
			}
			containerId := utils.MapGetInt64Or(params, "container", -1)
			if containerId != -1 {
				if container, err := NewInstructionWithCover(cache.program, containerId, ToValue); err == nil {
					blueprint.InitializeWithContainer(container)
				}
			}
		}
	default:
		return nil
	}

	// 设置类型ID为数据库中的ID，确保ID一致性
	if !utils.IsNil(typ) {
		typ.SetId(id)
		return typ
	}

	return nil
}

// GetTypeFromDB 从数据库恢复类型，使用 singleflight 避免并发重复查询
// 这是向后兼容的包装函数，内部调用带深度参数的版本
func GetTypeFromDB(cache *ProgramCache, id int64) Type {
	return GetTypeFromDBWithDepth(cache, id, 0)
}

// GetTypeFromDBWithDepth 从数据库恢复类型，支持递归深度限制和循环依赖检测
func GetTypeFromDBWithDepth(cache *ProgramCache, id int64, depth int) Type {
	if id <= 0 {
		if id == 0 {
			log.Warnf("GetTypeFromDB: called with id=0, likely a serialization bug")
		}
		return nil
	}

	// 防止递归过深
	if depth > maxRecursionDepth {
		log.Errorf("GetTypeFromDB: max recursion depth reached for type %d (depth: %d)", id, depth)
		return nil
	}

	programName := cache.program.GetProgramName()
	key := fmt.Sprintf("%s_%d", programName, id)

	// 检查是否正在加载（防止循环依赖）
	if loading, ok := loadingTypes.Load(key); ok && loading.(bool) {
		log.Warnf("GetTypeFromDB: circular dependency detected for type %d", id)
		return nil
	}

	// 使用 singleflight 确保同时只有一个协程查询相同的类型
	result, _, _ := typeFromDBSingleFlight.Do(key, func() (interface{}, error) {
		// 标记为正在加载
		loadingTypes.Store(key, true)
		defer loadingTypes.Delete(key)

		typ := getTypeFromDBInternalWithDepth(cache, id, depth+1)
		return typ, nil
	})

	if result == nil {
		return nil
	}
	return result.(Type)
}
