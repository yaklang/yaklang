package ssa

import (
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

type typeStore struct {
	mode        ProgramCacheKind
	program     *Program
	db          *gorm.DB
	programName string
	saveSize    int
	nextID      *atomic.Int64
	resident    *utils.SafeMapWithKey[int64, Type]
}

func newTypeStore(
	cfg *ssaconfig.Config,
	prog *Program,
	mode ProgramCacheKind,
	db *gorm.DB,
	programName string,
	saveSize int,
) *typeStore {
	return &typeStore{
		mode:        mode,
		program:     prog,
		db:          db,
		programName: programName,
		saveSize:    resolveTypeSaveSize(cfg, min(max(saveSize, defaultSaveSize), maxSaveSize)),
		nextID:      atomic.NewInt64(0),
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
}

func (s *typeStore) remember(typ Type) Type {
	if s == nil || utils.IsNil(typ) {
		return typ
	}
	id := typ.GetId()
	if id <= 0 {
		id = s.nextID.Inc()
		typ.SetId(id)
	} else {
		trackAtomicMax(s.nextID, id)
	}
	s.resident.Set(id, typ)
	return typ
}

func (s *typeStore) get(id int64) (Type, bool) {
	if s == nil || id <= 0 {
		return nil, false
	}
	if typ, ok := s.resident.Get(id); ok && !utils.IsNil(typ) {
		return typ, true
	}
	if s.mode == ProgramCacheMemory || s.db == nil || s.program == nil {
		return nil, false
	}

	irType := ssadb.GetIrTypeById(s.db, s.program.GetProgramName(), id)
	if utils.IsNil(irType) {
		log.Warnf("GetTypeFromDB: failed type is nil: id: %v", id)
		return nil, false
	}

	typ := typeFromIrType(s, irType)
	if utils.IsNil(typ) {
		return nil, false
	}
	return typ, true
}

func (s *typeStore) close() {
	if s == nil || s.mode != ProgramCacheDBWrite || s.db == nil {
		return
	}

	types := make([]Type, 0, s.resident.Count())
	s.resident.ForEach(func(_ int64, typ Type) bool {
		if !utils.IsNil(typ) {
			types = append(types, typ)
		}
		return true
	})
	if len(types) == 0 {
		return
	}

	saveBatch := saveIrType(s.program, s.db)
	batch := make([]*ssadb.IrType, 0, s.saveSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := saveBatch(batch); err != nil {
			log.Errorf("save ir type batch failed: %v", err)
		}
		batch = make([]*ssadb.IrType, 0, s.saveSize)
	}

	for _, typ := range types {
		irType, err := marshalIrType(s.programName)(typ, utils.EvictionReasonDeleted)
		if err != nil {
			log.Errorf("marshal ir type failed: %v", err)
			continue
		}
		if utils.IsNil(irType) {
			continue
		}
		batch = append(batch, irType)
		if len(batch) >= s.saveSize {
			flush()
		}
	}
	flush()
}

func saveTypeWithValue(value Value, typ Type) {
	if utils.IsNil(value) {
		log.Error("value is nil ")
		return
	}
	prog := value.GetProgram()
	if utils.IsNil(prog) {
		log.Error("no program")
		return
	}
	application := prog.GetApplication()
	if utils.IsNil(application) {
		log.Error("no application ")
		return
	}

	if cache := application.Cache; cache != nil {
		cache.rememberType(typ)
		saveType(typ)
		value.getAnValue().SetLazySaveType(nil)
	} else {
		log.Error("cache is nil ")
	}
}

func saveType(typ Type) int64 {
	if utils.IsNil(typ) {
		return -1
	}
	if id := typ.GetId(); id > 0 {
		return id
	}
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
	kind := typ.GetTypeKind()
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
		var parentBlueprintIDs []int64
		var interfaceBlueprintIDs []int64
		param["name"] = t.Name
		param["fullTypeName"] = t.GetFullTypeNames()
		param["kind"] = t.Kind
		for _, blueprint := range t.ParentBlueprints {
			parentBlueprintIDs = append(parentBlueprintIDs, blueprint.GetId())
		}
		for _, blueprint := range t.InterfaceBlueprints {
			interfaceBlueprintIDs = append(interfaceBlueprintIDs, blueprint.GetId())
		}
		param["parentBlueprints"] = parentBlueprintIDs
		param["interfaceBlueprints"] = interfaceBlueprintIDs
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

func marshalIrType(name string) func(Type, utils.EvictionReason) (*ssadb.IrType, error) {
	return func(typ Type, _ utils.EvictionReason) (*ssadb.IrType, error) {
		if typ.GetId() <= 0 {
			log.Errorf("[BUG] marshalIrType: type ID is invalid: %d, type: %s", typ.GetId(), typ.String())
		}
		ret := ssadb.EmptyIrType(name, uint64(typ.GetId()))
		marshalType(typ, ret)
		return ret, nil
	}
}

func saveIrType(prog *Program, db *gorm.DB) func([]*ssadb.IrType) error {
	return func(types []*ssadb.IrType) error {
		var saveErr error
		saveStep := func() error {
			saveErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irType := range types {
					if irType == nil {
						continue
					}
					if err := tx.Save(irType).Error; err != nil {
						return err
					}
				}
				return nil
			})
			return saveErr
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrTypeBatch", saveStep)
		} else {
			saveStep()
		}
		return saveErr
	}
}

func GetTypeFromDB(cache *ProgramCache, id int64) Type {
	if id <= 0 {
		if id == 0 {
			log.Warnf("GetTypeFromDB: called with id=0, likely a serialization bug")
		}
		return nil
	}
	if cache == nil || cache.types == nil {
		return nil
	}
	typ, _ := cache.types.get(id)
	return typ
}

func typeFromIrType(store *typeStore, irType *ssadb.IrType) Type {
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

	cacheView := &ProgramCache{
		program: store.program,
		types:   store,
	}

	switch TypeKind(kind) {
	case FunctionTypeKind:
		typ := NewFunctionType("", nil, nil, false)
		typ.SetId(int64(irType.TypeId))
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		return store.remember(typ)
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, TupleTypeKind, StructTypeKind:
		typ := NewObjectType()
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = TypeKind(kind)
		typ.SetId(int64(irType.TypeId))
		return store.remember(typ)
	case NumberTypeKind, StringTypeKind, ByteTypeKind, BytesTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		typ := NewBasicType(TypeKind(kind), getParamStr("name"))
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.SetId(int64(irType.TypeId))
		return store.remember(typ)
	case ClassBluePrintTypeKind:
		typ := &Blueprint{
			LazyBuilder: NewLazyBuilder("Blueprint:" + getParamStr("name")),
		}
		typ.Name = getParamStr("name")
		typ.fullTypeName = utils.InterfaceToStringSlice(params["fullTypeName"])
		typ.Kind = ValidBlueprintKind(getParamStr("kind"))
		typ.SetId(int64(irType.TypeId))
		store.remember(typ)

		parents, ok := params["parentBlueprints"].([]interface{})
		if ok {
			for _, typeID := range parents {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(cacheView, int64(utils.InterfaceToInt(typeID))))
				if isBlueprint {
					typ.ParentBlueprints = append(typ.ParentBlueprints, blueprint)
				}
			}
		}
		interfaces, ok := params["interfaceBlueprints"].([]interface{})
		if ok {
			for _, typeID := range interfaces {
				blueprint, isBlueprint := ToClassBluePrintType(GetTypeFromDB(cacheView, int64(utils.InterfaceToInt(typeID))))
				if isBlueprint {
					typ.InterfaceBlueprints = append(typ.InterfaceBlueprints, blueprint)
				}
			}
		}
		containerID := utils.MapGetInt64Or(params, "container", -1)
		if containerID != -1 && store.program != nil {
			if container, err := NewInstructionWithCover(store.program, containerID, ToValue); err == nil {
				typ.InitializeWithContainer(container)
			}
		}
		return typ
	default:
		return nil
	}
}
