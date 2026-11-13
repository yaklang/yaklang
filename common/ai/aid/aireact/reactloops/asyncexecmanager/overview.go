package asyncexecmanager

// Overview is a metadata-only view of all executions currently retained by a
// Manager. It never invokes a resource Inspector. Revision is monotonic within
// the Manager lifetime and covers lifecycle changes only.
type Overview struct {
	Revision   uint64    `json:"revision"`
	Executions []Summary `json:"executions"`
}

// RunningCount counts executions that still own an active slot. Pending and
// stopping executions are included because they have not finished execution
// and cleanup yet.
func (o Overview) RunningCount() int {
	count := 0
	for _, execution := range o.Executions {
		if !execution.State.IsTerminal() {
			count++
		}
	}
	return count
}

// Closed reports whether the manager has stopped accepting new executions.
func (m *Manager) Closed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}
