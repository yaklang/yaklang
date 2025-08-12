package aireact

import (
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ReActEventType represents different types of ReAct events
type ReActEventType string

const (
	EventTypeReActThought     ReActEventType = "react_thought"
	EventTypeReActAction      ReActEventType = "react_action"
	EventTypeReActObservation ReActEventType = "react_observation"
	EventTypeReActInfo        ReActEventType = "react_info"
	EventTypeReActError       ReActEventType = "react_error"
	EventTypeReActWarning     ReActEventType = "react_warning"
	EventTypeReActIteration   ReActEventType = "react_iteration"
	EventTypeReActResult      ReActEventType = "react_result"
)

// ReActEmitter handles event emission for ReAct instances
type ReActEmitter struct {
	coordinatorId string
	nodeId        string
	taskIndex     string
}

// newReActEmitter creates a new ReActEmitter
func newReActEmitter(coordinatorId, nodeId, taskIndex string) *ReActEmitter {
	if coordinatorId == "" {
		coordinatorId = uuid.New().String()
	}
	if nodeId == "" {
		nodeId = "react"
	}
	return &ReActEmitter{
		coordinatorId: coordinatorId,
		nodeId:        nodeId,
		taskIndex:     taskIndex,
	}
}

// createAiOutputEvent creates a basic AiOutputEvent and converts it to ypb.AIOutputEvent
func (e *ReActEmitter) createAiOutputEvent(eventType ReActEventType, content []byte, isJson bool) *ypb.AIOutputEvent {
	schemaEvent := &schema.AiOutputEvent{
		CoordinatorId: e.coordinatorId,
		Type:          schema.EventType(eventType),
		NodeId:        e.nodeId,
		TaskIndex:     e.taskIndex,
		Content:       content,
		IsJson:        isJson,
		IsSystem:      false,
		IsStream:      false,
		IsReason:      false,
		Timestamp:     time.Now().Unix(),
		EventUUID:     ksuid.New().String(),
	}

	return schemaEvent.ToGRPC()
}

// createStreamEvent creates a streaming event
func (e *ReActEmitter) createStreamEvent(eventType ReActEventType, delta []byte) *ypb.AIOutputEvent {
	schemaEvent := &schema.AiOutputEvent{
		CoordinatorId: e.coordinatorId,
		Type:          schema.EventType(eventType),
		NodeId:        e.nodeId,
		TaskIndex:     e.taskIndex,
		StreamDelta:   delta,
		IsJson:        false,
		IsSystem:      false,
		IsStream:      true,
		IsReason:      false,
		Timestamp:     time.Now().Unix(),
		EventUUID:     ksuid.New().String(),
	}

	return schemaEvent.ToGRPC()
}

// EmitThought emits a thought event
func (r *ReAct) EmitThought(thought string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "thought", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActThought, []byte(thought), false)
}

// EmitAction emits an action event
func (r *ReAct) EmitAction(action string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "action", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActAction, []byte(action), false)
}

// EmitObservation emits an observation event
func (r *ReAct) EmitObservation(observation string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "observation", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActObservation, []byte(observation), false)
}

// EmitInfo emits an info event
func (r *ReAct) EmitInfo(message string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "info", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActInfo, []byte(message), false)
}

// EmitError emits an error event
func (r *ReAct) EmitError(message string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "error", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActError, []byte(message), false)
}

// EmitWarning emits a warning event
func (r *ReAct) EmitWarning(message string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "warning", r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActWarning, []byte(message), false)
}

// EmitIteration emits an iteration start event
func (r *ReAct) EmitIteration(iteration int, maxIterations int) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "iteration", r.getTaskIndex())
	data := map[string]interface{}{
		"current":   iteration,
		"max":       maxIterations,
		"message":   fmt.Sprintf("ReAct iteration %d/%d started", iteration, maxIterations),
		"timestamp": time.Now().Unix(),
	}
	return emitter.createAiOutputEvent(EventTypeReActIteration, []byte(utils.Jsonify(data)), true)
}

// EmitResult emits a final result event
func (r *ReAct) EmitResult(result interface{}) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), "result", r.getTaskIndex())
	data := map[string]interface{}{
		"result":    result,
		"timestamp": time.Now().Unix(),
		"finished":  true,
	}
	return emitter.createAiOutputEvent(EventTypeReActResult, []byte(utils.Jsonify(data)), true)
}

