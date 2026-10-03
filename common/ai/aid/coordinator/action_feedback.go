package coordinator

// 任务结果仅保留可用于调度与验收的执行事实，不重复静态目标、依赖和任务书。
// Result 保留完整内容；不得为了缩短上下文丢失验收依据或 Evidence/artifact 引用。
type taskResultRecord struct {
	TaskID       string `json:"task_id"`
	AttemptID    uint64 `json:"attempt_id"`
	State        State  `json:"state"`
	Result       Result `json:"result"`
	ReviewReason string `json:"review_reason,omitempty"`
}

func taskResultRecords(tasks []Attempt) []taskResultRecord {
	out := make([]taskResultRecord, 0, len(tasks))
	for _, a := range tasks {
		out = append(out, taskResultRecord{a.Task.ID, a.ID, a.State, a.Result, a.ReviewReason})
	}
	return out
}
