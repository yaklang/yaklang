package sfvm

import (
	"fmt"
	"regexp"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type SFFrame struct {
	symbolTable *omap.OrderedMap[string, ValueOperator]
	stack       *utils.Stack[ValueOperator]
	Text        string
	Codes       []*SFI
	toLeft      bool
	debug       bool

	StatementStack *utils.Stack[int]
}

type Glob interface {
	Match(string) bool
	String() string
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
	for {
		if idx >= len(s.Codes) {
			break
		}
		i := s.Codes[idx]
		s.debugLog(i.String())
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
		case OpPushInput:
			s.debugSubLog(">> push input")
			s.stack.Push(input)
		case OpCheckStackTop:
			// if s.stack.Len() == 0 {
			// 	return utils.Errorf("stack top is empty")
			// }
		case OpPushSearchExact:
			s.debugSubLog(">> pop match exactly: %v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return utils.Errorf("search exact failed: stack top is empty")
			}

			result, next, err := value.ExactMatch(i.UnaryBool, i.UnaryStr)
			if err != nil {
				return utils.Wrapf(err, "search exact failed")
			}
			if !result {
				s.debugSubLog("result: %v, not found（exactly）, got: %s", i.UnaryStr, value.String())
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

			result, next, err := value.GlobMatch(i.UnaryBool, &GlobEx{Origin: globIns, Rule: i.UnaryStr})
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
			result, next, err := value.RegexpMatch(i.UnaryBool, regexpIns)
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
			s.debugSubLog("<< push top defs")
			s.stack.Push(vals)
		case OpNewRef:
			if i.UnaryStr == "" {
				s.debugSubLog("-")
				return
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
				return
			}
			s.debugSubLog(">> pop")
			value := s.stack.Pop()
			if value == nil {
				return utils.Error("BUG: get top defs failed, empty stack")
			}
			s.symbolTable.Set(i.UnaryStr, value)
		default:
			msg := fmt.Sprintf("unhandled default case, undefined opcode %v", i.String())
			panic(msg)
		}

		idx++
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
