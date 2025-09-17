package sfvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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

	VerifyFsInfo []*VerifyFsInfo

	// install meta info and result info
	result *SFFrameResult

	idx            int     // current opcode index
	currentProcess float64 // current process

	stack          *utils.Stack[ValueOperator] // for filter
	conditionStack *utils.Stack[[]bool]        // for condition
	iterStack      *utils.Stack[*IterContext]  // for loop
	popStack       *utils.Stack[ValueOperator] //pop stack,for sf

	// when cache err skip  statement/expr
	errorSkipStack *utils.Stack[*errorSkipContext]

	Text   string
	Codes  []*SFI // code list
	toLeft bool

	predCounter int
}

type VerifyFileSystem struct {
	vfs       filesys_interface.FileSystem
	checkInfo map[string]string
	language  consts.Language
}

func (s *SFFrame) GetResult() *SFFrameResult {
	return s.result
}

func (v *VerifyFileSystem) GetVirtualFs() filesys_interface.FileSystem {
	return v.vfs
}

func (v *VerifyFileSystem) GetLanguage() consts.Language {
	return v.language
}

func (v *VerifyFileSystem) GetExtraInfo(key string, backup ...string) string {
	result, ok := v.checkInfo[key]
	if ok {
		return result
	}
	for _, b := range backup {
		result, ok := v.checkInfo[b]
		if ok {
			return result
		}
	}
	return ""
}

func (v *VerifyFileSystem) GetExtraInfoInt(key string, backup ...string) int {
	result := v.GetExtraInfo(key, backup...)
	if result == "" {
		return 0
	}
	val, err := strconv.Atoi(result)
	if err != nil {
		return 0
	}
	return val
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

func newSfFrameEx(vars *omap.OrderedMap[string, ValueOperator], text string, codes []*SFI, rule *schema.SyntaxFlowRule, config *Config) *SFFrame {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, ValueOperator]()
	}
	if rule == nil {
		rule = &schema.SyntaxFlowRule{}
	}

	return &SFFrame{
		Text:         text,
		Codes:        codes,
		rule:         rule,
		VerifyFsInfo: make([]*VerifyFsInfo, 0),
	}
}

func NewSFFrame(vars *omap.OrderedMap[string, ValueOperator], text string, codes []*SFI) *SFFrame {
	return newSfFrameEx(vars, text, codes, nil, nil)
}

func (s *SFFrame) ExtractVerifyFilesystemAndLanguage() ([]*VerifyFileSystem, error) {
	ruleLanguage, err := consts.ValidateLanguage(s.rule.Language)
	if err != nil {
		log.Warnf("validate language failed: %s", err)
	}

	var result []*VerifyFileSystem
	hasVerifyFs := false
	for _, verifyFSInfo := range s.VerifyFsInfo {
		if len(verifyFSInfo.verifyFilesystem) == 0 {
			continue
		}
		hasVerifyFs = true
		language := ruleLanguage
		if l := verifyFSInfo.language; l != "" {
			language, _ = consts.ValidateLanguage(l)
		}
		verify := &VerifyFileSystem{}
		vfs := filesys.NewVirtualFs()
		for name, content := range verifyFSInfo.verifyFilesystem {
			if language == "" {
				lidx := strings.LastIndex(name, ".")
				if lidx > 0 {
					language, _ = consts.ValidateLanguage(name[lidx+1:])
				}
			}
			vfs.AddFile(name, content)
		}

		verify.vfs = vfs
		verify.language = language
		verify.checkInfo = verifyFSInfo.rawDesc
		result = append(result, verify)
	}
	if !hasVerifyFs {
		return result, nil
	}
	return result, nil
}

