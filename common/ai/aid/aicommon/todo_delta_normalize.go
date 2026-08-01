package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// NormalizeTodoDelta parses the optional todo_delta object. It deliberately
// rejects the former non-empty array/string shapes; this protocol has no wire
// alias. Empty objects (and the stream parser's equivalent empty slice) are
// treated as omitted so a harmless sidecar does not consume another AI turn.
func NormalizeTodoDelta(action *Action) (*TodoDelta, error) {
	if action == nil {
		return nil, nil
	}
	raw, exists := action.LookupParam("todo_delta")
	if !exists {
		return nil, nil
	}
	if raw == nil {
		return nil, utils.Error("todo_delta must be an object, not null")
	}
	if isEmptyTodoDeltaValue(raw) {
		return nil, nil
	}
	obj, ok := asInvokeParams(raw)
	if !ok {
		return nil, utils.Error("todo_delta must be an object")
	}
	delta := &TodoDelta{}
	if currentRaw, currentSet := obj["current"]; currentSet {
		delta.CurrentSet = true
		if currentRaw != nil {
			currentValue, ok := currentRaw.(string)
			if !ok {
				return nil, utils.Error("todo_delta.current must be a string or null")
			}
			current := strings.TrimSpace(currentValue)
			delta.Current = &current
		}
	}
	add, err := objectArray(obj, "add")
	if err != nil {
		return nil, err
	}
	for _, item := range add {
		delta.Add = append(delta.Add, TodoAdd{ID: strings.TrimSpace(item.GetString("id")), Text: strings.TrimSpace(item.GetString("text"))})
	}
	update, err := objectArray(obj, "update")
	if err != nil {
		return nil, err
	}
	for _, item := range update {
		delta.Update = append(delta.Update, TodoUpdate{ID: strings.TrimSpace(item.GetString("id")), Text: strings.TrimSpace(item.GetString("text"))})
	}
	closeItems, err := objectArray(obj, "close")
	if err != nil {
		return nil, err
	}
	for _, item := range closeItems {
		refs, err := stringArray(item, "refs")
		if err != nil {
			return nil, err
		}
		delta.Close = append(delta.Close, TodoClose{
			ID: strings.TrimSpace(item.GetString("id")), Outcome: TodoOutcome(strings.ToLower(strings.TrimSpace(item.GetString("outcome")))),
			Reason: strings.TrimSpace(item.GetString("reason")), Refs: refs,
		})
	}
	if !delta.HasChanges() {
		return nil, nil
	}
	if err := delta.ValidateShape(); err != nil {
		return nil, err
	}
	return delta, nil
}

func isEmptyTodoDeltaValue(raw any) bool {
	switch value := raw.(type) {
	case aitool.InvokeParams:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	case []any:
		return len(value) == 0
	case []aitool.InvokeParams:
		return len(value) == 0
	case []map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

func asInvokeParams(raw any) (aitool.InvokeParams, bool) {
	switch value := raw.(type) {
	case aitool.InvokeParams:
		return value, true
	case map[string]any:
		return aitool.InvokeParams(value), true
	default:
		return nil, false
	}
}

func objectArray(params aitool.InvokeParams, key string) ([]aitool.InvokeParams, error) {
	raw, exists := params[key]
	if !exists || raw == nil {
		return nil, nil
	}
	switch value := raw.(type) {
	case []aitool.InvokeParams:
		return value, nil
	case []map[string]any:
		out := make([]aitool.InvokeParams, 0, len(value))
		for _, item := range value {
			out = append(out, aitool.InvokeParams(item))
		}
		return out, nil
	case []any:
		out := make([]aitool.InvokeParams, 0, len(value))
		for index, item := range value {
			if obj, ok := asInvokeParams(item); ok {
				out = append(out, obj)
				continue
			}
			return nil, utils.Errorf("todo_delta.%s[%d] must be an object", key, index)
		}
		return out, nil
	default:
		return nil, utils.Errorf("todo_delta.%s must be an array of objects", key)
	}
}

func stringArray(params aitool.InvokeParams, key string) ([]string, error) {
	raw, exists := params[key]
	if !exists || raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]string); ok {
			return append([]string(nil), typed...), nil
		}
		return nil, utils.Errorf("todo_delta.close.refs must be an array of strings")
	}
	refs := make([]string, 0, len(values))
	for _, value := range values {
		ref, ok := value.(string)
		if !ok {
			return nil, utils.Errorf("todo_delta.close.refs must contain only strings")
		}
		refs = append(refs, strings.TrimSpace(ref))
	}
	return refs, nil
}

