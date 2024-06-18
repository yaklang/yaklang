package sfvm

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type SFFrameInfo struct {
	Description *omap.OrderedMap[string, string]
	CheckParams []string
	Errors      []string
}

func NewSFFrameInfo() *SFFrameInfo {
	return &SFFrameInfo{
		Description: omap.NewEmptyOrderedMap[string, string](),
		CheckParams: make([]string, 0),
	}
}

type SFFrame struct {
	config *Config

	info *SFFrameInfo

	symbolTable *omap.OrderedMap[string, ValueOperator]
	stack       *utils.Stack[ValueOperator]
	Text        string
	Codes       []*SFI
	toLeft      bool
	debug       bool

	StatementStack *utils.Stack[int]
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
		info:        NewSFFrameInfo(),
		symbolTable: v,
		stack:       utils.NewStack[ValueOperator](),
		Text:        text,
		Codes:       codes,

		StatementStack: utils.NewStack[int](),
	}
}

func (s *SFFrame) Debug(v ...bool) *SFFrame {
	if len(v) > 0 {
		s.debug = v[0]
	}
	return s
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, ValueOperator] {
	return s.symbolTable
}

func (s *SFFrame) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame) exec(input ValueOperator) (ret error) {
	// s.stack.Push(input)
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Errorf("sft panic: %v", err)
			log.Infof("%+v", ret)
		}
	}()
	s.stack.Push(input)
	idx := 0
	statementEnd := 0
	for {
		if idx >= len(s.Codes) {
			break
		}
		i := s.Codes[idx]
		s.debugLog(i.String())
		switch i.OpCode {
		case OpEnterStatement:
			s.StatementStack.Push(s.stack.Len())
			statementEnd = i.UnaryInt // for exit statement
		case OpExitStatement:
			checkLen := s.StatementStack.Pop()
			if s.stack.Len() != checkLen {
				log.Errorf("stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
				s.stack.PopN(s.stack.Len() - checkLen)
			}
			statementEnd = 0 // empty
		case OpPushInput:
			s.debugSubLog(">> push input")
			s.stack.Push(input)
		default:
			if err := s.execStatement(i); err != nil {
				if statementEnd == 0 {
					return utils.Wrapf(err, "exec statement failed")
				}
				idx = statementEnd
				continue
			}
		}
		idx++
	}
	return nil
}

