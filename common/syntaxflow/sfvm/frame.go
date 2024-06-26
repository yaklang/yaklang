package sfvm

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"golang.org/x/exp/slices"

	_ "github.com/yaklang/yaklang/common/sarif"
)

type filterExprContext struct {
	start      int
	end        int
	stackDepth int
}

type SFFrame struct {
	config *Config

	// install meta info and result info
	result *SFFrameResult

	idx             int
	stack           *utils.Stack[ValueOperator]
	filterExprStack *utils.Stack[*filterExprContext]
	conditionStack  *utils.Stack[[]bool]
	Text            string
	Codes           []*SFI
	toLeft          bool
	debug           bool

	predCounter int
}
type GlobEx struct {
	Origin glob.Glob
	Rule   string
}

func (g *GlobEx) Match(d string) bool {
	return g.Origin.Match(d)
}

func (g *GlobEx) String() string {
	return g.Rule
}

func NewSFFrame(vars *omap.OrderedMap[string, ValueOperator], text string, codes []*SFI) *SFFrame {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, ValueOperator]()
	}

	return &SFFrame{
		Text:  text,
		Codes: codes,
	}
}

func (s *SFFrame) Flush() {
	s.result = NewSFResult(s.Text)
	s.stack = utils.NewStack[ValueOperator]()
	s.filterExprStack = utils.NewStack[*filterExprContext]()
	s.conditionStack = utils.NewStack[[]bool]()
	s.idx = 0
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, ValueOperator] {
	return s.result.SymbolTable
}

func (s *SFFrame) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame) withPredecessorContext(label string) AnalysisContextOption {
	s.predCounter++
	return func(context *AnalysisContext) {
		context.Step = s.predCounter
		context.Label = label
	}
}

