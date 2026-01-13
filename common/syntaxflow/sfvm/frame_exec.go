package sfvm

// frame_exec.go
// This file contains the execution functions for SyntaxFlow Virtual Machine.
// It implements three categories of operations:
// 1. execFilterAndCondition: Handles condition/logic/comparison operations
//    - OpEmptyCompare, OpCompare*, OpVersionIn
//    - OpEq/Ne/Gt/GtEq/Lt/LtEq
//    - OpLogic*, OpCondition, OpCheckEmpty
// 2. execValueFilter: Handles ValueOperator navigation/search operations
//    - OpPush/RecursiveSearch* (Exact/Glob/Regexp)
//    - OpGetCall/CallArgs
//    - OpGetUsers/BottomUsers/Defs/TopDefs
// 3. execSyntaxFlowOp: Handles syntax flow and stack operations
//    - OpDuplicate/Pop/PopDuplicate
//    - OpNewRef/UpdateRef/Merge/Remove/Intersection
//    - OpCheckParams/Alert/AddDescription
//    - OpNativeCall/FileFilter
//    - OpPushNumber/Bool/String

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func recursiveDeepChain(element []ValueOperator, handle func(operator ValueOperator) bool, visited map[int64]struct{}) error {
	if visited == nil {
		visited = make(map[int64]struct{})
	}

	var next []ValueOperator

	for _, element := range element {
		val, _ := element.GetCalled()
		val.ForEach(func(vo ValueOperator) error {
			if idGetter, ok := vo.(ssa.GetIdIF); ok {
				if _, ok := visited[idGetter.GetId()]; ok {
					return nil
				}
				visited[idGetter.GetId()] = struct{}{}

				fields, _ := vo.GetFields()
				if fields != nil {
					_ = fields.ForEach(func(fieldElement ValueOperator) error {
						if idGetter, ok := fieldElement.(ssa.GetIdIF); ok {
							if _, ok := visited[idGetter.GetId()]; ok {
								return nil
							}
							visited[idGetter.GetId()] = struct{}{}

							if !handle(fieldElement) {
								next = append(next, fieldElement)
							}
						}
						return nil
					})
				}
			}
			return nil
		})

		fields, _ := element.GetFields()
		if fields != nil {
			_ = fields.ForEach(func(operator ValueOperator) error {
				if idGetter, ok := operator.(ssa.GetIdIF); ok {
					if _, ok := visited[idGetter.GetId()]; ok {
						return nil
					}
					visited[idGetter.GetId()] = struct{}{}

					if !handle(operator) {
						next = append(next, operator)
					}
				}
				return nil
			})
		}
	}

	if len(next) <= 0 {
		return nil
	}

	return recursiveDeepChain(next, handle, visited)
}

func (s *SFFrame) opPop(unName bool) (Values, error) {
	vs, err := s.stackPop()
	if err != nil {
		return nil, err
	}
	s.popStack.Push(vs)
	s.debugSubLog(">> pop %v", len(vs))
	if unName {
		s.debugSubLog("save-to $_")
		err := s.output("_", vs)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return nil, utils.Errorf("output '_' error: %v", err)
		}
	}
	return vs, nil
}
func (s *SFFrame) execStatement(i *SFI) error {
	// Try filter and condition operations first
	if handled, err := s.execFilterAndCondition(i); handled || err != nil {
		return err
	}

	// Try value filter operations
	if handled, err := s.execValueFilter(i); handled || err != nil {
		return err
	}

	// Try syntax flow operations
	if handled, err := s.execSyntaxFlowOp(i); handled || err != nil {
		return err
	}

	// If none matched, it's an undefined opcode
	msg := fmt.Sprintf("unhandled default case, undefined opcode %v", i.String())
	return utils.Wrap(CriticalError, msg)
}

