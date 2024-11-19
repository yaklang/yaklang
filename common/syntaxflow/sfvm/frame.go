package sfvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// skip  current statement or filter-expression when error
type errorSkipContext struct {
	start      int
	end        int
	stackDepth int
}

type SFFrame struct {
	vm *SyntaxFlowVirtualMachine

	config *Config
	rule   *schema.SyntaxFlowRule

	VerifyFs   map[string]string
	NegativeFs map[string]string
	ExtraDesc  map[string]string

	// install meta info and result info
	result *SFFrameResult

	idx            int     // current opcode index
	currentProcess float64 // current process

	stack          *utils.Stack[ValueOperator] // for filter
	conditionStack *utils.Stack[[]bool]        // for condition
	iterStack      *utils.Stack[*IterContext]  // for loop

	// when cache err skip  statement/expr
	statementStack  *utils.Stack[*errorSkipContext]
	filterExprStack *utils.Stack[*errorSkipContext]

	Text   string
	Codes  []*SFI
	toLeft bool

	predCounter int
}

func (s *SFFrame) GetRule() *schema.SyntaxFlowRule {
	return s.rule
}

func (s *SFFrame) GetContext() context.Context {
	if s == nil || s.config == nil {
		return context.Background()
	}
	return s.config.GetContext()
}

func (s *SFFrame) GetExtraInfo(key string, backup ...string) string {
	result, ok := s.ExtraDesc[key]
	if ok {
		return result
	}
	for _, b := range backup {
		result, ok := s.ExtraDesc[b]
		if ok {
			return result
		}
	}
	return ""
}

func (s *SFFrame) GetExtraInfoInt(key string, backup ...string) int {
	result := s.GetExtraInfo(key, backup...)
	if result == "" {
		return 0
	}
	val, err := strconv.Atoi(result)
	if err != nil {
		return 0
	}
	return val
}

func newSfFrameEx(vars *omap.OrderedMap[string, ValueOperator], text string, codes []*SFI, rule *schema.SyntaxFlowRule, config *Config) *SFFrame {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, ValueOperator]()
	}
	if rule == nil {
		rule = &schema.SyntaxFlowRule{}
	}

	return &SFFrame{
		Text:       text,
		Codes:      codes,
		rule:       rule,
		VerifyFs:   make(map[string]string),
		NegativeFs: make(map[string]string),
		ExtraDesc:  make(map[string]string),
	}
}

func NewSFFrame(vars *omap.OrderedMap[string, ValueOperator], text string, codes []*SFI) *SFFrame {
	return newSfFrameEx(vars, text, codes, nil, nil)
}

func (s *SFFrame) ExtractVerifyFilesystemAndLanguage() (consts.Language, filesys_interface.FileSystem, error) {
	val, err := consts.ValidateLanguage(s.rule.Language)
	if err != nil {
		log.Warnf("validate language failed: %s", err)
	}
	if s.VerifyFs == nil || len(s.VerifyFs) <= 0 {
		return "", nil, utils.Wrap(err, "validator verify failed")
	}

	vfs := filesys.NewVirtualFs()
	for name, content := range s.VerifyFs {
		if val == "" {
			lidx := strings.LastIndex(name, ".")
			if lidx > 0 {
				val, _ = consts.ValidateLanguage(name[lidx+1:])
			}
		}
		vfs.AddFile(name, content)
	}

	if val == "" {
		return "", nil, utils.Wrap(err, "validator language not found")
	}
	return val, vfs, nil
}

func (s *SFFrame) ExtractNegativeFilesystemAndLanguage() (consts.Language, filesys_interface.FileSystem, error) {
	val, err := consts.ValidateLanguage(s.rule.Language)
	if err != nil {
		log.Warnf("validate language failed: %s", err)
	}
	if s.NegativeFs == nil || len(s.NegativeFs) <= 0 {
		return "", nil, utils.Wrap(err, "validator verify failed")
	}

	vfs := filesys.NewVirtualFs()
	for name, content := range s.NegativeFs {
		if val == "" {
			lidx := strings.LastIndex(name, ".")
			if lidx > 0 {
				val, _ = consts.ValidateLanguage(name[lidx+1:])
			}
		}
		vfs.AddFile(name, content)
	}

	if val == "" {
		return "", nil, utils.Wrap(err, "validator language not found")
	}
	return val, vfs, nil
}

