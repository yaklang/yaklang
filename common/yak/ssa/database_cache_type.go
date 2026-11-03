package ssa

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	"github.com/yaklang/gorm"
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
	concurrency int
	nextID      *atomic.Int64
	resident    *utils.SafeMapWithKey[int64, Type]
	flushMu     sync.Mutex
	persisted   typeFingerprints
	// interned maps a type content fingerprint to the id that already stores
	// it. Equal types reuse one id, so a program keeps one row per distinct
	// content instead of one row per created type object.
	interned map[[sha256.Size]byte]int64
	internMu sync.Mutex
	// identity remembers the last persisted map-cache key. A matching key
	// whose fingerprint is already stored skips snapshot and hashing.
	identity map[int64]typePersistIdentity
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
		concurrency: cfg.GetCompileConcurrency(),
		nextID:      atomic.NewInt64(0),
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
}

func (s *typeStore) remember(typ Type) Type {
	if s == nil || utils.IsNil(typ) {
		return typ
	}
	if s.program != nil {
		internTypeStrings(typ, s.program.NameCache)
	}
	id := typ.GetId()
	if id <= 0 {
		// Interning exists to stop duplicate database rows. A memory-only
		// program stores no rows and keeps each object's identity.
		if s.mode == ProgramCacheDBWrite && s.internTypeContent(typ) {
			return typ
		}
		id = s.claimTypeID(typ)
	}
	// Imported/shared types may already own an ID. Reserve it before the
	// next new allocation, otherwise a primitive can replace that resident.
	setAtomicMaxIfGreater(s.nextID, id)
	s.resident.Set(id, typ)
	return typ
}

// internTypeContent reuses the id of an already stored type whose persisted
// body is identical, and reports whether it did. Equal types would otherwise
// write one row each: Hue stored 3.47M type ids for 106K distinct bodies, and
// almost every duplicate was a primitive such as string or number.
// typeIsInternable decides which type kinds may share an id.
func (s *typeStore) internTypeContent(typ Type) bool {
	if !typeIsInternable(typ) {
		return false
	}
	fingerprint := snapshotTypePersistence(typ).fingerprint()

	s.internMu.Lock()
	defer s.internMu.Unlock()
	if existing, ok := s.interned[fingerprint]; ok {
		typ.SetId(existing)
		s.adoptInternedType(existing, typ)
		return true
	}
	id := s.claimTypeID(typ)
	if s.interned == nil {
		s.interned = make(map[[sha256.Size]byte]int64)
	}
	s.interned[fingerprint] = id
	return false
}

// adoptInternedType keeps the canonical object resident and folds the
// duplicate's type names into it, so a name learned through the second use is
// still written for the shared id. The duplicate object is dropped: equal
// fingerprints already guarantee an equal stored body.
func (s *typeStore) adoptInternedType(id int64, duplicate Type) {
	if existing, ok := s.resident.Get(id); ok && !utils.IsNil(existing) {
		for _, name := range duplicate.GetFullTypeNames() {
			existing.AddFullTypeName(name)
		}
		return
	}
	s.resident.Set(id, duplicate)
}

// claimTypeID assigns the next free id. A shared type may have been claimed
// by another worker, in which case the object itself returns the winner's id.
func (s *typeStore) claimTypeID(typ Type) int64 {
	if claimer, ok := typ.(interface {
		claimIdIfUnset(int64) (int64, bool)
	}); ok {
		// Draw a candidate from the shared counter, then let the object
		// itself decide the single winner.
		if id, _ := claimer.claimIdIfUnset(s.nextID.Inc()); id > 0 {
			return id
		}
	}
	id := s.nextID.Inc()
	typ.SetId(id)
	return id
}

