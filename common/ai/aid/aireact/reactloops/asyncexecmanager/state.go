package asyncexecmanager

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateStopping  State = "stopping"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
	StateTimedOut  State = "timed_out"
)

func (s State) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCancelled, StateTimedOut:
		return true
	default:
		return false
	}
}
