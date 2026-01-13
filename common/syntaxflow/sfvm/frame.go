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

	stack          *utils.Stack[Values] // for filter
	conditionStack *utils.Stack[[]bool] // for condition
	popStack       *utils.Stack[Values] //pop stack,for sf

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

func newSfFrameEx(vars VarMap, text string, codes []*SFI, rule *schema.SyntaxFlowRule, config *Config) *SFFrame {
	v := vars
	if v.IsNil() {
		v = NewVarMap()
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

func NewSFFrame(vars VarMap, text string, codes []*SFI) *SFFrame {
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
	s.stack = utils.NewStack[Values]()
	s.errorSkipStack = utils.NewStack[*errorSkipContext]()
	s.conditionStack = utils.NewStack[[]bool]()
	s.popStack = utils.NewStack[Values]()
	s.idx = 0
}

// stackPop safely pops a value from the stack and checks for nil
// Returns the ValueList and an error if the stack is empty
func (s *SFFrame) stackPop() (Values, error) {
	if s.stack.Len() == 0 {
		s.debugSubLog(">> pop Error: empty stack")
		return nil, utils.Errorf("E: stack is empty, cannot pop")
	}
	val := s.stack.Pop()
	if val == nil {
		s.debugSubLog(">> pop Error: nil value")
		return nil, utils.Errorf("E: stack pop returned nil")
	}
	s.debugSubLog(">> pop %v", len(val))
	return val, nil
}

// stackPush safely pushes a value to the stack, ensuring width consistency
// It uses Foreach to maintain the same width as the current stack top
func (s *SFFrame) stackPush(value Values) {
	s.debugSubLog("<< push %v", len(value))
	s.stack.Push(value)
	return
}

// stackPeek safely peeks at the top of the stack
func (s *SFFrame) stackPeek() (Values, error) {
	if s.stack.Len() == 0 {
		s.debugSubLog(">> peek Error: empty stack")
		return nil, utils.Errorf("E: stack is empty, cannot peek")
	}
	val := s.stack.Peek()
	if val == nil {
		s.debugSubLog(">> peek Error: nil value")
		return nil, utils.Errorf("E: stack peek returned nil")
	}
	return val, nil
}

func (s *SFFrame) GetSymbolTable() VarMap {
	return s.result.SymbolTable
}
func (s *SFFrame) GetSymbol(sfi *SFI) (Values, bool) {
	if val, b := s.result.SymbolTable.Get(sfi.UnaryStr); b {
		return val, b
	}
	if initVars := s.config.initialContextVars; !initVars.IsNil() {
		val, ok := initVars.Get(sfi.UnaryStr)
		if ok {
			return val, ok
		}
		return Values{}, false
	} else {
		return Values{}, true
	}
}
func (s *SFFrame) GetSymbolByName(name string) (Values, bool) {
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
func (s *SFFrame) exec(feedValue Values) (ret error) {
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

func (s *SFFrame) execRule(feedValue Values) error {
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
				s.stackPush(feedValue)
			}
		case OpEnterStatement:
			s.errorSkipStack.Push(&errorSkipContext{
				start:      s.idx,
				end:        i.UnaryInt,
				stackDepth: s.stack.Len(),
			})

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

func (s *SFFrame) output(resultName string, values Values) error {
	// save to result, even if value is empty or nil
	if resultName == "_" {
		if unnameValue := s.result.UnNameValue; len(unnameValue) != 0 {
			values = append(values, s.result.UnNameValue...)
		}
		s.result.UnNameValue = values // for merge
	} else {
		if originValue, existed := s.GetSymbolTable().Get(resultName); existed {
			values = append(values, originValue...)
		}
		value := values
		s.GetSymbolTable().Set(resultName, value)
	}
	if s.config != nil {
		for _, callback := range s.config.onResultCapturedCallbacks {
			if err := callback(resultName, values); err != nil {
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
