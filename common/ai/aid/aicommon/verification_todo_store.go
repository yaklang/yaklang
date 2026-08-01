package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/ytoken"
)

const VerificationTodoSnapshotLimit = 10 * 1024

type VerificationTodoStatus string

const (
	VerificationTodoStatusPending VerificationTodoStatus = "PENDING"
	VerificationTodoStatusDoing   VerificationTodoStatus = "DOING"
	VerificationTodoStatusDone    VerificationTodoStatus = "DONE"
	VerificationTodoStatusDeleted VerificationTodoStatus = "DELETED"
	VerificationTodoStatusSkipped VerificationTodoStatus = "SKIPPED"
)

type VerificationTodoStats struct {
	Pending int `json:"pending"`
	Doing   int `json:"doing"`
	Done    int `json:"done"`
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
}

type VerificationTodoScope struct {
	TaskID    string `json:"task_id,omitempty"`
	TaskIndex string `json:"task_index,omitempty"`
}

func (s VerificationTodoScope) normalize() VerificationTodoScope {
	s.TaskID = strings.TrimSpace(s.TaskID)
	s.TaskIndex = strings.TrimSpace(s.TaskIndex)
	return s
}

func (s VerificationTodoScope) IsZero() bool { return strings.TrimSpace(s.TaskID) == "" }

// VerificationTodoItem is the compatibility projection used by existing UI
// events. Canonical persistence uses TodoScopeState below.
type VerificationTodoItem struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Status    VerificationTodoStatus `json:"status"`
	CreatedAt int                    `json:"created_at"`
	UpdatedAt int                    `json:"updated_at"`

	ScopeTaskID    string      `json:"scope_task_id,omitempty"`
	ScopeTaskIndex string      `json:"scope_task_index,omitempty"`
	Outcome        TodoOutcome `json:"outcome,omitempty"`
	Reason         string      `json:"reason,omitempty"`
	Refs           []string    `json:"refs,omitempty"`
}

func (i VerificationTodoItem) scope() VerificationTodoScope {
	return VerificationTodoScope{TaskID: i.ScopeTaskID, TaskIndex: i.ScopeTaskIndex}.normalize()
}

type TodoOpenItem struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt int    `json:"created_at"`
	UpdatedAt int    `json:"updated_at"`
}

type TodoClosedItem struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Outcome   TodoOutcome `json:"outcome"`
	Reason    string      `json:"reason"`
	Refs      []string    `json:"refs"`
	CreatedAt int         `json:"created_at"`
	UpdatedAt int         `json:"updated_at"`
}

type TodoScopeState struct {
	TaskID        string            `json:"task_id,omitempty"`
	TaskIndex     string            `json:"task_index,omitempty"`
	OpenTodos     []*TodoOpenItem   `json:"open_todos"`
	CurrentTodoID string            `json:"current_todo_id,omitempty"`
	ClosedTodos   []*TodoClosedItem `json:"closed_todos"`
	Counter       int               `json:"counter"`
	Revision      int               `json:"revision"`
}

func (s *TodoScopeState) scope() VerificationTodoScope {
	if s == nil {
		return VerificationTodoScope{}
	}
	return VerificationTodoScope{TaskID: s.TaskID, TaskIndex: s.TaskIndex}.normalize()
}

type VerificationTodoStore struct {
	Scopes []*TodoScopeState `json:"scopes"`
}

func NewVerificationTodoStore() *VerificationTodoStore {
	return &VerificationTodoStore{Scopes: make([]*TodoScopeState, 0)}
}

func (s *VerificationTodoStore) IsEmpty() bool {
	if s == nil {
		return true
	}
	for _, state := range s.Scopes {
		if state != nil && (len(state.OpenTodos) > 0 || len(state.ClosedTodos) > 0) {
			return false
		}
	}
	return true
}

func (s *VerificationTodoStore) Clone() *VerificationTodoStore {
	if s == nil {
		return NewVerificationTodoStore()
	}
	raw, _ := json.Marshal(s)
	var clone VerificationTodoStore
	_ = json.Unmarshal(raw, &clone)
	clone.normalize()
	return &clone
}

