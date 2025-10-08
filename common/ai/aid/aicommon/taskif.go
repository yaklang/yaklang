package aicommon

import (
	"context"
	"sync"
	"time"
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
	GetCreatedAt() time.Time
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
}

func (s *AIStatefulTaskBase) SetName(name string) {
	if s == nil {
		return
	}
	s.name = name
}

func (s *AIStatefulTaskBase) GetIndex() string {
	if s == nil {
		return ""
	}
	return s.id
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

func (s AIStatefulTaskBase) Cancel() {
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

func (s *AIStatefulTaskBase) SetStatus(state AITaskState) {
	s.status = state
}

func (s *AIStatefulTaskBase) GetCreatedAt() time.Time {
	return s.createdAt
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
