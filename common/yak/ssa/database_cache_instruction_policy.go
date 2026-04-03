package ssa

import (
	"os"
	"time"

	yaklog "github.com/yaklang/yaklang/common/log"
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
	largeProjectInstructionSave  = 1024
	largeProjectPersistLimit     = 16384
	largeProjectAuxiliarySave    = 512
	largeProjectTypeSave         = 256
)

var hotInstructionOpcodeBlacklist = map[Opcode]struct{}{
	SSAOpcodeBasicBlock: {},
	SSAOpcodeFunction:   {},
	SSAOpcodeConstInst:  {},
	SSAOpcodeUndefined:  {},
	SSAOpcodeMake:       {},
}

func instructionCacheDebugEnabled() bool {
	return yaklog.GetLevel() >= yaklog.DebugLevel
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

func shouldDelayInstructionEviction(inst Instruction) bool {
	if utils.IsNil(inst) {
		return true
	}
	fn := inst.GetFunc()
	if utils.IsNil(fn) {
		return true
	}
	return !fn.IsFinished()
}

func useAdaptiveInstructionFastPath(cfg *ssaconfig.Config) bool {
	cfg = ensureProgramConfig(cfg)
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
	if ttl == time.Second && maxEntries == 5000 && cfg.GetCompileProjectBytes() > 0 && cfg.GetCompileProjectBytes() <= fastPathProjectByteThreshold {
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