type VerificationTodoApplyResult struct {
	Operation TodoOperation
	Success   bool
	Reason    string
	// NoOp marks an idempotent request that was already true in the working
	// state. A no-op does not invalidate other effective operations in the
	// same delta. A delta made entirely of no-ops is accepted and normalized to
	// an empty delta so callers can suppress events without retrying the model.
	NoOp bool
	// RolledBack distinguishes an operation that was valid on the working copy
	// but was not committed because another operation in the same delta failed.
	// It lets validation report the real cause instead of the rollback symptom.
	RolledBack bool
}

func FormatVerificationTodoApplyResults(results []VerificationTodoApplyResult) string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		label := strings.ToUpper(strings.TrimSpace(result.Operation.Op))
		if result.Success {
			lines = append(lines, fmt.Sprintf("OK %s[%s]", label, result.Operation.ID))
		} else {
			lines = append(lines, fmt.Sprintf("FAILED %s[%s]: %s", label, result.Operation.ID, result.Reason))
		}
	}
	return strings.Join(lines, "\n")
}

func FormatVerificationTodoApplyErrors(results []VerificationTodoApplyResult) string {
	failed := make([]VerificationTodoApplyResult, 0)
	for _, result := range results {
		if !result.Success {
			failed = append(failed, result)
		}
	}
	return FormatVerificationTodoApplyResults(failed)
}

// ApplyTodoDelta validates and atomically applies add -> update -> close ->
// current. The input delta is updated with generated IDs for event emission.
func (s *VerificationTodoStore) ApplyTodoDelta(scope VerificationTodoScope, delta *TodoDelta) []VerificationTodoApplyResult {
	if s == nil || delta == nil {
		return nil
	}
	if err := delta.ValidateShape(); err != nil {
		return []VerificationTodoApplyResult{{Operation: TodoOperation{Op: "validate"}, Reason: err.Error()}}
	}
	working := s.Clone()
	workingDelta := cloneTodoDelta(delta)
	results := working.applyTodoDelta(scope, workingDelta)
	causes := todoDeltaFailureCauses(results)
	if len(causes) > 0 {
		cause := strings.Join(causes, "; ")
		for index := range results {
			if results[index].Success {
				results[index].Success = false
				results[index].RolledBack = true
				results[index].Reason = "valid operation was rolled back atomically because " + cause
			}
		}
		return results
	}
	effectiveDelta := todoDeltaWithoutNoOps(workingDelta, results)
	if effectiveDelta == nil || !effectiveDelta.HasChanges() {
		// Idempotent model output is valid. Real models occasionally repeat the
		// already-current focus or an unchanged TODO text even though the prompt
		// asks them to omit no-op deltas. Treating that as a validation failure
		// retries the entire AI transaction without improving state.
		//
		// Mutate the caller-visible delta to an empty normalized delta so the
		// shared apply/emit path can silently suppress snapshots, breadcrumbs and
		// reason streams. Invalid operations still return through the atomic
		// failure branch above.
		*delta = TodoDelta{}
		return results
	}
	s.Scopes = working.Scopes
	*delta = *effectiveDelta
	return results
}

// todoDeltaWithoutNoOps keeps applied_delta and the compatibility operation
// stream truthful: idempotent fields accepted for robustness are not emitted
// as if they changed state.
func todoDeltaWithoutNoOps(delta *TodoDelta, results []VerificationTodoApplyResult) *TodoDelta {
	if delta == nil {
		return nil
	}
	effective := &TodoDelta{}
	resultIndex := 0
	for _, item := range delta.Add {
		if resultIndex < len(results) && !results[resultIndex].NoOp {
			effective.Add = append(effective.Add, item)
		}
		resultIndex++
	}
	for _, item := range delta.Update {
		if resultIndex < len(results) && !results[resultIndex].NoOp {
			effective.Update = append(effective.Update, item)
		}
		resultIndex++
	}
	for _, item := range delta.Close {
		if resultIndex < len(results) && !results[resultIndex].NoOp {
			effective.Close = append(effective.Close, item)
		}
		resultIndex++
	}
	if delta.CurrentSet && resultIndex < len(results) && !results[resultIndex].NoOp {
		effective.CurrentSet = true
		if delta.Current != nil {
			current := *delta.Current
			effective.Current = &current
		}
	}
	return effective
}

