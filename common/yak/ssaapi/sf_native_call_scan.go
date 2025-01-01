package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
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
			results := searchAlongBasicBlock(val.node, prog, frame, params, direction)
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
		currentBlock:    nil,
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
		b.searchInsts()
		if b.isFinish {
			return
		}
	}
	b.index++
	switch b.direction {
	case Previous:
		for _, pred := range block.Preds {
			b.searchBlock(pred)
			if b.isFinish {
				break
			}
		}
	case Next:
		for _, succs := range block.Succs {
			b.searchBlock(succs)
			if b.isFinish {
				break
			}
		}
	case Current:
		b.searchInsts()
		if b.isFinish {
			break
		}
		return
	}
}

func (b *basicBlockInfo) searchInsts() {
	for _, inst := range b.currentBlock.Insts {
		if lz, ok := ssa.ToLazyInstruction(inst); ok {
			inst = lz.Self()
		}
		if v, ok := ssa.ToValue(inst); ok {
			value := b.prog.NewValue(v)
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
}