func (s *SFFrame) Flush() {
	if s.result == nil {
		s.result = NewSFResult(s.rule, s.config) // TODO: This code affects the reentrancy of the function
	}
	s.stack = utils.NewStack[ValueOperator]()
	s.statementStack = utils.NewStack[*errorSkipContext]()
	s.filterExprStack = utils.NewStack[*errorSkipContext]()
	s.conditionStack = utils.NewStack[[]bool]()
	s.iterStack = utils.NewStack[*IterContext]()
	s.idx = 0
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, ValueOperator] {
	return s.result.SymbolTable
}
func (s *SFFrame) GetSymbol(sfi *SFI) (ValueOperator, bool) {
	if val, b := s.result.SymbolTable.Get(sfi.UnaryStr); b {
		return val, b
	}
	switch sfi.OpCode {
	// variable +/-/&& can read from context
	case OpMergeRef, OpRemoveRef, OpIntersectionRef:
		if initVars := s.config.initialContextVars; initVars != nil {
			return initVars.Get(sfi.UnaryStr)
		}
	}
	return nil, false
}
func (s *SFFrame) GetSymbolByName(name string) (ValueOperator, bool) {
	return s.result.SymbolTable.Get(name)
}
func (s *SFFrame) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame) WithPredecessorContext(label string) AnalysisContextOption {
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
	for {
		if s.config.processCallback != nil {
			var msg string
			if s.idx < len(s.Codes) {
				msg = s.Codes[s.idx].String()
			} else {
				msg = "exec rule finished"
			}
			s.config.processCallback(s.idx, msg)
		}
		if s.idx >= len(s.Codes) {
			break
		}
		select {
		case <-s.GetContext().Done():
			return utils.Errorf("context done")
		default:
		}

		i := s.Codes[s.idx]

		s.debugLog(i.String())

		switch i.OpCode {
		case OpFilterExprEnter:
			s.filterExprStack.Push(&errorSkipContext{
				start:      s.idx,
				end:        i.UnaryInt,
				stackDepth: s.stack.Len(),
			})
		case OpFilterExprExit:
			checkLen := s.filterExprStack.Pop().stackDepth
			if s.stack.Len() != checkLen {
				err := utils.Errorf("filter expr stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				log.Errorf("%v", err)
				if s.config.debug {
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
			s.statementStack.Push(&errorSkipContext{
				start:      s.idx,
				end:        i.UnaryInt,
				stackDepth: s.stack.Len(),
			})
		case OpExitStatement:
			checkLen := s.statementStack.Pop().stackDepth
			if s.stack.Len() != checkLen {
				err := utils.Errorf("filter statement stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				log.Errorf("%v", err)
				if s.config.debug {
					return err
				}
				s.stack.PopN(s.stack.Len() - checkLen)
			}
		case OpCreateIter:
			s.debugSubLog(">> peek")
			vs := s.stack.Peek()
			if vs == nil {
				return utils.Wrapf(CriticalError, "BUG: iterCreate: stack top is empty")
			}
			s.IterStart(vs)
		case OpIterNext:
			vs, next, err := s.IterNext()
			if err != nil {
				return err
			}
			if !next {
				// jump to end
				end := i.Iter.End
				s.debugSubLog("no next data, to %v", end)
				s.idx = end
				continue
			}
			s.debugLog("next value: %v", ValuesLen(vs))
			s.stack.Push(vs)
		case OpIterLatch:
			if s.stack.IsEmpty() {
				return utils.Wrapf(CriticalError, "BUG: iterLatch: stack top is empty")
			}
			if err := s.IterLatch(s.stack.Pop()); err != nil {
				return err
			}
			// jump to next
			next := i.Iter.Next
			s.debugSubLog("jump to next code: %v", next)
			s.idx = next
			continue
		case OpIterEnd:
			// end iter, pop and collect results to conditionStack
			if err := s.IterEnd(); err != nil {
				return err
			}
		default:
			if err := s.execStatement(i); err != nil {
				if errors.Is(err, CriticalError) {
					return err
				}
				// go to expression end
				if result := s.filterExprStack.Peek(); result != nil {
					s.idx = result.end
					continue
				}
				if result := s.statementStack.Peek(); result != nil {
					s.idx = result.end
					continue
				}
				return err
			}
		}
		s.idx++
	}
	return nil
}

var CriticalError = utils.Error("CriticalError(Immediately Abort)")

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
		result, next, err := value.ExactMatch(s.GetContext(), mod, i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(err, "search exact failed")
		}
		if !result {
			err = utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
		}

		s.debugSubLog("result next: %v", next.String())
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search "+i.UnaryStr))
		s.stack.Push(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return err
		}
	case OpRecursiveSearchExact:
		s.debugSubLog(">> pop recursive search exactly: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "recursive search exact failed: stack top is empty")
		}
		var next []ValueOperator
		err := recursiveDeepChain(value, func(operator ValueOperator) bool {
			ok, results, _ := operator.ExactMatch(s.GetContext(), BothMatch, i.UnaryStr)
			if ok {
				have := false
				log.Infof("recursive search exact: %v from: %v", results.String(), operator.String())
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
			err = utils.Wrapf(err, "recursive search exact failed")
		}

		results := NewValues(next)
		s.debugSubLog("result next: %v", results.String())
		_ = results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
		s.stack.Push(results)
		s.debugSubLog("<< push next")
	case OpRecursiveSearchGlob:
		s.debugSubLog(">> pop recursive search glob: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "recursive search glob failed: stack top is empty")
		}

		mod := i.UnaryInt

		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			err = utils.Wrap(CriticalError, "compile glob failed")
		}
		_ = globIns

		var next []ValueOperator
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			ok, results, _ := operator.GlobMatch(s.GetContext(), mod|NameMatch, i.UnaryStr)
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
		s.debugSubLog("result next: %v", results.String())
		_ = results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
		s.stack.Push(results)
		s.debugSubLog("<< push next")
	case OpRecursiveSearchRegexp:
		s.debugSubLog(">> pop recursive search regexp: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "recursive search regexp failed: stack top is empty")
		}
		mod := i.UnaryInt

		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			return utils.Wrapf(CriticalError, "compile regexp[%v] failed: %v", i.UnaryStr, err)
		}
		_ = regexpIns

		var next []ValueOperator
		err = recursiveDeepChain(value, func(operator ValueOperator) bool {
			//log.Infof("recursive search regexp: %v", operator.String())
			//if strings.Contains(operator.String(), "aaa") {
			//	spew.Dump(1)
			//}
			ok, results, _ := operator.RegexpMatch(s.GetContext(), mod|NameMatch, i.UnaryStr)
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
		s.debugSubLog("result next: %v", results.String())
		_ = results.AppendPredecessor(value, s.WithPredecessorContext("recursive search "+i.UnaryStr))
		s.stack.Push(results)
		s.debugSubLog("<< push next")
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
		_ = globIns

		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.GlobMatch(s.GetContext(), mod, i.UnaryStr)
		if err != nil {
			err = utils.Wrapf(err, "search glob failed")
		}
		if !result {
			err = utils.Errorf("search glob failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", next.String())
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search: "+i.UnaryStr))
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
		result, next, err := value.RegexpMatch(s.GetContext(), mod, regexpIns.String())
		if err != nil {
			err = utils.Wrap(err, "search regexp failed")
		}
		if !result {
			err = utils.Errorf("search regexp failed: not found: %v", i.UnaryStr)
		}
		s.debugSubLog("result next: %v", next.String())
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search: "+i.UnaryStr))
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
		s.debugSubLog(">> pop %v", ValuesLen(i))
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
		callLen := ValuesLen(results)
		s.debugSubLog("<< push len: %v", callLen)
		// _ = results.AppendPredecessor(value, s.WithPredecessorContext("call"))
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
		callLen := ValuesLen(results)
		s.debugSubLog("- get argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")

		_ = results.AppendPredecessor(value, s.WithPredecessorContext("actual-args["+fmt.Sprint(i.UnaryInt)+"]"))
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
		callLen := ValuesLen(results)
		s.debugSubLog("- get all argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")
		// _ = results.AppendPredecessor(value, s.WithPredecessorContext("all-actual-args"))
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
		s.stack.Push(vals)
	case OpGetBottomUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")
		vals, err := value.GetSyntaxFlowBottomUse(s.result, s.config, i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.debugSubLog("<< push bottom uses %v", ValuesLen(vals))
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
		s.debugSubLog("<< push users %v", ValuesLen(vals))
		s.stack.Push(vals)
	case OpGetTopDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		s.debugSubLog("- call TopDefs")
		vals, err := value.GetSyntaxFlowTopDef(s.result, s.config, i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.debugSubLog("<< push top defs %v", ValuesLen(vals))
		s.stack.Push(vals)
	case OpNewRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return utils.Errorf("new ref failed: empty name")
		}
		s.debugSubLog(">> from ref: %v ", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		if ok {
			if vs == nil {
				return utils.Errorf("new ref failed: empty value: %v", i.UnaryStr)
			}
			s.debugSubLog(">> get value: %v ", vs)
			s.stack.Push(vs)
		} else {
			return utils.Errorf("new ref failed: not found: %v", i.UnaryStr)
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

		result, ok := s.GetSymbol(i)
		if ok {
			res := make([]ValueOperator, 0, ValuesLen(result))
			tmp := make(map[int64]struct{})
			_ = result.Recursive(func(operator ValueOperator) error {
				if i, ok := operator.(ssa.GetIdIF); ok {
					if i.GetId() == -1 {
						// syntax-flow  runtime will create new template value
						// the "fileFilter" function will create.
						res = append(res, operator)
					} else {
						_, ok := tmp[i.GetId()]
						if !ok {
							res = append(res, operator)
							tmp[i.GetId()] = struct{}{}
						}
					}
				}
				return nil
			})
			s.GetSymbolTable().Set(i.UnaryStr, NewValues(res))
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
		value, ok := s.GetSymbol(i)
		if !ok || value == nil {
			return utils.Errorf("alert failed: not found: %v", i.UnaryStr)
		}
		//m := s.result.rule.AlertDesc[i.UnaryStr]
		//m := s.result.AlertMsgTable[i.UnaryStr]
		//lo.ForEach(i.SyntaxFlowConfig, func(item *RecursiveConfigItem, index int) {
		//	if m == nil || len(m) == 0 {
		//		m = make(map[string]string)
		//	}
		//	m[item.Key] = item.Value
		//})
		s.result.AlertSymbolTable[i.UnaryStr] = value
		//alStr := i.ValueByIndex(0)
		//if alStr != "" {
		//	m["__extra__"] = alStr
		//}
	case OpCheckParams:
		if i.UnaryStr == "" {
			return utils.Errorf("check params failed: empty name")
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
		res := make([]bool, 0, ValuesLen(values))
		_ = values.Recursive(func(vo ValueOperator) error {
			for _, element := range i.Values {
				code := vo.GetOpcode()
				if element == code {
					res = append(res, true)
					return nil
				} else if results := BinOpRegexp.FindStringSubmatch(element); len(results) > 2 {
					if code == results[1] {
						// vo.GetOpcode()
						switch ret := ssa.BinaryOpcode(vo.GetBinaryOperator()); ret {
						case ssa.OpAdd:
							if results[2] == "+" {
								res = append(res, true)
								return nil
							}
						}
					}
				}
			}

			res = append(res, false)
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
		res := make([]bool, 0, ValuesLen(values))
		var (
			conditionCache = make(map[string]any)
		)

		matchFunc := func(index int, str, raw string) bool {
			if len(i.MultiOperator) <= index {
				log.Errorf("sfi values or mutiOperator out size %v", index)
				return false
			}
			condition, exit := conditionCache[codec.Md5(str)]
			conditionMode := i.MultiOperator[index]
			switch conditionMode {
			case GlobalConditionFilter:
				var global glob.Glob
				if exit {
					if _global, ok := condition.(glob.Glob); ok {
						global = _global
					}
				}
				if global == nil {
					compile, err := glob.Compile(str)
					if err != nil {
						log.Errorf("global compile fail: %s", err)
						return false
					}
					conditionCache[codec.Md5(str)] = compile
					global = compile
				}
				return global.Match(raw)
			case RegexpConditionFilter:
				var regexpCondition *regexp.Regexp
				if exit {
					if r, ok := condition.(*regexp.Regexp); ok {
						regexpCondition = r
					}
				}
				if regexpCondition == nil {
					compile, err := regexp.Compile(str)
					if err != nil {
						log.Errorf("regexp compile fail: %s", err)
						return false
					}
					conditionCache[codec.Md5(str)] = compile
					regexpCondition = compile
				}
				result := regexpCondition.MatchString(raw)
				return result
			case ExactConditionFilter:
				result := strings.Contains(raw, str)
				return result
			default:
				return false
			}

		}
		_ = values.Recursive(func(vo ValueOperator) error {
			raw := vo.String()
			if mode == CompareStringAnyMode {
				match := false
				for index, v := range i.Values {
					if matchFunc(index, v, yakunquote.TryUnquote(raw)) {
						match = true
						break
					}
				}
				res = append(res, match)
			}
			if mode == CompareStringHaveMode {
				match := true
				for index, v := range i.Values {
					if !matchFunc(index, v, yakunquote.TryUnquote(raw)) {
						match = false
						break
					}
				}
				res = append(res, match)
			}
			return nil
		})
		s.conditionStack.Push(res)
	case OpVersionIn:
		value := s.stack.Peek()
		if value == nil {
			return utils.Wrap(CriticalError, "compare version failed: stack top is empty")
		}
		call, err := GetNativeCall("versionIn")
		if err != nil {
			s.debugSubLog("Err: %v", err)
			log.Errorf("native call failed, not an existed native call-versionIn")
			return utils.Errorf("get native call failed: %v", err)
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
		if len(conds) != ValuesLen(vs) {
			return utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", ValuesLen(vs), len(conds))
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
	case OpMergeRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
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
		vs, ok := s.GetSymbol(i)
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
	case OpIntersectionRef:
		s.debugSubLog("fetch: %v", i.UnaryStr)
		vs, ok := s.GetSymbol(i)
		//vs, ok := s.result.SymbolTable.Get(i.UnaryStr)
		if vs == nil || !ok {
			s.debugLog("cannot find $%v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
			}
			s.stack.Push(NewValues(nil))
			return nil
		}
		s.debugLog(">> pop")
		m1 := make(map[int64]ValueOperator)
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
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
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
			s.stack.Push(NewValues(nil))
		} else {
			s.debugSubLog("intersection:%v", buf.String())
			s.stack.Push(NewValues(vals))
		}
	case OpNativeCall:
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
			s.stack.Push(NewValues(nil))
			return utils.Errorf("get native call failed: %v", err)
		}

		ok, ret, err := call(value, s, NewNativeCallActualParams(i.SyntaxFlowConfig...))
		if err != nil || !ok {
			s.debugSubLog("No Result in [%v]", i.UnaryStr)
			s.stack.Push(NewValues(nil))
			if errors.Is(err, CriticalError) {
				return err
			}
			return utils.Errorf("get native call failed: %v", err)
		}
		s.debugSubLog("<< push: %v", ret)
		s.stack.Push(ret)
	case OpFileFilterJsonPath:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "native call failed: stack top is empty")
		}
		s.debugSubLog(">> pop file name: %v", i.UnaryStr)
		name := i.UnaryStr
		if name == "" {
			return utils.Errorf("file filter failed: file name is empty")
		}
		paramList := i.Values
		paramMap := i.FileFilterMethodItem
		res, err := value.FileFilter(name, "jsonpath", paramMap, paramList)
		if err != nil {
			return utils.Errorf("file filter failed: %v", err)
		}
		s.stack.Push(res)
	case OpFileFilterXpath:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "native call failed: stack top is empty")
		}
		s.debugSubLog(">> pop file name: %v", i.UnaryStr)
		name := i.UnaryStr
		if name == "" {
			return utils.Errorf("file filter failed: file name is empty")
		}
		paramList := i.Values
		paramMap := i.FileFilterMethodItem
		res, err := value.FileFilter(name, "xpath", paramMap, paramList)
		if err != nil {
			return utils.Errorf("file filter failed: %v", err)
		}
		s.stack.Push(res)
		_ = paramList
		_ = paramMap
	case OpFileFilterReg:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Wrap(CriticalError, "native call failed: stack top is empty")
		}
		s.debugSubLog(">> pop file name: %v", i.UnaryStr)
		name := i.UnaryStr
		if name == "" {
			return utils.Errorf("file filter failed: file name is empty")
		}
		paramList := i.Values
		paramMap := i.FileFilterMethodItem
		res, err := value.FileFilter(name, "regexp", paramMap, paramList)
		if err != nil {
			return utils.Errorf("file filter failed: %v", err)
		}
		s.stack.Push(res)
		// _ = paramList
		// _ = paramMap
	default:
		msg := fmt.Sprintf("unhandled default case, undefined opcode %v", i.String())
		return utils.Wrap(CriticalError, msg)
	}
	return nil
}

// func (s *SFFrame) saveUnName(operator ValueOperator) error {
// 	s.result.UnNameValue = append(s.result.UnNameValue, operator)
// 	return nil
// }

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

	if resultName == "_" {
		s.result.UnNameValue = append(s.result.UnNameValue, value)
	} else {

		s.GetSymbolTable().Set(resultName, value)
	}
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
	if !s.config.debug {
		return
	}

	filterStackLen := s.statementStack.Len()

	prefix := strings.Repeat(" ", filterStackLen)
	prefix = "sf" + fmt.Sprintf("%4d", s.idx) + "| " + prefix
	for _, line := range strings.Split(fmt.Sprintf(i, item...), "\n") {
		fmt.Print(prefix + line + "\n")
	}
}

func (s *SFFrame) debugSubLog(i string, item ...any) {
	prefix := "  |-- "
	results := fmt.Sprintf(i, item...)
	var result bytes.Buffer
	lines := strings.Split(results, "\n")
	for idx, line := range lines {
		if line == "" && idx == len(lines)-1 {
			break
		}
		if idx > 0 {
			result.WriteString("\n")
			prefix = "  |       "
		}
		result.WriteString(prefix + line)
	}
	s.debugLog(result.String())
}

func (s *SFFrame) SetSFResult(sfResult *SFFrameResult) {
	s.result = sfResult
}

func (s *SFFrame) GetSFResult() (*SFFrameResult, error) {
	if s.result == nil {
		return nil, utils.Error("BUG: result is nil")
	}
	return s.result, nil
}

func (s *SFFrame) GetVM() *SyntaxFlowVirtualMachine {
	return s.vm
}

func (s *SFFrame) GetConfig() *Config {
	return s.config
}

func fetchId(i any) (int64, bool) {
	result, ok := i.(ssa.GetIdIF)
	if !ok {
		return 0, false
	}
	if result.GetId() > 0 {
		return result.GetId(), true
	}
	return 0, false
}
