package aicommon

import "strings"

// BuildVerificationTodoScope extracts the task ownership information used by
// the session-level TODO store to distinguish current-task TODOs from sibling
// or historical ones.
func BuildVerificationTodoScope(task AIStatefulTask) VerificationTodoScope {
	if task == nil {
		return VerificationTodoScope{}
	}
	return VerificationTodoScope{
		TaskID:    strings.TrimSpace(task.GetId()),
		TaskIndex: strings.TrimSpace(task.GetIndex()),
	}.normalize()
}

// GetBlockingVerificationTodoItems returns the active TODOs owned by the given
// task. A nil config/task, or a task without an id, never blocks completion.
func GetBlockingVerificationTodoItems(cfg AICallerConfigIf, task AIStatefulTask) []VerificationTodoItem {
	if cfg == nil || task == nil {
		return nil
	}
	scope := BuildVerificationTodoScope(task)
	if scope.IsZero() {
		return nil
	}
	return cfg.ActiveVerificationTodoItemsByScope(scope)
}
