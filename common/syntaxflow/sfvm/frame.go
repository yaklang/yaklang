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
	condDepth  int
	scopeDepth int
}

type anchorScopeState struct {
	conditionDepth int
	stackDepth     int

	mode ConditionMode

	anchorBase     int
	anchorWidth    int
	source         Values
	slotAnchorBits []*utils.BitVector
	anchorRestore  []anchorRestoreEntry
}

type SFFrame struct {
	vm *SyntaxFlowVirtualMachine

	config *Config
	rule   *schema.SyntaxFlowRule

	VerifyFsInfo []*VerifyFsInfo
	// initialContextVars is the symbol table snapshot used as the fallback
	// for OpNewRef when the current execution result has no such variable.
	//
	// It is separate from Config.initialContextVars to keep NewSFFrame usable
	// without requiring callers to thread options/config.
	initialContextVars *omap.OrderedMap[string, Values]

	// install meta info and result info
	result *SFFrameResult

	idx            int     // current opcode index
	currentProcess float64 // current process
	// callbackEvery is the per-opcode progress-callback emission period, scaled
	// to len(Codes) at exec start (exec) so a rule emits ~64 callbacks max. 1 for
	// short rules (preserve mid-rule cancel-at-threshold callers), total/64 for
	// long rules (cut SFI.String/Sprintf ~64x). Set in exec(); 0 = no callback
	// configured.
	callbackEvery int

	stack          *utils.Stack[Values]           // for filter
	conditionStack *utils.Stack[ConditionEntry]   // for condition
	anchorScope    *utils.Stack[anchorScopeState] // for anchor scope
	popStack       *utils.Stack[Values]           //pop stack,for sf

	// when cache err skip  statement/expr
	errorSkipStack *utils.Stack[*errorSkipContext]

	Text   string
	Codes  []*SFI // code list
	toLeft bool

	predCounter int
}

func (s *SFFrame) ActiveAnchorWidth() (int, bool) {
	_, w, ok := s.ActiveAnchorScope()
	return w, ok
}

func (s *SFFrame) ActiveAnchorScope() (int, int, bool) {
	if s == nil || s.anchorScope == nil || s.anchorScope.Len() == 0 {
		return 0, 0, false
	}
	state := s.anchorScope.Peek()
	if state.anchorWidth <= 0 {
		return 0, 0, false
	}
	return state.anchorBase, state.anchorWidth, true
}

func (s *SFFrame) nextAnchorScopeBase() int {
	if s == nil || s.anchorScope == nil || s.anchorScope.Len() == 0 {
		return 0
	}
	parent := s.anchorScope.Peek()
	return parent.anchorBase + parent.anchorWidth
}

func (s *SFFrame) beginAnchorScope(source Values) anchorScopeState {
	state := anchorScopeState{
		conditionDepth: s.conditionStack.Len(),
		stackDepth:     s.stack.Len(),
		mode:           conditionModeFromSource(source),
		anchorBase:     s.nextAnchorScopeBase(),
		anchorWidth:    len(source),
		source:         source.Clone(),
	}
	state.slotAnchorBits = buildSlotAnchorBitVectors(source, state.anchorBase)
	state.anchorRestore = assignLocalAnchorBitVector(source, state.anchorBase)
	return state
}

func (s *SFFrame) restoreAnchorScope(scope anchorScopeState) {
	restoreAnchorBitVector(scope.anchorRestore)
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

func newSfFrameEx(vars *omap.OrderedMap[string, Values], text string, codes []*SFI, rule *schema.SyntaxFlowRule, config *Config) *SFFrame {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, Values]()
	}
	if rule == nil {
		rule = &schema.SyntaxFlowRule{}
	}

	return &SFFrame{
		Text:               text,
		Codes:              codes,
		rule:               rule,
		config:             config,
		VerifyFsInfo:       make([]*VerifyFsInfo, 0),
		initialContextVars: v,
	}
}

