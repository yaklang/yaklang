package ssa

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/sync/singleflight"
)

// TypeCacheManager 统一管理两层缓存的类型缓存
// 解决并发竞态条件、缓存不一致等问题
type TypeCacheManager struct {
	programCache *ProgramCache
	// 使用 sync.Map 存储正在加载的类型，防止重复加载
	// key: typeID, value: *sync.Once
	loading sync.Map

	// singleflight 用于防止缓存击穿
	singleFlight singleflight.Group
}

// NewTypeCacheManager 创建类型缓存管理器
func NewTypeCacheManager(cache *ProgramCache) *TypeCacheManager {
	return &TypeCacheManager{
		programCache: cache,
	}
}

// GetType 统一入口：获取类型，自动处理两层缓存
// 第一层：ProgramCache.TypeCache（最快）
// 第二层：ssadb.GetIrTypeCache（中等速度）
// 第三层：从数据库加载（最慢，使用 singleflight 防止击穿）
func (m *TypeCacheManager) GetType(typeID int64) Type {
	if typeID <= 0 {
		if typeID < 0 {
			if typ, ok := fallbackTypeCache.Get(typeID); ok && !utils.IsNil(typ) {
				return typ
			}
		}
		return defaultAnyType
	}

	// 第一层：检查 ProgramCache.TypeCache（最快）
	if typ, ok := m.programCache.TypeCache.Get(typeID); ok && !utils.IsNil(typ) {
		return typ
	}

	// 第二层：检查 ssadb.GetIrTypeCache（中等速度）
	if m.programCache.HaveDatabaseBackend() {
		programName := m.programCache.program.GetProgramName()
		irTypeCache := ssadb.GetIrTypeCache(programName)
		if irType, ok := irTypeCache.Get(typeID); ok && !utils.IsNil(irType) {
			// 从 IrType 反序列化为 Type，并缓存到第一层
			typ := m.deserializeType(irType, typeID)
			if !utils.IsNil(typ) {
				// 使用原子性操作更新第一层缓存
				m.setTypeToFirstCache(typeID, typ)
				return typ
			}
		}
	}

	// 第三层：从数据库加载（最慢，使用 singleflight 防止击穿）
	if !m.programCache.HaveDatabaseBackend() {
		return defaultAnyType
	}

	key := fmt.Sprintf("%s_%d", m.programCache.program.GetProgramName(), typeID)
	result, _, _ := m.singleFlight.Do(key, func() (interface{}, error) {
		// Double-check：再次检查第一层缓存（防止并发重复加载）
		if typ, ok := m.programCache.TypeCache.Get(typeID); ok && !utils.IsNil(typ) {
			return typ, nil
		}

		// 从数据库加载（使用改进后的 GetTypeFromDBWithDepth）
		typ := GetTypeFromDBWithDepth(m.programCache, typeID, 0)
		if utils.IsNil(typ) {
			return nil, nil
		}

		// 同时更新两层缓存（原子性操作）
		m.setTypeToBothCaches(typeID, typ)

		return typ, nil
	})

	if result == nil {
		return defaultAnyType
	}
	return result.(Type)
}

// setTypeToBothCaches 原子性地设置到两层缓存
func (m *TypeCacheManager) setTypeToBothCaches(typeID int64, typ Type) {
	// 使用 sync.Once 确保只设置一次
	once, _ := m.loading.LoadOrStore(typeID, &sync.Once{})
	once.(*sync.Once).Do(func() {
		// 设置第一层缓存
		m.programCache.TypeCache.Set(typ)

		// 设置第二层缓存（IrType）
		if m.programCache.HaveDatabaseBackend() {
			programName := m.programCache.program.GetProgramName()
			irTypeCache := ssadb.GetIrTypeCache(programName)

			// 序列化 Type 为 IrType
			irType := ssadb.EmptyIrType(programName, uint64(typeID))
			if marshalType(typ, irType) {
				irTypeCache.Set(typeID, irType)
			}
		}

		// 清理 loading 标记（但保留 once，避免重复创建）
		// 注意：不删除 loading 中的 once，因为 sync.Once 已经执行过，不会再次执行
	})
}

// setTypeToFirstCache 仅设置第一层缓存（用于从第二层缓存反序列化时）
func (m *TypeCacheManager) setTypeToFirstCache(typeID int64, typ Type) {
	once, _ := m.loading.LoadOrStore(typeID, &sync.Once{})
	once.(*sync.Once).Do(func() {
		m.programCache.TypeCache.Set(typ)
	})
}

// deserializeType 从 IrType 反序列化为 Type
// 复用 getTypeFromDBInternal 的逻辑，但不查询数据库
func (m *TypeCacheManager) deserializeType(irType *ssadb.IrType, typeID int64) Type {
	if utils.IsNil(irType) {
		return nil
	}

	kind, str, extra := irType.Kind, irType.String, irType.ExtraInformation
	_ = str
	_ = extra
	params := make(map[string]any)
	if err := json.Unmarshal(utils.UnsafeStringToBytes(extra), &params); err != nil {
		log.Errorf("deserializeType: %v: extra: %v", err, extra)
		return nil
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
			// 注意：这里不递归加载 parentBlueprints 和 interfaceBlueprints
			// 因为它们会在需要时通过 GetType 自动加载
			parents, ok := params["parentBlueprints"].([]interface{})
			if ok {
				for _, typeId := range parents {
					parentID := int64(utils.InterfaceToInt(typeId))
					// 使用 TypeCacheManager 加载，确保缓存一致性
					parentBlueprint, isBlueprint := ToClassBluePrintType(m.GetType(parentID))
					if isBlueprint {
						blueprint.ParentBlueprints = append(blueprint.ParentBlueprints, parentBlueprint)
					}
				}
			}
			interfaces, ok := params["interfaceBlueprints"].([]interface{})
			if ok {
				for _, typeId := range interfaces {
					interfaceID := int64(utils.InterfaceToInt(typeId))
					// 使用 TypeCacheManager 加载，确保缓存一致性
					interfaceBlueprint, isBlueprint := ToClassBluePrintType(m.GetType(interfaceID))
					if isBlueprint {
						blueprint.InterfaceBlueprints = append(blueprint.InterfaceBlueprints, interfaceBlueprint)
					}
				}
			}
			containerId := utils.MapGetInt64Or(params, "container", -1)
			if containerId != -1 {
				if container, err := NewInstructionWithCover(m.programCache.program, containerId, ToValue); err == nil {
					blueprint.InitializeWithContainer(container)
				}
			}
		}
	default:
		return nil
	}

	// 设置类型ID为数据库中的ID，确保ID一致性
	if !utils.IsNil(typ) {
		typ.SetId(typeID)
		return typ
	}

	return nil
}

// Invalidate 使缓存失效
func (m *TypeCacheManager) Invalidate(typeID int64) {
	// 删除第一层缓存
	m.programCache.TypeCache.Delete(typeID)

	// 删除第二层缓存
	if m.programCache.HaveDatabaseBackend() {
		programName := m.programCache.program.GetProgramName()
		irTypeCache := ssadb.GetIrTypeCache(programName)
		irTypeCache.Delete(typeID)
	}

	// 清理 loading 标记
	m.loading.Delete(typeID)
}

