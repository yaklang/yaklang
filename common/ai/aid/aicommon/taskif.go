package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AITask interface {
	GetIndex() string
	GetName() string
	GetSemanticIdentifier() string
	SetSemanticIdentifier(string)
	PushToolCallResult(result *aitool.ToolResult)
	GetAllToolCallResults() []*aitool.ToolResult
	GetSummary() string
	SetSummary(summary string)
}

type AITaskRetrievalInfo struct {
	Tags      []string `json:"tags,omitempty"`
	Questions []string `json:"questions,omitempty"`
	Target    string   `json:"target,omitempty"`
}

func (a *AITaskRetrievalInfo) Clone() *AITaskRetrievalInfo {
	if a == nil {
		return nil
	}
	return &AITaskRetrievalInfo{
		Tags:      append([]string{}, a.Tags...),
		Questions: append([]string{}, a.Questions...),
		Target:    a.Target,
	}
}

type AITaskState string

const (
	AITaskState_Created    AITaskState = "created"
	AITaskState_Queueing   AITaskState = "queueing"
	AITaskState_Processing AITaskState = "processing"
	AITaskState_Completed  AITaskState = "completed"
	AITaskState_Aborted    AITaskState = "aborted" // 任务执行失败或异常终止
	AITaskState_Skipped    AITaskState = "skipped" // 用户主动跳过
)

type AIStatefulTask interface {
	AITask

	GetId() string
	GetTaskRetrievalInfo() *AITaskRetrievalInfo
	SetTaskRetrievalInfo(*AITaskRetrievalInfo)
	SetAsyncDeferCallback(func(err error))
	CallAsyncDeferCallback(err error)
	SetResult(string)
	GetResult() string
	GetContext() context.Context
	Cancel(reasons ...string)
	GetCancelReason() string
	IsFinished() bool
	GetUserInput() string
	GetOriginUserInput() string
	SetUserInput(string)
	SetAttachedDatas([]*AttachedResource)
	GetAttachedDatas() []*AttachedResource
	GetStatus() AITaskState
	SetStatus(state AITaskState)
	ForceSetStatus(state AITaskState)
	AppendErrorToResult(i error)
	GetCreatedAt() time.Time
	Finish(i error)
	SetAsyncMode(async bool)
	IsAsyncMode() bool
	GetEmitter() *Emitter
	SetEmitter(emitter *Emitter)
	SetReActLoop(loop ReActLoopIF)
	GetReActLoop() ReActLoopIF

	SetDB(db *gorm.DB)
	GetRisks() []*schema.Risk
	GetUUID() string
	GetUserInputUUID() string
	SetUserInputUUID(uuid string)

	GetFocusMode() string
	SetFocusMode(mode string)

	IsSubAgent() bool
	SetSubAgent(isSubAgent bool)

	IsUserCancelled() bool
	SetUserCancelled()
}

type AIStatefulTaskBase struct {
	*Emitter

	id        string
	name      string
	userInput string
	result    string
	ctx       context.Context
	cancel    context.CancelFunc
	taskMutex *sync.Mutex
	status    AITaskState
	createdAt time.Time
	asyncMode bool

	summary       string
	statusSummary string

	// index info
	toolCallResultIds *omap.OrderedMap[int64, *aitool.ToolResult]
	reActLoop         ReActLoopIF
	db                *gorm.DB

	uuid          string
	userInputUUID string
	attachedDatas []*AttachedResource

	focusMode         string
	semanticLabel     string
	taskRetrievalInfo *AITaskRetrievalInfo

	asyncDeferCallback func(err error)

	skipTaskStatusChangeEmit bool
	isSubAgent               bool

	cancelReason string

	userCancelled bool
}

func (s *AIStatefulTaskBase) GetFocusMode() string {
	return s.focusMode
}

func (s *AIStatefulTaskBase) GetSemanticLabel() string {
	return s.semanticLabel
}

func (s *AIStatefulTaskBase) SetSemanticLabel(label string) {
	s.semanticLabel = label
}

// GetSemanticIdentifier returns the semantic identifier for directory naming.
// Falls back to semanticLabel if set, otherwise returns the task name.
func (s *AIStatefulTaskBase) GetSemanticIdentifier() string {
	if s == nil {
		return ""
	}
	if s.semanticLabel != "" {
		return s.semanticLabel
	}
	return s.name
}

