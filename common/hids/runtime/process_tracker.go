//go:build hids && linux

package runtime

import (
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
)

type processTracker struct {
	byPID map[int]model.Process
}

func newProcessTracker() *processTracker {
	return &processTracker{
		byPID: make(map[int]model.Process),
	}
}

func (t *processTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	switch event.Type {
	case model.EventTypeProcessExec:
		event.Process = t.enrichExec(event.Process)
		t.remember(event.Process)
	case model.EventTypeProcessExit:
		event.Process = t.enrichCached(event.Process)
		t.forget(event.Process)
	default:
		event.Process = t.enrichCached(event.Process)
	}
	return event
}

func (t *processTracker) remember(process *model.Process) {
	if t == nil || process == nil || process.PID <= 0 {
		return
	}

	cloned := *process
	normalizeProcessContext(&cloned)
	t.enrichParentContext(&cloned)
	t.byPID[cloned.PID] = cloned
}

func (t *processTracker) enrichExec(process *model.Process) *model.Process {
	if process == nil {
		return nil
	}

	cloned := *process
	if cloned.PID <= 0 {
		normalizeProcessContext(&cloned)
		return &cloned
	}

	if cached, ok := t.byPID[cloned.PID]; ok {
		if cloned.ParentPID == 0 {
			cloned.ParentPID = cached.ParentPID
		}
		if strings.TrimSpace(cloned.Username) == "" {
			cloned.Username = cached.Username
		}
		if strings.TrimSpace(cloned.ParentName) == "" {
			cloned.ParentName = cached.ParentName
		}
	}
	normalizeProcessContext(&cloned)
	t.enrichParentContext(&cloned)
	return &cloned
}

func (t *processTracker) enrichCached(process *model.Process) *model.Process {
	if process == nil {
		return nil
	}

	cloned := *process
	if cloned.PID <= 0 {
		normalizeProcessContext(&cloned)
		return &cloned
	}

	cached, ok := t.byPID[cloned.PID]
	if !ok {
		normalizeProcessContext(&cloned)
		t.enrichParentContext(&cloned)
		return &cloned
	}

	if cloned.ParentPID == 0 {
		cloned.ParentPID = cached.ParentPID
	}
	if strings.TrimSpace(cloned.Name) == "" {
		cloned.Name = cached.Name
	}
	if strings.TrimSpace(cloned.Username) == "" {
		cloned.Username = cached.Username
	}
	if strings.TrimSpace(cloned.Image) == "" {
		cloned.Image = cached.Image
	}
	if strings.TrimSpace(cloned.Command) == "" {
		cloned.Command = cached.Command
	}
	if strings.TrimSpace(cloned.ParentName) == "" {
		cloned.ParentName = cached.ParentName
	}
	normalizeProcessContext(&cloned)
	t.enrichParentContext(&cloned)
	return &cloned
}

func (t *processTracker) enrichParentContext(process *model.Process) {
	if t == nil || process == nil || process.ParentPID <= 0 || strings.TrimSpace(process.ParentName) != "" {
		return
	}

	parent, ok := t.byPID[process.ParentPID]
	if !ok {
		return
	}
	process.ParentName = firstNonEmptyString(parent.Name, baseProcessName(parent.Image), baseProcessNameFromCommand(parent.Command))
}

func (t *processTracker) forget(process *model.Process) {
	if t == nil || process == nil || process.PID <= 0 {
		return
	}
	delete(t.byPID, process.PID)
}

func normalizeProcessContext(process *model.Process) {
	if process == nil {
		return
	}
	process.Name = strings.TrimSpace(process.Name)
	process.Username = strings.TrimSpace(process.Username)
	process.Image = strings.TrimSpace(process.Image)
	process.Command = strings.TrimSpace(process.Command)
	process.ParentName = strings.TrimSpace(process.ParentName)

	if process.Name == "" {
		process.Name = firstNonEmptyString(baseProcessName(process.Image), baseProcessNameFromCommand(process.Command))
	}
	if process.Command == "" && process.Image != "" {
		process.Command = process.Image
	}
}

func baseProcessName(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	lastSlash := strings.LastIndexByte(image, '/')
	if lastSlash < 0 || lastSlash == len(image)-1 {
		return image
	}
	return image[lastSlash+1:]
}

func baseProcessNameFromCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return baseProcessName(strings.Trim(fields[0], `"'`))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
