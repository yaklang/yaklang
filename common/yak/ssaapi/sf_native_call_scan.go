package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type basicBlockap map[int64]struct{}

type direction string

const (
	Previous direction = "Previous"
	Next	direction="Next"
)

var nativeCallScanPrevious = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	basicBlockMap := make(basicBlockap)
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		results, _ := searchAlongBasicBlock(val.node, prog, frame, params, basicBlockMap, Previous)
		if !ok {
			return nil
		}
		vals = append(vals, results...)
		return nil
	})
	return true, sfvm.NewValues(vals), nil
}

var nativeCallScanNext = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	basicBlockMap := make(basicBlockap)
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		results, _ := searchAlongBasicBlock(val.node, prog, frame, params, basicBlockMap, Next)
		if !ok {
			return nil
		}
		vals = append(vals, results...)
		return nil
	})
	return true, sfvm.NewValues(vals), nil
}

func searchAlongBasicBlock(
		value ssa.Value,
		prog *Program,
		frame *sfvm.SFFrame,
		params *sfvm.NativeCallActualParams,
		basicBlockMap basicBlockap,
		direction direction,
	) ([]sfvm.ValueOperator, bool) {
	if value == nil {
		return nil, true
	}

	var results []sfvm.ValueOperator
	block, ok := ssa.ToBasicBlock(value)
	if !ok {
		block = value.GetBlock()
	}
	if block == nil {
		return nil, true
	}

	if _, ok := basicBlockMap[block.GetId()]; ok {
		return nil, true
	}
	basicBlockMap[block.GetId()] = struct{}{}

	for _, inst := range block.Insts {
		if lz, ok := ssa.ToLazyInstruction(inst); ok {
			inst = lz.Self()
		}
		if v, ok := ssa.ToValue(inst); ok {
			token := utils.RandStringBytes(16)
			token = "a" + token
			var sfRule string
			if rule := params.GetString("until"); rule != "" {
				sfRule = fmt.Sprintf("%s as $%s;", rule, token)
				val := prog.NewValue(v)
				res, err :=SyntaxFlowWithError(val, sfRule,sfvm.WithEnableDebug(true))
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				allValues := res.GetAllValuesChain()
				for  _,val := range allValues{
					_=val.AppendPredecessor(val,frame.WithPredecessorContext(fmt.Sprintf("scan%s",direction)))
				}
				results = append(results, allValues)
				if len(allValues) > 0 {
					return results, false
				}
			}
			if rule := params.GetString("hook"); rule != "" {
				sfRule = fmt.Sprintf("%s as $%s;", rule, token)
				val := prog.NewValue(v)
				res, err := SyntaxFlowWithError(val, sfRule)
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				allValues := res.GetAllValuesChain()
				for  _,val := range allValues{
					_=val.AppendPredecessor(val,frame.WithPredecessorContext(fmt.Sprintf("scan%s",direction)))
				}
				results = append(results, allValues)
			}
			if rule := params.GetString("exclude"); rule != "" {
				sfRule = fmt.Sprintf("%s as $%s;", rule, token)
				val := prog.NewValue(v)
				res, err := SyntaxFlowWithError(val, sfRule)
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				allValues := res.GetAllValuesChain()
				for  _,val := range allValues{
					_=val.AppendPredecessor(val,frame.WithPredecessorContext(fmt.Sprintf("scan%s",direction)))
				}
				if len(allValues) == 0 {
					results = append(results, val)
				}
			}
			if sfRule == "" {
				val := prog.NewValue(v)
				results = append(results, val)
			}
		}
	}

	// 向前寻找
	if direction == Previous {
		for _, pred := range block.Preds {
			res, needContinue := searchAlongBasicBlock(pred, prog, frame, params, basicBlockMap, direction)
			results = append(results, res...)
			if !needContinue {
				break
			}
		}
	}
	// 向后寻找
	if direction == Next {
		for _, next := range block.Succs {
			res, isContinue := searchAlongBasicBlock(next, prog, frame, params, basicBlockMap, direction)
			results = append(results, res...)
			if !isContinue {
				break
			}
		}
	}

	return results, true
}
