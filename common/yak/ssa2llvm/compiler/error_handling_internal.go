package compiler

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) prepareErrorHandling(fn *ssa.Function) error {
	c.exceptionValueIDs = nil
	c.activeHandlerByBlock = nil
	c.catchBodyByHandler = nil
	c.catchTargetByBlock = nil

	if fn == nil {
		return nil
	}

	handlerByID := make(map[int64]*ssa.ErrorHandler)
	for _, blockID := range fn.Blocks {
		bb, ok := fn.GetBasicBlockByID(blockID)
		if !ok || bb == nil {
			continue
		}
		for _, instID := range bb.Insts {
			instVal, ok := fn.GetInstructionById(instID)
			if !ok || instVal == nil {
				continue
			}
			if instVal.IsLazy() {
				instVal = instVal.Self()
			}
			if instVal == nil {
				continue
			}
			if eh, ok := instVal.(*ssa.ErrorHandler); ok && eh != nil {
				handlerByID[eh.GetId()] = eh
			}
		}
	}

	if len(handlerByID) == 0 {
		return nil
	}

	exceptionIDs := make(map[int64]struct{})
	catchBodyByHandler := make(map[int64]int64)
	catchTargetByBlock := make(map[int64]int64)

	for handlerID, handler := range handlerByID {
		if handler == nil {
			continue
		}

		target := handler.Done
		if handler.Final > 0 {
			target = handler.Final
		}

		var firstCatchBody int64
		for _, catchID := range handler.Catch {
			catchInstAny, ok := fn.GetInstructionById(catchID)
			if !ok || catchInstAny == nil {
				continue
			}
			if catchInstAny.IsLazy() {
				catchInstAny = catchInstAny.Self()
			}
			catchInst, ok := catchInstAny.(*ssa.ErrorCatch)
			if !ok || catchInst == nil {
				continue
			}
			if firstCatchBody == 0 {
				firstCatchBody = catchInst.CatchBody
			}
			if catchInst.CatchBody > 0 && target > 0 {
				catchTargetByBlock[catchInst.CatchBody] = target
			}
			if catchInst.Exception > 0 {
				exceptionIDs[catchInst.Exception] = struct{}{}
			}
		}
		if firstCatchBody > 0 {
			catchBodyByHandler[handlerID] = firstCatchBody
		}
	}

	activeHandlerByBlock := make(map[int64]int64, len(fn.Blocks))
	var resolve func(int64) int64
	resolve = func(blockID int64) int64 {
		if existing, ok := activeHandlerByBlock[blockID]; ok {
			return existing
		}

		bb, ok := fn.GetBasicBlockByID(blockID)
		if !ok || bb == nil {
			activeHandlerByBlock[blockID] = 0
			return 0
		}

		if bb.Handler > 0 {
			if _, ok := handlerByID[bb.Handler]; ok {
				activeHandlerByBlock[blockID] = bb.Handler
				return bb.Handler
			}
		}

		var handlerID int64
		for _, predID := range bb.Preds {
			predHandler := resolve(predID)
			if predHandler == 0 {
				continue
			}
			if handlerID == 0 {
				handlerID = predHandler
				continue
			}
			if handlerID != predHandler {
				// Ambiguous nesting; keep the first one for now.
				break
			}
		}

		activeHandlerByBlock[blockID] = handlerID
		return handlerID
	}

	for _, blockID := range fn.Blocks {
		_ = resolve(blockID)
	}

	c.exceptionValueIDs = exceptionIDs
	c.activeHandlerByBlock = activeHandlerByBlock
	c.catchBodyByHandler = catchBodyByHandler
	c.catchTargetByBlock = catchTargetByBlock

	for handlerID, catchBody := range catchBodyByHandler {
		if catchBody == 0 {
			continue
		}
		if _, ok := handlerByID[handlerID]; !ok {
			return fmt.Errorf("prepareErrorHandling: handler %d not found", handlerID)
		}
	}

	return nil
}

