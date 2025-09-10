package aireactdeps

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CLIConfig 包含 CLI 的配置选项
type CLIConfig struct {
	Language        string
	Query           string
	DebugMode       bool
	InteractiveMode bool
	BreakpointMode  bool
	FilePath        string // File to monitor with traced file context provider
}

// ReactApp 包含 ReAct 应用的核心组件
type ReactApp struct {
	ReactInstance *aireact.ReAct
	InputChan     chan *ypb.AIInputEvent
	OutputChan    chan *schema.AiOutputEvent
	Config        *CLIConfig
}

// GlobalState 管理全局应用状态
type GlobalState struct {
	// 用户输入
	UserInput chan string

	// 审核状态
	WaitingForReview bool
	ReviewOptions    []ReviewOption
	ReviewEventID    string
	ReviewMutex      sync.Mutex

	// 断点状态
	WaitingForBreakpoint bool
	BreakpointMutex      sync.Mutex

	// 流处理状态
	StreamingActive bool
	StreamingMutex  sync.Mutex
	StreamStartTime time.Time
	StreamCharCount int
	StreamDisplayed bool
	PendingResponse *aicommon.AIResponse

	// 活动指示器状态
	SpinnerActive bool
	SpinnerStop   chan bool
	SpinnerMutex  sync.Mutex
}

// ReviewOption 表示审核选择选项
type ReviewOption struct {
	Value  string `json:"value"`
	Prompt string `json:"prompt"`
}

// 全局状态实例
var globalState = &GlobalState{
	UserInput:   make(chan string, 100),
	SpinnerStop: make(chan bool, 1),
}

// GetGlobalState 返回全局状态实例
func GetGlobalState() *GlobalState {
	return globalState
}

// IsWaitingForReview 检查是否正在等待审核输入
func (gs *GlobalState) IsWaitingForReview() bool {
	gs.ReviewMutex.Lock()
	defer gs.ReviewMutex.Unlock()
	return gs.WaitingForReview
}

// SetReviewState 设置审核状态
func (gs *GlobalState) SetReviewState(waiting bool, options []ReviewOption, eventID string) {
	gs.ReviewMutex.Lock()
	defer gs.ReviewMutex.Unlock()
	gs.WaitingForReview = waiting
	gs.ReviewOptions = options
	gs.ReviewEventID = eventID
}

// GetReviewState 获取审核状态
func (gs *GlobalState) GetReviewState() (bool, []ReviewOption, string) {
	gs.ReviewMutex.Lock()
	defer gs.ReviewMutex.Unlock()
	return gs.WaitingForReview, gs.ReviewOptions, gs.ReviewEventID
}

// IsWaitingForBreakpoint 检查是否正在等待断点输入
func (gs *GlobalState) IsWaitingForBreakpoint() bool {
	gs.BreakpointMutex.Lock()
	defer gs.BreakpointMutex.Unlock()
	return gs.WaitingForBreakpoint
}

// SetBreakpointWaiting 设置断点等待状态
func (gs *GlobalState) SetBreakpointWaiting(waiting bool) {
	gs.BreakpointMutex.Lock()
	defer gs.BreakpointMutex.Unlock()
	gs.WaitingForBreakpoint = waiting
}

// IsStreamingActive 检查流是否处于活动状态
func (gs *GlobalState) IsStreamingActive() bool {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	return gs.StreamingActive
}

// SetStreamingState 设置流状态
func (gs *GlobalState) SetStreamingState(active bool, startTime time.Time, charCount int, displayed bool) {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	gs.StreamingActive = active
	gs.StreamStartTime = startTime
	gs.StreamCharCount = charCount
	gs.StreamDisplayed = displayed
}

// GetStreamingState 获取流状态
func (gs *GlobalState) GetStreamingState() (bool, time.Time, int, bool) {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	return gs.StreamingActive, gs.StreamStartTime, gs.StreamCharCount, gs.StreamDisplayed
}

// SetPendingResponse 设置待处理的响应
func (gs *GlobalState) SetPendingResponse(resp *aicommon.AIResponse) {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	gs.PendingResponse = resp
}

// GetPendingResponse 获取待处理的响应
func (gs *GlobalState) GetPendingResponse() *aicommon.AIResponse {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	return gs.PendingResponse
}

// ClearPendingResponse 清除待处理的响应
func (gs *GlobalState) ClearPendingResponse() *aicommon.AIResponse {
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()
	resp := gs.PendingResponse
	gs.PendingResponse = nil
	return resp
}

// IsSpinnerActive 检查活动指示器是否活跃
func (gs *GlobalState) IsSpinnerActive() bool {
	gs.SpinnerMutex.Lock()
	defer gs.SpinnerMutex.Unlock()
	return gs.SpinnerActive
}

// SetSpinnerActive 设置活动指示器状态
func (gs *GlobalState) SetSpinnerActive(active bool) {
	gs.SpinnerMutex.Lock()
	defer gs.SpinnerMutex.Unlock()
	gs.SpinnerActive = active
}

// 便捷函数
func setPendingResponse(resp *aicommon.AIResponse) {
	globalState.SetPendingResponse(resp)
}
