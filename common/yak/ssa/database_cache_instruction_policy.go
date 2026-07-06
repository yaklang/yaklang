package ssa

import (
	"os"
	"time"

	stdlog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	defaultSaveSize              = 200
	maxSaveSize                  = 40000
	saveTime                     = time.Second
	fastPathProjectByteThreshold = 2 * 1024 * 1024
	largeProjectByteThreshold    = 16 * 1024 * 1024
	largeProjectCacheTTL         = 250 * time.Millisecond
	largeProjectCacheMax         = 1024
	// Instruction DB batches for repos >= largeProjectByteThreshold: larger batches
	// mean fewer SQLite transactions per million IR rows (bounded by maxSaveSize).
	// Persist limit stays >= 4x batch so dbcache eviction backpressure matches writer.
	largeProjectInstructionSave = 4096
	largeProjectPersistLimit    = 32768
	largeProjectAuxiliarySave   = 512
	largeProjectTypeSave        = 256
)

var hotInstructionOpcodeBlacklist = map[Opcode]struct{}{
	SSAOpcodeBasicBlock: {},
	SSAOpcodeFunction:   {},
	SSAOpcodeConstInst:  {},
	SSAOpcodeUndefined:  {},
	SSAOpcodeMake:       {},
}

func instructionCacheDebugEnabled() bool {
	return log.Level >= stdlog.DebugLevel
}

func instructionCacheEventDebugEnabled() bool {
	return instructionCacheDebugEnabled() && os.Getenv("YAK_SSA_IR_CACHE_EVENT_DEBUG") != ""
}

func instructionReloadStackDebugEnabled() bool {
	return os.Getenv("YAK_SSA_IR_CACHE_RELOAD_STACK_DEBUG") != ""
}

func shouldKeepInstructionResident(inst Instruction) bool {
	if utils.IsNil(inst) {
		return false
	}
	_, ok := hotInstructionOpcodeBlacklist[inst.GetOpcode()]
	return ok
}

func shouldKeepCompileUnitBoundaryResident(inst Instruction) bool {
	if utils.IsNil(inst) {
		return false
	}
	switch inst.GetOpcode() {
	case SSAOpcodeFunction,
		SSAOpcodeParameter,
		SSAOpcodeFreeValue,
		SSAOpcodeParameterMember,
		SSAOpcodeSideEffect,
		SSAOpcodeExternLib:
		return true
	case SSAOpcodeBasicBlock:
		// BasicBlocks carry the ScopeTable that FunctionBuilder relies on for
		// CreateVariable/NewParam during lazy/deferred builds. The scope is not
		// restored when a spilled block is reloaded from DB, so a reloaded block
		// has a nil ScopeTable and the next NewParam panics (nil Variable). Keep
		// blocks resident under compile-unit split; they are small (metadata +
		// scope) compared to the instruction stream that the split path spills.
		return true
	default:
		return false
	}
}

func shouldDelayInstructionEviction(inst Instruction) bool {
	if utils.IsNil(inst) {
		return true
	}
	// Check if instruction has a cached function pointer (fast path, no cache access).
	// We MUST NOT call GetFunc() here because this runs inside the cache eviction callback
	// which holds the cache mutex. Calling GetFunc() -> resolveFunctionByID() -> GetInstruction()
	// would try to re-acquire the same mutex, causing a deadlock.
	type funGetter interface {
		GetCachedFunc() *Function
	}
	if fg, ok := inst.(funGetter); ok {
		if fn := fg.GetCachedFunc(); fn != nil {
			return !fn.IsFinished()
		}
	}
	// No cached function pointer available without re-entering the cache mutex.
	// Evict rather than pin: instructions without a resolvable owning function
	// (e.g. reloaded lazy instructions whose owning function already finished)
	// should be eligible for eviction so dirty state writes back to the DB.
	return false
}

func useAdaptiveInstructionFastPath(cfg *ssaconfig.Config) bool {
	cfg = ensureProgramConfig(cfg)
	if cfg.GetCompileUnitSplit() {
		return false
	}
	ttl, maxEntries := resolveInstructionCacheSettings(cfg)
	return ttl == 0 &&
		maxEntries == 0 &&
		cfg.GetCompileProjectBytes() > 0 &&
		cfg.GetCompileProjectBytes() <= fastPathProjectByteThreshold
}

func isLargeProjectCompile(cfg *ssaconfig.Config) bool {
	cfg = ensureProgramConfig(cfg)
	return cfg.GetCompileProjectBytes() >= largeProjectByteThreshold
}

func resolveInstructionPersistenceTuning(cfg *ssaconfig.Config, saveSize int) (int, int) {
	if isLargeProjectCompile(cfg) {
		return largeProjectInstructionSave, largeProjectPersistLimit
	}
	return saveSize, 0
}

func resolveAuxiliarySaveSize(cfg *ssaconfig.Config, saveSize int) int {
	if isLargeProjectCompile(cfg) {
		return largeProjectAuxiliarySave
	}
	if saveSize < IndexSaveSize {
		return IndexSaveSize
	}
	return saveSize
}

func resolveTypeSaveSize(cfg *ssaconfig.Config, saveSize int) int {
	if isLargeProjectCompile(cfg) {
		return largeProjectTypeSave
	}
	return min(max(saveSize*10, 2000), maxSaveSize)
}

func resolveInstructionCacheSettings(cfg *ssaconfig.Config) (time.Duration, int) {
	cfg = ensureProgramConfig(cfg)
	ttl := cfg.GetCompileIrCacheTTL()
	maxEntries := cfg.GetCompileIrCacheMax()
	if ttl == time.Second && maxEntries == 5000 && !cfg.GetCompileUnitSplit() && cfg.GetCompileProjectBytes() > 0 && cfg.GetCompileProjectBytes() <= fastPathProjectByteThreshold {
		return 0, 0
	}
	if ttl == time.Second && maxEntries == 5000 && cfg.GetCompileProjectBytes() >= largeProjectByteThreshold {
		return largeProjectCacheTTL, largeProjectCacheMax
	}
	return ttl, maxEntries
}

func collectFinishedFunctionInstructionIDs(function *Function) []int64 {
	if function == nil {
		return nil
	}

	ids := make([]int64, 0, len(function.Blocks)*8)
	seen := make(map[int64]struct{})
	addID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, id := range function.Params {
		addID(id)
	}
	for _, id := range function.FreeValues {
		addID(id)
	}
	for _, id := range function.ParameterMembers {
		addID(id)
	}
	for _, id := range function.Throws {
		addID(id)
	}
	for _, id := range function.Return {
		addID(id)
	}
	for _, id := range function.ChildFuncs {
		addID(id)
	}
	addID(function.EnterBlock)
	addID(function.ExitBlock)
	addID(function.DeferBlock)

	for _, sideEffect := range function.SideEffects {
		if sideEffect == nil {
			continue
		}
		addID(sideEffect.Modify)
		addID(sideEffect.MemberCallKey)
	}
	for _, sideEffects := range function.SideEffectsReturn {
		for _, sideEffect := range sideEffects {
			if sideEffect == nil {
				continue
			}
			addID(sideEffect.Modify)
			addID(sideEffect.MemberCallKey)
		}
	}

	for _, blockID := range function.Blocks {
		blockValue, ok := function.GetInstructionById(blockID)
		if !ok || blockValue == nil {
			continue
		}
		block, ok := ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		addID(block.GetId())
		for _, instID := range block.Insts {
			addID(instID)
		}
		for _, phiID := range block.Phis {
			addID(phiID)
		}
	}

	return ids
}