// typeIsInternable reports whether two objects with one persisted body are
// interchangeable for the whole run.
//
// Blueprints are excluded: their parents, interfaces and container arrive
// after the first reference and differ per class. Struct, tuple and plain
// object types are excluded too: the frontend grows them field by field, so
// two declarations can start equal and diverge. Function types are fixed by
// their parameter list, and slice/map wrappers are fixed at construction.
func typeIsInternable(typ Type) bool {
	switch t := typ.(type) {
	case *Blueprint:
		return false
	case *FunctionType:
		return true
	case *ObjectType:
		switch t.GetTypeKind() {
		case SliceTypeKind, MapTypeKind:
			return true
		}
	}
	_, stable := typePersistIdentityOf(typ)
	return stable
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
		// TODO(scan-log): type id not in DB after split-compile flush (resident cleared per
		// unit); scan dataflow recovers with a nil type. Common on large projects; debug to
		// avoid 5k+ lines of log noise.
		log.Debugf("GetTypeFromDB: failed type is nil: id: %v", id)
		return nil, false
	}

	typ := typeFromIrType(s, irType)
	if utils.IsNil(typ) {
		return nil, false
	}
	return typ, true
}

func (s *typeStore) close() error {
	return s.flush()
}

func (s *typeStore) flush() error {
	if s == nil || s.mode != ProgramCacheDBWrite || s.db == nil {
		return nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	if s.persisted == nil {
		s.persisted = make(typeFingerprints)
	}

	types := make([]Type, 0, s.resident.Count())
	s.resident.ForEach(func(_ int64, typ Type) bool {
		if !utils.IsNil(typ) {
			types = append(types, typ)
		}
		return true
	})
	if len(types) == 0 {
		return nil
	}
	started := time.Now()
	changed := 0
	identitySkipped := 0
	defer func() {
		log.Debugf("[ssa-type-flush] checked=%d changed=%d skipped=%d identity=%d duration=%s", len(types), changed, len(types)-changed, identitySkipped, time.Since(started))
	}()

	saveBatch := saveIrType(s.program, s.db)
	batchSize := max(1, s.saveSize)
	batch := make([]typePersistenceSnapshot, 0, batchSize)
	fingerprints := make([][sha256.Size]byte, 0, batchSize)
	batchIdentity := make([]typePersistIdentity, 0, batchSize)
	batchStable := make([]bool, 0, batchSize)
	type writeRequest struct {
		rows         []*ssadb.IrType
		fingerprints [][sha256.Size]byte
		identities   []typePersistIdentity
		stable       []bool
		ids          []int64
	}
	type writeResult struct {
		request writeRequest
		err     error
	}
	requests := make(chan writeRequest, 1)
	results := make(chan writeResult, 1)
	workerDone := make(chan struct{})
	workerStarted := false
	startWorker := func() {
		if workerStarted {
			return
		}
		workerStarted = true
		go func() {
			defer close(workerDone)
			for request := range requests {
				err := saveBatch(request.rows)
				results <- writeResult{request: request, err: err}
			}
		}()
	}
	pending := 0
	var firstErr error
	collect := func() {
		result := <-results
		pending--
		if result.err != nil {
			log.Errorf("save ir type batch failed: %v", result.err)
			if firstErr == nil {
				firstErr = result.err
			}
			return
		}
		for i, id := range result.request.ids {
			s.persisted.set(id, result.request.fingerprints[i])
			if result.request.stable[i] && id > 0 {
				s.rememberIdentity(id, result.request.identities[i])
			}
		}
	}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Snapshots are complete before they cross the goroutine boundary.
		// Marshal and batch-write this immutable chunk while the compile thread
		// prepares the next chunk. The one-item queue bounds retained memory.
		rows := marshalTypeSnapshots(s.programName, batch, fingerprints, s.concurrency)
		ids := make([]int64, len(batch))
		for i := range batch {
			ids[i] = batch[i].id
		}
		request := writeRequest{
			rows: rows, fingerprints: fingerprints, identities: batchIdentity,
			stable: batchStable, ids: ids,
		}
		startWorker()
		requests <- request
		pending++
		batch = make([]typePersistenceSnapshot, 0, batchSize)
		fingerprints = make([][sha256.Size]byte, 0, batchSize)
		batchIdentity = make([]typePersistIdentity, 0, batchSize)
		batchStable = make([]bool, 0, batchSize)
		if pending >= 2 {
			collect()
		}
	}

	for _, typ := range types {
		id := typ.GetId()
		key, stable := typePersistIdentityOf(typ)
		if stable && id > 0 && s.identity != nil {
			if previous, seen := s.identity[id]; seen && previous == key {
				if _, persisted := s.persisted.get(id); persisted {
					identitySkipped++
					continue
				}
			}
		}
		snapshot := snapshotTypePersistence(typ)
		s.internSnapshot(&snapshot)
		fingerprint := snapshot.fingerprint()
		// Check ALL persisted fields before JSON encoding. ID/pointer-only
		// checks would lose cross-unit updates, including in-place slice edits.
		if previous, ok := s.persisted.get(snapshot.id); ok && previous == fingerprint {
			if stable && id > 0 {
				s.rememberIdentity(id, key)
			}
			continue
		}
		changed++
		batch = append(batch, snapshot)
		fingerprints = append(fingerprints, fingerprint)
		batchIdentity = append(batchIdentity, key)
		batchStable = append(batchStable, stable)
		if len(batch) >= batchSize {
			flush()
		}
	}
	flush()
	if workerStarted {
		close(requests)
		for pending > 0 {
			collect()
		}
		<-workerDone
	}
	return firstErr
}

