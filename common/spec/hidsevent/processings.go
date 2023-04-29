package hidsevent

type ProcessInfo struct {
	Count     int            `json:"count"`
	Processes []*ProcessMeta `json:"processes"`
}

type ProcessMeta struct {
	Pid           int32   `json:"pid"`
	ProcessName   string  `json:"process_name"`
	CommandLine   string  `json:"command_line"`
	ChildrenPid   []int32 `json:"children_pid"`
	ParentPid     int32   `json:"parent_pid"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"mem_percent"`
	Username      string  `json:"username"`
}

type ProcessEventType string

const (
	ProcessEventType_New       ProcessEventType = "new"
	ProcessEventType_Disappear ProcessEventType = "disappear"
)

type ProcessEvent struct {
	EventName   ProcessEventType `json:"event_name"`
	ProcessMeta *ProcessMeta     `json:"process"`
}
