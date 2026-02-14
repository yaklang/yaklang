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
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func recursiveDeepChain(element ValueOperator, handle func(operator ValueOperator) bool, visited map[int64]struct{}) error {
	if visited == nil {
		visited = make(map[int64]struct{})
	}

	var next []ValueOperator

	val, _ := element.GetCalled()
	if val != nil {
		_ = val.Recursive(func(operator ValueOperator) error {
			if idGetter, ok := operator.(ssa.GetIdIF); ok {
				if _, ok := visited[idGetter.GetId()]; ok {
					return nil
				}
				visited[idGetter.GetId()] = struct{}{}

				fields, _ := operator.GetFields()
				if fields != nil {
					_ = fields.Recursive(func(fieldElement ValueOperator) error {
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
	}

	val, _ = element.GetFields()
	if val != nil {
		_ = val.Recursive(func(operator ValueOperator) error {
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

	if len(next) <= 0 {
		return nil
	}

	nextValues := NewValues(next)
	return recursiveDeepChain(nextValues, handle, visited)
}

func (s *SFFrame) opPop(unName bool) (ValueOperator, error) {
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
		s.conditionStack.Push(flag)
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

		var newVal ValueOperator
		var condition []bool
		if trackErr := s.track("value-op:CompareOpcode", func() error {
			done := s.startValueOpTiming("CompareOpcode")
			defer done()
			newVal, condition = values.CompareOpcode(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		s.stack.Push(newVal)
		s.conditionStack.Push(condition)
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
			s.conditionStack.Push([]bool{false})
			return true, utils.Wrapf(CriticalError, "sfi values or mutiOperator out size %v", len(i.Values))
		}
		for index, v := range i.Values {
			comparator.AddCondition(v, ValidConditionFilter(i.MultiOperator[index]))
		}
		var newVal ValueOperator
		var condition []bool
		if trackErr := s.track("value-op:CompareString", func() error {
			done := s.startValueOpTiming("CompareString")
			defer done()
			newVal, condition = values.CompareString(comparator)
			return nil
		}); trackErr != nil {
			return true, trackErr
		}
		s.stack.Push(newVal)
		s.conditionStack.Push(condition)
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
			ok, _, _ := call(v, s, params)
			res = append(res, ok)
			return nil
		})
		s.conditionStack.Push(res)
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
		s.conditionStack.Push(conds)
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
		s.conditionStack.Push(conds)
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
		s.conditionStack.Push(conds)
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
		s.conditionStack.Push(conds)
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
		s.conditionStack.Push(conds)
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
		s.conditionStack.Push(conds)
		return true, nil
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
		vs := s.stack.Pop()
		if vs == nil {
			return true, utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		conds := s.conditionStack.Pop()
		if len(conds) != ValuesLen(vs) {
			return true, utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", ValuesLen(vs), len(conds))
		}
		//log.Infof("condition: %v", conds)
		res := make([]ValueOperator, 0, ValuesLen(vs))
		for i := 0; i < len(conds); i++ {
			if conds[i] {
				if v, err := vs.ListIndex(i); err == nil {
					res = append(res, v)
				}
			}
		}
		s.stack.Push(NewValues(res))
		return true, nil
	case OpCheckEmpty:
		if i.Iter == nil {
			return true, utils.Wrap(CriticalError, "check empty failed: stack top is empty")
		}
		index := i.Iter.currentIndex
		conditions := s.conditionStack.Peek()
		//如果是null
		val := s.stack.Pop()
		if len(conditions) == index+1 && !conditions[index] {
			return true, nil
		}
		conditions = s.conditionStack.Pop()
		if len(conditions) < index+1 {
			return true, utils.Errorf("check empty failed: stack top is empty")
		}
		conditions[index] = !val.IsEmpty()
		s.conditionStack.Push(conditions)
		s.popStack.Free()
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
		var next ValueOperator
		var err error
		if trackErr := s.track("value-op:ExactMatch", func() error {
			done := s.startValueOpTiming("ExactMatch")
			defer done()
			result, next, err = value.ExactMatch(s.GetContext(), mod, i.UnaryStr)
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
		s.stack.Push(next)
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
		var next []ValueOperator
		err := recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveExactMatch")
			defer done()
			ok, results, _ := operator.ExactMatch(s.GetContext(), ssadb.BothMatch, i.UnaryStr)
			if ok {
				have := false
				// log.Infof("recursive search exact: %v from: %v", results.String(), operator.String())
				_ = results.Recursive(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
				next = append(next, results)
				if have {
					return true
				}
			}
			return false
		}, nil)
		if err != nil {
			err = utils.Wrapf(err, "recursive search exact failed")
		}

		results := NewValues(next)
		s.debugSubLog("result next: %v", ValuesLen(results))
		s.stack.Push(results)
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

		var next []ValueOperator
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveGlobMatch")
			defer done()
			ok, results, _ := operator.GlobMatch(s.GetContext(), mod|ssadb.NameMatch, i.UnaryStr)
			if ok {
				have := false
				_ = results.Recursive(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				next = append(next, results)
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
		results := NewValues(next)
		s.debugSubLog("result next: %v", ValuesLen(results))
		_ = results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
		s.stack.Push(results)
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

		var next []ValueOperator
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			done := s.startValueOpTiming("RecursiveRegexpMatch")
			defer done()
			//log.Infof("recursive search regexp: %v", operator.String())
			//if strings.Contains(operator.String(), "aaa") {
			//	spew.Dump(1)
			//}
			ok, results, _ := operator.RegexpMatch(s.GetContext(), mod|ssadb.NameMatch, i.UnaryStr)
			if ok {
				have := false
				_ = results.Recursive(func(operator ValueOperator) error {
					_, ok := operator.(ssa.GetIdIF)
					if ok {
						have = true
						return utils.Error("normal abort")
					}
					return nil
				})
				next = append(next, results)
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
		results := NewValues(next)
		s.debugSubLog("result next: %v", ValuesLen(results))
		_ = results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
		s.stack.Push(results)
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
		var next ValueOperator
		if trackErr := s.track("value-op:GlobMatch", func() error {
			done := s.startValueOpTiming("GlobMatch")
			defer done()
			result, next, err = value.GlobMatch(s.GetContext(), mod, i.UnaryStr)
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
		s.stack.Push(next)
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
		var next ValueOperator
		if trackErr := s.track("value-op:RegexpMatch", func() error {
			done := s.startValueOpTiming("RegexpMatch")
			defer done()
			result, next, err = value.RegexpMatch(s.GetContext(), mod, regexpIns.String())
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
		s.stack.Push(next)
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
		var results ValueOperator
		var err error
		if trackErr := s.track("value-op:GetCalled", func() error {
			done := s.startValueOpTiming("GetCalled")
			defer done()
			results, err = value.GetCalled()
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
			s.stack.Push(NewEmptyValues())
			s.debugSubLog("<< push")
			return true, err
		}
		callLen := ValuesLen(results)
		s.debugSubLog("<< push len: %v", callLen)
		s.stack.Push(results)
		return true, nil

	case OpGetCallArgs:
		s.debugSubLog("-- getCallArgs pop call args")
		//in iterStack
		value := s.stack.Peek()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		var results ValueOperator
		var err error
		if trackErr := s.track("value-op:GetCallActualParams", func() error {
			done := s.startValueOpTiming("GetCallActualParams")
			defer done()
			results, err = value.GetCallActualParams(i.UnaryInt, i.UnaryBool)
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

		s.stack.Push(results)
		return true, nil

	case OpGetUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetUser")

		// diagnostics: track value operation timing
		var vals ValueOperator
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowUse")
			defer done()
			vals, err = value.GetSyntaxFlowUse()
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowUse() failed: %v", err)
		}
		vals.AppendPredecessor(value, s.WithPredecessorContext("getUser"))
		s.debugSubLog("<< push users")
		s.stack.Push(vals)
		return true, nil
	case OpGetBottomUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")

		// diagnostics: track value operation timing
		var vals ValueOperator
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowBottomUse", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowBottomUse")
			defer done()
			vals, err = value.GetSyntaxFlowBottomUse(s.result, s.config, i.SyntaxFlowConfig...)
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.debugSubLog("<< push bottom uses %v", ValuesLen(vals))
		s.stack.Push(vals)
		return true, nil
	case OpGetDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return true, utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetDefs")
		var vals ValueOperator
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowDef")
			defer done()
			vals, err = value.GetSyntaxFlowDef()
			return err
		}); trackErr != nil {
			return true, trackErr
		}
		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowDef() failed: %v", err)
		}
		s.debugSubLog("<< push users %v", ValuesLen(vals))
		s.stack.Push(vals)
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
		var vals ValueOperator
		var err error
		if trackErr := s.track("value-op:GetSyntaxFlowTopDef", func() error {
			done := s.startValueOpTiming("GetSyntaxFlowTopDef")
			defer done()
			vals, err = value.GetSyntaxFlowTopDef(s.result, s.config, i.SyntaxFlowConfig...)
			return err
		}); trackErr != nil {
			return true, trackErr
		}

		if err != nil {
			return true, utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.debugSubLog("<< push top defs %v", ValuesLen(vals))
		s.stack.Push(vals)
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
		s.stack.Push(v)
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
			s.stack.Push(vs)
		} else {
			values := NewEmptyValues()
			s.result.SymbolTable.Set(i.UnaryStr, values)
			s.stack.Push(values)
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
		val, err := value.Merge(vs)
		if err != nil {
			return true, utils.Wrapf(CriticalError, "merge failed: %v", err)
		}
		s.stack.Push(val)
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
		newVal, err := value.Remove(vs)
		if err != nil {
			return true, utils.Wrapf(CriticalError, "remove failed: %v", err)
		}
		s.stack.Push(newVal)
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
			s.stack.Push(NewEmptyValues())
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
			s.stack.Push(NewEmptyValues())
		} else {
			s.debugSubLog("intersection:%v", buf.String())
			s.stack.Push(NewValues(vals))
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
			ret ValueOperator
			ok  bool
			err error
		)
		if trackErr := diagnostics.TrackLow(name, func() error {
			s.debugSubLog(">> pop")
			value := s.stack.Pop()
			if value == nil {
				return utils.Wrap(CriticalError, "native call failed: stack top is empty")
			}

			s.debugSubLog("native call: [%v]", i.UnaryStr)
			call, err := GetNativeCall(i.UnaryStr)
			if err != nil {
				s.debugSubLog("Err: %v", err)
				log.Errorf("native call failed, not an existed native call[%v]: %v", i.UnaryStr, err)
				s.stack.Push(NewEmptyValues())
				return utils.Errorf("get native call failed: %v", err)
			}

			ok, ret, err = call(value, s, NewNativeCallActualParams(i.SyntaxFlowConfig...))
			if err != nil || !ok {
				s.debugSubLog("No Result in [%v]", i.UnaryStr)
				s.stack.Push(NewEmptyValues())
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
		s.stack.Push(ret)
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
		var res ValueOperator
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
		s.stack.Push(res)
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
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
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
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
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
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
		return true, nil
	default:
		return false, nil
	}
}