func NewSFFrame(vars *omap.OrderedMap[string, Values], text string, codes []*SFI) *SFFrame {
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
	s.conditionStack = utils.NewStack[ConditionEntry]()
	s.anchorScope = utils.NewStack[anchorScopeState]()
	s.popStack = utils.NewStack[Values]()
	s.idx = 0
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, Values] {
	return s.result.SymbolTable
}
func (s *SFFrame) GetSymbol(sfi *SFI) (Values, bool) {
	if val, b := s.result.SymbolTable.Get(sfi.UnaryStr); b {
		return val, b
	}
	if s.config != nil {
		if initVars := s.config.initialContextVars; initVars != nil {
			return initVars.Get(sfi.UnaryStr)
		}
	}
	if s.initialContextVars != nil {
		if v, ok := s.initialContextVars.Get(sfi.UnaryStr); ok {
			return v, ok
		}
	}
	return NewEmptyValues(), true
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
		// Avoid fmt.Sprintf when there are no args: it still allocates a new
		// string (and parses the format verb). ProcessCallback is called per
		// opcode from execRule, so this is a hot path (Sprintf via
		// ProcessCallback was ~32% of fmt.Sprintf allocs on large projects).
		if len(args) == 0 {
			s.config.processCallback(s.idx, msg)
		} else {
			s.config.processCallback(s.idx, fmt.Sprintf(msg, args...))
		}
	}
}
func (s *SFFrame) exec(feedValue Values) (ret error) {
	s.predCounter = 0
	defer func() {
		s.predCounter = 0
	}()

	// Scale the per-opcode progress-callback emission period to the rule size:
	// ~64 callbacks max per rule. Short rules (<=64 opcodes) emit every opcode
	// so a caller cancelling mid-execution from a progress threshold
	// (Test_Context cancels at >=0.5) sees the mid-rule progress step; long
	// rules emit every (total/64) opcodes, cutting SFI.String/Sprintf ~64x on
	// the large-project hot path. 1 (not 0) so the modulo never divides by zero
	// and the first opcode always emits.
	const targetCallbacks = 64
	if total := len(s.Codes); total > targetCallbacks {
		s.callbackEvery = total / targetCallbacks
		if s.callbackEvery < 1 {
			s.callbackEvery = 1
		}
	} else {
		s.callbackEvery = 1
	}

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
	ruleLabel := "unknown-rule"
	if s.rule != nil {
		if s.rule.Title != "" {
			ruleLabel = s.rule.Title
		} else if s.rule.RuleName != "" {
			ruleLabel = s.rule.RuleName
		}
	}
	for {
		// Throttle the per-opcode progress callback: s.Codes[s.idx].String()
		// formats a debug string with fmt.Sprintf on EVERY opcode, and the scan
		// runner always registers a progress callback, so per-opcode emission
		// fired ~117M times on large projects (~5% of all allocations). The
		// progress consumer (UpdateRuleStatus) only needs a recent opcode label,
		// not every one. Scale the emission period to the rule size so we emit a
		// BOUNDED number of callbacks per rule (~64) regardless of opcode count:
		//   - short rules (<=64 opcodes): every opcode — preserves callers that
		//     cancel mid-execution from the callback at a progress threshold
		//     (e.g. Test_Context cancels at >=0.5; with few opcodes each one is a
		//     large progress step, so a mid-rule callback must fire to let it).
		//   - long rules (>64 opcodes): every (total/64) opcodes — cuts
		//     SFI.String + ProcessCallback Sprintf ~64x on the large-project hot
		// path. The final "finished" step always emits.
		//
		// The ctx.Done() check (below) still runs on EVERY opcode so a cancelled
		// rule ctx is honored quickly; only the label formatting is throttled.
		if s.config.processCallback != nil && (s.idx%s.callbackEvery == 0 || s.idx >= len(s.Codes)) {
			var msg string
			if s.idx < len(s.Codes) {
				msg = s.Codes[s.idx].String()
			} else {
				msg = "exec rule finished"
			}
			s.ProcessCallback(msg)
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
		// Diagnostics (rule-perf profiling) is OFF by default for code-scan
		// (QueryWithRuleDiagnosticsRecorder only when enableRulePerf). When off,
		// run the opcode directly — no closure, no "sfvm.op:..." name string
		// concat, no diagnostics.TrackLow variadic slice. Those used to allocate
		// per opcode (~117M opcodes on large projects) for zero benefit when
		// profiling was off — a top churn driver attributed through execRule/
		// execSyntaxFlowOp in pprof. When on (rare, profiling), build the label
		// and wrap in recorder.Track so the profile records per-opcode timing.
		recorder := s.GetDiagnosticsRecorder()
		var flow opFlow
		var err error
		if recorder != nil {
			name := "sfvm.op:" + i.OpCode.String()
			if ruleLabel != "" {
				name += ":" + ruleLabel
			}
			err = recorder.Track(name, func() error {
				f, e := s.execOneOpcode(i, feedValue)
				flow = f
				return e
			})
		} else {
			flow, err = s.execOneOpcode(i, feedValue)
		}
		if err != nil {
			return err
		}
		if flow == opReturn {
			return nil
		}
	}
	return nil
}

// opFlow is the per-opcode control signal returned by execOneOpcode.
// Declared at package scope (was inside the execRule loop, redeclared each
// iteration). opContinue = proceed to next opcode; opReturn = stop the rule.
type opFlow int

const (
	opContinue opFlow = iota
	opReturn
)

// execOneOpcode runs one SFI in the rule-execution loop. Extracted from the
// in-loop closure so the no-diagnostics path (the default code-scan case) is
// a plain method call with zero closure/name/TrackLow allocation; only the
// diagnostics-on path wraps it in recorder.Track (see execRule).
func (s *SFFrame) execOneOpcode(i *SFI, feedValue Values) (opFlow, error) {
	// special handler this exist opcode, because this shuold pop then debugLog it
	if i.OpCode == OpExitStatement {
		ctx := s.errorSkipStack.Pop()
		checkLen := ctx.stackDepth
		s.debugLog("%s\t|stack %d", i.String(), s.stack.Len())
		if s.stack.Len() != checkLen {
			err := utils.Errorf("filter statement stack unbalanced: %v vs want(%v)", s.stack.Len(), checkLen)
			s.debugSubLog("exit statement error:%v", err)
			if s.config.debug {
				return opContinue, err
			}
			s.stack.PopN(s.stack.Len() - checkLen)
		}

		// Error-skip can jump over scope-end opcodes; unwind scopes created in this statement.
		//
		// Each anchor scope start temporarily overwrites anchor bits on the current
		// source list (see OpAnchorScopeStart). If we skip past OpAnchorScopeEnd
		// we MUST restore those overwritten bits, otherwise anchor provenance would leak
		// across statements and break subsequent mask alignment.
		for s.anchorScope.Len() > ctx.scopeDepth {
			scope := s.anchorScope.Pop()
			s.restoreAnchorScope(scope)
		}
		for s.conditionStack.Len() > ctx.condDepth {
			s.conditionStack.Pop()
		}
		return opContinue, nil
	}

	s.debugLog("%s\t|stack %d", i.String(), s.stack.Len())

	switch i.OpCode {
	case OpCheckStackTop:
		if s.stack.Len() == 0 {
			s.debugSubLog(">> stack top is nil (push input)")
			s.pushStack(feedValue)
		}
	case OpAnchorScopeStart:
		if s.stack.Len() == 0 {
			return opContinue, utils.Wrap(CriticalError, "anchor scope start failed: stack top is empty")
		}
		// Anchor scopes always enable anchor bookkeeping so that derived values can map
		// back to their originating source slots via AnchorBitVector.
		//
		// For the current scope source list (stack top) with width = len(source):
		// - We reserve a disjoint bit-range [anchorBase, anchorBase+anchorWidth) for this scope.
		// - For each source slot i: localBits(i) = {anchorBase + i}.
		// - We write: source[i].bits = localBits(i) OR oldBits, and remember oldBits in anchorRestore.
		//
		// Nested scopes stack their ranges by shifting anchorBase = parent.base + parent.width
		// so inner scopes can add local provenance without overwriting outer provenance.
		sourceValues := s.stack.Peek()
		s.anchorScope.Push(s.beginAnchorScope(sourceValues))
	case OpAnchorScopeEnd:
		if s.anchorScope.Len() == 0 {
			break
		}
		scopeState := s.anchorScope.Pop()
		// Restore anchor bits overwritten at scope start so they don't leak to outer scopes
		// and other statements.
		s.restoreAnchorScope(scopeState)
		if s.stack.Len() != scopeState.stackDepth {
			return opContinue, utils.Wrapf(
				CriticalError,
				"anchor scope stack unbalanced: %d vs want(%d)",
				s.stack.Len(),
				scopeState.stackDepth,
			)
		}
		scopeLen := scopeState.conditionDepth
		if s.conditionStack.Len() <= scopeLen {
			break
		}
		beforeLen := s.conditionStack.Len()
		latest := s.conditionStack.Pop()
		dropped := 0
		for s.conditionStack.Len() > scopeLen {
			s.conditionStack.Pop()
			dropped++
		}
		if dropped > 0 {
			s.debugSubLog(
				"anchor scope end: dropped %d extra condition entries (before=%d wantDepth=%d)",
				dropped,
				beforeLen,
				scopeLen,
			)
		}
		s.conditionStack.Push(latest)
	case OpEnterStatement:
		s.errorSkipStack.Push(&errorSkipContext{
			start:      s.idx,
			end:        i.UnaryInt,
			stackDepth: s.stack.Len(),
			condDepth:  s.conditionStack.Len(),
			scopeDepth: s.anchorScope.Len(),
		})

	default:
		if err := s.execStatement(i); err != nil {
			s.debugSubLog("execStatement error: %v", err)
			if errors.Is(err, AbortError) {
				return opReturn, nil
			}
			if errors.Is(err, CriticalError) {
				return opContinue, err
			}
			// go to expression end
			if result := s.errorSkipStack.Peek(); result != nil {
				s.idx = result.end
				return opContinue, nil
			}
			return opContinue, err
		}
	}
	return opContinue, nil
}

var CriticalError = utils.Error("CriticalError(Immediately Abort)")
var AbortError = utils.Error("AbortError(Normal Abort)")

func (s *SFFrame) pushStack(value Values) {
	s.stack.Push(value)
}

func (s *SFFrame) output(resultName string, operator Values) error {
	// save to result, even if value is empty or nil
	if resultName == "_" {
		s.result.UnNameValue = MergeValues(operator, s.result.UnNameValue)
	} else {
		originValue, _ := s.GetSymbolTable().Get(resultName)
		s.GetSymbolTable().Set(resultName, MergeValues(operator, originValue))
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
