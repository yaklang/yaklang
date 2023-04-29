package spec

type ScriptRuntimeMessageType string

const (
	ScriptRuntimeMessageType_Output   ScriptRuntimeMessageType = "output"
	ScriptRuntimeMessageType_Finished ScriptRuntimeMessageType = "finished"
	ScriptRuntimeMessageType_Alert    ScriptRuntimeMessageType = "alert"
	ScriptRuntimeMessageType_Failed   ScriptRuntimeMessageType = "failed"
	ScriptRuntimeMessageType_Log      ScriptRuntimeMessageType = "log"
)

type ScriptRuntimeMessage struct {
	NodeID      string                   `json:"node_id"`
	TaskID      string                   `json:"task_id"`
	MessageType ScriptRuntimeMessageType `json:"message_type"`
	Message     string                   `json:"message"`
	Timestamp   int64                    `json:"timestamp"`
}

type ScriptTask struct {
	ScriptName string `json:"script_name"`
	TaskID     string `json:"task_id"`
	ScriptCode string `json:"script_code"`
}
