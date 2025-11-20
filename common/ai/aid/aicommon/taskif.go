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
	PushToolCallResult(result *aitool.ToolResult)
	GetAllToolCallResults() []*aitool.ToolResult
	GetSummary() string
	SetSummary(summary string)
}

type AITaskState string

const (
	AITaskState_Created    AITaskState = "created"
	AITaskState_Queueing   AITaskState = "queueing"
	AITaskState_Processing AITaskState = "processing"
	AITaskState_Completed  AITaskState = "completed"
	AITaskState_Aborted    AITaskState = "aborted"
)

type AIStatefulTask interface {
	AITask

	GetId() string
	SetResult(string)
	GetResult() string
	GetContext() context.Context
	Cancel()
	IsFinished() bool
	GetUserInput() string
	SetUserInput(string)
	GetStatus() AITaskState
	SetStatus(state AITaskState)
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

	uuid string
}

func (s *AIStatefulTaskBase) GetUUID() string {
	return s.uuid
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

func (s *AIStatefulTaskBase) Finish(i error) {
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

func (s *AIStatefulTaskBase) Cancel() {
	if s.cancel == nil {
		return
	}
	s.cancel()
}

func (s *AIStatefulTaskBase) IsFinished() bool {
	switch s.status {
	case AITaskState_Completed, AITaskState_Aborted:
		return true
	default:
		return false
	}
}

func (s *AIStatefulTaskBase) GetUserInput() string {
	return s.userInput
}

func (s *AIStatefulTaskBase) SetUserInput(s2 string) {
	s.userInput = s2
}

func (s *AIStatefulTaskBase) GetStatus() AITaskState {
	return s.status
}

func (s *AIStatefulTaskBase) SetStatus(status AITaskState) {
	old := s.status
	s.status = status

	defer func() {
		if s.IsFinished() {
			s.Cancel()
		}
	}()

	// 输出调试日志记录状态变化
	if old != status {
		log.Debugf("Task %s status changed: %s -> %s", s.GetId(), old, status)
		if s.Emitter != nil {
			s.Emitter.EmitStructured("react_task_status_changed", map[string]any{
				"react_task_id":         s.GetId(),
				"react_task_old_status": old,
				"react_task_now_status": status,
			})
		}
	}
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
func (s *AIStatefulTaskBase) GetReActLoop() ReActLoopIF {
	return s.reActLoop
}

func (s *AIStatefulTaskBase) SetReActLoop(loop ReActLoopIF) {
	s.reActLoop = loop
}

var _ AIStatefulTask = (*AIStatefulTaskBase)(nil)

func NewStatefulTaskBase(
	taskId string,
	userInput string,
	ctx context.Context,
	Emitter *Emitter,
	skipEvent ...bool,
) *AIStatefulTaskBase {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	base := &AIStatefulTaskBase{
		id:                taskId,
		userInput:         userInput,
		ctx:               ctx,
		cancel:            cancel,
		taskMutex:         &sync.Mutex{},
		Emitter:           Emitter,
		status:            AITaskState_Created,
		createdAt:         time.Now(),
		toolCallResultIds: omap.NewOrderedMap[int64, *aitool.ToolResult](make(map[int64]*aitool.ToolResult)),
		uuid:              ksuid.New().String(),
	}
	if base.Emitter != nil {
		base.Emitter = base.Emitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
			if event != nil {
				event.TaskUUID = base.GetUUID()
			}
			return event
		})
		if len(skipEvent) > 0 && skipEvent[0] {

		} else {
			base.Emitter.EmitStructured(
				"react_task_created",
				map[string]any{
					"react_task_status": base.status,
					"react_user_input":  userInput,
					"react_task_id":     taskId,
					"react_task_uuid":   base.GetUUID(),
				},
			)
		}
	}
	return base
}
