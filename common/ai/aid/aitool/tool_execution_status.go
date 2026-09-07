package aitool

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ToolExecutionStatus describes explicit execution semantics reported by a
// tool. It is deliberately separate from ToolResult.Success, which only says
// that the invocation protocol produced a result envelope.
type ToolExecutionStatus string

const (
	ToolExecutionStatusUnknown   ToolExecutionStatus = "unknown"
	ToolExecutionStatusSucceeded ToolExecutionStatus = "succeeded"
	ToolExecutionStatusFailed    ToolExecutionStatus = "failed"
)

// GetExecutionStatus classifies only explicit, machine-readable outcome
// fields. Arbitrary tool results (including HTTP status codes) stay unknown so
// the task verifier can decide whether the observed result satisfies the goal.
func (t *ToolResult) GetExecutionStatus() (ToolExecutionStatus, string) {
	if t == nil || !t.Success {
		return ToolExecutionStatusUnknown, ""
	}

	result := t.Data
	switch value := t.Data.(type) {
	case *ToolExecutionResult:
		if value == nil {
			return ToolExecutionStatusUnknown, ""
		}
		result = value.Result
	case ToolExecutionResult:
		result = value.Result
	default:
		if envelope := utils.InterfaceToGeneralMap(t.Data); len(envelope) > 0 {
			if semanticResult, ok := envelope["result"]; ok {
				result = semanticResult
			}
		}
	}

	fields := utils.InterfaceToGeneralMap(result)
	if len(fields) == 0 {
		return ToolExecutionStatusUnknown, ""
	}

	if timedOut, ok := explicitBool(fields["timed_out"]); ok && timedOut {
		return ToolExecutionStatusFailed, executionStatusDetail(fields,
			"timed_out", "exit_code", "exit_code_accepted", "termination_reason")
	}
	if accepted, ok := explicitBool(fields["exit_code_accepted"]); ok {
		status := ToolExecutionStatusFailed
		if accepted {
			status = ToolExecutionStatusSucceeded
		}
		return status, executionStatusDetail(fields,
			"exit_code", "exit_code_accepted", "timed_out", "termination_reason")
	}
	if transportError := strings.TrimSpace(utils.InterfaceToString(fields["transport_error"])); transportError != "" {
		return ToolExecutionStatusFailed, executionStatusDetail(fields,
			"transport_error", "response_received", "status_code")
	}
	if processError := strings.TrimSpace(utils.InterfaceToString(fields["process_error"])); processError != "" {
		return ToolExecutionStatusFailed, executionStatusDetail(fields,
			"process_error", "exit_code", "termination_reason")
	}

	return ToolExecutionStatusUnknown, ""
}

func explicitBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

func executionStatusDetail(fields map[string]any, keys ...string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := fields[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(utils.InterfaceToString(value))
		if text == "" {
			continue
		}
		if runes := []rune(text); len(runes) > 160 {
			text = string(runes[:157]) + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, text))
	}
	return strings.Join(parts, "; ")
}