func todoDeltaFailureCauses(results []VerificationTodoApplyResult) []string {
	causes := make([]string, 0)
	for _, result := range results {
		if result.Success || result.RolledBack {
			continue
		}
		operation := strings.ToUpper(strings.TrimSpace(result.Operation.Op))
		if operation == "" {
			operation = "OPERATION"
		}
		id := strings.TrimSpace(result.Operation.ID)
		if id == "" {
			causes = append(causes, fmt.Sprintf("%s: %s", operation, result.Reason))
		} else {
			causes = append(causes, fmt.Sprintf("%s[%s]: %s", operation, id, result.Reason))
		}
	}
	return causes
}

func FormatTodoDeltaValidationError(results []VerificationTodoApplyResult) string {
	return strings.Join(todoDeltaFailureCauses(results), "; ")
}

func cloneTodoDelta(delta *TodoDelta) *TodoDelta {
	if delta == nil {
		return nil
	}
	copyDelta := *delta
	copyDelta.Add = append([]TodoAdd(nil), delta.Add...)
	copyDelta.Update = append([]TodoUpdate(nil), delta.Update...)
	copyDelta.Close = append([]TodoClose(nil), delta.Close...)
	for index := range copyDelta.Close {
		copyDelta.Close[index].Refs = append([]string(nil), delta.Close[index].Refs...)
	}
	if delta.Current != nil {
		value := *delta.Current
		copyDelta.Current = &value
	}
	return &copyDelta
}

func (s *VerificationTodoStore) applyTodoDelta(scope VerificationTodoScope, delta *TodoDelta) []VerificationTodoApplyResult {
	state := s.ensureScope(scope)
	state.Revision++
	revision := state.Revision
	results := make([]VerificationTodoApplyResult, 0, len(delta.Add)+len(delta.Update)+len(delta.Close)+1)
	generatedAddIDs := make(map[string]struct{})
	for index := range delta.Add {
		item := &delta.Add[index]
		if item.ID == "" {
			item.ID = state.nextID()
			generatedAddIDs[item.ID] = struct{}{}
		}
		operation := TodoOperation{Op: "add", ID: item.ID, Content: item.Text}
		if open := state.findOpen(item.ID); open != nil {
			if strings.TrimSpace(open.Text) == strings.TrimSpace(item.Text) {
				results = append(results, todoDeltaNoOp(operation, "identical open todo already exists"))
			} else {
				results = append(results, todoDeltaFailure(operation, "todo id already exists as an open item with different text; use todo_delta.update instead"))
			}
			continue
		}
		if state.findClosed(item.ID) != nil {
			results = append(results, todoDeltaFailure(operation, "todo id already exists as closed history and cannot be reopened"))
			continue
		}
		state.OpenTodos = append(state.OpenTodos, &TodoOpenItem{ID: item.ID, Text: item.Text, CreatedAt: revision, UpdatedAt: revision})
		results = append(results, todoDeltaSuccess(operation))
	}
	for _, item := range delta.Update {
		operation := TodoOperation{Op: "update", ID: item.ID, Content: item.Text}
		open := state.findOpen(item.ID)
		if open == nil {
			results = append(results, todoDeltaFailure(operation, "todo is not open in current task scope; open todo ids: "+state.openIDSummary()))
			continue
		}
		if open.Text == item.Text {
			results = append(results, todoDeltaNoOp(operation, "todo text is already unchanged"))
			continue
		}
		open.Text, open.UpdatedAt = item.Text, revision
		results = append(results, todoDeltaSuccess(operation))
	}
	for _, item := range delta.Close {
		operation := TodoOperation{Op: string(item.Outcome), ID: item.ID, Reason: item.Reason, Refs: append([]string(nil), item.Refs...)}
		openIndex, open := state.openIndex(item.ID)
		if open == nil {
			results = append(results, todoDeltaFailure(operation, "todo is not open in current task scope; open todo ids: "+state.openIDSummary()))
			continue
		}
		state.OpenTodos = append(state.OpenTodos[:openIndex], state.OpenTodos[openIndex+1:]...)
		state.ClosedTodos = append(state.ClosedTodos, &TodoClosedItem{
			ID: open.ID, Text: open.Text, Outcome: item.Outcome, Reason: item.Reason,
			Refs: append([]string(nil), item.Refs...), CreatedAt: open.CreatedAt, UpdatedAt: revision,
		})
		if state.CurrentTodoID == item.ID {
			state.CurrentTodoID = ""
		}
		results = append(results, todoDeltaSuccess(operation))
	}
	if delta.CurrentSet {
		current := ""
		if delta.Current != nil {
			current = strings.TrimSpace(*delta.Current)
		}
		operation := TodoOperation{Op: "current", ID: current}
		if _, generatedThisRound := generatedAddIDs[current]; current != "" && generatedThisRound {
			results = append(results, todoDeltaFailure(operation, "same-round current requires an explicit todo_delta.add.id; generated IDs cannot be referenced by prediction"))
		} else if current != "" && state.findOpen(current) == nil {
			results = append(results, todoDeltaFailure(operation, "current must reference an open TODO in the current task scope after add/update/close are applied; open todo ids: "+state.openIDSummary()))
		} else if state.CurrentTodoID == current {
			results = append(results, todoDeltaNoOp(operation, "current focus already matches"))
		} else {
			state.CurrentTodoID = current
			results = append(results, todoDeltaSuccess(operation))
		}
	}
	return results
}