// execFilterAndCondition handles condition/logic/comparison operations
func (s *SFFrame) execFilterAndCondition(i *SFI) (handler bool, e error) {
	handler = true
	e = nil
	switch i.OpCode {
	case OpEmptyCompare:
		vals, err := s.stackPeek()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		flag := make([]bool, 0, len(vals))
		for i := 0; i < len(vals); i++ {
			flag = append(flag, true)
		}
		s.conditionStack.Push(flag)
		return
	case OpCompareOpcode:
		s.debugSubLog(">> pop")
		values, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}

		comparator := NewOpcodeComparator(s.GetContext())
		for _, v := range i.Values {
			op := validSSAOpcode(v)
			if op != -1 {
				comparator.AddOpcode(op)
				continue
			}
			binOp := validSSABinOpcode(v)
			if binOp != "" {
				comparator.AddBinOrUnaryOpcode(binOp)
				continue
			}
			log.Infof("invalid opcode: %v", v)
		}

		var newVal Values
		var condition []bool

		if trackErr := s.track("value-op:CompareOpcode", func() error {
			done := s.startValueOpTiming("CompareOpcode")
			defer done()
			newVal, condition = values.mapValuesWithBool(func(operator ValueOperator) (Values, bool) {
				return operator.CompareOpcode(comparator)
			})
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		s.stackPush(newVal)
		s.conditionStack.Push(condition)
		return true, nil
	case OpCompareString:
		s.debugSubLog(">> pop")
		//pop到原值
		valList, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		values := valList

		mode := ValidStringMatchMode(i.UnaryInt)
		if mode == -1 {
			return true, utils.Wrapf(CriticalError, "compare string failed: invalid mode %v", mode)
		}

		comparator := NewStringComparator(mode, s.GetContext())
		if len(i.Values) != len(i.MultiOperator) {
			s.conditionStack.Push([]bool{false})
			return true, utils.Wrapf(CriticalError, "sfi values or mutiOperator out size %v", len(i.Values))
		}
		for index, v := range i.Values {
			comparator.AddCondition(v, ValidConditionFilter(i.MultiOperator[index]))
		}
		var newVal Values
		var condition []bool
		if trackErr := s.track("value-op:CompareString", func() error {
			done := s.startValueOpTiming("CompareString")
			defer done()
			newVal, condition = values.mapValuesWithBool(func(operator ValueOperator) (Values, bool) {
				return operator.CompareString(comparator)
			})
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		s.stackPush(newVal)
		s.conditionStack.Push(condition)
		return true, nil
	case OpVersionIn:
		valList, err := s.stackPeek()
		if err != nil {
			return true, utils.Wrap(CriticalError, "compare version failed: stack top is empty")
		}
		value := valList
		call, err := GetNativeCall("versionIn")
		if err != nil {
			s.debugSubLog("Err: %v", err)
			log.Errorf("native call failed, not an existed native call-versionIn")
			return true, utils.Errorf("get native call failed: %v", err)
		}
		params := NewNativeCallActualParams(i.SyntaxFlowConfig...)
		gt := params.GetString("greaterThan")  // <
		ge := params.GetString("greaterEqual") // <=
		lt := params.GetString("lessThan")     // >
		le := params.GetString("lessEqual")    // >=

		var buffer bytes.Buffer
		buffer.WriteString("compare version in")
		if ge != "" {
			buffer.WriteString("[" + ge)
		} else {
			buffer.WriteString("(" + gt)
		}
		buffer.WriteString(",")
		if le != "" {
			buffer.WriteString(le + "]")
		} else {
			buffer.WriteString(lt + ")")
		}
		s.debugSubLog(buffer.String())
		var res []bool
		for _, v := range value {
			_, value, _ := call(v, s, params)
			res = append(res, !value.IsEmpty())
		}
		s.conditionStack.Push(res)
		return true, nil
	case OpEq:
		return true, utils.Errorf("unimplement")
		// s.debugSubLog(">> pop")
		// vs, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// s.debugSubLog(">> peek")
		// vs2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionEqual)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(Equal)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(Equal)")
		// 	defer done()
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		return true, nil
	case OpNotEq:
		return true, utils.Errorf("unimplement")

		// s.debugSubLog(">> pop")
		// valList1, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// vs1 := NewValueList(valList1)
		// s.debugSubLog(">> peek")
		// valList2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionNotEqual)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(NotEqual)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(NotEqual)")
		// 	defer done()
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		// return true, nil
	case OpGt:
		return true, utils.Errorf("unimplement")

		// s.debugSubLog(">> pop")
		// valList1, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// vs1 := NewValueList(valList1)
		// s.debugSubLog(">> peek")
		// valList2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionGt)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(Gt)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(Gt)")
		// 	defer done()
		// 	// conds = vs2.CompareConst(comparator)
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		// return true, nil
	case OpGtEq:
		return true, utils.Errorf("unimplement")

		// s.debugSubLog(">> pop")
		// valList1, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// vs1 := NewValueList(valList1)
		// s.debugSubLog(">> peek")
		// valList2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionGtEq)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(GtEq)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(GtEq)")
		// 	defer done()
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		// return true, nil
	case OpLt:
		return true, utils.Errorf("unimplement")

		// s.debugSubLog(">> pop")
		// valList1, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// vs1 := NewValueList(valList1)
		// s.debugSubLog(">> peek")
		// valList2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionLt)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(Lt)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(Lt)")
		// 	defer done()
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		// return true, nil
	case OpLtEq:
		return true, utils.Errorf("unimplement")
		// s.debugSubLog(">> pop")
		// valList1, err := s.stackPop()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// vs1 := NewValueList(valList1)
		// s.debugSubLog(">> peek")
		// valList2, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		// }
		// comparator := NewConstComparator(vs1.String(), BinaryConditionLtEq)
		// var conds []bool
		// if trackErr := s.track("value-op:CompareConst(LtEq)", func() error {
		// 	done := s.startValueOpTiming("CompareConst(LtEq)")
		// 	defer done()
		// 	for _, operator := range valList2 {
		// 		conds = append(conds, operator.CompareConst(comparator))
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// s.conditionStack.Push(conds)
		// return true, nil
	case OpLogicBang:
		conds := s.conditionStack.Pop()
		for i := 0; i < len(conds); i++ {
			conds[i] = !conds[i]
		}
		s.conditionStack.Push(conds)
		return true, nil
	case OpLogicAnd:
		conds1 := s.conditionStack.Pop()
		conds2 := s.conditionStack.Pop()
		if len(conds1) != len(conds2) {
			return true, utils.Errorf("condition failed: stack top(%v) vs conds(%v)", len(conds1), len(conds2))
		}
		res := make([]bool, 0, len(conds1))
		for i := 0; i < len(conds1); i++ {
			res = append(res, conds1[i] && conds2[i])
		}
		s.conditionStack.Push(res)
		return true, nil
	case OpLogicOr:
		conds1 := s.conditionStack.Pop()
		conds2 := s.conditionStack.Pop()
		if len(conds1) != len(conds2) {
			return true, utils.Errorf("condition failed: stack top(%v) vs conds(%v)", len(conds1), len(conds2))
		}
		res := make([]bool, 0, len(conds1))
		for i := 0; i < len(conds1); i++ {
			res = append(res, conds1[i] || conds2[i])
		}
		s.conditionStack.Push(res)
		return true, nil
	case OpCondition:
		s.debugSubLog(">> pop")
		valList, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		vs := valList
		conds := s.conditionStack.Pop()
		if len(conds) != len(vs) {
			return true, utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", len(vs), len(conds))
		}
		//log.Infof("condition: %v", conds)
		res := make([]ValueOperator, 0, len(vs))
		for i := 0; i < len(conds); i++ {
			if conds[i] {
				if i < len(vs) {
					res = append(res, vs[i])
				}
			}
		}
		s.stackPush(res)
		return true, nil
	case OpCheckEmpty:
		return true, utils.Errorf("unimplement")
		// if i.Iter == nil {
		// 	return true, utils.Wrap(CriticalError, "check empty failed: stack top is empty")
		// }
		// index := i.Iter.currentIndex
		// conditions := s.conditionStack.Peek()
		// //如果是null
		// vs, err := s.stackPop()
		// if err != nil {
		// 	return true, err
		// }
		// val := valList
		// for _, val := range vs {
		// 	if len(conditions) == index+1 && !conditions[index] {
		// 		return true, nil
		// 	}
		// }
		// if len(conditions) == index+1 && !conditions[index] {
		// 	return true, nil
		// }
		// conditions = s.conditionStack.Pop()
		// if len(conditions) < index+1 {
		// 	return true, utils.Errorf("check empty failed: stack top is empty")
		// }
		// conditions[index] = !val.IsEmpty()
		// s.conditionStack.Push(conditions)
		// s.popStack.Free()
		// return true, nil
	case OpToBool:
		valList, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "to bool failed: stack top is empty")
		}
		value := valList
		conditions := make([]bool, len(value))
		for i, val := range value {
			conditions[i] = !val.IsEmpty()
		}
		log.Errorf("toBool: %#v", conditions)
		s.conditionStack.Push(conditions)
		return
	default:
		return false, nil
	}
}

// execValueFilter handles ValueOperator navigation/search operations
func (s *SFFrame) execValueFilter(i *SFI) (bool, error) {
	switch i.OpCode {
	case OpPushSearchExact:
		s.debugSubLog(">> pop match exactly: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Errorf("search exact failed: stack top is empty")
		}
		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}

		// diagnostics: track value operation timing
		var result Values
		if trackErr := s.track("value-op:ExactMatch", func() error {
			done := s.startValueOpTiming("ExactMatch")
			defer done()
			result, _ = vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.ExactMatch(s.GetContext(), mod, i.UnaryStr), nil
			})
			// result, next, err = value.ExactMatch(s.GetContext(), mod, i.UnaryStr)
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			err = utils.Wrapf(err, "search exact failed")
		}
		if len(result) == 0 {
			return true, utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
		}

		s.debugSubLog("result next: %v", len(result))
		s.stackPush(result)
		s.debugSubLog("<< push next")
		return true, nil
	case OpRecursiveSearchExact:
		s.debugSubLog(">> pop recursive search exactly: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "recursive search exact failed: stack top is empty")
		}
		value := vs
		var next Values
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveExactMatch")
			defer done()
			results := operator.ExactMatch(s.GetContext(), BothMatch, i.UnaryStr)
			if !results.IsEmpty() {
				have := false
				// log.Infof("recursive search exact: %v from: %v", results.String(), operator.String())
				_ = results.ForEach(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				results.ForEach(func(operator ValueOperator) error {
					operator.AppendPredecessor(operator, s.WithPredecessorContext("recursive search "+i.UnaryStr))
					return nil
				})
				next = append(next, results...)
				if have {
					return true
				}
			}
			return false
		}, nil)
		if err != nil {
			err = utils.Wrapf(err, "recursive search exact failed")
		}

		results := next
		s.debugSubLog("result next: %v", len(results))
		s.stackPush(results)
		s.debugSubLog("<< push next")
		return true, nil
	case OpRecursiveSearchGlob:
		s.debugSubLog(">> pop recursive search glob: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "recursive search glob failed: stack top is empty")
		}
		value := vs

		mod := i.UnaryInt

		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}
		_ = globIns

		var next Values
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveGlobMatch")
			defer done()
			results := operator.GlobMatch(s.GetContext(), mod|NameMatch, i.UnaryStr)
			if !results.IsEmpty() {
				have := false
				_ = results.ForEach(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				next = append(next, results...)
				if have {
					return true
				}
			}
			return false
		}, nil)
		if err != nil {
			err = utils.Wrapf(err, "recursive search glob failed")
			s.debugSubLog("ERROR: %v", err)
		}
		s.debugSubLog("result next: %v", len(next))
		_ = next.ForEach(func(operator ValueOperator) error {
			operator.AppendPredecessor(operator, s.WithPredecessorContext("recursive search "+i.UnaryStr))
			return nil
		})
		s.stackPush(next)
		s.debugSubLog("<< push next")
		return true, nil
	case OpRecursiveSearchRegexp:
		s.debugSubLog(">> pop recursive search regexp: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "recursive search regexp failed: stack top is empty")
		}
		value := vs
		mod := i.UnaryInt

		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			return true, utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		_ = regexpIns

		var next Values
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveRegexpMatch")
			defer done()
			//log.Infof("recursive search regexp: %v", operator.String())
			//if strings.Contains(operator.String(), "aaa") {
			//	spew.Dump(1)
			//}
			results := operator.RegexpMatch(s.GetContext(), mod|NameMatch, i.UnaryStr)
			if !results.IsEmpty() {
				have := false
				_ = results.ForEach(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				next = append(next, results...)
				if have {
					return true
				}
			}
			return false
		}, nil)
		if err != nil {
			err = utils.Wrapf(err, "recursive search regexp failed")
			s.debugSubLog("ERROR: %v", err)
		}
		s.debugSubLog("result next: %v", len(next))
		_ = next.ForEach(func(operator ValueOperator) error {
			operator.AppendPredecessor(operator, s.WithPredecessorContext("recursive search "+i.UnaryStr))
			return nil
		})
		s.stackPush(next)
		s.debugSubLog("<< push next")
		return true, nil
	case OpPushSearchGlob:
		s.debugSubLog(">> pop search glob: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "search glob failed: stack top is empty")
		}
		value := vs
		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}
		_ = globIns

		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		var next Values
		if trackErr := s.track("value-op:GlobMatch", func() error {
			done := s.startValueOpTiming("GlobMatch")
			defer done()
			next, err = value.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GlobMatch(s.GetContext(), mod, i.UnaryStr), nil
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Wrapf(err, "search glob failed")
		}
		s.debugSubLog("result next: %v", len(next))
		_ = next.ForEach(func(operator ValueOperator) error {
			operator.AppendPredecessor(operator, s.WithPredecessorContext("search: "+i.UnaryStr))
			return nil
		})
		s.stackPush(next)
		s.debugSubLog("<< push next")
		return true, nil
	case OpPushSearchRegexp:
		s.debugSubLog(">> pop search regexp: %v", i.UnaryStr)
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "search regexp failed: stack top is empty")
		}
		value := vs
		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		var next Values
		if trackErr := s.track("value-op:RegexpMatch", func() error {
			done := s.startValueOpTiming("RegexpMatch")
			defer done()
			next, err = value.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.RegexpMatch(s.GetContext(), mod, regexpIns.String()), nil
			})
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Wrap(err, "search regexp failed")
		}
		s.debugSubLog("result next: %v", len(next))
		_ = next.ForEach(func(operator ValueOperator) error {
			operator.AppendPredecessor(operator, s.WithPredecessorContext("search: "+i.UnaryStr))
			return nil
		})
		s.stackPush(next)
		s.debugSubLog("<< push next")
		return true, nil
	case OpGetCall:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "get call instruction failed: stack top is empty")
		}
		value := vs
		var results Values
		if trackErr := s.track("value-op:GetCalled", func() error {
			done := s.startValueOpTiming("GetCalled")
			defer done()
			results, err = value.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetCalled()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Errorf("get calling instruction failed: %s", err)
		}
		if err != nil {
			s.debugSubLog("error: %v", err)
			s.debugSubLog("recover origin value")
			s.stackPush(nil)
			s.debugSubLog("<< push")
			return true, err
		}
		callLen := len(results)
		s.debugSubLog("<< push len: %v", callLen)
		s.stackPush(results)
		return true, nil

	case OpGetCallArgs:
		s.debugSubLog("-- getCallArgs pop call args")
		//in iterStack
		vs, err := s.stackPeek()
		if err != nil {
			return true, utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		value := vs
		var results Values
		if trackErr := s.track("value-op:GetCallActualParams", func() error {
			done := s.startValueOpTiming("GetCallActualParams")
			defer done()
			results, err = value.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetCallActualParams(i.UnaryInt, i.UnaryBool)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := len(results)
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")
		s.stackPush(results)
		return true, nil

	case OpGetUsers:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		value := vs
		s.debugSubLog("- call GetUser")

		// diagnostics: track value operation timing
		var vals Values
		if trackErr := s.track("value-op:GetSyntaxFlowUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowUse")
			defer done()
			vals, err = value.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowUse()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowUse() failed: %v", err)
		}
		s.stackPush(vals)
		return true, nil
	case OpGetBottomUsers:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")

		// diagnostics: track value operation timing
		var vals Values
		if trackErr := s.track("value-op:GetSyntaxFlowBottomUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowBottomUse")
			defer done()
			vals, err = vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowBottomUse(s.result, s.config, i.SyntaxFlowConfig...)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.stackPush(vals)
		return true, nil
	case OpGetDefs:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetDefs")
		var vals Values
		if trackErr := s.track("value-op:GetSyntaxFlowDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowDef")
			defer done()
			vals, err = vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowDef()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowDef() failed: %v", err)
		}
		s.stackPush(vals)
		return true, nil
	case OpGetTopDefs:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		s.debugSubLog("- call TopDefs")
		s.ProcessCallback("get topdef %v(%v)", len(vs), i.SyntaxFlowConfig)

		// diagnostics: track value operation timing
		var vals Values
		if trackErr := s.track("value-op:GetSyntaxFlowTopDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowTopDef")
			defer done()
			vals, err = vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowTopDef(s.result, s.config, i.SyntaxFlowConfig...)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.stackPush(vals)
		return true, nil
	default:
		return false, nil
	}
}