// SetSemanticIdentifier sets the semantic identifier used for directory naming.
func (s *AIStatefulTaskBase) SetSemanticIdentifier(id string) {
	if s == nil {
		return
	}
	s.semanticLabel = id
}

func (s *AIStatefulTaskBase) GetTaskRetrievalInfo() *AITaskRetrievalInfo {
	if s == nil {
		return nil
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	return s.taskRetrievalInfo.Clone()
}

func (s *AIStatefulTaskBase) SetTaskRetrievalInfo(info *AITaskRetrievalInfo) {
	if s == nil {
		return
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	s.taskRetrievalInfo = info.Clone()
}

func (s *AIStatefulTaskBase) SetFocusMode(mode string) {
	s.focusMode = mode
}

func (s *AIStatefulTaskBase) IsSubAgent() bool {
	if s == nil {
		return false
	}
	return s.isSubAgent
}

func (s *AIStatefulTaskBase) SetSubAgent(isSubAgent bool) {
	if s == nil {
		return
	}
	s.isSubAgent = isSubAgent
}

func (s *AIStatefulTaskBase) GetUUID() string {
	return s.uuid
}

func (s *AIStatefulTaskBase) GetUserInputUUID() string {
	if s == nil {
		return ""
	}
	return s.userInputUUID
}

func (s *AIStatefulTaskBase) SetUserInputUUID(uuid string) {
	if s == nil {
		return
	}
	s.userInputUUID = uuid
}

func (s *AIStatefulTaskBase) GetDB() *gorm.DB {
	return s.db
}

func (s *AIStatefulTaskBase) SetDB(db *gorm.DB) {
	if s == nil {
		return
	}
	s.db = db
}

func (r *AIStatefulTaskBase) GetRisks() []*schema.Risk {
	if r == nil {
		return nil
	}
	if r.GetDB() == nil {
		return nil
	}
	events, err := yakit.QueryAIEvent(r.GetDB(), &ypb.AIEventFilter{
		TaskUUID:  []string{r.GetUUID()},
		EventType: []string{schema.EVENT_TYPE_YAKIT_RISK},
	})
	if err != nil {
		return nil
	}

	risks := []*schema.Risk{}
	for _, event := range events {
		if event.Type == schema.EVENT_TYPE_YAKIT_RISK {
			riskInfo := map[string]any{}
			err := json.Unmarshal(event.Content, &riskInfo)
			if err != nil {
				continue
			}
			riskId, ok := riskInfo["risk_id"]
			if ok && riskId != nil {
				id := utils.InterfaceToInt(riskId)
				risk, err := yakit.GetRisk(r.GetDB(), int64(id))
				if err != nil {
					continue
				}
				risks = append(risks, risk)
			}
		}
	}
	return risks
}

func (s *AIStatefulTaskBase) PushToolCallResult(result *aitool.ToolResult) {
	if s.toolCallResultIds == nil {
		s.toolCallResultIds = omap.NewOrderedMap[int64, *aitool.ToolResult](make(map[int64]*aitool.ToolResult))
	}
	s.toolCallResultIds.Set(result.ID, result)
}

func (s *AIStatefulTaskBase) GetAllToolCallResults() []*aitool.ToolResult {
	results := make([]*aitool.ToolResult, 0, s.toolCallResultIds.Len())
	for _, result := range s.toolCallResultIds.Values() {
		results = append(results, result)
	}
	return results
}

func (s *AIStatefulTaskBase) ToolCallResultsID() []int64 {
	return s.toolCallResultIds.Keys()
}

func (s *AIStatefulTaskBase) GetSummary() string {
	return s.summary
}

func (s *AIStatefulTaskBase) SetSummary(summary string) {
	s.summary = summary
}

func (s *AIStatefulTaskBase) AppendErrorToResult(i error) {
	if utils.IsNil(i) {
		return
	}
	result := s.GetResult()
	if result != "" {
		result += "\n\n"
	}
	result += fmt.Sprintf("[ERR] %v", i)
	s.SetResult(result)
}

func (s *AIStatefulTaskBase) SetName(name string) {
	if s == nil {
		return
	}
	s.name = name
}

func (s *AIStatefulTaskBase) SetAsyncMode(async bool) {
	if s == nil {
		return
	}
	s.asyncMode = async
}

func (s *AIStatefulTaskBase) IsAsyncMode() bool {
	if s == nil {
		return false
	}
	return s.asyncMode
}

func (s *AIStatefulTaskBase) GetIndex() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *AIStatefulTaskBase) SetAsyncDeferCallback(callback func(err error)) {
	if s == nil {
		return
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	s.asyncDeferCallback = callback
}

func (s *AIStatefulTaskBase) CallAsyncDeferCallback(err error) {
	if s == nil {
		return
	}

	var callback func(error)
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		if s.asyncDeferCallback == nil {
			s.taskMutex.Unlock()
			return
		}
		callback = s.asyncDeferCallback
		s.taskMutex.Unlock()
	} else {
		if s.asyncDeferCallback == nil {
			return
		}
		callback = s.asyncDeferCallback
	}

	callback(err)
}

func (s *AIStatefulTaskBase) Finish(i error) {
	if s.IsFinished() {
		return
	}

	if utils.IsNil(i) {
		s.SetStatus(AITaskState_Completed)
		return
	}
	s.AppendErrorToResult(i)
	s.SetStatus(AITaskState_Aborted)
}

func (s *AIStatefulTaskBase) GetName() string {
	if s == nil {
		return ""
	}
	return s.name
}

func (s *AIStatefulTaskBase) GetId() string {
	return s.id
}

func (s *AIStatefulTaskBase) SetResult(s2 string) {
	s.result = s2
}

func (s *AIStatefulTaskBase) GetResult() string {
	return s.result
}

func (s *AIStatefulTaskBase) GetContext() context.Context {
	if s.ctx == nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	return s.ctx
}

func (s *AIStatefulTaskBase) Cancel(reasons ...string) {
	if s == nil {
		return
	}
	var reason string
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	// first-writer-wins: 首个带原因的 Cancel 调用决定原因；后续 Cancel
	// 调用不覆盖。注意 setStatus 进入终止态的兜底取消直接调用裸
	// cancel()，不经此方法，因此不会写入原因，彻底消除顺序歧义。
	if s.cancelReason == "" && reason != "" {
		s.cancelReason = reason
		log.Debugf("Task %s cancelled: %s", s.GetId(), reason)
	}
	if s.cancel == nil {
		return
	}
	s.cancel()
}

// GetCancelReason 返回任务被取消时记录的原因。
// 采用 first-writer-wins 语义：首个带原因的 Cancel 调用决定原因，
// 后续调用（包括进入终止态时的自动取消）不会覆盖它。
func (s *AIStatefulTaskBase) GetCancelReason() string {
	if s == nil {
		return ""
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	return s.cancelReason
}

// SetUserCancelled marks the task as cancelled by the user via a sync event
// (e.g. cancel task, jump queue, skip subtask). When set, the ReAct loop's
// abort() will skip setting Aborted / appending [Error] so the user-initiated
// terminal state (Skipped) survives the race with the loop's own teardown.
func (s *AIStatefulTaskBase) SetUserCancelled() {
	if s == nil {
		return
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	s.userCancelled = true
}

func (s *AIStatefulTaskBase) IsUserCancelled() bool {
	if s == nil {
		return false
	}
	if s.taskMutex != nil {
		s.taskMutex.Lock()
		defer s.taskMutex.Unlock()
	}
	return s.userCancelled
}

func (s *AIStatefulTaskBase) IsFinished() bool {
	switch s.status {
	case AITaskState_Completed, AITaskState_Aborted, AITaskState_Skipped:
		return true
	default:
		return false
	}
}

func (s *AIStatefulTaskBase) GetOriginUserInput() string {
	return s.userInput
}

func (s *AIStatefulTaskBase) GetUserInput() string {
	return s.GetOriginUserInput()
}

func (s *AIStatefulTaskBase) SetUserInput(s2 string) {
	s.userInput = s2
}

func (s *AIStatefulTaskBase) GetStatus() AITaskState {
	return s.status
}

func (s *AIStatefulTaskBase) SetStatus(status AITaskState) {
	s.setStatus(status, false)
}

// ForceSetStatus bypasses the finished-state guard while preserving
// lifecycle side effects such as event emission and cancellation when
// entering a terminal state. Callers must ensure the task has a usable
// context when reviving a previously finished task.
func (s *AIStatefulTaskBase) ForceSetStatus(status AITaskState) {
	s.setStatus(status, true)
}

func (s *AIStatefulTaskBase) setStatus(status AITaskState, force bool) {
	if s == nil {
		return
	}
	if !force && s.IsFinished() {
		return // 已完成的任务状态不可更改
	}
	old := s.status
	s.status = status

	defer func() {
		if s.IsFinished() {
			if s.cancel != nil {
				s.cancel()
			}
		}
	}()

	// 输出调试日志记录状态变化
	if old != status {
		log.Debugf("Task %s status changed: %s -> %s", s.GetId(), old, status)
		if !s.skipTaskStatusChangeEmit && s.Emitter != nil {
			s.Emitter.EmitStructured("react_task_status_changed", map[string]any{
				"react_task_id":         s.GetId(),
				"react_task_old_status": old,
				"react_task_now_status": status,
			})
		}
	}
}

// RestoreStatus rehydrates a persisted status without triggering cancellation or events.
// This is used when rebuilding task state from storage and the task may need a fresh context.
func (s *AIStatefulTaskBase) RestoreStatus(status AITaskState) {
	s.status = status
}

func (s *AIStatefulTaskBase) GetCreatedAt() time.Time {
	return s.createdAt
}

func (s *AIStatefulTaskBase) SetID(id string) {
	s.id = id
}

func (s *AIStatefulTaskBase) GetEmitter() *Emitter {
	return s.Emitter
}

func (s *AIStatefulTaskBase) SetEmitter(emitter *Emitter) {
	s.Emitter = emitter
}

// WithEmitterProcessor temporarily pushes processor onto this task's emitter under taskMutex.
// Used so concurrent tool calls do not corrupt a shared emitter chain via push/pop races.
func (s *AIStatefulTaskBase) WithEmitterProcessor(processor EventProcesser, fn func()) {
	if s == nil {
		if fn != nil {
			fn()
		}
		return
	}
	if fn == nil {
		return
	}
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()
	prev := s.Emitter
	if processor != nil && prev != nil {
		s.Emitter = prev.PushEventProcesser(processor)
	}
	defer func() {
		s.Emitter = prev
	}()
	fn()
}

// EmitterTaskBase returns the concrete task base used for emitter push/pop scopes.
func (s *AIStatefulTaskBase) EmitterTaskBase() *AIStatefulTaskBase {
	return s
}

// WithEmitterProcessorOnTask runs fn with a temporary processor on the task emitter.
// Uses mutex serialization on AIStatefulTaskBase (including thin wrappers that expose EmitterTaskBase).
func WithEmitterProcessorOnTask(task AIStatefulTask, processor EventProcesser, fn func()) {
	if fn == nil {
		return
	}
	if task == nil {
		fn()
		return
	}
	type emitterTaskBase interface {
		EmitterTaskBase() *AIStatefulTaskBase
	}
	if holder, ok := task.(emitterTaskBase); ok {
		if base := holder.EmitterTaskBase(); base != nil {
			base.WithEmitterProcessor(processor, fn)
			return
		}
	}
	emitter := task.GetEmitter()
	if processor != nil && emitter != nil {
		task.SetEmitter(emitter.PushEventProcesser(processor))
		defer task.SetEmitter(emitter.PopEventProcesser())
	}
	fn()
}

func (s *AIStatefulTaskBase) GetReActLoop() ReActLoopIF {
	return s.reActLoop
}

func (s *AIStatefulTaskBase) SetReActLoop(loop ReActLoopIF) {
	s.reActLoop = loop
}

func (s *AIStatefulTaskBase) GetAttachedDatas() []*AttachedResource {
	return s.attachedDatas
}

func (s *AIStatefulTaskBase) SetAttachedDatas(attachedDatas []*AttachedResource) {
	s.attachedDatas = attachedDatas
}

var _ AIStatefulTask = (*AIStatefulTaskBase)(nil)

// NewSubTaskBase 创建一个"子任务"——从父 task 的 context 派生独立 context。
// 子任务完成/取消时只影响自身 context，不会取消父任务的 context，
// 因此多个子任务可以串行复用同一父 task 的生命周期。
func NewSubTaskBase(
	parentTask AIStatefulTask,
	subTaskId string,
	userInput string,
	skipTaskStatusChangeEmit ...bool,
) *AIStatefulTaskBase {
	opts := []StatefulTaskBaseOption{}
	if len(skipTaskStatusChangeEmit) > 0 && skipTaskStatusChangeEmit[0] {
		opts = append(opts, WithStatefulTaskBaseSkipTaskStatusChangeEmit())
	}
	return NewSubTaskBaseWithOptions(parentTask, subTaskId, userInput, opts...)
}

func NewSubTaskBaseWithOptions(
	parentTask AIStatefulTask,
	subTaskId string,
	userInput string,
	opts ...StatefulTaskBaseOption,
) *AIStatefulTaskBase {
	var parentCtx context.Context
	if parentTask != nil {
		parentCtx = parentTask.GetContext()
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	var emitter *Emitter
	if parentTask != nil {
		emitter = parentTask.GetEmitter()
	}
	baseOpts := []StatefulTaskBaseOption{
		WithStatefulTaskBaseContext(parentCtx),
		WithStatefulTaskBaseEmitter(emitter),
	}
	baseOpts = append(baseOpts, opts...)
	return newStatefulTaskBase(subTaskId, userInput, baseOpts...)
}

func NewStatefulTaskBase(
	taskId string,
	userInput string,
	ctx context.Context,
	Emitter *Emitter,
	skipTaskStatusChangeEmit ...bool,
) *AIStatefulTaskBase {
	opts := []StatefulTaskBaseOption{
		WithStatefulTaskBaseContext(ctx),
		WithStatefulTaskBaseEmitter(Emitter),
	}
	if len(skipTaskStatusChangeEmit) > 0 && skipTaskStatusChangeEmit[0] {
		opts = append(opts, WithStatefulTaskBaseSkipTaskStatusChangeEmit())
	}
	return newStatefulTaskBase(taskId, userInput, opts...)
}

func newStatefulTaskBase(taskId string, userInput string, opts ...StatefulTaskBaseOption) *AIStatefulTaskBase {
	base := &AIStatefulTaskBase{
		id:                taskId,
		userInput:         userInput,
		taskMutex:         &sync.Mutex{},
		status:            AITaskState_Created,
		createdAt:         time.Now(),
		toolCallResultIds: omap.NewOrderedMap[int64, *aitool.ToolResult](make(map[int64]*aitool.ToolResult)),
		uuid:              ksuid.New().String(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(base)
		}
	}
	if base.ctx == nil {
		base.ctx, base.cancel = context.WithCancel(context.Background())
	}

	if base.Emitter != nil {
		base.Emitter = base.Emitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
			if event != nil {
				event.TaskUUID = base.GetUUID()
				event.TaskId = base.GetId()
			}
			return event
		})
		if !base.skipTaskStatusChangeEmit {
			base.Emitter.EmitStructured(
				"react_task_created",
				map[string]any{
					"react_task_status":       base.status,
					"react_user_input":        userInput,
					"react_task_id":           taskId,
					"react_task_uuid":         base.GetUUID(),
					"react_task_name":         base.GetName(),
					"react_task_is_sub_agent": base.IsSubAgent(),
				},
			)
		}
	}
	return base
}

type StatefulTaskBaseOption func(*AIStatefulTaskBase)

func WithStatefulTaskBaseContext(ctx context.Context) StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		if ctx == nil {
			ctx = context.Background()
		}
		task.ctx, task.cancel = context.WithCancel(ctx)
	}
}

func WithStatefulTaskBaseContextAndCancel(ctx context.Context, cancel context.CancelFunc) StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		if ctx == nil {
			ctx = context.Background()
		}
		task.ctx, task.cancel = ctx, cancel
	}
}

func WithStatefulTaskBaseEmitter(emitter *Emitter) StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		task.Emitter = emitter
	}
}

func WithStatefulTaskBaseSkipTaskStatusChangeEmit() StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		task.skipTaskStatusChangeEmit = true
	}
}

func WithStatefulTaskBaseName(name string) StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		task.name = name
	}
}

func WithStatefulTaskBaseSubAgent() StatefulTaskBaseOption {
	return func(task *AIStatefulTaskBase) {
		if task == nil {
			return
		}
		task.isSubAgent = true
	}
}
