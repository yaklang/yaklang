package aitool

import (
	"encoding/json"
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

// GetExecutionStatus classifies tool execution, not task satisfaction. A
// protocol-complete callback defaults to succeeded, while explicit machine
// fields such as timeout, transport failure, unaccepted exit code or
// operation_status can override that verdict. Callers still inspect the result
// to decide whether the returned observation satisfies the user's goal.
func (t *ToolResult) GetExecutionStatus() (ToolExecutionStatus, string) {
	if t == nil {
		return ToolExecutionStatusUnknown, ""
	}
	if !t.Success {
		return ToolExecutionStatusFailed, ""
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
	case string:
		if strings.HasPrefix(value, "OBSERVATIONS:\n") || strings.HasPrefix(value, "COMBINED OUTPUT:\n") {
			if strings.Contains(value, "[error]") {
				return ToolExecutionStatusFailed, "observation contains an error log"
			}
			return ToolExecutionStatusUnknown, ""
		}
		result = canonicalExecutionResult(value)
	default:
		if envelope := utils.InterfaceToGeneralMap(t.Data); len(envelope) > 0 {
			if semanticResult, ok := envelope["result"]; ok {
				result = semanticResult
			}
		}
	}

	fields := utils.InterfaceToGeneralMap(result)
	if len(fields) == 0 {
		if text := strings.TrimSpace(utils.InterfaceToString(result)); text != "" {
			return ToolExecutionStatusSucceeded, ""
		}
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

	if operationStatus, ok := fields["operation_status"].(string); ok {
		switch operationStatus {
		case "error", "dialog_open":
			return ToolExecutionStatusFailed, executionStatusDetail(fields, "operation", "operation_status", "session", "error", "recovery_operation")
		case "completed":
			return ToolExecutionStatusSucceeded, executionStatusDetail(fields, "operation", "operation_status", "session")
		}
	}
	return ToolExecutionStatusSucceeded, ""
}

// canonicalExecutionResult extracts the semantic RESULT section from the
// normalized Timeline envelope. The artifact layer intentionally replaces Data
// with this envelope, so status inspection must decode it rather than losing
// the original structured result (for example bash exit_code_accepted).
func canonicalExecutionResult(data string) any {
	const resultMarker = "RESULT:\n"
	if !strings.HasPrefix(data, resultMarker) {
		return data
	}
	body := data[len(resultMarker):]
	end := len(body)
	// Section markers are appended after the result. Search from the right so
	// file text that itself contains "OBSERVATIONS:" or "ARTIFACT:" does not
	// truncate the semantic result before the real framework sections.
	for _, marker := range []string{"\n\nARTIFACT:\n", "\n\nHINT:\n", "\n\nOBSERVATIONS:\n"} {
		if idx := strings.LastIndex(body, marker); idx >= 0 && idx < end {
			end = idx
		}
	}
	body = strings.TrimSpace(body[:end])
	if body == "" {
		return nil
	}
	if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err == nil {
			return decoded
		}
	}
	return body
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
