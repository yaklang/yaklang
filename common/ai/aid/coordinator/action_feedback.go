package coordinator

// 计划正文和任务书只展示在稳定的 PLAN DEFINITION / PLAN DOCUMENT 中。
// 查询回执不复制计划，避免大文档随每次 action 写入动态反馈和调用历史。
type planContextReceipt struct {
	DraftVersion     uint64 `json:"draft_version"`
	ApprovedVersion  uint64 `json:"approved_version"`
	SubmittedVersion uint64 `json:"submitted_version"`
}

func planReceipt(s Snapshot) planContextReceipt {
	return planContextReceipt{s.DraftVersion, s.ApprovedVersion, s.SubmittedVersion}
}

func (c *Controller) planReceipt() planContextReceipt {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 核对版本无需克隆完整计划和全部任务结果。
	return planReceipt(c.state)
}

// 任务结果仅保留可用于调度与验收的执行事实，不重复静态目标、依赖和任务书。
// Result 保留完整内容；不得为了缩短上下文丢失验收依据或 Evidence/artifact 引用。
type taskResultRecord struct {
	TaskID       string `json:"task_id"`
	AttemptID    uint64 `json:"attempt_id"`
	PlanVersion  uint64 `json:"plan_version"`
	State        State  `json:"state"`
	Result       Result `json:"result"`
	ReviewReason string `json:"review_reason,omitempty"`
}

func taskResultRecords(tasks []Attempt) []taskResultRecord {
	out := make([]taskResultRecord, 0, len(tasks))
	for _, a := range tasks {
		out = append(out, taskResultRecord{a.Task.ID, a.ID, a.PlanVersion, a.State, a.Result, a.ReviewReason})
	}
	return out
}