func (d *TodoDelta) HasChanges() bool {
	return d != nil && (d.CurrentSet || len(d.Add) > 0 || len(d.Update) > 0 || len(d.Close) > 0)
}

func (d *TodoDelta) ValidateShape() error {
	if d == nil || !d.HasChanges() {
		return utils.Error("todo_delta has no effective changes")
	}
	seenAdds := make(map[string]string)
	seenUpdates := make(map[string]struct{})
	seenCloses := make(map[string]struct{})
	for index, item := range d.Add {
		if strings.TrimSpace(item.Text) == "" {
			return utils.Errorf("todo_delta.add[%d].text is required", index)
		}
		if item.ID != "" {
			if previousText, exists := seenAdds[item.ID]; exists {
				if strings.TrimSpace(previousText) != strings.TrimSpace(item.Text) {
					return utils.Errorf("todo_delta.add contains duplicate id %q with conflicting text", item.ID)
				}
				continue
			}
			seenAdds[item.ID] = item.Text
		}
	}
	for index, item := range d.Update {
		if item.ID == "" || item.Text == "" {
			return utils.Errorf("todo_delta.update[%d] requires non-empty id and text", index)
		}
		if _, exists := seenUpdates[item.ID]; exists {
			return utils.Errorf("todo_delta.update contains duplicate id %q", item.ID)
		}
		seenUpdates[item.ID] = struct{}{}
	}
	for index, item := range d.Close {
		if item.ID == "" {
			return utils.Errorf("todo_delta.close[%d].id is required", index)
		}
		if item.Reason == "" {
			return utils.Errorf("todo_delta.close[%d].reason is required", index)
		}
		switch item.Outcome {
		case TodoOutcomeResolved, TodoOutcomeDismissed, TodoOutcomeDeferred:
		default:
			return utils.Errorf("todo_delta.close[%d].outcome must be resolved, dismissed, or deferred", index)
		}
		if _, exists := seenCloses[item.ID]; exists {
			return utils.Errorf("todo_delta.close contains duplicate id %q", item.ID)
		}
		seenCloses[item.ID] = struct{}{}
	}
	return nil
}

// TodoDeltaToOperations deterministically projects the new protocol into the
// applied_ops shape used by existing frontend consumers.
func TodoDeltaToOperations(delta *TodoDelta) []TodoOperation {
	if delta == nil {
		return nil
	}
	operations := make([]TodoOperation, 0, len(delta.Add)+len(delta.Update)+len(delta.Close)+1)
	for _, item := range delta.Add {
		operations = append(operations, TodoOperation{Op: "add", ID: item.ID, Content: item.Text})
	}
	for _, item := range delta.Update {
		operations = append(operations, TodoOperation{Op: "add", ID: item.ID, Content: item.Text})
	}
	for _, item := range delta.Close {
		op := map[TodoOutcome]string{TodoOutcomeResolved: "done", TodoOutcomeDismissed: "delete", TodoOutcomeDeferred: "skip"}[item.Outcome]
		operations = append(operations, TodoOperation{Op: op, ID: item.ID, Reason: item.Reason, Refs: append([]string(nil), item.Refs...)})
	}
	if delta.CurrentSet && delta.Current != nil && strings.TrimSpace(*delta.Current) != "" {
		operations = append(operations, TodoOperation{Op: "doing", ID: strings.TrimSpace(*delta.Current)})
	}
	return operations
}

func FormatTodoOperationDisplayLine(operation TodoOperation) string {
	id, content := strings.TrimSpace(operation.ID), strings.TrimSpace(operation.Content)
	switch strings.ToLower(strings.TrimSpace(operation.Op)) {
	case "add":
		return fmt.Sprintf("- [+]: [id: %s]: %s", id, content)
	case "doing":
		return fmt.Sprintf("- [CURRENT]: [id: %s]", id)
	case "done":
		return fmt.Sprintf("- [RESOLVED]: [id: %s]: %s", id, operation.Reason)
	case "delete":
		return fmt.Sprintf("- [DISMISSED]: [id: %s]: %s", id, operation.Reason)
	case "skip":
		return fmt.Sprintf("- [DEFERRED]: [id: %s]: %s", id, operation.Reason)
	default:
		return ""
	}
}

func FormatTodoDeltaBreadcrumb(delta *TodoDelta) string {
	operations := TodoDeltaToOperations(delta)
	lines := make([]string, 0, len(operations))
	for _, operation := range operations {
		if line := FormatTodoOperationDisplayLine(operation); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