func (s *TodoScopeState) openIDSummary() string {
	if s == nil || len(s.OpenTodos) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(s.OpenTodos))
	for _, item := range s.OpenTodos {
		if item != nil && strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}
	return "[" + strings.Join(ids, ", ") + "]"
}

func todoDeltaSuccess(operation TodoOperation) VerificationTodoApplyResult {
	return VerificationTodoApplyResult{Operation: operation, Success: true}
}

func todoDeltaNoOp(operation TodoOperation, reason string) VerificationTodoApplyResult {
	return VerificationTodoApplyResult{Operation: operation, Success: true, Reason: reason, NoOp: true}
}

func todoDeltaFailure(operation TodoOperation, reason string) VerificationTodoApplyResult {
	return VerificationTodoApplyResult{Operation: operation, Reason: reason}
}

func (s *VerificationTodoStore) ensureScope(scope VerificationTodoScope) *TodoScopeState {
	scope = scope.normalize()
	for _, state := range s.Scopes {
		if state != nil && state.TaskID == scope.TaskID {
			if state.TaskIndex == "" {
				state.TaskIndex = scope.TaskIndex
			}
			return state
		}
	}
	state := &TodoScopeState{TaskID: scope.TaskID, TaskIndex: scope.TaskIndex, OpenTodos: make([]*TodoOpenItem, 0), ClosedTodos: make([]*TodoClosedItem, 0)}
	s.Scopes = append(s.Scopes, state)
	return state
}

func (s *VerificationTodoStore) findScope(scope VerificationTodoScope) *TodoScopeState {
	if s == nil {
		return nil
	}
	scope = scope.normalize()
	for _, state := range s.Scopes {
		if state != nil && state.TaskID == scope.TaskID {
			return state
		}
	}
	return nil
}

func (s *TodoScopeState) nextID() string {
	for {
		s.Counter++
		id := "todo-" + strconv.Itoa(s.Counter)
		if s.findOpen(id) == nil && s.findClosed(id) == nil {
			return id
		}
	}
}

func (s *TodoScopeState) findOpen(id string) *TodoOpenItem {
	_, item := s.openIndex(id)
	return item
}

func (s *TodoScopeState) openIndex(id string) (int, *TodoOpenItem) {
	for index, item := range s.OpenTodos {
		if item != nil && item.ID == strings.TrimSpace(id) {
			return index, item
		}
	}
	return -1, nil
}