func (s *SFFrame) exec(input ValueOperator) (ret error) {
	s.predCounter = 0
	defer func() {
		s.predCounter = 0
	}()

	// clear
	s.Flush()
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Errorf("sft panic: %v", err)
			log.Infof("%+v", ret)
		}
	}()
	statementStackDeepth := utils.NewStack[int]()
	for {
		if s.idx >= len(s.Codes) {
			break
		}

		i := s.Codes[s.idx]

		s.debugLog(i.String())
		switch i.OpCode {
		case OpFilterExprEnter:
			// if s.stack.Len() == 0 {
			// 	return utils.Errorf("(BUG) stack top is empty")
			// }
			s.filterExprStack.Push(&filterExprContext{
				start:      s.idx,
				end:        i.UnaryInt,
				stackDepth: s.stack.Len(),
			})
		case OpFilterExprExit:
			checkLen := s.filterExprStack.Pop().stackDepth
			if s.stack.Len() != checkLen {
				err := utils.Errorf("filter expr stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				log.Errorf("%v", err)
				if s.debug {
					return err
				}
				s.stack.PopN(s.stack.Len() - checkLen)
			}
		case OpCheckStackTop:
			if s.stack.Len() == 0 {
				s.debugSubLog(">> stack top is nil (push input)")
				s.stack.Push(input)
			}
		case OpEnterStatement:
			statementStackDeepth.Push(s.stack.Len())
		case OpExitStatement:
			checkLen := statementStackDeepth.Pop()
			if s.stack.Len() != checkLen {
				err := utils.Errorf("filter statement stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				log.Errorf("%v", err)
				if s.debug {
					return err
				}
				s.stack.PopN(s.stack.Len() - checkLen)
			}
		case OpCreateIter:
			s.debugSubLog(">> pop")
			vs := s.stack.Peek()
			channel := make(chan ValueOperator)
			go func() {
				defer close(channel)
				_ = vs.Recursive(func(vo ValueOperator) error {
					channel <- vo
					return nil
				})
			}()
			i.iter.originValues = channel
		case OpIterNext:
			if i.iter == nil {
				return utils.Error("BUG: iterContext is nil")
			}
			c := i.iter.originValues
			if c == nil {
				return utils.Error("BUG: iterContext.originValues is nil")
			}

			val, ok := <-c
			if !ok {
				// finish this iter
				next := i.iter.end
				s.debugLog("no next data, to %v", next)
				// jump to end
				s.idx = next
				i.iter.originValues = nil
				continue
			}
			s.debugLog("next value: %v", valuesLen(val))
			s.debugLog(">> push")
			s.stack.Push(val)
		case OpIterEnd:
			if i.iter == nil {
				return utils.Error("BUG: iterContext is nil")
			}

			if s.stack.Len() <= 0 {
				return utils.Error("BUG: stack is empty (next/iter should keep stack balanced)")
			}
			finished := false
			if i.iter.originValues != nil {
				val := s.stack.Pop()
				s.debugSubLog("iter index: %d", i.iter._counter)
				i.iter._counter++

				s.debugLog(">> pop: %v", val)
				if val.IsList() {
					ele, _ := val.ListIndex(0)
					if ele != nil {
						s.debugLog("   peeked idx: %v", i.iter._counter)
						i.iter.results = append(i.iter.results, true)
						finished = true
					}
				} else {
					if val != nil {
						i.iter.results = append(i.iter.results, true)
						finished = true
					}
				}
				if !finished {
					i.iter.results = append(i.iter.results, false)
					finished = true
				}

				s.debugSubLog("idx: %v", i.iter.results[len(i.iter.results)-1])

				next := i.iter.next
				s.debugSubLog("jump to next code: %v", next)
				s.idx = next
				continue
			}

			results := i.iter.results
			if len(results) == 0 {
				return utils.Errorf("iter results is empty")
			}
			s.debugSubLog("<< push condition results[len: %v]", results)
			s.conditionStack.Push(results)
		default:
			if err := s.execStatement(i); err != nil {
				if errors.Is(err, CriticalError) {
					return err
				}
				// go to expression end
				result := s.filterExprStack.Peek()
				if result == nil {
					return utils.Wrap(CriticalError, "filter expr stack is empty")
				}
				s.idx = result.end
				continue
			}
		}
		s.idx++
	}
	if len(s.result.Errors) > 0 {
		return utils.Errorf("check params failed: %v", s.result.Errors)
	}
	return nil
}

var CriticalError = utils.Error("CriticalError(BUG)")

func (s *SFFrame) execStatement(i *SFI) error {
	switch i.OpCode {
	case OpDuplicate:
		if s.stack.Len() == 0 {
			return utils.Wrap(CriticalError, "stack top is empty")
		}
		s.debugSubLog(">> duplicate (stack grow)")
		v := s.stack.Peek()
		s.stack.Push(v)
	case OpPushSearchExact:
		s.debugSubLog(">> pop match exactly: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Errorf("search exact failed: stack top is empty")
		}
		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.ExactMatch(mod, i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(err, "search exact failed")
		}
		if !result {
			err = utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
		}

		s.debugSubLog("result next: %v", next.String())
		_ = next.AppendPredecessor(value, s.withPredecessorContext("search "+i.UnaryStr))
		s.stack.Push(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return err
		}
	case OpPushSearchGlob:
		s.debugSubLog(">> pop search glob: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "search glob failed: stack top is empty")
		}
		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}

		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.GlobMatch(mod, &GlobEx{Origin: globIns, Rule: i.UnaryStr})
		if err != nil {
			err = utils.Wrapf(err, "search glob failed")
		}
		if !result {
			err = utils.Errorf("search glob failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", next.String())
		_ = next.AppendPredecessor(value, s.withPredecessorContext("search: "+i.UnaryStr))
		s.stack.Push(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return err
		}
	case OpPushSearchRegexp:
		s.debugSubLog(">> pop search regexp: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "search regexp failed: stack top is empty")
		}
		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.RegexpMatch(mod, regexpIns)
		if err != nil {
			err = utils.Wrap(err, "search regexp failed")
		}
		if !result {
			err = utils.Errorf("search regexp failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", next.String())
		_ = next.AppendPredecessor(value, s.withPredecessorContext("search: "+i.UnaryStr))
		s.stack.Push(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return err
		}
	case OpPop:
		if s.stack.Len() == 0 {
			s.debugSubLog(">> pop Error: empty stack")
			return utils.Wrap(CriticalError, "E: stack is empty, cannot pop")
		}
		i := s.stack.Pop()
		s.debugSubLog(">> pop %v", i.String())
		s.debugSubLog("save-to $_")
		err := s.output("_", i)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return utils.Wrapf(CriticalError, "output '_' error: %v", err)
		}
	case OpGetCall:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "get call instruction failed: stack top is empty")
		}
		results, err := value.GetCalled()
		if err != nil {
			err = utils.Errorf("get calling instruction failed: %s", err)
		}
		if err != nil {
			s.debugSubLog("error: %v", err)
			s.debugSubLog("recover origin value")
			s.stack.Push(NewValues(nil))
			s.debugSubLog("<< push")
			return err
		}
		callLen := valuesLen(results)
		s.debugSubLog("- call Called: %v", results.String())
		s.debugSubLog("<< push len: %v", callLen)
		_ = results.AppendPredecessor(value, s.withPredecessorContext("call"))
		s.stack.Push(results)

	case OpGetCallArgs:
		s.debugSubLog("-- peek")
		value := s.stack.Peek()
		if value == nil {
			return utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		results, err := value.GetCallActualParams(i.UnaryInt)
		if err != nil {
			err = utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := valuesLen(results)
		s.debugSubLog("- get argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")

		_ = results.AppendPredecessor(value, s.withPredecessorContext("actual-args["+fmt.Sprint(i.UnaryInt)+"]"))
		s.stack.Push(results)

	case OpGetAllCallArgs:
		s.debugSubLog("-- peek")
		value := s.stack.Peek()
		if value == nil {
			return utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		results, err := value.GetAllCallActualParams()
		if err != nil {
			return utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := valuesLen(results)
		s.debugSubLog("- get all argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")
		_ = results.AppendPredecessor(value, s.withPredecessorContext("all-actual-args"))
		s.stack.Push(results)

	case OpGetUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetUser")
		vals, err := value.GetSyntaxFlowUse()
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowUse() failed: %v", err)
		}
		s.debugSubLog("<< push users")
		_ = vals.AppendPredecessor(value, s.withPredecessorContext("effect"))
		s.stack.Push(vals)
	case OpGetBottomUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")
		vals, err := value.GetSyntaxFlowBottomUse(i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.debugSubLog("<< push bottom uses")
		_ = vals.AppendPredecessor(value, s.withPredecessorContext("bottom-effect"))
		s.stack.Push(vals)
	case OpGetDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetDefs")
		vals, err := value.GetSyntaxFlowDef()
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowDef() failed: %v", err)
		}
		s.debugSubLog("<< push users")
		_ = vals.AppendPredecessor(value, s.withPredecessorContext("definition"))
		s.stack.Push(vals)
	case OpGetTopDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		s.debugSubLog("- call TopDefs")
		vals, err := value.GetSyntaxFlowTopDef(i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.debugSubLog("<< push top defs %s", vals.String())
		_ = vals.AppendPredecessor(value, s.withPredecessorContext("top-definition"))
		s.stack.Push(vals)
	case OpNewRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return utils.Errorf("new ref failed: empty name")
		}
		s.debugSubLog(">> from ref: %v ", i.UnaryStr)
		vs, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if ok {
			s.debugSubLog(">> get value: %v ", vs)
			s.stack.Push(vs)
		} else {
			s.debugSubLog(">> no this variable %v ", i.UnaryStr)
		}
	case OpUpdateRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return utils.Errorf("update ref failed: empty name")
		}
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("BUG: get top defs failed, empty stack")
		}
		err := s.output(i.UnaryStr, value)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return err
		}

		result, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if ok {
			om := omap.NewEmptyOrderedMap[int64, ValueOperator]()
			_ = result.Recursive(func(operator ValueOperator) error {
				if i, ok := operator.(ssa.GetIdIF); ok {
					om.Set(i.GetId(), operator)
				}
				return nil
			})
			s.GetSymbolTable().Set(i.UnaryStr, NewValues(om.Values()))
		}

		s.debugSubLog(" -> save $" + i.UnaryStr)
	case OpAddDescription:
		if i.UnaryStr == "" {
			return utils.Errorf("add description failed: empty name")
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
	case OpAlert:
		if i.UnaryStr == "" {
			return utils.Errorf("echo failed: empty name")
		}
		value, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if !ok || value == nil {
			return utils.Errorf("alert failed: not found: %v", i.UnaryStr)
		}
		s.result.AlertSymbolTable[i.UnaryStr] = value
	case OpCheckParams:
		if i.UnaryStr == "" {
			return utils.Errorf("check params failed: empty name")
		}

		s.debugSubLog("- check: $%v", i.UnaryStr)

		var thenStr = i.ValueByIndex(0)
		var elseStr = i.ValueByIndex(1)
		if elseStr == "" {
			elseStr = "$" + i.UnaryStr + "is not found"
		}
		results, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if !ok || results == nil {
			s.debugSubLog("-   error: " + elseStr)
			if s.config.FailFast {
				return utils.Wrapf(CriticalError, "check params failed: %v", elseStr)
			}
			s.result.Errors = append(s.result.Errors, elseStr)
		} else {
			s.result.CheckParams = append(s.result.CheckParams, i.UnaryStr)
			if thenStr != "" {
				s.result.Description.Set("$"+i.UnaryStr, thenStr)
			}
		}
	case OpCompareOpcode:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		res := make([]bool, 0, valuesLen(values))
		_ = values.Recursive(func(vo ValueOperator) error {
			if slices.Contains(i.Values, vo.GetOpcode()) {
				res = append(res, true)
			} else {
				res = append(res, false)
			}
			return nil
		})
		s.conditionStack.Push(res)
	case OpCompareString:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		mode := i.UnaryInt
		if mode != CompareStringAnyMode && mode != CompareStringHaveMode {
			return utils.Wrapf(CriticalError, "compare string failed: invalid mode %v", mode)
		}
		res := make([]bool, 0, valuesLen(values))
		_ = values.Recursive(func(vo ValueOperator) error {
			raw := vo.String()
			if mode == CompareStringAnyMode {
				match := false
				for _, v := range i.Values {
					if strings.Contains(raw, v) {
						match = true
						break
					}
				}
				res = append(res, match)
			}
			if mode == CompareStringHaveMode {
				match := true
				for _, v := range i.Values {
					if !strings.Contains(raw, v) {
						match = false
						break
					}
				}
				res = append(res, match)
			}
			return nil
		})
		s.conditionStack.Push(res)
	case OpLogicBang:
		conds := s.conditionStack.Pop()
		for i := 0; i < len(conds); i++ {
			conds[i] = !conds[i]
		}
		s.conditionStack.Push(conds)
	case OpLogicAnd:
		conds1 := s.conditionStack.Pop()
		conds2 := s.conditionStack.Pop()
		if len(conds1) != len(conds2) {
			return utils.Errorf("condition failed: stack top(%v) vs conds(%v)", len(conds1), len(conds2))
		}
		res := make([]bool, 0, len(conds1))
		for i := 0; i < len(conds1); i++ {
			res = append(res, conds1[i] && conds2[i])
		}
		s.conditionStack.Push(res)
	case OpLogicOr:
		conds1 := s.conditionStack.Pop()
		conds2 := s.conditionStack.Pop()
		if len(conds1) != len(conds2) {
			return utils.Errorf("condition failed: stack top(%v) vs conds(%v)", len(conds1), len(conds2))
		}
		res := make([]bool, 0, len(conds1))
		for i := 0; i < len(conds1); i++ {
			res = append(res, conds1[i] || conds2[i])
		}
		s.conditionStack.Push(res)
	case OpCondition:
		s.debugSubLog(">> pop")
		vs := s.stack.Pop()
		if vs == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		conds := s.conditionStack.Pop()
		if len(conds) != valuesLen(vs) {
			return utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", valuesLen(vs), len(conds))
		}
		//log.Infof("condition: %v", conds)
		res := make([]ValueOperator, 0, valuesLen(vs))
		for i := 0; i < len(conds); i++ {
			if conds[i] {
				if v, err := vs.ListIndex(i); err == nil {
					res = append(res, v)
				}
			}
		}
		s.stack.Push(NewValues(res))
	case OpMergeRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if !ok || vs == nil {
			s.debugLog("cannot find $%v", i.UnaryStr)
			return nil
		}
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		value.IsList()
		val, err := value.Merge(vs)
		if err != nil {
			return utils.Wrapf(CriticalError, "merge failed: %v", err)
		}
		s.stack.Push(val)
		s.debugSubLog("<< push")
	case OpRemoveRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbolTable().Get(i.UnaryStr)
		if !ok || vs == nil {
			s.debugLog("cannot find $%v", i.UnaryStr)
			return nil
		}
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		newVal, err := value.Remove(vs)
		if err != nil {
			return utils.Wrapf(CriticalError, "remove failed: %v", err)
		}
		s.stack.Push(newVal)
		s.debugSubLog("<< push")
	default:
		msg := fmt.Sprintf("unhandled default case, undefined opcode %v", i.String())
		return utils.Wrap(CriticalError, msg)
	}
	return nil
}

