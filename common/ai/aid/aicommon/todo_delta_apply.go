package aicommon

import "strings"

func BuildDeferredDeltaForOpenTodos(items []VerificationTodoItem, reason string) *TodoDelta {
	delta := &TodoDelta{}
	for _, item := range items {
		if item.Status != VerificationTodoStatusPending && item.Status != VerificationTodoStatusDoing {
			continue
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			delta.Close = append(delta.Close, TodoClose{ID: id, Outcome: TodoOutcomeDeferred, Reason: strings.TrimSpace(reason), Refs: []string{}})
		}
	}
	if len(delta.Close) == 0 {
		return nil
	}
	return delta
}

// DeferOpenTodosOnAsyncHandoff records unfinished work without claiming that
// it completed when the synchronous ReAct loop hands execution to async mode.
func DeferOpenTodosOnAsyncHandoff(cfg AICallerConfigIf, emitter *Emitter, task AIStatefulTask, iterationIndex int, timelineHook func(string, string)) {
	if cfg == nil || task == nil {
		return
	}
	scope := BuildVerificationTodoScope(task)
	delta := BuildDeferredDeltaForOpenTodos(cfg.ActiveVerificationTodoItemsByScope(scope), "Execution was handed off to asynchronous processing after the recorded attempts; unfinished work is deferred until that task reports back.")
	ApplyTodoDeltaAndEmit(cfg, emitter, task, scope, iterationIndex, delta, timelineHook)
}

// ApplyTodoDeltaAndEmit is the single mutation path for normal ReAct actions.
// It applies the canonical delta, emits backward-compatible projections, and
// records failures under TODO_DELTA_ERROR.
func ApplyTodoDeltaAndEmit(cfg AICallerConfigIf, emitter *Emitter, task AIStatefulTask, scope VerificationTodoScope, iterationIndex int, delta *TodoDelta, timelineHook func(string, string)) []VerificationTodoApplyResult {
	if cfg == nil || delta == nil {
		return nil
	}
	results := cfg.ApplyTodoDelta(scope, delta)
	// ApplyTodoDelta removes idempotent operations from delta. A pure no-op is
	// accepted for model robustness, but it is not a state transition and must
	// not generate frontend snapshots, progress streams, or timeline noise.
	if FormatVerificationTodoApplyErrors(results) == "" && !delta.HasChanges() {
		return results
	}
	appliedOps := []TodoOperation{}
	var appliedDelta *TodoDelta
	if FormatVerificationTodoApplyErrors(results) == "" {
		appliedDelta = delta
		appliedOps = TodoDeltaToOperations(delta)
		appliedOps = enrichTodoOperationContent(appliedOps, cfg.SnapshotVerificationTodoItemsByScope(scope))
	}
	if emitter != nil {
		open, current, closed := cfg.SnapshotCanonicalTodos(scope)
		payload := TodoListUpdatePayload{
			Items: cfg.SnapshotVerificationTodoItems(), Stats: cfg.GetVerificationTodoStats(),
			AppliedOps: appliedOps, IterationIndex: iterationIndex,
			TaskID: scope.TaskID, TaskIndex: scope.TaskIndex,
			OpenTodos: open, CurrentTodoID: current, ClosedTodos: closed, AppliedDelta: appliedDelta,
		}
		emitter.EmitTodoListUpdates(cfg, task, payload)
		emitter.EmitTodoDeltaReasonStreams("todo_reason", appliedOps, scope.TaskIndex)
	}
	if timelineHook != nil {
		if errors := FormatVerificationTodoApplyErrors(results); errors != "" {
			timelineHook("TODO_DELTA_ERROR", errors)
		} else if breadcrumb := FormatTodoDeltaBreadcrumb(delta); breadcrumb != "" {
			timelineHook("TODO_DELTA", breadcrumb)
		}
	}
	return results
}

// enrichTodoOperationContent preserves the existing Yakit applied_ops
// projection: status-only operations (current/close) still carry the TODO text
// in structured snapshots. reason and refs remain attached independently for
// the new audit trail; user-visible streams remain reason-only.
func enrichTodoOperationContent(operations []TodoOperation, items []VerificationTodoItem) []TodoOperation {
	contentByID := make(map[string]string, len(items))
	for _, item := range items {
		contentByID[item.ID] = item.Content
	}
	for index := range operations {
		if strings.TrimSpace(operations[index].Content) == "" {
			operations[index].Content = contentByID[operations[index].ID]
		}
	}
	return operations
}
