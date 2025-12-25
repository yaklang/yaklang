package sfvm

// frame.go
// This file contains the core data structures and utility functions for SyntaxFlow Virtual Machine frame.
// It defines:
// - SFFrame: The main execution frame structure
// - VerifyFileSystem: Filesystem verification context
// - Utility functions: Symbol table access, context management, debugging, etc.
// - Entry points: exec() and execRule() for starting execution
// - Helper functions: output(), debugLog(), debugSubLog(), etc.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
	language  ssaconfig.Language
}

func (s *SFFrame) GetResult() *SFFrameResult {
	return s.result
}

func (v *VerifyFileSystem) GetVirtualFs() filesys_interface.FileSystem {
	return v.vfs
}

func (v *VerifyFileSystem) GetLanguage() ssaconfig.Language {
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
	ruleLanguage := s.rule.Language

	var result []*VerifyFileSystem
	hasVerifyFs := false
	for _, verifyFSInfo := range s.VerifyFsInfo {
		if len(verifyFSInfo.verifyFilesystem) == 0 {
			continue
		}
		hasVerifyFs = true
		language := ruleLanguage
		if l := verifyFSInfo.language; l != "" {
			language, _ = ssaconfig.ValidateLanguage(l)
		}
		verify := &VerifyFileSystem{}
		vfs := filesys.NewVirtualFs()
		for name, content := range verifyFSInfo.verifyFilesystem {
			if language == "" {
				lidx := strings.LastIndex(name, ".")
				if lidx > 0 {
					language, _ = ssaconfig.ValidateLanguage(name[lidx+1:])
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
	ruleLanguage := s.rule.Language
	var result []*VerifyFileSystem
	for _, verifyFSInfo := range s.VerifyFsInfo {
		if len(verifyFSInfo.negativeFilesystem) == 0 {
			continue
		}
		language := ruleLanguage
		if l := verifyFSInfo.language; l != "" {
			language, _ = ssaconfig.ValidateLanguage(l)
		}
		verify := &VerifyFileSystem{}
		vfs := filesys.NewVirtualFs()
		for name, content := range verifyFSInfo.negativeFilesystem {
			if language == "" {
				lidx := strings.LastIndex(name, ".")
				if lidx > 0 {
					language, _ = ssaconfig.ValidateLanguage(name[lidx+1:])
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

func (s *SFFrame) ProcessCallback(msg string, args ...any) {
	if s.config.processCallback != nil {
		s.config.processCallback(s.idx, fmt.Sprintf(msg, args...))
	}
}
func (s *SFFrame) exec(feedValue ValueOperator) (ret error) {
	s.predCounter = 0
	defer func() {
		s.predCounter = 0
	}()

	// clear
	s.Flush()

	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Errorf("sft panic: %v", err)
			log.Infof("%+v", ret)
		}
		// 输出性能统计报告
		totalDuration := time.Since(start)
		enableRulePerf := s.config.diagnosticsEnabled
		s.logScanPerformance(totalDuration, enableRulePerf)
	}()

	// diagnostics: track rule execution timing
	ruleName := "unknown-rule"
	if s.rule != nil && s.rule.Title != "" {
		ruleName = s.rule.Title
	}

	return s.track("rule-execution:"+ruleName, func() error {
		return s.execRule(feedValue)
	})
}

func (s *SFFrame) execRule(feedValue ValueOperator) error {
	for {
		var msg string
		if s.idx < len(s.Codes) {
			msg = s.Codes[s.idx].String()
		} else {
			msg = "exec rule finished"
		}
		s.ProcessCallback(msg)
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
	if !s.config.debug {
		return
	}
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

func (s *SFFrame) startValueOpTiming(name string) func() {
	if s == nil || s.config == nil || !s.config.debug {
		return func() {}
	}
	start := time.Now()
	s.debugSubLog("value-op %s start", name)
	return func() {
		s.debugSubLog("value-op %s done (%s)", name, time.Since(start))
	}
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