func (s *TodoScopeState) findClosed(id string) *TodoClosedItem {
	for _, item := range s.ClosedTodos {
		if item != nil && item.ID == strings.TrimSpace(id) {
			return item
		}
	}
	return nil
}

func (s *VerificationTodoStore) SnapshotItems() []VerificationTodoItem {
	if s == nil {
		return nil
	}
	var items []VerificationTodoItem
	for _, state := range s.Scopes {
		items = append(items, projectScope(state)...)
	}
	return items
}

func (s *VerificationTodoStore) SnapshotItemsByScope(scope VerificationTodoScope) []VerificationTodoItem {
	return projectScope(s.findScope(scope))
}

func projectScope(state *TodoScopeState) []VerificationTodoItem {
	if state == nil {
		return nil
	}
	items := make([]VerificationTodoItem, 0, len(state.OpenTodos)+len(state.ClosedTodos))
	for _, item := range state.OpenTodos {
		if item == nil {
			continue
		}
		status := VerificationTodoStatusPending
		if item.ID == state.CurrentTodoID {
			status = VerificationTodoStatusDoing
		}
		items = append(items, VerificationTodoItem{ID: item.ID, Content: item.Text, Status: status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ScopeTaskID: state.TaskID, ScopeTaskIndex: state.TaskIndex})
	}
	for _, item := range state.ClosedTodos {
		if item == nil {
			continue
		}
		status := map[TodoOutcome]VerificationTodoStatus{TodoOutcomeResolved: VerificationTodoStatusDone, TodoOutcomeDismissed: VerificationTodoStatusDeleted, TodoOutcomeDeferred: VerificationTodoStatusSkipped}[item.Outcome]
		items = append(items, VerificationTodoItem{ID: item.ID, Content: item.Text, Status: status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ScopeTaskID: state.TaskID, ScopeTaskIndex: state.TaskIndex, Outcome: item.Outcome, Reason: item.Reason, Refs: append([]string(nil), item.Refs...)})
	}
	return items
}

func (s *VerificationTodoStore) HasActiveTodos() bool {
	for _, state := range s.Scopes {
		if state != nil && len(state.OpenTodos) > 0 {
			return true
		}
	}
	return false
}

func (s *VerificationTodoStore) HasActiveTodosByScope(scope VerificationTodoScope) bool {
	state := s.findScope(scope)
	return state != nil && len(state.OpenTodos) > 0
}

func (s *VerificationTodoStore) ActiveTodoItems() []VerificationTodoItem {
	items := s.SnapshotItems()
	return filterOpenProjection(items)
}

func (s *VerificationTodoStore) ActiveTodoItemsByScope(scope VerificationTodoScope) []VerificationTodoItem {
	return filterOpenProjection(s.SnapshotItemsByScope(scope))
}

func filterOpenProjection(items []VerificationTodoItem) []VerificationTodoItem {
	out := make([]VerificationTodoItem, 0, len(items))
	for _, item := range items {
		if item.Status == VerificationTodoStatusPending || item.Status == VerificationTodoStatusDoing {
			out = append(out, item)
		}
	}
	return out
}

func (s *VerificationTodoStore) Stats() VerificationTodoStats { return statsFor(s.SnapshotItems()) }
func (s *VerificationTodoStore) StatsByScope(scope VerificationTodoScope) VerificationTodoStats {
	return statsFor(s.SnapshotItemsByScope(scope))
}

func statsFor(items []VerificationTodoItem) VerificationTodoStats {
	var stats VerificationTodoStats
	for _, item := range items {
		switch item.Status {
		case VerificationTodoStatusPending:
			stats.Pending++
		case VerificationTodoStatusDoing:
			stats.Doing++
		case VerificationTodoStatusDone:
			stats.Done++
		case VerificationTodoStatusDeleted:
			stats.Deleted++
		case VerificationTodoStatusSkipped:
			stats.Skipped++
		}
	}
	return stats
}

