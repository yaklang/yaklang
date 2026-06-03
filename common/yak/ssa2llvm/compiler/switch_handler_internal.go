package compiler

import "github.com/yaklang/yaklang/common/yak/ssa"

func collectSwitchHandlers(fn *ssa.Function) map[int64]*switchHandlerInfo {
	if fn == nil {
		return nil
	}
	out := make(map[int64]*switchHandlerInfo)
	for _, blockID := range collectFunctionBlockIDs(fn) {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok || blockVal == nil {
			continue
		}
		block, ok := blockVal.(*ssa.BasicBlock)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			if lazy, ok := inst.(*ssa.LazyInstruction); ok && lazy != nil {
				inst = lazy.Self()
			}
			sw, ok := inst.(*ssa.Switch)
			if !ok || sw == nil {
				continue
			}
			addSwitchHandlersForSwitch(out, sw)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addSwitchHandlersForSwitch(out map[int64]*switchHandlerInfo, sw *ssa.Switch) {
	if out == nil || sw == nil {
		return
	}
	defaultID := int64(0)
	if sw.DefaultBlock != nil {
		defaultID = sw.DefaultBlock.GetId()
	}
	for i, label := range sw.Label {
		if label.Dest <= 0 || label.Value <= 0 {
			continue
		}
		info := out[label.Dest]
		if info == nil {
			info = &switchHandlerInfo{
				condID:       sw.Cond,
				trueBlockID:  firstSwitchHandlerSucc(label.Dest, defaultID, sw),
				falseBlockID: nextSwitchHandlerID(sw, i, defaultID),
			}
			out[label.Dest] = info
		}
		info.labelIDs = append(info.labelIDs, label.Value)
	}
}

func firstSwitchHandlerSucc(handlerID, defaultID int64, sw *ssa.Switch) int64 {
	if sw == nil || handlerID <= 0 {
		return 0
	}
	fn := sw.GetFunc()
	if fn == nil {
		return 0
	}
	blockVal, ok := fn.GetValueById(handlerID)
	if !ok || blockVal == nil {
		return 0
	}
	block, ok := blockVal.(*ssa.BasicBlock)
	if !ok || block == nil {
		return 0
	}
	for _, succID := range block.Succs {
		if succID > 0 && succID != defaultID {
			return succID
		}
	}
	if len(block.Succs) > 0 {
		return block.Succs[0]
	}
	return 0
}

func nextSwitchHandlerID(sw *ssa.Switch, labelIndex int, defaultID int64) int64 {
	if sw == nil {
		return defaultID
	}
	currentDest := int64(0)
	if labelIndex >= 0 && labelIndex < len(sw.Label) {
		currentDest = sw.Label[labelIndex].Dest
	}
	for i := labelIndex + 1; i < len(sw.Label); i++ {
		dest := sw.Label[i].Dest
		if dest > 0 && dest != currentDest {
			return dest
		}
	}
	return defaultID
}