// EmitStructured emits a structured JSON event
func (r *ReAct) EmitStructured(nodeId string, data interface{}) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), nodeId, r.getTaskIndex())
	return emitter.createAiOutputEvent(EventTypeReActInfo, []byte(utils.Jsonify(data)), true)
}

// EmitStream emits a streaming text event
func (r *ReAct) EmitStream(nodeId string, content string) *ypb.AIOutputEvent {
	emitter := newReActEmitter(r.getCoordinatorId(), nodeId, r.getTaskIndex())
	return emitter.createStreamEvent(EventTypeReActInfo, []byte(content))
}

// EmitStreamReader emits streaming content from a reader
func (r *ReAct) EmitStreamReader(nodeId string, reader io.Reader, outputChan chan *ypb.AIOutputEvent) {
	emitter := newReActEmitter(r.getCoordinatorId(), nodeId, r.getTaskIndex())

	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				event := emitter.createStreamEvent(EventTypeReActInfo, buffer[:n])
				r.safeEmit(outputChan, event)
			}
			if err != nil {
				if err != io.EOF {
					errEvent := r.EmitError(fmt.Sprintf("Stream read error: %v", err))
					r.safeEmit(outputChan, errEvent)
				}
				break
			}
		}
	}()
}

// Utility methods for ReAct instances

// getCoordinatorId returns the coordinator ID for the ReAct instance
func (r *ReAct) getCoordinatorId() string {
	r.config.mu.RLock()
	defer r.config.mu.RUnlock()

	// For now, generate a simple ID. In a full implementation,
	// this would be integrated with the coordinator system
	return fmt.Sprintf("react-%p", r)
}

// getTaskIndex returns the task index for the ReAct instance
func (r *ReAct) getTaskIndex() string {
	r.config.mu.RLock()
	defer r.config.mu.RUnlock()

	return fmt.Sprintf("react-task-%d", r.config.currentIteration)
}

// Event emission methods with safe channel handling

// emitThought emits a thought event to the output channel
func (r *ReAct) emitThought(outputChan chan *ypb.AIOutputEvent, thought string) {
	event := r.EmitThought(thought)
	r.safeEmit(outputChan, event)
}

// emitAction emits an action event to the output channel
func (r *ReAct) emitAction(outputChan chan *ypb.AIOutputEvent, action string) {
	event := r.EmitAction(action)
	r.safeEmit(outputChan, event)
}

// emitObservation emits an observation event to the output channel
func (r *ReAct) emitObservation(outputChan chan *ypb.AIOutputEvent, observation string) {
	event := r.EmitObservation(observation)
	r.safeEmit(outputChan, event)
}

// emitInfo emits an info event to the output channel
func (r *ReAct) emitInfo(outputChan chan *ypb.AIOutputEvent, message string) {
	event := r.EmitInfo(message)
	r.safeEmit(outputChan, event)
}

// emitError emits an error event to the output channel
func (r *ReAct) emitError(outputChan chan *ypb.AIOutputEvent, message string) {
	event := r.EmitError(message)
	r.safeEmit(outputChan, event)
}

// emitWarning emits a warning event to the output channel
func (r *ReAct) emitWarning(outputChan chan *ypb.AIOutputEvent, message string) {
	event := r.EmitWarning(message)
	r.safeEmit(outputChan, event)
}

// emitIteration emits an iteration event to the output channel
func (r *ReAct) emitIteration(outputChan chan *ypb.AIOutputEvent, iteration int, maxIterations int) {
	event := r.EmitIteration(iteration, maxIterations)
	r.safeEmit(outputChan, event)
}

// emitResult emits a result event to the output channel
func (r *ReAct) emitResult(outputChan chan *ypb.AIOutputEvent, result interface{}) {
	event := r.EmitResult(result)
	r.safeEmit(outputChan, event)
}

// safeEmit safely emits an event to the output channel with context handling
func (r *ReAct) safeEmit(outputChan chan *ypb.AIOutputEvent, event *ypb.AIOutputEvent) {
	if event == nil {
		return
	}

	select {
	case outputChan <- event:
		if r.config.debugEvent {
			fmt.Printf("ReAct event emitted: %s - %s\n", event.Type, string(event.Content))
		}
		if r.config.eventHandler != nil {
			r.config.eventHandler(event)
		}
	case <-r.config.ctx.Done():
		// Context cancelled, don't block
		return
	default:
		// Channel full, emit warning and drop event
		if r.config.debugEvent {
			fmt.Printf("ReAct warning: Output channel full, dropping event: %s\n", event.Type)
		}
	}
}