func (s *VerificationTodoStore) CanonicalSnapshot(scope VerificationTodoScope) (open []TodoOpenItem, current string, closed []TodoClosedItem) {
	state := s.findScope(scope)
	if state == nil {
		return []TodoOpenItem{}, "", []TodoClosedItem{}
	}
	for _, item := range state.OpenTodos {
		if item != nil {
			open = append(open, *item)
		}
	}
	for _, item := range state.ClosedTodos {
		if item != nil {
			copyItem := *item
			copyItem.Refs = append([]string(nil), item.Refs...)
			closed = append(closed, copyItem)
		}
	}
	return open, state.CurrentTodoID, closed
}

func (s *VerificationTodoStore) Render() string {
	return renderTodoItems(s.SnapshotItems(), VerificationTodoScope{})
}

func (s *VerificationTodoStore) RenderWithCurrentScope(scope VerificationTodoScope) string {
	if s == nil || s.IsEmpty() {
		return "- no tracked TODO items"
	}
	lines := []string{formatVerificationTodoCurrentTaskHeader(scope), "- TODOs are a short-term work set. Maintain only the current task section; other scopes are read-only."}
	current := s.SnapshotItemsByScope(scope)
	if len(current) == 0 {
		lines = append(lines, "- (no TODO items tracked for the current task yet)")
	} else {
		lines = append(lines, renderTodoProjection(current)...)
	}
	for _, state := range s.Scopes {
		if state == nil || state.TaskID == scope.normalize().TaskID {
			continue
		}
		lines = append(lines, "", "### OTHER TASK (read-only) "+formatScope(state.scope()))
		lines = append(lines, renderTodoProjection(projectScope(state))...)
	}
	return truncateVerificationTodoLines(lines)
}

func renderTodoItems(items []VerificationTodoItem, scope VerificationTodoScope) string {
	if len(items) == 0 {
		return "- no tracked TODO items"
	}
	return truncateVerificationTodoLines(renderTodoProjection(items))
}

func renderTodoProjection(items []VerificationTodoItem) []string {
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	lines := make([]string, 0, len(items))
	for _, item := range items {
		content := strings.Join(strings.Fields(item.Content), " ")
		switch item.Status {
		case VerificationTodoStatusPending:
			lines = append(lines, fmt.Sprintf("- [ ] [id: %s]: %s", item.ID, content))
		case VerificationTodoStatusDoing:
			lines = append(lines, fmt.Sprintf("- [CURRENT] [id: %s]: %s", item.ID, content))
		default:
			lines = append(lines, fmt.Sprintf("- [%s] [id: %s]: %s; reason: %s; refs: %s", item.Outcome, item.ID, content, item.Reason, strings.Join(item.Refs, ", ")))
		}
	}
	return lines
}

func formatVerificationTodoCurrentTaskHeader(scope VerificationTodoScope) string {
	return "### CURRENT TASK " + formatScope(scope)
}

func formatScope(scope VerificationTodoScope) string {
	scope = scope.normalize()
	return fmt.Sprintf("[task_index=%s, task_id=%s]", scope.TaskIndex, scope.TaskID)
}

