package aicommon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AITask interface {
	GetIndex() string
	GetName() string
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
	SetReActLoop(loop ReActLoopIF)
	GetReActLoop() ReActLoopIF
}

type AIStatefulTaskBase struct {
	id        string
	name      string
	userInput string
	result    string
	ctx       context.Context
	cancel    context.CancelFunc
	taskMutex *sync.Mutex
	emitter   *Emitter
	status    AITaskState
	createdAt time.Time
	asyncMode bool
	reActLoop ReActLoopIF
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
		if s.emitter != nil {
			s.emitter.EmitStructured("react_task_status_changed", map[string]any{
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
	emitter *Emitter,
) *AIStatefulTaskBase {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	base := &AIStatefulTaskBase{
		id:        taskId,
		userInput: userInput,
		ctx:       ctx,
		cancel:    cancel,
		taskMutex: &sync.Mutex{},
		emitter:   emitter,
		status:    AITaskState_Created,
		createdAt: time.Now(),
	}
	if base.emitter != nil {
		base.emitter.EmitStructured(
			"react_task_created",
			map[string]any{
				"react_task_status": base.status,
				"react_user_input":  userInput,
				"react_task_id":     taskId,
			},
		)
	}
	return base
}
