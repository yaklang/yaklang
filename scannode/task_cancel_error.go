package scannode

type TaskCancelledError struct {
	Reason string
}

func (e *TaskCancelledError) Error() string {
	if e == nil || e.Reason == "" {
		return "task cancelled"
	}
	return "task cancelled: " + e.Reason
}
