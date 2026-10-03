package coordinator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

func applyPlanEdit(current *Plan, params map[string]any, dir string) (*Plan, PlanEditReceipt, error) {
	r := PlanEditReceipt{Status: "updated"}
	if len(params) == 0 {
		return nil, r, fmt.Errorf("modify_plan requires at least one component")
	}
	for k, v := range params {
		if k != "document" && k != "document_patch" && k != "tasks" && k != "tasks_patch" {
			return nil, r, fmt.Errorf("unknown modify_plan parameter %q", k)
		}
		if nullPlanValue(v) {
			return nil, r, fmt.Errorf("%s cannot be null", k)
		}
	}
	_, doc := params["document"]
	_, diff := params["document_patch"]
	_, tasks := params["tasks"]
	_, patch := params["tasks_patch"]
	if (doc && diff) || (tasks && patch) {
		return nil, r, fmt.Errorf("replacement and patch are mutually exclusive for each component")
	}
	p := clone(current)
	if doc || diff {
		key := "document"
		if diff {
			key = "document_patch"
		}
		text, ok := params[key].(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, r, fmt.Errorf("%s must be a nonempty string", key)
		}
		if diff {
			var err error
			r.PatchArtifact, err = saveDocumentPatch(dir, text)
			if err != nil {
				return nil, r, err
			}
			p.Document, err = applyDocumentPatch(p.Document, text)
			if err != nil {
				return nil, r, err
			}
		} else {
			p.Document = text
		}
		if p.Document != current.Document {
			r.Components = append(r.Components, "document")
		}
	}
	if tasks || patch {
		var data []byte
		var err error
		if tasks {
			object, ok := params["tasks"].(map[string]any)
			if !ok || len(object) == 0 {
				return nil, r, fmt.Errorf("tasks must be a nonempty nested plan object")
			}
			definition, e := taskDefinition(object)
			if e != nil {
				return nil, r, e
			}
			data, err = json.Marshal(definitionTree(definition))
		} else {
			data, r.TaskIDs, err = applyTaskPatches(p.Tree, params["tasks_patch"])
		}
		if err != nil {
			return nil, r, err
		}
		p, err = ParsePlan(string(data), p.Document, current)
		if err != nil {
			return nil, r, err
		}
		if !reflect.DeepEqual(p.Tree, current.Tree) || !reflect.DeepEqual(p.Tasks, current.Tasks) {
			r.Components = append(r.Components, "tasks")
		}
	}
	if err := validateDocument(p); err != nil {
		return nil, r, err
	}
	if len(r.Components) == 0 {
		r.Status = "unchanged"
		r.TaskIDs = nil
	}
	return p, r, nil
}

func saveDocumentPatch(dir, patch string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "coordinator-plan-patches-")
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(patch))
	path := filepath.Join(dir, fmt.Sprintf("document-%x.patch", digest[:12]))
	if err := os.WriteFile(path, []byte(patch), 0600); err != nil {
		return "", err
	}
	return path, nil
}