func (s *typeStore) rememberIdentity(id int64, key typePersistIdentity) {
	if s.identity == nil {
		s.identity = make(map[int64]typePersistIdentity)
	}
	s.identity[id] = key
}

func (s *typeStore) internSnapshot(snapshot *typePersistenceSnapshot) {
	if s == nil || snapshot == nil || s.program == nil || s.program.NameCache == nil {
		return
	}
	pool := s.program.NameCache
	snapshot.text = pool.Intern(snapshot.text)
	snapshot.name = pool.Intern(snapshot.name)
	for i, name := range snapshot.fullNames {
		snapshot.fullNames[i] = pool.Intern(name)
	}
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
	snapshotTypePersistence(typ).writeTo(ir)
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
				return ssadb.SaveIrTypeBatch(tx, types)
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
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
		return store.remember(typ)
	case ObjectTypeKind, SliceTypeKind, MapTypeKind, TupleTypeKind, StructTypeKind:
		typ := NewObjectType()
		typ.Name = getParamStr("name")
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
		typ.Kind = TypeKind(kind)
		typ.SetId(int64(irType.TypeId))
		return store.remember(typ)
	case NumberTypeKind, StringTypeKind, ByteTypeKind, BytesTypeKind, BooleanTypeKind,
		UndefinedTypeKind, NullTypeKind, AnyTypeKind, ErrorTypeKind:
		typ := NewBasicType(TypeKind(kind), getParamStr("name"))
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
		typ.SetId(int64(irType.TypeId))
		return store.remember(typ)
	case ClassBluePrintTypeKind:
		typ := &Blueprint{
			LazyBuilder: NewLazyBuilder("Blueprint:" + getParamStr("name")),
		}
		typ.Name = getParamStr("name")
		typ.SetFullTypeNames(utils.InterfaceToStringSlice(params["fullTypeName"]))
		typ.Kind = ValidBlueprintKind(getParamStr("kind"))
		typ.SetId(int64(irType.TypeId))
		store.remember(typ)

		parents, ok := params["parentBlueprints"].([]interface{})
		if ok {
			for _, typeID := range parents {
				blueprint, isBlueprint := ToBluePrintType(GetTypeFromDB(cacheView, int64(utils.InterfaceToInt(typeID))))
				if isBlueprint {
					typ.ParentBlueprints = append(typ.ParentBlueprints, blueprint)
				}
			}
		}
		interfaces, ok := params["interfaceBlueprints"].([]interface{})
		if ok {
			for _, typeID := range interfaces {
				blueprint, isBlueprint := ToBluePrintType(GetTypeFromDB(cacheView, int64(utils.InterfaceToInt(typeID))))
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