// execSyntaxFlowOp handles syntax flow and stack operations
func (s *SFFrame) execSyntaxFlowOp(i *SFI) (bool, error) {
	switch i.OpCode {
	case OpDuplicate:
		if s.stack.Len() == 0 {
			return true, utils.Wrap(CriticalError, "stack top is empty")
		}
		s.debugSubLog(">> duplicate (stack grow)")
		valList, err := s.stackPeek()
		if err != nil {
			return true, err
		}
		s.stackPush(valList)
		return true, nil
	case OpPopDuplicate:
		vs, err := s.stackPeek()
		if err != nil {
			return true, err
		}
		if vs == nil {
			return true, utils.Wrap(CriticalError, "pop duplicate failed: stack top is empty")
		}
		s.stackPush(vs)
		return true, nil
	case OpPop:
		vs, err := s.opPop(true)
		if err != nil {
			return true, err
		}
		s.stackPush(vs)
		return true, nil
	case OpNewRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return true, utils.Errorf("new ref failed: empty name")
		}
		s.debugSubLog(">> from ref: %v ", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		if ok {
			if vs.IsEmpty() {
				return true, utils.Errorf("new ref failed: empty value: %v", i.UnaryStr)
			}
			s.debugSubLog(">> get value: %v ", vs)
			s.stackPush(vs)
			return true, nil
		} else {
			vs := NewEmptyValues()
			s.result.SymbolTable.Set(i.UnaryStr, vs)
			s.stackPush(vs)
		}
		return true, nil
	case OpUpdateRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return true, utils.Errorf("update ref failed: empty name")
		}
		s.debugSubLog(">> pop")
		vs, err := s.opPop(false)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return true, err
		}
		if vs == nil {
			return true, utils.Error("BUG: get top defs failed, empty stack")
		}
		err = s.output(i.UnaryStr, vs)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return true, err
		}
		s.debugSubLog(">> save $%s [%v]", i.UnaryStr, len(vs))
		return true, nil
	case OpAddDescription:
		if i.UnaryStr == "" {
			return true, utils.Errorf("add description failed: empty name")
		}
		ret := i.ValueByIndex(1)
		if ret == "" {
			ret = i.ValueByIndex(0)
		}
		s.result.Description.Set(i.UnaryStr, ret)
		if ret != "" {
			s.debugSubLog("- key: %v, value: %v", i.UnaryStr, ret)
		} else {
			s.debugSubLog("- key: %v", i.UnaryStr)
		}
		return true, nil
	case OpAlert:
		if i.UnaryStr == "" {
			return true, utils.Errorf("echo failed: empty name")
		}
		value, ok := s.GetSymbol(i)
		if !ok || value == nil {
			return true, utils.Errorf("alert failed: not found: %v", i.UnaryStr)
		}
		//m := s.result.rule.AlertDesc[i.UnaryStr]
		//m := s.result.AlertMsgTable[i.UnaryStr]
		//lo.ForEach(i.SyntaxFlowConfig, func(item *RecursiveConfigItem, index int) {
		//	if m == nil || len(m) == 0 {
		//		m = make(map[string]string)
		//	}
		//	m[item.Key] = item.Value
		//})
		s.result.AlertSymbolTable.Set(i.UnaryStr, value)
		//alStr := i.ValueByIndex(0)
		//if alStr != "" {
		//	m["__extra__"] = alStr
		//}
		return true, nil
	case OpCheckParams:
		if i.UnaryStr == "" {
			return true, utils.Errorf("check params failed: empty name")
		}

		s.debugSubLog("- check: $%v", i.UnaryStr)

		var thenStr = i.ValueByIndex(0)
		var elseStr = i.ValueByIndex(1)
		if elseStr == "" {
			elseStr = "$" + i.UnaryStr + " is not found"
		}

		haveResult := false

		results, ok := s.GetSymbol(i)
		if !ok {
			haveResult = false
		} else if results == nil {
			haveResult = false
		} else {
			_ = results.ForEach(func(operator ValueOperator) error {
				return nil
			})
			if haveResult {
				s.result.CheckParams = append(s.result.CheckParams, i.UnaryStr)
				if thenStr != "" {
					s.result.Description.Set("$"+i.UnaryStr, thenStr)
				}
			} else {
				s.debugSubLog("-   error: " + elseStr)
				s.result.Errors = append(s.result.Errors, elseStr)
				if s.config.FailFast {
					return true, utils.Wrapf(AbortError, "check params failed: %v", elseStr)
				}
			}
		}

		if !haveResult {
			s.debugSubLog("-   error: " + elseStr)
			s.result.Errors = append(s.result.Errors, elseStr)
			if s.config.FailFast {
				return true, utils.Wrapf(AbortError, "check params failed: %v", elseStr)
			}
		} else {
			s.result.CheckParams = append(s.result.CheckParams, i.UnaryStr)
			if thenStr != "" {
				s.result.Description.Set("$"+i.UnaryStr, thenStr)
			}
		}
		return true, nil
	case OpMergeRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		if !ok || vs == nil {
			s.debugLog("cannot find $%v", i.UnaryStr)
			return true, nil
		}
		s.debugSubLog(">> pop")
		value, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		ret, err := value.Merge(vs)
		if err != nil {
			return true, utils.Wrapf(CriticalError, "merge failed: %v", err)
		}
		s.stackPush(ret)
		s.debugSubLog("<< push")
		return true, nil
	case OpRemoveRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		if !ok || vs == nil {
			s.debugLog("cannot find $%v", i.UnaryStr)
			return true, nil
		}
		s.debugSubLog(">> pop")
		value, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		_ = value
		// ret, err := value.Remove(vs)
		// if err != nil {
		// 	return true, utils.Wrapf(CriticalError, "remove failed: %v", err)
		// }
		// s.stackPush(ret)
		s.debugSubLog("<< push")
		return true, nil
	case OpIntersectionRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		//vs, ok := s.result.SymbolTable.Get(i.UnaryStr)
		if vs == nil || !ok {
			s.debugLog("cannot find $%v", i.UnaryStr)
			valList, err := s.stackPop()
			if err != nil {
				return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
			}
			_ = valList
			s.stackPush(nil)
			return true, nil
		}
		s.debugLog(">> pop")
		m1 := make(map[int64]ValueOperator, len(vs))
		_ = vs.ForEach(func(operator ValueOperator) error {
			id, ok := fetchId(operator)
			if ok {
				m1[id] = operator
			}
			return nil
		})
		// s.debugSubLog("map: %v", lo.Keys(m1))

		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}

		var buf bytes.Buffer
		var vals []ValueOperator
		_ = vs.ForEach(func(operator ValueOperator) error {
			id, ok := fetchId(operator)
			if ok {
				if _, ok := m1[id]; ok {
					buf.WriteString(fmt.Sprintf(" %v", id))
					vals = append(vals, operator)
				}
			}
			return nil
		})
		if len(vals) == 0 {
			s.debugSubLog("no intersection")
			s.stackPush(nil)
		} else {
			s.debugSubLog("intersection:%v", buf.String())
			s.stackPush(vals)
		}
		return true, nil
	case OpNativeCall:
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "native call failed: stack top is empty")
		}

		s.debugSubLog("native call: [%v]", i.UnaryStr)
		call, err := GetNativeCall(i.UnaryStr)
		if err != nil {
			s.debugSubLog("Err: %v", err)
			log.Errorf("native call failed, not an existed native call[%v]: %v", i.UnaryStr, err)
			s.stack.Push(nil)
			return true, utils.Errorf("get native call failed: %v", err)
		}
		ret, err := vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
			ok, ret, err := call(vo, s, NewNativeCallActualParams(i.SyntaxFlowConfig...))
			_ = ok
			if err != nil {
				return nil, err
			}
			return ret, nil
		})
		if err != nil {
			return true, utils.Errorf("get native call failed: %v", err)
		}
		s.stackPush(ret)
		return true, nil
	case OpFileFilterJsonPath, OpFileFilterReg, OpFileFilterXpath:
		opcode2strMap := map[SFVMOpCode]string{
			OpFileFilterJsonPath: "jsonpath",
			OpFileFilterReg:      "regexp",
			OpFileFilterXpath:    "xpath",
		}
		s.debugSubLog(">> pop")
		vs, err := s.stackPop()
		if err != nil {
			return true, utils.Wrap(CriticalError, "native call failed: stack top is empty")
		}
		s.debugSubLog(">> pop file name: %v", i.UnaryStr)
		name := i.UnaryStr
		if name == "" {
			return true, utils.Errorf("file filter failed: file name is empty")
		}
		paramList := i.Values
		paramMap := i.FileFilterMethodItem
		strOpcode := opcode2strMap[i.OpCode]
		var ret Values
		if trackErr := s.track("value-op:FileFilter", func() error {
			done := s.startValueOpTiming("FileFilter")
			defer done()
			ret, err = vs.pipeLineRun(func(vo ValueOperator) (Values, error) {
				return vo.FileFilter(name, strOpcode, paramMap, paramList)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("file filter failed: %v", err)
		}
		s.stackPush(ret)
		return true, nil
	case OpPushNumber:
		return true, utils.Errorf("unimplement")
		// s.debugSubLog(">> peek")
		// valList, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrapf(CriticalError, "BUG: pushNumber: stack top is empty")
		// }
		// var val Values
		// if trackErr := s.track("value-op:NewConst", func() error {
		// 	done := s.startValueOpTiming("NewConst")
		// 	defer done()
		// 	res := vs.NewConst(i.UnaryInt)
		// 	if vl, ok := res.(*ValueList); ok {
		// 		val = vl.Values
		// 	} else if !res.IsEmpty() {
		// 		val = Values{res}
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// if len(val) > 0 {
		// 	s.debugSubLog(">> push: %v", len(val))
		// 	s.stackPush(val)
		// }
		// return true, nil
	case OpPushBool:
		return true, utils.Errorf("unimplement")
		// s.debugSubLog(">> peek")
		// valList, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrapf(CriticalError, "BUG: pushBool: stack top is empty")
		// }
		// vs := NewValueList(valList)
		// var val Values
		// if trackErr := s.track("value-op:NewConst", func() error {
		// 	done := s.startValueOpTiming("NewConst")
		// 	defer done()
		// 	res := vs.NewConst(i.UnaryBool)
		// 	if vl, ok := res.(*ValueList); ok {
		// 		val = vl.Values
		// 	} else if !res.IsEmpty() {
		// 		val = Values{res}
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// if len(val) > 0 {
		// 	s.debugSubLog(">> push: %v", len(val))
		// 	s.stackPush(val)
		// }
		return true, nil
	case OpPushString:
		return true, utils.Errorf("unimplement")
		// s.debugSubLog(">> peek")
		// vs, err := s.stackPeek()
		// if err != nil {
		// 	return true, utils.Wrapf(CriticalError, "BUG: pushString: stack top is empty")
		// }
		// var val Values
		// if trackErr := s.track("value-op:NewConst", func() error {
		// 	done := s.startValueOpTiming("NewConst")
		// 	defer done()
		// 	res := vs.NewConst(i.UnaryStr)
		// 	if vl, ok := res.(*ValueList); ok {
		// 		val = vl.Values
		// 	} else if !res.IsEmpty() {
		// 		val = Values{res}
		// 	}
		// 	return nil
		// }); trackErr != nil {
		// 	return true, trackErr
		// }
		// if len(val) > 0 {
		// 	s.debugSubLog(">> push: %v", len(val))
		// 	s.stackPush(val)
		// }
		// return true, nil
	default:
		return false, nil
	}
}
