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
	currentBlock    *ssa.BasicBlock
	prog            *Program
	frame           *sfvm.SFFrame
	recursiveConfig *RecursiveConfig
	visited         map[int64]struct{}
	direction       direction
	hasInclude      bool
	results         []sfvm.ValueOperator
	isFinish        bool
	index           int
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
		prog:            prog,
		frame:           frame,
		recursiveConfig: nil,
		visited:         make(map[int64]struct{}),
		direction:       direction,
		results:         make([]sfvm.ValueOperator, 0),
		isFinish:        false,
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
	b.recursiveConfig, b.hasInclude = CreateRecursiveConfigFromNativeCallParams(sfResult, frame.GetConfig(), params)
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
			pred := block.GetValueById(pred)
			b.searchBlock(pred)
			if b.isFinish {
				break
			}
		}
	case Next:
		for _, succ := range block.Succs {
			succ := block.GetValueById(succ)
			b.searchBlock(succ)
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
		inst := block.GetInstructionById(inst)
		if jump, ok := ssa.ToJump(inst); ok {
			_ = jump
			block := block.GetBasicBlockByID(jump.To)
			if block != nil && b.currentBlock.HaveSubBlock(block) {
				b.searchInsts(block)
			}
			continue
		}
		value, err := b.prog.NewValue(inst)
		if err != nil {
			log.Warnf("NewValue error: %s", err)
			continue
		}
		if b.recursiveConfig.configItems == nil {
			b.results = append(b.results, value)
			continue
		} else {
			matchedConfig := b.recursiveConfig.compileAndRun(value)
			if _, ok := matchedConfig[sfvm.RecursiveConfig_Include]; ok {
				b.results = append(b.results, value)
				continue
			}
			if _, ok := matchedConfig[sfvm.RecursiveConfig_Until]; ok {
				b.isFinish = true
				break
			}
			if _, ok := matchedConfig[sfvm.RecursiveConfig_Exclude]; ok {
				// nothing todo
				// this value skip
				continue
			}
			if !b.hasInclude {
				// if has include, only match value can save to results
				b.results = append(b.results, value)
			}
		}
	}
}
