package node

import (
	"encoding/json"
	"yaklang/common/spec"
	"time"
)

func (n *NodeBase) initScriptEngine() {
	if n.NodeType != spec.NodeType_Scanner {
		n.ScriptExecutor.RegisterOutputHook("nodebase", func(taskId string, data string) {
			msg := n.CreateOutputScriptRuntimeMessage(taskId, data)
			raw, err := json.Marshal(msg)
			if err != nil {
				return
			}
			o := n.NewBaseMessage(spec.MessageType_ScriptRuntime)
			o.Content = raw
			n.Notify(
				spec.GetScriptRuntimeMessageKey(n.NodeId, taskId),
				o,
			)
		})
		n.ScriptExecutor.RegisterFinishHook("nodebase", func(taskId string, data string) {
			msg := n.CreateFinishedScriptRuntimeMessage(taskId, data)
			raw, err := json.Marshal(msg)
			if err != nil {
				return
			}
			o := n.NewBaseMessage(spec.MessageType_ScriptRuntime)
			o.Content = raw
			n.Notify(
				spec.GetScriptRuntimeMessageKey(n.NodeId, taskId),
				o,
			)

		})
		n.ScriptExecutor.RegisterFailedHook("nodebase", func(taskId string, data string) {
			msg := n.CreateFailedScriptRuntimeMessage(taskId, data)
			raw, err := json.Marshal(msg)
			if err != nil {
				return
			}
			o := n.NewBaseMessage(spec.MessageType_ScriptRuntime)
			o.Content = raw
			n.Notify(
				spec.GetScriptRuntimeMessageKey(n.NodeId, taskId),
				o,
			)
		})
		n.ScriptExecutor.RegisterAlertHook("nodebase", func(taskId string, data string) {
			msg := n.CreateAlertScriptRuntimeMessage(taskId, data)
			raw, err := json.Marshal(msg)
			if err != nil {
				return
			}
			o := n.NewBaseMessage(spec.MessageType_ScriptRuntime)
			o.Content = raw
			n.Notify(
				spec.GetScriptRuntimeMessageKey(n.NodeId, taskId),
				o,
			)
		})
		n.ScriptExecutor.RegisterLogHook("nodebase", func(taskId string, data string) {
			msg := n.CreateLogScriptRuntimeMessage(taskId, data)
			raw, err := json.Marshal(msg)
			if err != nil {
				return
			}
			o := n.NewBaseMessage(spec.MessageType_ScriptRuntime)
			o.Content = raw
			n.Notify(
				spec.GetScriptRuntimeMessageKey(n.NodeId, taskId),
				o,
			)
		})
	}
}

func (n *NodeBase) CreateOutputScriptRuntimeMessage(taskId, msg string) *spec.ScriptRuntimeMessage {
	return &spec.ScriptRuntimeMessage{
		NodeID:      n.NodeId,
		TaskID:      taskId,
		MessageType: spec.ScriptRuntimeMessageType_Output,
		Message:     msg,
		Timestamp:   time.Now().Unix(),
	}
}

func (n *NodeBase) CreateFinishedScriptRuntimeMessage(taskId, msg string) *spec.ScriptRuntimeMessage {
	return &spec.ScriptRuntimeMessage{
		NodeID:      n.NodeId,
		TaskID:      taskId,
		MessageType: spec.ScriptRuntimeMessageType_Finished,
		Message:     msg,
		Timestamp:   time.Now().Unix(),
	}
}

func (n *NodeBase) CreateFailedScriptRuntimeMessage(taskId, msg string) *spec.ScriptRuntimeMessage {
	return &spec.ScriptRuntimeMessage{
		NodeID:      n.NodeId,
		TaskID:      taskId,
		MessageType: spec.ScriptRuntimeMessageType_Failed,
		Message:     msg,
		Timestamp:   time.Now().Unix(),
	}
}

func (n *NodeBase) CreateAlertScriptRuntimeMessage(taskId, msg string) *spec.ScriptRuntimeMessage {
	return &spec.ScriptRuntimeMessage{
		NodeID:      n.NodeId,
		TaskID:      taskId,
		MessageType: spec.ScriptRuntimeMessageType_Alert,
		Message:     msg,
		Timestamp:   time.Now().Unix(),
	}
}

func (n *NodeBase) CreateLogScriptRuntimeMessage(taskId, msg string) *spec.ScriptRuntimeMessage {
	return &spec.ScriptRuntimeMessage{
		NodeID:      n.NodeId,
		TaskID:      taskId,
		MessageType: spec.ScriptRuntimeMessageType_Log,
		Message:     msg,
		Timestamp:   time.Now().Unix(),
	}
}
