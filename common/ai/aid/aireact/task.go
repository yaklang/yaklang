package aireact

//
//// each single query/input create a task
//type Task struct {
//	ctx    context.Context
//	cancel context.CancelFunc
//
//	emitter *aicommon.Emitter
//
//	Id        string
//	UserInput string
//	result    *bytes.Buffer
//	Status    aicommon.AITaskState
//	CreatedAt time.Time
//}
//
//var _ aicommon.AIStatefulTask = (*Task)(nil)
//
//func (t *Task) SetUserInput(i string) {
//	if t == nil {
//		return
//	}
//	t.UserInput = i
//}
//
//func (t *Task) GetName() string {
//	if t == nil {
//		return ""
//	}
//	return t.Id
//}
//
//func (t *Task) GetIndex() string {
//	if t == nil {
//		return ""
//	}
//	return t.Id
//}
//
//func (t *Task) SetResult(i string) {
//	t.result.WriteString(fmt.Sprintf("[%v]: %v\n", utils.DatetimePretty2(), i))
//}
//
//func (t *Task) GetResult() string {
//	return t.result.String()
//}
//
//func (t *Task) GetContext() context.Context {
//	if t == nil {
//		return context.Background()
//	}
//	return t.ctx
//}
//
//func (t *Task) Cancel() {
//	if t == nil {
//		return
//	}
//	t.cancel()
//}
//
//func (t *Task) IsFinished() bool {
//	t.RLock()
//	defer t.RUnlock()
//
//	switch t.Status {
//	case aicommon.AITaskState_Completed, aicommon.AITaskState_Aborted:
//		return true
//	default:
//		return false
//	}
//}
//
//func (t *Task) GetId() string {
//	return t.Id
//}
//
//func (t *Task) GetUserInput() string {
//	return t.UserInput
//}
//
//func (t *Task) GetStatus() aicommon.AITaskState {
//	return t.Status
//}
//
//func (t *Task) SetStatus(status aicommon.AITaskState) {
//	t.Lock()
//	defer t.Unlock()
//
//	oldStatus := t.Status
//	t.Status = status
//
//	// 输出调试日志记录状态变化
//	if oldStatus != status {
//		log.Debugf("Task %s status changed: %s -> %s", t.Id, oldStatus, status)
//		if t.emitter != nil {
//			t.emitter.EmitStructured("react_task_status_changed", map[string]any{
//				"react_task_id":         t.Id,
//				"react_task_old_status": oldStatus,
//				"react_task_now_status": status,
//			})
//		}
//	}
//}
//
//func (t *Task) GetCreatedAt() time.Time {
//	return t.CreatedAt
//}
//
//func NewTask(id string, userInput string, emitter *aicommon.Emitter) *Task {
//	task := &Task{
//		RWMutex:   &sync.RWMutex{},
//		Id:        id,
//		UserInput: userInput,
//		Status:    aicommon.AITaskState_Created,
//		result:    new(bytes.Buffer),
//		CreatedAt: time.Now(),
//		emitter:   emitter,
//	}
//	if task.emitter != nil {
//		task.emitter.EmitStructured("react_task_created", map[string]any{
//			"react_task_id":     task.Id,
//			"react_user_input":  task.UserInput,
//			"react_task_status": task.Status,
//		})
//		log.Debugf("Task created: %s with input: %s", task.Id, task.UserInput)
//	} else {
//		//log.Warnf("Task created without emitter: %s with input: %s", task.Id, task.UserInput)
//	}
//	return task
//}
