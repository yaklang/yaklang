package sfvm

// frame_exec.go
// This file contains the execution functions for SyntaxFlow Virtual Machine.
// It implements three categories of operations:
// 1. execFilterAndCondition: Handles condition/logic/comparison operations
//    - OpEmptyCompare, OpCompare*, OpVersionIn
//    - OpEq/Ne/Gt/GtEq/Lt/LtEq
//    - OpLogic*, OpCondition
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
	"errors"
	"fmt"
	"regexp"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func recursiveDeepChain(frame *SFFrame, element Values, handle func(operator ValueOperator) (bool, error), visited map[int64]struct{}) error {
	if visited == nil {
		visited = make(map[int64]struct{})
	}

	var next []ValueOperator

	val, err := RunValueOperatorPipeline(element, ValuePipelineOptions{Frame: frame}, func(operator ValueOperator) (Values, error) {
		return operator.GetCalled()
	})
	if err != nil {
		return err
	}
	if !val.IsEmpty() {
		if err := val.Recursive(func(operator ValueOperator) error {
			if idGetter, ok := operator.(ssa.GetIdIF); ok {
				if _, ok := visited[idGetter.GetId()]; ok {
					return nil
				}
				visited[idGetter.GetId()] = struct{}{}

				fields, err := RunValueOperatorPipeline(ValuesOf(operator), ValuePipelineOptions{Frame: frame}, func(vo ValueOperator) (Values, error) {
					return vo.GetFields()
				})
				if err != nil {
					return err
				}
				if !fields.IsEmpty() {
					if err := fields.Recursive(func(fieldElement ValueOperator) error {
						if idGetter, ok := fieldElement.(ssa.GetIdIF); ok {
							if _, ok := visited[idGetter.GetId()]; ok {
								return nil
							}
							visited[idGetter.GetId()] = struct{}{}

							stop, err := handle(fieldElement)
							if err != nil {
								return err
							}
							if !stop {
								next = append(next, fieldElement)
							}
						}
						return nil
					}); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	val, err = RunValueOperatorPipeline(element, ValuePipelineOptions{Frame: frame}, func(operator ValueOperator) (Values, error) {
		return operator.GetFields()
	})
	if err != nil {
		return err
	}
	if !val.IsEmpty() {
		if err := val.Recursive(func(operator ValueOperator) error {
			if idGetter, ok := operator.(ssa.GetIdIF); ok {
				if _, ok := visited[idGetter.GetId()]; ok {
					return nil
				}
				visited[idGetter.GetId()] = struct{}{}

				stop, err := handle(operator)
				if err != nil {
					return err
				}
				if !stop {
					next = append(next, operator)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if len(next) <= 0 {
		return nil
	}

	nextValues := NewValues(next)
	return recursiveDeepChain(frame, nextValues, handle, visited)
}

func (s *SFFrame) recursiveSearch(value Values, timingName string, label string, match func(ValueOperator) (Values, error)) (Values, error) {
	var next []ValueOperator
	err := recursiveDeepChain(s, value, func(operator ValueOperator) (bool, error) {
		done := s.startValueOpTiming(timingName)
		defer done()

		results, err := s.runValueOperatorPipeline(ValuesOf(operator), "recursive search "+label, func(vo ValueOperator) (Values, error) {
			return match(vo)
		})
		if err != nil {
			return false, err
		}
		if results.IsEmpty() {
			return false, nil
		}

		have := false
		_ = results.Recursive(func(item ValueOperator) error {
			if _, ok := item.(ssa.GetIdIF); ok {
				have = true
				return utils.Error("normal abort")
			}
			return nil
		})

		next = append(next, results...)
		return have, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return NewValues(next), nil
}

func (s *SFFrame) opPop(unName bool) (Values, error) {
	if s.stack.Len() == 0 {
		s.debugSubLog(">> pop Error: empty stack")
		return nil, utils.Errorf("E: stack is empty, cannot pop")
	}
	i := s.stack.Pop()
	s.popStack.Push(i)
	s.debugSubLog(">> pop %v", ValuesLen(i))
	if unName {
		s.debugSubLog("save-to $_")
		err := s.output("_", i)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return nil, utils.Errorf("output '_' error: %v", err)
		}
	}
	return i, nil
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
func (s *SFFrame) execFilterAndCondition(i *SFI) (bool, error) {
	switch i.OpCode {
	case OpEmptyCompare:
		vals := s.stack.Peek()
		if vals == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		var flag []bool
		vals.Recursive(func(operator ValueOperator) error {
			flag = append(flag, true)
			return nil
		})
		if err := s.pushCondition(flag, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpCompareOpcode:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
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
			newVal, condition = values.CompareOpcode(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		// Compare only produces condition mask; keep source value stack unchanged in shape.
		s.pushStack(values)
		if err := s.pushCondition(condition, newVal, true); err != nil {
			return true, err
		}
		return true, nil
	case OpCompareString:
		s.debugSubLog(">> pop")
		//pop到原值
		values := s.stack.Pop()
		if values == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}

		mode := ValidStringMatchMode(i.UnaryInt)
		if mode == -1 {
			return true, utils.Wrapf(CriticalError, "compare string failed: invalid mode %v", mode)
		}

		comparator := NewStringComparator(mode, s.GetContext())
		if len(i.Values) != len(i.MultiOperator) {
			if err := s.pushCondition([]bool{false}, nil, false); err != nil {
				return true, err
			}
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
			newVal, condition = values.CompareString(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		// Compare only produces condition mask; keep source value stack unchanged in shape.
		s.pushStack(values)
		if err := s.pushCondition(condition, newVal, true); err != nil {
			return true, err
		}
		return true, nil
	case OpVersionIn:
		value := s.stack.Peek()
		if value == nil {
			return true, utils.Wrap(CriticalError, "compare version failed: stack top is empty")
		}
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
		_ = value.Recursive(func(v ValueOperator) error {
			ok, _, _ := call(ValuesOf(v), s, params)
			res = append(res, ok)
			return nil
		})
		if err := s.pushCondition(res, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionEqual)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(Equal)", func() error {
			done := s.startValueOpTiming("CompareConst(Equal)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpNotEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionNotEqual)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(NotEqual)", func() error {
			done := s.startValueOpTiming("CompareConst(NotEqual)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpGt:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionGt)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(Gt)", func() error {
			done := s.startValueOpTiming("CompareConst(Gt)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpGtEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionGtEq)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(GtEq)", func() error {
			done := s.startValueOpTiming("CompareConst(GtEq)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpLt:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionLt)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(Lt)", func() error {
			done := s.startValueOpTiming("CompareConst(Lt)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpLtEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionLtEq)
		var conds []bool
		if trackErr := s.track("value-op:CompareConst(LtEq)", func() error {
			done := s.startValueOpTiming("CompareConst(LtEq)")
			defer done()
			conds = vs2.CompareConst(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if err := s.pushCondition(conds, nil, false); err != nil {
			return true, err
		}
		return true, nil
	case OpLogicBang:
		if err := s.applyLogicBangCondition(); err != nil {
			return true, err
		}
		return true, nil
	case OpLogicAnd:
		if err := s.applyLogicBinaryCondition(true); err != nil {
			return true, err
		}
		return true, nil
	case OpLogicOr:
		if err := s.applyLogicBinaryCondition(false); err != nil {
			return true, err
		}
		return true, nil
	case OpCondition:
		s.debugSubLog(">> pop")
		vs := s.stack.Pop()
		if vs == nil {
			return true, utils.Wrap(CriticalError, "condition failed: stack top is empty")
		}
		entry := s.popCondition()
		if entry == nil {
			return true, utils.Wrap(CriticalError, "condition failed: empty condition stack")
		}
		filtered, err := entry.Apply(vs)
		if err != nil {
			return true, err
		}
		// Keep condition output bitvector unchanged; it will be consumed by filter.
		s.stack.Push(filtered)
		return true, nil
	case OpFilter:
		s.debugSubLog(">> pop condition values")
		cond := s.stack.Pop()
		if cond == nil {
			return true, utils.Wrap(CriticalError, "filter condition failed: empty condition")
		}
		s.debugSubLog(">> peek source values")
		source := s.stack.Peek()
		if source == nil {
			return true, utils.Wrap(CriticalError, "filter condition failed: empty source")
		}
		_ = source
		if err := s.pushFilterCondition(cond); err != nil {
			return true, err
		}
		return true, nil
	default:
		return false, nil
	}
}

// execValueFilter handles ValueOperator navigation/search operations
func (s *SFFrame) execValueFilter(i *SFI) (bool, error) {
	switch i.OpCode {
	case OpPushSearchExact:
		s.debugSubLog(">> pop match exactly: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Errorf("search exact failed: stack top is empty")
		}
		mod := ssadb.MatchMode(i.UnaryInt)
		if !s.config.StrictMatch {
			mod |= ssadb.KeyMatch
		}

		// diagnostics: track value operation timing
		var result bool
		var next Values
		var err error
		if trackErr := s.track("value-op:ExactMatch", func() error {
			done := s.startValueOpTiming("ExactMatch")
			defer done()
			next, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				_, matched, err := vo.ExactMatch(s.GetContext(), mod, i.UnaryStr)
				return matched, err
			})
			result = !next.IsEmpty()
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			err = utils.Wrapf(err, "search exact failed")
		}
		if !result {
			err = utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
		}

		s.debugSubLog("result next: %v", ValuesLen(next))
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search "+i.UnaryStr))
		s.pushStack(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return true, err
		}
		return true, nil
	case OpRecursiveSearchExact:
		s.debugSubLog(">> pop recursive search exactly: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "recursive search exact failed: stack top is empty")
		}

		results, err := s.recursiveSearch(value, "RecursiveExactMatch", i.UnaryStr, func(operator ValueOperator) (Values, error) {
			_, matched, err := operator.ExactMatch(s.GetContext(), ssadb.BothMatch, i.UnaryStr)
			return matched, err
		})
		if err != nil {
			err = utils.Wrapf(err, "recursive search exact failed")
		}
		s.debugSubLog("result next: %v", ValuesLen(results))
		s.pushStack(results)
		s.debugSubLog("<< push next")
		return true, nil
	case OpRecursiveSearchGlob:
		s.debugSubLog(">> pop recursive search glob: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "recursive search glob failed: stack top is empty")
		}

		mod := ssadb.MatchMode(i.UnaryInt)

		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}
		_ = globIns

		results, err := s.recursiveSearch(value, "RecursiveGlobMatch", i.UnaryStr, func(operator ValueOperator) (Values, error) {
			_, matched, err := operator.GlobMatch(s.GetContext(), mod|ssadb.NameMatch, i.UnaryStr)
			return matched, err
		})
		if err != nil {
			err = utils.Wrapf(err, "recursive search glob failed")
			s.debugSubLog("ERROR: %v", err)
		}
		s.debugSubLog("result next: %v", ValuesLen(results))
		s.pushStack(results)
		s.debugSubLog("<< push next")
		return true, nil
	case OpRecursiveSearchRegexp:
		s.debugSubLog(">> pop recursive search regexp: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "recursive search regexp failed: stack top is empty")
		}
		mod := ssadb.MatchMode(i.UnaryInt)

		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			return true, utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		_ = regexpIns

		results, err := s.recursiveSearch(value, "RecursiveRegexpMatch", i.UnaryStr, func(operator ValueOperator) (Values, error) {
			_, matched, err := operator.RegexpMatch(s.GetContext(), mod|ssadb.NameMatch, i.UnaryStr)
			return matched, err
		})
		if err != nil {
			err = utils.Wrapf(err, "recursive search regexp failed")
			s.debugSubLog("ERROR: %v", err)
		}
		s.debugSubLog("result next: %v", ValuesLen(results))
		s.pushStack(results)
		s.debugSubLog("<< push next")
		return true, nil
	case OpPushSearchGlob:
		s.debugSubLog(">> pop search glob: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "search glob failed: stack top is empty")
		}
		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}
		_ = globIns

		mod := ssadb.MatchMode(i.UnaryInt)
		if !s.config.StrictMatch {
			mod |= ssadb.KeyMatch
		}
		var result bool
		var next Values
		if trackErr := s.track("value-op:GlobMatch", func() error {
			done := s.startValueOpTiming("GlobMatch")
			defer done()
			next, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				_, matched, err := vo.GlobMatch(s.GetContext(), mod, i.UnaryStr)
				return matched, err
			})
			result = !next.IsEmpty()
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Wrapf(err, "search glob failed")
		}
		if !result {
			err = utils.Errorf("search glob failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", ValuesLen(next))
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search: "+i.UnaryStr))
		s.pushStack(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return true, err
		}
		return true, nil
	case OpPushSearchRegexp:
		s.debugSubLog(">> pop search regexp: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "search regexp failed: stack top is empty")
		}
		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		mod := ssadb.MatchMode(i.UnaryInt)
		if !s.config.StrictMatch {
			mod |= ssadb.KeyMatch
		}
		var result bool
		var next Values
		if trackErr := s.track("value-op:RegexpMatch", func() error {
			done := s.startValueOpTiming("RegexpMatch")
			defer done()
			next, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				_, matched, err := vo.RegexpMatch(s.GetContext(), mod, regexpIns.String())
				return matched, err
			})
			result = !next.IsEmpty()
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Wrap(err, "search regexp failed")
		}
		if !result {
			err = utils.Errorf("search regexp failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", ValuesLen(next))
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search: "+i.UnaryStr))
		s.pushStack(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return true, err
		}
		return true, nil
	case OpGetCall:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get call instruction failed: stack top is empty")
		}
		var results Values
		var err error
		if trackErr := s.track("value-op:GetCalled", func() error {
			done := s.startValueOpTiming("GetCalled")
			defer done()
			results, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				return vo.GetCalled()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			err = utils.Errorf("get calling instruction failed: %s", err)
			s.debugSubLog("error: %v", err)
			s.debugSubLog("recover origin value")
			s.pushStack(NewEmptyValues())
			s.debugSubLog("<< push")
			return true, err
		}
		callLen := ValuesLen(results)
		s.debugSubLog("<< push len: %v", callLen)
		s.pushStack(results)
		return true, nil

	case OpGetCallArgs:
		s.debugSubLog("-- getCallArgs pop call args")
		value := s.stack.Peek()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		var results Values
		var err error
		if trackErr := s.track("value-op:GetCallActualParams", func() error {
			done := s.startValueOpTiming("GetCallActualParams")
			defer done()
			results, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				return vo.GetCallActualParams(i.UnaryInt, i.UnaryBool)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := ValuesLen(results)
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")

		s.pushStack(results)
		return true, nil

	case OpGetUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetUser")

		// diagnostics: track value operation timing
		var vals Values
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowUse")
			defer done()
			vals, err = s.runValueOperatorPipeline(value, "getUser", func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowUse()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowUse() failed: %v", err)
		}
		s.debugSubLog("<< push users")
		s.pushStack(vals)
		return true, nil
	case OpGetBottomUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")

		// diagnostics: track value operation timing
		var vals Values
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowBottomUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowBottomUse")
			defer done()
			vals, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowBottomUse(s.result, s.config, i.SyntaxFlowConfig...)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.debugSubLog("<< push bottom uses %v", ValuesLen(vals))
		s.pushStack(vals)
		return true, nil
	case OpGetDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetDefs")
		var vals Values
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowDef")
			defer done()
			vals, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowDef()
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowDef() failed: %v", err)
		}
		s.debugSubLog("<< push users %v", ValuesLen(vals))
		s.pushStack(vals)
		return true, nil
	case OpGetTopDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		s.debugSubLog("- call TopDefs")
		s.ProcessCallback("get topdef %v(%v)", ValuesLen(value), i.SyntaxFlowConfig)

		// diagnostics: track value operation timing
		var vals Values
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowTopDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowTopDef")
			defer done()
			vals, err = s.runValueOperatorPipeline(value, "", func(vo ValueOperator) (Values, error) {
				return vo.GetSyntaxFlowTopDef(s.result, s.config, i.SyntaxFlowConfig...)
			})
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.debugSubLog("<< push top defs %v", ValuesLen(vals))
		s.pushStack(vals)
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
		v := s.stack.Peek()
		s.pushStack(v)
		return true, nil
	case OpPopDuplicate:
		val := s.popStack.Peek()
		if val == nil {
			log.Errorf("pop duplicate failed: stack top is empty")
			return true, nil
		}
		s.stack.Push(val)
		return true, nil
	case OpPop:
		if _, err := s.opPop(true); err != nil {
			return true, err
		}
		return true, nil
	case OpNewRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return true, utils.Errorf("new ref failed: empty name")
		}
		s.debugSubLog(">> from ref: %v ", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		if ok {
			if vs == nil {
				return true, utils.Errorf("new ref failed: empty value: %v", i.UnaryStr)
			}
			var operator0 ValueOperator
			count := 0
			vs.Recursive(func(operator ValueOperator) error {
				if count == 0 {
					operator0 = operator
				}
				count++
				return nil
			})
			_ = operator0
			s.debugSubLog(">> get value: %v ", vs)
			s.pushStack(vs)
		} else {
			values := NewEmptyValues()
			s.result.SymbolTable.Set(i.UnaryStr, values)
			s.pushStack(values)
			return true, nil
			//return utils.Errorf("new ref failed: not found: %v", i.UnaryStr)
		}
		return true, nil
	case OpUpdateRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return true, utils.Errorf("update ref failed: empty name")
		}
		s.debugSubLog(">> pop")
		value, err := s.opPop(false)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return true, err
		}
		if value == nil {
			return true, utils.Error("BUG: get top defs failed, empty stack")
		}
		err = s.output(i.UnaryStr, value)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return true, err
		}
		s.debugSubLog(">> save $%s [%v]", i.UnaryStr, ValuesLen(value))
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
			_ = results.Recursive(func(operator ValueOperator) error {
				if _, ok := operator.(ssa.GetIdIF); ok {
					haveResult = true
					return utils.Error("abort")
				}
				return nil
			})
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
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		val := MergeValues(value, vs)
		s.pushStack(val)
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
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		newVal := RemoveValues(value, vs)
		s.pushStack(newVal)
		s.debugSubLog("<< push")
		return true, nil
	case OpIntersectionRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		//vs, ok := s.result.SymbolTable.Get(i.UnaryStr)
		if vs == nil || !ok {
			s.debugLog("cannot find $%v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
			}
			s.pushStack(NewEmptyValues())
			return true, nil
		}
		s.debugLog(">> pop")
		m1 := make(map[int64]ValueOperator, ValuesLen(vs))
		_ = vs.Recursive(func(operator ValueOperator) error {
			id, ok := fetchId(operator)
			if ok {
				m1[id] = operator
			}
			return nil
		})
		// s.debugSubLog("map: %v", lo.Keys(m1))

		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}

		var buf bytes.Buffer
		var vals []ValueOperator
		_ = value.Recursive(func(operator ValueOperator) error {
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
			s.pushStack(NewEmptyValues())
		} else {
			s.debugSubLog("intersection:%v", buf.String())
			s.pushStack(NewValues(vals))
		}
		return true, nil
	case OpNativeCall:
		ruleLabel := ""
		if s.rule != nil {
			if s.rule.Title != "" {
				ruleLabel = s.rule.Title
			} else if s.rule.RuleName != "" {
				ruleLabel = s.rule.RuleName
			}
		}
		name := "sfvm.nativecall:" + i.UnaryStr
		if ruleLabel != "" {
			name += ":" + ruleLabel
		}
		var (
			value Values
			ret   Values
			ok    bool
		)
		if trackErr := diagnostics.TrackLow(name, func() error {
			s.debugSubLog(">> pop")
			value = s.stack.Pop()
			if value == nil {
				return utils.Wrap(CriticalError, "native call failed: stack top is empty")
			}

			s.debugSubLog("native call: [%v]", i.UnaryStr)
			call, err := GetNativeCall(i.UnaryStr)
			if err != nil {
				s.debugSubLog("Err: %v", err)
				log.Errorf("native call failed, not an existed native call[%v]: %v", i.UnaryStr, err)
				s.pushStack(NewEmptyValues())
				return utils.Errorf("get native call failed: %v", err)
			}

			ok, ret, err = call(value, s, NewNativeCallActualParams(i.SyntaxFlowConfig...))
			if err != nil || !ok {
				s.debugSubLog("No Result in [%v]", i.UnaryStr)
				s.pushStack(NewEmptyValues())
				if errors.Is(err, CriticalError) {
					return err
				}
				return utils.Errorf("get native call failed: %v", err)
			}
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		s.debugSubLog("<< push: %v", ValuesLen(ret))
		s.pushStack(ret)
		return true, nil
	case OpFileFilterJsonPath, OpFileFilterReg, OpFileFilterXpath:
		opcode2strMap := map[SFVMOpCode]string{
			OpFileFilterJsonPath: "jsonpath",
			OpFileFilterReg:      "regexp",
			OpFileFilterXpath:    "xpath",
		}
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
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
		var res Values
		var err error
		if trackErr := s.track("value-op:FileFilter", func() error {
			done := s.startValueOpTiming("FileFilter")
			defer done()
			res, err = value.FileFilter(name, strOpcode, paramMap, paramList)
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("file filter failed: %v", err)
		}
		s.pushStack(res)
		return true, nil
	case OpPushNumber:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return true, utils.Wrapf(CriticalError, "BUG: pushNumber: stack top is empty")
		}
		var val ValueOperator
		if trackErr := s.track("value-op:NewConst", func() error {
			done := s.startValueOpTiming("NewConst")
			defer done()
			val = vs.NewConst(i.UnaryInt)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if utils.IsNil(val) {
			s.pushStack(NewEmptyValues())
			return true, nil
		}
		next := ValuesOf(val)
		s.debugSubLog(">> push: %v", ValuesLen(next))
		s.pushStack(next)
		return true, nil
	case OpPushBool:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return true, utils.Wrapf(CriticalError, "BUG: pushBool: stack top is empty")
		}
		var val ValueOperator
		if trackErr := s.track("value-op:NewConst", func() error {
			done := s.startValueOpTiming("NewConst")
			defer done()
			val = vs.NewConst(i.UnaryBool)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if utils.IsNil(val) {
			s.pushStack(NewEmptyValues())
			return true, nil
		}
		next := ValuesOf(val)
		s.debugSubLog(">> push: %v", ValuesLen(next))
		s.pushStack(next)
		return true, nil
	case OpPushString:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return true, utils.Wrapf(CriticalError, "BUG: pushString: stack top is empty")
		}
		var val ValueOperator
		if trackErr := s.track("value-op:NewConst", func() error {
			done := s.startValueOpTiming("NewConst")
			defer done()
			val = vs.NewConst(i.UnaryStr)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		if utils.IsNil(val) {
			s.pushStack(NewEmptyValues())
			return true, nil
		}
		next := ValuesOf(val)
		s.debugSubLog(">> push: %v", ValuesLen(next))
		s.pushStack(next)
		return true, nil
	default:
		return false, nil
	}
}