func truncateVerificationTodoLines(lines []string) string {
	if len(lines) == 0 {
		return "- no tracked TODO items"
	}
	for len(lines) > 1 && ytoken.CalcTokenCount(strings.Join(lines, "\n")) > VerificationTodoSnapshotLimit {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (s *VerificationTodoStore) RenderMarkdownDelta(scope VerificationTodoScope, delta *TodoDelta) string {
	clone := s.Clone()
	_ = clone.ApplyTodoDelta(scope, cloneTodoDelta(delta))
	return clone.RenderWithCurrentScope(scope)
}

func (s *VerificationTodoStore) Marshal() string {
	if s == nil {
		return ""
	}
	s.normalize()
	raw, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *VerificationTodoStore) normalize() {
	if s.Scopes == nil {
		s.Scopes = make([]*TodoScopeState, 0)
	}
	for _, state := range s.Scopes {
		if state == nil {
			continue
		}
		state.TaskID = strings.TrimSpace(state.TaskID)
		state.TaskIndex = strings.TrimSpace(state.TaskIndex)
		if state.OpenTodos == nil {
			state.OpenTodos = make([]*TodoOpenItem, 0)
		}
		if state.ClosedTodos == nil {
			state.ClosedTodos = make([]*TodoClosedItem, 0)
		}
		if state.CurrentTodoID != "" && state.findOpen(state.CurrentTodoID) == nil {
			state.CurrentTodoID = ""
		}
		for _, item := range state.OpenTodos {
			if item != nil {
				if number, ok := parseTodoNumber(item.ID); ok && number > state.Counter {
					state.Counter = number
				}
			}
		}
		for _, item := range state.ClosedTodos {
			if item != nil {
				if number, ok := parseTodoNumber(item.ID); ok && number > state.Counter {
					state.Counter = number
				}
			}
		}
	}
}

func UnmarshalVerificationTodoStore(data string) *VerificationTodoStore {
	store := NewVerificationTodoStore()
	if strings.TrimSpace(data) == "" {
		return store
	}
	var envelope struct {
		Scopes []json.RawMessage       `json:"scopes"`
		Items  []*VerificationTodoItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return store
	}
	if len(envelope.Scopes) > 0 {
		if err := json.Unmarshal([]byte(data), store); err == nil {
			store.normalize()
			return store
		}
	}
	return migrateLegacyTodoItems(envelope.Items)
}

const legacyTodoReason = "Migrated from an older Yaklang session; the previous version did not record a closure reason."

func migrateLegacyTodoItems(items []*VerificationTodoItem) *VerificationTodoStore {
	store := NewVerificationTodoStore()
	newestDoing := make(map[string]*VerificationTodoItem)
	for _, item := range items {
		if item == nil || item.Status != VerificationTodoStatusDoing {
			continue
		}
		key := strings.TrimSpace(item.ScopeTaskID)
		if newestDoing[key] == nil || item.UpdatedAt > newestDoing[key].UpdatedAt {
			newestDoing[key] = item
		}
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		scope := VerificationTodoScope{TaskID: item.ScopeTaskID, TaskIndex: item.ScopeTaskIndex}
		state := store.ensureScope(scope)
		if item.UpdatedAt > state.Revision {
			state.Revision = item.UpdatedAt
		}
		if number, ok := parseTodoNumber(item.ID); ok && number > state.Counter {
			state.Counter = number
		}
		switch item.Status {
		case VerificationTodoStatusPending, VerificationTodoStatusDoing:
			state.OpenTodos = append(state.OpenTodos, &TodoOpenItem{ID: item.ID, Text: item.Content, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
			if item.Status == VerificationTodoStatusDoing && newestDoing[state.TaskID] == item {
				state.CurrentTodoID = item.ID
			}
		case VerificationTodoStatusDone, VerificationTodoStatusDeleted, VerificationTodoStatusSkipped:
			outcome := map[VerificationTodoStatus]TodoOutcome{VerificationTodoStatusDone: TodoOutcomeResolved, VerificationTodoStatusDeleted: TodoOutcomeDismissed, VerificationTodoStatusSkipped: TodoOutcomeDeferred}[item.Status]
			state.ClosedTodos = append(state.ClosedTodos, &TodoClosedItem{ID: item.ID, Text: item.Content, Outcome: outcome, Reason: legacyTodoReason, Refs: []string{}, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
		}
	}
	return store
}

func parseTodoNumber(id string) (int, bool) {
	if !strings.HasPrefix(id, "todo-") {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, "todo-"))
	return number, err == nil
}

func FormatVerificationTodoLine(item VerificationTodoItem) string {
	return strings.TrimPrefix(strings.Join(renderTodoProjection([]VerificationTodoItem{item}), ""), "- ")
}

func FormatVerificationTodoMarkdownLine(item VerificationTodoItem, marker string) string {
	line := FormatVerificationTodoLine(item)
	if strings.TrimSpace(marker) == "" {
		return "- " + line
	}
	return fmt.Sprintf("- **(%s)** %s", marker, line)
}

func SanitizeVerificationTodoMarkdownContent(content string) string {
	return strings.Join(strings.Fields(content), " ")
}
