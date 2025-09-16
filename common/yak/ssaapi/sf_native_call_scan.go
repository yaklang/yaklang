package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type direction string

const (
	Previous direction = "previous"
	Current  direction = "current"
	Next     direction = "next"
)

type basicBlockInfo struct {
	currentBlock *ssa.BasicBlock
	prog         *Program
	frame        *sfvm.SFFrame
	matchCheck   *sfCheck
	visited      map[int64]struct{}
	direction    direction
	hasInclude   bool
	results      []sfvm.ValueOperator
	isFinish     bool
	index        int
}

var nativeCallScan = func(direction direction) func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	return func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var vals []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		v.Recursive(func(operator sfvm.ValueOperator) error {
			val, ok := operator.(*Value)
			if !ok {
				return nil
			}
			results := searchAlongBasicBlock(val.innerValue, prog, frame, params, direction)
			if !ok {
				return nil
			}
			vals = append(vals, results...)
			return nil
		})
		return true, sfvm.NewValues(vals), nil
	}
}

func searchAlongBasicBlock(
	value ssa.Value,
	prog *Program,
	frame *sfvm.SFFrame,
	params *sfvm.NativeCallActualParams,
	direction direction,
) []sfvm.ValueOperator {
	basicBlockInfo := &basicBlockInfo{
		prog:       prog,
		frame:      frame,
		matchCheck: nil,
		visited:    make(map[int64]struct{}),
		direction:  direction,
		results:    make([]sfvm.ValueOperator, 0),
		isFinish:   false,
	}
	basicBlockInfo.createRecursiveConfig(frame, params)
	basicBlockInfo.searchBlock(value)
	return basicBlockInfo.results
}

func (b *basicBlockInfo) createRecursiveConfig(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) {
	sfResult, err := frame.GetSFResult()
	if err != nil {
		log.Warnf("Get sfResult error:%s", err)
		return
	}
	b.matchCheck = CreateCheckFromNativeCallParam(sfResult, frame.GetConfig(), params)
}

func (b *basicBlockInfo) searchBlock(value ssa.Value) {
	if value == nil {
		return
	}

	block, ok := ssa.ToBasicBlock(value)
	if !ok {
		block = value.GetBlock()
	}
	if block == nil {
		return
	}
	b.currentBlock = block
	blockId := block.GetId()
	if _, ok := b.visited[blockId]; ok {
		return
	}
	b.visited[blockId] = struct{}{}
	if b.index != 0 {
		b.searchInsts(block)
		if b.isFinish {
			return
		}
	}
	b.index++
	switch b.direction {
	case Previous:
		for _, pred := range block.Preds {
			pred, ok := block.GetValueById(pred)
			if ok && pred != nil {
				b.searchBlock(pred)
			}
			if b.isFinish {
				break
			}
		}
	case Next:
		for _, succ := range block.Succs {
			succ, ok := block.GetValueById(succ)
			if ok && succ != nil {
				b.searchBlock(succ)
			}
			if b.isFinish {
				break
			}
		}
	case Current:
		b.searchInsts(block)
		if b.isFinish {
			break
		}
		return
	}
}

func (b *basicBlockInfo) searchInsts(block *ssa.BasicBlock) {
	for _, inst := range block.Insts {
		inst, ok := block.GetInstructionById(inst)
		if !ok {
			continue
		}
		if jump, ok := ssa.ToJump(inst); ok {
			_ = jump
			block, ok := block.GetBasicBlockByID(jump.To)
			if ok && block != nil && b.currentBlock.HaveSubBlock(block) {
				b.searchInsts(block)
			}
			continue
		}
		value, err := b.prog.NewValue(inst)
		if err != nil {
			log.Warnf("NewValue error: %s", err)
			continue
		}
		if b.matchCheck.Empty() {
			b.results = append(b.results, value)
			continue
		}

		if b.matchCheck.CheckUntil(value) {
			b.isFinish = true
			break
		}
		if b.matchCheck.CheckMatch(value) {
			b.results = append(b.results, value)
		}
	}
}