func (s *SFFrame) execStatement(i *SFI) error {
	switch i.OpCode {
	case OpEnterStatement:
		s.StatementStack.Push(s.stack.Len())
	case OpExitStatement:
		checkLen := s.StatementStack.Pop()
		if s.stack.Len() != checkLen {
			log.Errorf("stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
			s.stack.PopN(s.stack.Len() - checkLen)
		}
	case OpDuplicate:
		if s.stack.Len() == 0 {
			return utils.Errorf("stack top is empty")
		}
		s.debugSubLog(">> duplicate ")
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
			return utils.Wrapf(err, "search exact failed")
		}
		if !result {
			s.debugSubLog("result: %v, not found(exactly), got: %s", i.UnaryStr, value.String())
			return utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
		}
		if next != nil {
			s.debugSubLog("result next: %v", next.String())
			s.stack.Push(next)
			s.debugSubLog("<< push next")
		} else {
			s.debugSubLog("result: %v", value.String())
			s.stack.Push(value)
			s.debugSubLog("<< push")
		}
	case OpPushSearchGlob:
		s.debugSubLog(">> pop search glob: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Errorf("search glob failed: stack top is empty")
		}
		globIns, err := glob.Compile(i.UnaryStr)
		if err != nil {
			return utils.Wrap(err, "compile glob failed")
		}

		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.GlobMatch(mod, &GlobEx{Origin: globIns, Rule: i.UnaryStr})
		if err != nil {
			return utils.Wrap(err, "search glob failed")
		}
		if !result {
			s.debugSubLog("result: %v, not found(glob search)", i.UnaryStr)
			return utils.Errorf("search glob failed: not found: %v", i.UnaryStr)
		}
		if next != nil {
			s.debugSubLog("result: %v", next.String())
			s.stack.Push(next)
			s.debugSubLog("<< push")
		} else {
			s.debugSubLog("result: %v", value.String())
			s.stack.Push(value)
			s.debugSubLog("<< push")
		}
	case OpPushSearchRegexp:
		s.debugSubLog(">> pop search regexp: %v", i.UnaryStr)
		value := s.stack.Pop()
		if value == nil {
			return utils.Errorf("search regexp failed: stack top is empty")
		}
		regexpIns, err := regexp.Compile(i.UnaryStr)
		if err != nil {
			return utils.Wrap(err, "compile regexp failed")
		}
		mod := i.UnaryInt
		if !s.config.StrictMatch {
			mod |= KeyMatch
		}
		result, next, err := value.RegexpMatch(mod, regexpIns)
		if err != nil {
			return utils.Wrap(err, "search regexp failed")
		}
		if !result {
			s.debugSubLog("result: %v, not found(regexp search)", i.UnaryStr)
			return utils.Errorf("search regexp failed: not found: %v", i.UnaryStr)
		}
		if next != nil {
			s.debugSubLog("result: %v", next.String())
			s.stack.Push(next)
			s.debugSubLog("<< push")
			// return nil
		} else {
			s.debugSubLog("result: %v", value.String())
			s.stack.Push(value)
			s.debugSubLog("<< push")
		}
	case OpPop:
		if s.stack.Len() == 0 {
			s.debugSubLog(">> pop Error: empty stack")
			return utils.Error("E: stack is empty, cannot pop")
		}
		i := s.stack.Pop()
		s.debugSubLog(">> pop %v", i.String())
		s.debugSubLog("save-to $_")
		err := s.output("_", i)
		if err != nil {
			s.debugSubLog("ERROR: %v", err)
			return err
		}
	case opGetCall:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("get call instruction failed: stack top is empty")
		}
		results, err := value.GetCalled()
		if err != nil {
			return utils.Errorf("get calling instruction failed: %s", err)
		}
		callLen := valuesLen(results)
		s.debugSubLog("- call Called: %v", results.String())
		s.debugSubLog("<< push len: %v", callLen)
		s.stack.Push(results)

	case OpGetCallArgs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("get call args failed: stack top is empty")
		}
		results, err := value.GetCallActualParams(i.UnaryInt)
		if err != nil {
			return utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := valuesLen(results)
		s.debugSubLog("- get argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.stack.Push(results)

	case OpGetAllCallArgs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("get call args failed: stack top is empty")
		}
		results, err := value.GetAllCallActualParams()
		if err != nil {
			return utils.Errorf("get calling argument failed: %s", err)
		}
		callLen := valuesLen(results)
		s.debugSubLog("- get all argument: %v", results.String())
		s.debugSubLog("<< push arg len: %v", callLen)
		s.stack.Push(results)

	case OpGetUsers:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("get users failed: stack top is empty")
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
			return utils.Error("BUG: get bottom uses failed, empty stack")
		}
		s.debugSubLog("- call BottomUses")
		vals, err := value.GetSyntaxFlowBottomUse(i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowBottomUse() failed: %v", err)
		}
		s.debugSubLog("<< push bottom uses")
		s.stack.Push(vals)
	case OpGetDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("get users failed: stack top is empty")
		}
		s.debugSubLog("- call GetDefs")
		vals, err := value.GetSyntaxFlowDef()
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowDef() failed: %v", err)
		}
		s.debugSubLog("<< push users")
		s.stack.Push(vals)
	case OpGetTopDefs:
		s.debugSubLog(">> pop")
		value := s.stack.Pop()
		if value == nil {
			return utils.Error("BUG: get top defs failed, empty stack")
		}
		s.debugSubLog("- call TopDefs")
		vals, err := value.GetSyntaxFlowTopDef(i.SyntaxFlowConfig...)
		if err != nil {
			return utils.Errorf("Call .GetSyntaxFlowTopDef() failed: %v", err)
		}
		s.debugSubLog("<< push top defs %s", vals.String())
		s.stack.Push(vals)
	case OpNewRef:
		if i.UnaryStr == "" {
			s.debugSubLog("-")
			return utils.Errorf("new ref failed: empty name")
		}
		s.debugSubLog(">> from ref: %v ", i.UnaryStr)
		vs, ok := s.symbolTable.Get(i.UnaryStr)
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
		s.debugSubLog(" -> save $" + i.UnaryStr)
	case OpAddDescription:
		if i.UnaryStr == "" {
			return utils.Errorf("add description failed: empty name")
		}
		ret := i.ValueByIndex(0)
		s.info.Description.Set(i.UnaryStr, ret)
		if ret != "" {
			s.debugSubLog("- key: %v, value: %v", i.UnaryStr, ret)
		} else {
			s.debugSubLog("- key: %v", i.UnaryStr)
		}
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
			s.info.Errors = append(s.info.Errors, elseStr)
		} else {
			s.info.CheckParams = append(s.info.CheckParams, i.UnaryStr)
			if thenStr != "" {
				s.info.Description.Set("$"+i.UnaryStr, thenStr)
			}
		}
	case OpCompareOpcode:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
			return utils.Error("BUG: get top defs failed, empty stack")
		}
		res := make([]ValueOperator, 0)
		values.Recursive(func(vo ValueOperator) error {
			if slices.Contains(i.Values, vo.GetOpcode()) {
				res = append(res, vo)
			}
			return nil
		})
		callLen := len(res)
		s.debugSubLog("<< push arg len: %v", callLen)
		s.stack.Push(NewValues(res))
	case OpCompareString:
		s.debugSubLog(">> pop")
		values := s.stack.Pop()
		if values == nil {
			return utils.Error("BUG: get top defs failed, empty stack")
		}
		res := make([]ValueOperator, 0)
		values.Recursive(func(vo ValueOperator) error {
			if utils.StringContainsAnyOfSubString(vo.String(), i.Values) {
				res = append(res, vo)
			}
			return nil
		})
		callLen := len(res)
		s.debugSubLog("<< push arg len: %v", callLen)
		s.stack.Push(NewValues(res))
	default:
		msg := fmt.Sprintf("unhandled default case, undefined opcode %v", i.String())
		panic(msg)
	}

	return nil
}

func (s *SFFrame) output(resultName string, operator ValueOperator) error {
	var value = operator
	originValue, existed := s.symbolTable.Get(resultName)
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

	s.symbolTable.Set(resultName, value)
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
	if len(item) > 0 {
		fmt.Printf("sf | "+i+"\n", item...)
	} else {
		fmt.Printf("sf | " + i + "\n")
	}
}

func (s *SFFrame) debugSubLog(i string, item ...any) {
	s.debugLog("  |-- "+i, item...)
}
