package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type basicBlockap map[int64]struct{}

type direction int

const (
    Previous direction = iota 
    Next                      
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
		results := searchAlongBasicBlock(val.node,prog,frame,params, basicBlockMap,Previous)
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
		results := searchAlongBasicBlock(val.node, prog,frame,params,basicBlockMap,Next)
		if !ok {
			return nil
		}
		vals = append(vals, results...)
		return nil
	})
	return true, sfvm.NewValues(vals), nil
}

func searchAlongBasicBlock(value ssa.Value, prog *Program,frame *sfvm.SFFrame,params *sfvm.NativeCallActualParams ,basicBlockMap basicBlockap,direction direction) []sfvm.ValueOperator {
	if value == nil {
		return nil
	}

	var results []sfvm.ValueOperator
	block, ok := ssa.ToBasicBlock(value)
	if !ok {
		block = value.GetBlock()
	}
	if block == nil {
		return nil
	}

	if _, ok := basicBlockMap[block.GetId()]; ok {
		return nil
	}
	basicBlockMap[block.GetId()] = struct{}{}

	
	for _, inst := range block.Insts {
		if lz,ok := ssa.ToLazyInstruction(inst); ok {
			inst = lz.Self()
		}
		if v, ok := ssa.ToValue(inst); ok {
			token := utils.RandStringBytes(16)
			token = "a" + token

			if sfRule:=params.GetString("until");sfRule!=""{
				sfRule =fmt.Sprintf("%s as $%s;",sfRule,token)
				val := prog.NewValue(v)
				res, err :=SyntaxFlowWithError(val,sfRule)
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				results = append(results, res.GetAllValuesChain())
				if len(res.GetAllValuesChain()) > 0 {
					return results
				}
			}
			if sfRule := params.GetString("hook");sfRule!=""{
				sfRule =fmt.Sprintf("%s as $%s;",sfRule,token)
				val := prog.NewValue(v)
				res, err :=SyntaxFlowWithError(val,sfRule)
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				results = append(results, res.GetAllValuesChain())
			}
			if sfRule := params.GetString("exclude");sfRule!=""{
				sfRule =fmt.Sprintf("%s as $%s;",sfRule,token)
				val := prog.NewValue(v)
				res, err :=SyntaxFlowWithError(val,sfRule,sfvm.WithEnableDebug(true))
				if err != nil {
					log.Warnf("scanPrevious/scanNext error: %v", err)
				}
				if len(res.GetAllValuesChain()) == 0 {
					results = append(results,val)
				}
			}
		}
	}

	// 向前寻找
	if direction == Previous{
		for _, pred := range block.Preds {
			res := searchAlongBasicBlock(pred, prog, frame,params,basicBlockMap,direction)
			results = append(results, res...)
		}
	}
	// 向后寻找
	if direction ==Next{
		for _,next := range block.Succs{
			res := searchAlongBasicBlock(next, prog,frame, params,basicBlockMap,direction)
			results = append(results, res...)
		}
	}
	
	return results
}