var documentHunk = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?: .*)?$`)

// Strict single-document unified diff. No subprocess, fuzzy offsets or replacement fallback.
func applyDocumentPatch(document, patch string) (string, error) {
	return applyTextPatch(document, patch, "plan_document.md")
}

func applyTextPatch(document, patch, target string) (string, error) {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	pos := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "diff --git ") {
		if lines[0] != "diff --git a/"+target+" b/"+target {
			return "", fmt.Errorf("patch target must be %s", target)
		}
		pos++
		if pos < len(lines) && strings.HasPrefix(lines[pos], "index ") {
			pos++
		}
	}
	if len(lines) < pos+3 {
		return "", fmt.Errorf("document_patch requires file headers and a hunk")
	}
	for i, prefix := range []string{"--- ", "+++ "} {
		line := lines[pos+i]
		if !strings.HasPrefix(line, prefix) {
			return "", fmt.Errorf("missing unified diff header")
		}
		path := strings.SplitN(strings.TrimPrefix(line, prefix), "\t", 2)[0]
		if path != target && path != []string{"a/" + target, "b/" + target}[i] {
			return "", fmt.Errorf("patch target must be %s", target)
		}
	}
	pos += 2
	old := strings.SplitAfter(document, "\n")
	if len(old) > 0 && old[len(old)-1] == "" {
		old = old[:len(old)-1]
	}
	cursor := 0
	var out []string
	hunks := 0
	for pos < len(lines) {
		match := documentHunk.FindStringSubmatch(lines[pos])
		if match == nil {
			return "", fmt.Errorf("invalid or multi-file patch at line %d", pos+1)
		}
		numbers := [4]int{0, 1, 0, 1}
		for i, field := range match[1:5] {
			if field == "" {
				continue
			}
			n, err := strconv.Atoi(field)
			if err != nil {
				return "", fmt.Errorf("invalid hunk coordinate: %w", err)
			}
			numbers[i] = n
		}
		start, count, next, newCount := numbers[0], numbers[1], numbers[2], numbers[3]
		offset := start
		if count > 0 {
			offset--
		}
		if offset < cursor || offset > len(old) {
			return "", fmt.Errorf("hunk location does not match current document")
		}
		out = append(out, old[cursor:offset]...)
		cursor = offset
		expected := next
		if newCount > 0 {
			expected--
		}
		if expected != len(out) {
			return "", fmt.Errorf("new hunk location is inconsistent")
		}
		pos++
		used, added := 0, 0
		for pos < len(lines) && !strings.HasPrefix(lines[pos], "@@ ") {
			line := lines[pos]
			if len(line) == 0 || !strings.ContainsRune(" +-", rune(line[0])) {
				return "", fmt.Errorf("invalid patch line %d", pos+1)
			}
			kind := line[0]
			body := line[1:] + "\n"
			pos++
			if pos < len(lines) && lines[pos] == `\ No newline at end of file` {
				body = strings.TrimSuffix(body, "\n")
				pos++
			}
			if kind != '+' {
				if cursor >= len(old) || old[cursor] != body {
					return "", fmt.Errorf("document patch context mismatch at line %d", cursor+1)
				}
				cursor++
				used++
			}
			if kind != '-' {
				out = append(out, body)
				added++
			}
		}
		if used != count || added != newCount {
			return "", fmt.Errorf("hunk line counts do not match header")
		}
		hunks++
	}
	if hunks == 0 {
		return "", fmt.Errorf("empty document patch")
	}
	out = append(out, old[cursor:]...)
	result := strings.Join(out, "")
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("patched document must be nonempty")
	}
	return result, nil
}

// Definition fields only. Runtime fields and stable identity are never editable.
var editableTaskFields = map[string]string{
	"name": "name", "subtask_name": "name", "main_task": "name",
	"goal": "goal", "subtask_goal": "goal", "main_task_goal": "goal",
	"identifier": "semantic_identifier", "semantic_identifier": "semantic_identifier", "subtask_identifier": "semantic_identifier", "main_task_identifier": "semantic_identifier",
	"depends_on": "depends_on", "sub_subtasks": "subtasks", "subtasks": "subtasks", "tasks": "subtasks",
}

func taskDefinition(object map[string]any) (*PlanNode, error) {
	n := &PlanNode{}
	if err := changeTaskDefinition(n, object); err != nil {
		return nil, err
	}
	return n, nil
}

func changeTaskDefinition(n *PlanNode, changes map[string]any) error {
	if len(changes) == 0 {
		return fmt.Errorf("task changes must be nonempty")
	}
	seen := map[string]bool{}
	for key, value := range changes {
		field, ok := editableTaskFields[key]
		if !ok || seen[field] {
			return fmt.Errorf("uneditable or repeated task field %q", key)
		}
		seen[field] = true
		if nullPlanValue(value) {
			return fmt.Errorf("task field %s cannot be null", key)
		}
		switch field {
		case "name", "goal", "semantic_identifier":
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("task field %s must be a nonempty string", key)
			}
			switch field {
			case "name":
				n.Name = text
			case "goal":
				n.Goal = text
			case "semantic_identifier":
				n.Identifier = text
			}
		case "depends_on":
			raw, err := json.Marshal(value)
			if err != nil || json.Unmarshal(raw, &n.DependsOn) != nil {
				return fmt.Errorf("depends_on must be a string array")
			}
			if reflect.TypeOf(value).Kind() != reflect.Slice {
				return fmt.Errorf("depends_on must be a string array")
			}
		case "subtasks":
			children, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s must be a task array", key)
			}
			n.Subtasks = nil
			for _, raw := range children {
				object, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("task child must be an object")
				}
				child, err := taskDefinition(object)
				if err != nil {
					return err
				}
				n.Subtasks = append(n.Subtasks, child)
			}
		}
	}
	return nil
}

func applyTaskPatches(tree json.RawMessage, value any) ([]byte, []string, error) {
	ops, ok := value.([]any)
	if !ok || len(ops) == 0 {
		return nil, nil, fmt.Errorf("tasks_patch must be a nonempty operation array")
	}
	var root PlanNode
	if err := json.Unmarshal(tree, &root); err != nil {
		return nil, nil, err
	}
	var affected []string
	groups := map[*PlanNode]bool{}
	root.walk(func(n *PlanNode) {
		if len(n.Subtasks) > 0 {
			groups[n] = true
		}
	})
	find := func(id string) (*PlanNode, *PlanNode) {
		var found, parent *PlanNode
		var walk func(*PlanNode, *PlanNode)
		walk = func(n, p *PlanNode) {
			if n.TaskID == id {
				found, parent = n, p
			}
			for _, child := range n.Subtasks {
				walk(child, n)
			}
		}
		walk(&root, nil)
		return found, parent
	}
	for i, raw := range ops {
		op, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("task operation %d must be an object", i)
		}
		kind, ok := op["operator"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("task operator must be add/delete/update")
		}
		allowed := map[string]bool{"operator": true}
		switch kind {
		case "add":
			allowed["parent_task_id"] = true
			allowed["task"] = true
		case "delete":
			allowed["task_id"] = true
		case "update":
			allowed["task_id"] = true
			allowed["changes"] = true
		default:
			return nil, nil, fmt.Errorf("unknown task operator %q", kind)
		}
		for k, v := range op {
			if !allowed[k] || nullPlanValue(v) {
				return nil, nil, fmt.Errorf("invalid %s parameter %q", kind, k)
			}
		}
		if kind == "add" {
			parent := &root
			if raw, exists := op["parent_task_id"]; exists {
				id, ok := raw.(string)
				if !ok || id == "" {
					return nil, nil, fmt.Errorf("parent_task_id must be nonempty")
				}
				parent, _ = find(id)
			}
			if parent == nil || (!groups[parent] && len(parent.Subtasks) == 0) {
				return nil, nil, fmt.Errorf("add requires an existing task group")
			}
			object, ok := op["task"].(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("add requires task object")
			}
			n, err := taskDefinition(object)
			if err != nil {
				return nil, nil, err
			}
			n.walk(func(child *PlanNode) {
				if len(child.Subtasks) > 0 {
					groups[child] = true
				}
			})
			parent.Subtasks = append(parent.Subtasks, n)
			affected = append(affected, parent.TaskID)
			continue
		}
		id, ok := op["task_id"].(string)
		if !ok || id == "" {
			return nil, nil, fmt.Errorf("task_id must be a nonempty stable ID")
		}
		n, parent := find(id)
		if n == nil {
			return nil, nil, fmt.Errorf("unknown task_id %q", id)
		}
		affected = append(affected, id)
		if kind == "delete" {
			if parent == nil {
				return nil, nil, fmt.Errorf("cannot delete the root task group")
			}
			for j, child := range parent.Subtasks {
				if child == n {
					parent.Subtasks = append(parent.Subtasks[:j], parent.Subtasks[j+1:]...)
					break
				}
			}
		} else {
			changes, ok := op["changes"].(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("update requires changes object")
			}
			if err := changeTaskDefinition(n, changes); err != nil {
				return nil, nil, err
			}
			n.walk(func(child *PlanNode) {
				if len(child.Subtasks) > 0 {
					groups[child] = true
				}
			})
		}
	}
	var emptyGroup string
	root.walk(func(n *PlanNode) {
		if groups[n] && len(n.Subtasks) == 0 {
			emptyGroup = n.Name
		}
	})
	if emptyGroup != "" {
		return nil, nil, fmt.Errorf("patch would empty task group %q", emptyGroup)
	}
	data, err := json.Marshal(definitionTree(&root))
	return data, affected, err
}

// JSON null and typed nil values at direct Go entry points are equally invalid.
func nullPlanValue(value any) bool {
	if value == nil {
		return true
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Ptr, reflect.Interface:
		return v.IsNil()
	}
	return false
}