func (s *SFFrame) ExtractNegativeFilesystemAndLanguage() ([]*VerifyFileSystem, error) {
	ruleLanguage, err := consts.ValidateLanguage(s.rule.Language)
	if err != nil {
		log.Warnf("validate language failed: %s", err)
	}
	var result []*VerifyFileSystem
	for _, verifyFSInfo := range s.VerifyFsInfo {
		if len(verifyFSInfo.negativeFilesystem) == 0 {
			continue
		}
		language := ruleLanguage
		if l := verifyFSInfo.language; l != "" {
			language, _ = consts.ValidateLanguage(l)
		}
		verify := &VerifyFileSystem{}
		vfs := filesys.NewVirtualFs()
		for name, content := range verifyFSInfo.negativeFilesystem {
			if language == "" {
				lidx := strings.LastIndex(name, ".")
				if lidx > 0 {
					language, _ = consts.ValidateLanguage(name[lidx+1:])
				}
			}
			vfs.AddFile(name, content)
		}
		verify.vfs = vfs
		verify.checkInfo = verifyFSInfo.rawDesc
		verify.language = language
		result = append(result, verify)
	}
	return result, nil
}

func (s *SFFrame) Flush() {
	s.result = NewSFResult(s.rule, s.config)
	s.stack = utils.NewStack[ValueOperator]()
	s.errorSkipStack = utils.NewStack[*errorSkipContext]()
	s.conditionStack = utils.NewStack[[]bool]()
	s.iterStack = utils.NewStack[*IterContext]()
	s.popStack = utils.NewStack[ValueOperator]()
	s.idx = 0
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, ValueOperator] {
	return s.result.SymbolTable
}
func (s *SFFrame) GetSymbol(sfi *SFI) (ValueOperator, bool) {
	if val, b := s.result.SymbolTable.Get(sfi.UnaryStr); b {
		return val, b
	}
	if initVars := s.config.initialContextVars; initVars != nil {
		return initVars.Get(sfi.UnaryStr)
	} else {
		return NewEmptyValues(), true
	}
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

func (s *SFFrame) exec(feedValue ValueOperator) (ret error) {
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
		s.idx++

		// special handler this exist opcode, because this shuold pop then debugLog it
		if i.OpCode == OpExitStatement {
			checkLen := s.errorSkipStack.Pop().stackDepth
			s.debugLog("%s\t|stack %d", i.String(), s.stack.Len())
			if s.stack.Len() != checkLen {
				err := utils.Errorf("filter statement stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				s.debugSubLog("exit statement error:%v", err)
				if s.config.debug {
					return err
				}
				s.stack.PopN(s.stack.Len() - checkLen)
			}
			continue
		}

		s.debugLog("%s\t|stack %d", i.String(), s.stack.Len())

		switch i.OpCode {
		case OpCheckStackTop:
			if s.stack.Len() == 0 {
				s.debugSubLog(">> stack top is nil (push input)")
				s.stack.Push(feedValue)
			}
		case OpEnterStatement:
			s.errorSkipStack.Push(&errorSkipContext{
				start:      s.idx,
				end:        i.UnaryInt,
				stackDepth: s.stack.Len(),
			})

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
			i.Iter.currentIndex++
			s.debugSubLog("jump to next code: %v", next)
			s.idx = next
			continue
		case OpIterEnd:
			i.Iter.currentIndex = 0
			// end iter, pop and collect results to conditionStack
			if err := s.IterEnd(); err != nil {
				return err
			}
		default:
			if err := s.execStatement(i); err != nil {
				s.debugSubLog("execStatement error: %v", err)
				if errors.Is(err, AbortError) {
					return nil
				}
				if errors.Is(err, CriticalError) {
					return err
				}
				// go to expression end
				if result := s.errorSkipStack.Peek(); result != nil {
					s.idx = result.end
					continue
				}
				return err
			}
		}
	}
	return nil
}

var CriticalError = utils.Error("CriticalError(Immediately Abort)")
var AbortError = utils.Error("AbortError(Normal Abort)")

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

		s.debugSubLog("result next: %v", ValuesLen(next))
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
		s.debugSubLog("result next: %v", ValuesLen(results))
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
		s.debugSubLog("result next: %v", ValuesLen(results))
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
		s.debugSubLog("result next: %v", ValuesLen(next))
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
		s.debugSubLog("result next: %v", ValuesLen(next))
		// _ = next.AppendPredecessor(value, s.WithPredecessorContext("search: "+i.UnaryStr))
		s.stack.Push(next)
		s.debugSubLog("<< push next")
		if next == nil || err != nil {
			s.debugSubLog("error: %v", err)
			return err
		}
	case OpPopDuplicate:
		val := s.popStack.Peek()
		if val == nil {
			log.Errorf("pop duplicate failed: stack top is empty")
			return nil
		}
		s.stack.Push(val)
	case OpCheckEmpty:
		if i.Iter == nil {
			return utils.Wrap(CriticalError, "check empty failed: stack top is empty")
		}
		index := i.Iter.currentIndex
		conditions := s.conditionStack.Peek()
		//如果是null
		val := s.stack.Pop()
		if len(conditions) == index+1 && !conditions[index] {
			return nil
		}
		conditions = s.conditionStack.Pop()
		if len(conditions) < index+1 {
			return utils.Errorf("check empty failed: stack top is empty")
		}
		conditions[index] = !val.IsEmpty()
		s.conditionStack.Push(conditions)
		s.popStack.Free()
	case OpPop:
		if _, err := s.opPop(true); err != nil {
			return err
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
			s.stack.Push(NewEmptyValues())
			s.debugSubLog("<< push")
			return err
		}
		callLen := ValuesLen(results)
		s.debugSubLog("<< push len: %v", callLen)
		s.stack.Push(results)

	case OpGetCallArgs:
		s.debugSubLog("-- getCallArgs pop call args")
		//in iterStack
		value := s.stack.Peek()
		if value == nil {
			return utils.Wrap(CriticalError, "get call args failed: stack top is empty")
		}
		results, err := value.GetCallActualParams(i.UnaryInt, i.UnaryBool)
		if err != nil {
			return utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := ValuesLen(results)
		s.debugSubLog("<< push arg len: %v", callLen)
		s.debugSubLog("<< stack grow")

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
		vals.AppendPredecessor(value, s.WithPredecessorContext("getUser"))
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
			return nil
			//return utils.Errorf("new ref failed: not found: %v", i.UnaryStr)
		}
	case OpUpdateRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return utils.Errorf("update ref failed: empty name")
		}
		s.debugSubLog(">> pop")
		value, err := s.opPop(false)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return err
		}
		if value == nil {
			return utils.Error("BUG: get top defs failed, empty stack")
		}
		err = s.output(i.UnaryStr, value)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return err
		}
		s.debugSubLog(">> save $%s [%v]", i.UnaryStr, ValuesLen(value))
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
		s.result.AlertSymbolTable.Set(i.UnaryStr, value)
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
			s.result.Errors = append(s.result.Errors, elseStr)
			if s.config.FailFast {
				return utils.Wrapf(AbortError, "check params failed: %v", elseStr)
			}
		} else {
			s.result.CheckParams = append(s.result.CheckParams, i.UnaryStr)
			if thenStr != "" {
				s.result.Description.Set("$"+i.UnaryStr, thenStr)
			}
		}
	case OpEmptyCompare:
		vals := s.stack.Peek()
		if vals == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}
		var flag []bool
		vals.Recursive(func(operator ValueOperator) error {
			flag = append(flag, true)
			return nil
		})
		s.conditionStack.Push(flag)
	case OpCompareOpcode:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
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

		newVal, condition := values.CompareOpcode(comparator)
		s.stack.Push(newVal)
		s.conditionStack.Push(condition)
	case OpCompareString:
		s.debugSubLog(">> pop")
		//pop到原值
		values := s.stack.Pop()
		if values == nil {
			return utils.Wrap(CriticalError, "BUG: get top defs failed, empty stack")
		}

		mode := ValidStringMatchMode(i.UnaryInt)
		if mode == -1 {
			return utils.Wrapf(CriticalError, "compare string failed: invalid mode %v", mode)
		}

		comparator := NewStringComparator(mode, s.GetContext())
		if len(i.Values) != len(i.MultiOperator) {
			s.conditionStack.Push([]bool{false})
			return utils.Wrapf(CriticalError, "sfi values or mutiOperator out size %v", len(i.Values))
		}
		for index, v := range i.Values {
			comparator.AddCondition(v, ValidConditionFilter(i.MultiOperator[index]))
		}
		newVal, condition := values.CompareString(comparator)
		s.stack.Push(newVal)
		s.conditionStack.Push(condition)
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
	case OpEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionEqual)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
	case OpNotEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionNotEqual)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
	case OpGt:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionGt)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
	case OpGtEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionGtEq)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
	case OpLt:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionLt)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
	case OpLtEq:
		s.debugSubLog(">> pop")
		vs1 := s.stack.Pop()
		if vs1 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		s.debugSubLog(">> peek")
		vs2 := s.stack.Peek()
		if vs2 == nil {
			return utils.Wrap(CriticalError, "BUG: get stack top failed, empty stack")
		}
		comparator := NewConstComparator(vs1.String(), BinaryConditionLtEq)
		conds := vs2.CompareConst(comparator)
		s.conditionStack.Push(conds)
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
			s.stack.Push(NewEmptyValues())
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
			s.stack.Push(NewEmptyValues())
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
			s.stack.Push(NewEmptyValues())
			return utils.Errorf("get native call failed: %v", err)
		}

		ok, ret, err := call(value, s, NewNativeCallActualParams(i.SyntaxFlowConfig...))
		if err != nil || !ok {
			s.debugSubLog("No Result in [%v]", i.UnaryStr)
			s.stack.Push(NewEmptyValues())
			if errors.Is(err, CriticalError) {
				return err
			}
			return utils.Errorf("get native call failed: %v", err)
		}
		s.debugSubLog("<< push: %v", ValuesLen(ret))
		s.stack.Push(ret)
	case OpFileFilterJsonPath, OpFileFilterReg, OpFileFilterXpath:
		opcode2strMap := map[SFVMOpCode]string{
			OpFileFilterJsonPath: "jsonpath",
			OpFileFilterReg:      "regexp",
			OpFileFilterXpath:    "xpath",
		}
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
		strOpcode := opcode2strMap[i.OpCode]
		res, err := value.FileFilter(name, strOpcode, paramMap, paramList)
		if err != nil {
			return utils.Errorf("file filter failed: %v", err)
		}
		s.stack.Push(res)
	case OpPushNumber:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return utils.Wrapf(CriticalError, "BUG: pushNumber: stack top is empty")
		}
		val := vs.NewConst(i.UnaryInt)
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
	case OpPushBool:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return utils.Wrapf(CriticalError, "BUG: pushBool: stack top is empty")
		}
		val := vs.NewConst(i.UnaryBool)
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
	case OpPushString:
		s.debugSubLog(">> peek")
		vs := s.stack.Peek()
		if vs == nil {
			return utils.Wrapf(CriticalError, "BUG: pushString: stack top is empty")
		}
		val := vs.NewConst(i.UnaryStr)
		if !val.IsEmpty() {
			s.debugSubLog(">> push: %v", ValuesLen(val))
			s.stack.Push(val)
		}
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
	var values = []ValueOperator{operator}
	// save to result, even if value is empty or nil
	if resultName == "_" {
		if unnameValue := s.result.UnNameValue; ValuesLen(unnameValue) != 0 {
			values = append(values, s.result.UnNameValue)
		}
		s.result.UnNameValue = NewValues(values) // for merge
	} else {
		if originValue, existed := s.GetSymbolTable().Get(resultName); existed {
			values = append(values, originValue)
		}
		value := NewValues(values) // for merge
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

	filterStackLen := s.errorSkipStack.Len()
	prefix := strings.Repeat("\t", filterStackLen)
	prefix = "sf" + fmt.Sprintf("%4d", s.idx) + "| " + prefix
	for _, line := range strings.Split(fmt.Sprintf(i, item...), "\n") {
		log.Infof(prefix + line)
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