func (s *SFFrame) output(resultName string, operator ValueOperator) error {
	var value = operator
	originValue, existed := s.GetSymbolTable().Get(resultName)
	if existed {
		if originList, ok := originValue.(*ValueList); ok {
			newList, isListToo := operator.(*ValueList)
			if isListToo {
				value = NewValues(append(originList.values, newList.values...))
			} else {
				value = NewValues(append(originList.values, operator))
			}
		} else {
			newList, isListToo := operator.(*ValueList)
			if isListToo {
				value = NewValues(append([]ValueOperator{
					operator,
				}, newList.values...))
			} else {
				value = NewValues([]ValueOperator{
					originValue, operator,
				})
			}
		}
	}

	s.GetSymbolTable().Set(resultName, value)
	if s.config != nil {
		for _, callback := range s.config.onResultCapturedCallbacks {
			if err := callback(resultName, operator); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SFFrame) debugLog(i string, item ...any) {
	if !s.debug {
		return
	}

	filterStackLen := s.filterExprStack.Len()

	prefix := strings.Repeat(" ", filterStackLen)

	formatter := "sf" + fmt.Sprintf("%4d", s.idx) + "| " + prefix + i + "\n"
	if len(item) > 0 {
		fmt.Printf(formatter, item...)
	} else {
		fmt.Printf(formatter)
	}
}

func (s *SFFrame) debugSubLog(i string, item ...any) {
	s.debugLog("  |-- "+i, item...)
}
