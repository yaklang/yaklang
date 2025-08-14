package aireact

import (
	"fmt"
	"io"
)

// EmitThought emits a thought event using the embedded Emitter
func (r *ReAct) EmitThought(thought string) {
	r.Emitter.EmitThought("thought", thought)
}

// EmitAction emits an action event using the embedded Emitter
func (r *ReAct) EmitAction(action string) {
	r.Emitter.EmitAction("action", action, "react")
}

// EmitObservation emits an observation event using the embedded Emitter
func (r *ReAct) EmitObservation(observation string) {
	r.Emitter.EmitObservation("observation", observation, "react")
}

// EmitIteration emits an iteration start event using the embedded Emitter
func (r *ReAct) EmitIteration(iteration int, maxIterations int) {
	description := fmt.Sprintf("ReAct iteration %d/%d started", iteration, maxIterations)
	r.Emitter.EmitIteration("iteration", iteration, maxIterations, description)
}

// EmitResult emits a final result event using the embedded Emitter
func (r *ReAct) EmitResult(result interface{}) {
	r.Emitter.EmitResult("result", result, true)
}

// ReAct 可以直接使用嵌入的 Emitter 的通用方法：
// r.EmitStructured(nodeId, data) 用于结构化数据
// r.EmitStream(nodeId, content) 用于流式数据
// r.EmitInfo(message), r.EmitError(message), r.EmitWarning(message) 用于日志

// EmitStreamReader emits streaming content from a reader using embedded Emitter
func (r *ReAct) EmitStreamReader(nodeId string, reader io.Reader) {
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				r.Emitter.EmitStream(nodeId, string(buffer[:n]))
			}
			if err != nil {
				if err != io.EOF {
					r.Emitter.EmitError(fmt.Sprintf("Stream read error: %v", err))
				}
				break
			}
		}
	}()
}

// Utility methods for ReAct instances

// getCoordinatorId returns the coordinator ID for the ReAct instance
func (r *ReAct) getCoordinatorId() string {
	if r.Emitter != nil {
		// Use the embedded Emitter's coordinator ID
		return fmt.Sprintf("react-%p", r.config)
	}
	return fmt.Sprintf("react-%p", r)
}

// getTaskIndex returns the task index for the ReAct instance
func (r *ReAct) getTaskIndex() string {
	// Read currentIteration without lock since it's an atomic read of an int
	// and we're not modifying it here
	return fmt.Sprintf("react-task-%d", r.config.currentIteration)
}
