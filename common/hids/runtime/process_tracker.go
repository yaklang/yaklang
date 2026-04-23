//go:build hids && linux

package runtime

import (
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type processTracker struct {
	byPID      map[int]trackedProcess
	window     time.Duration
	maxEntries int
	nextPrune  time.Time
	sequence   uint64
	order      []trackedProcessOrder
}

type trackedProcess struct {
	process  model.Process
	lastSeen time.Time
	sequence uint64
}

type trackedProcessOrder struct {
	pid      int
	sequence uint64
	lastSeen time.Time
}

func newProcessTracker() *processTracker {
	return newProcessTrackerWithConfig(shortTermContextConfigFromSpec(model.DesiredSpec{}))
}

func newProcessTrackerWithConfig(config shortTermContextConfig) *processTracker {
	if config.window <= 0 {
		config.window = time.Duration(model.DefaultShortTermWindowMinutes) * time.Minute
	}
	if config.maxProcesses <= 0 {
		config.maxProcesses = defaultShortTermContextMaxProcesses
	}
	return &processTracker{
		byPID:      make(map[int]trackedProcess),
		window:     config.window,
		maxEntries: config.maxProcesses,
	}
}

func (t *processTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	observedAt := normalizedEventTimestamp(event.Timestamp)
	t.prune(observedAt)
	switch event.Type {
	case model.EventTypeProcessExec:
		event.Process = t.enrichExec(event.Process)
		t.remember(event.Process, observedAt)
	case model.EventTypeProcessExit:
		event.Process = t.enrichCached(event.Process)
		t.forget(event.Process)
	default:
		event.Process = t.enrichCached(event.Process)
	}
	return event
}

func (t *processTracker) remember(process *model.Process, observedAt time.Time) {
	if t == nil || process == nil || process.PID <= 0 {
		return
	}

	cloned := *process
	normalizeProcessContext(&cloned)
	t.enrichParentContext(&cloned)
	t.sequence++
	t.byPID[cloned.PID] = trackedProcess{
		process:  cloned,
		lastSeen: observedAt,
		sequence: t.sequence,
	}
	t.order = append(t.order, trackedProcessOrder{
		pid:      cloned.PID,
		sequence: t.sequence,
		lastSeen: observedAt,
	})
	t.prune(observedAt)
}

func (t *processTracker) enrichExec(process *model.Process) *model.Process {
	if process == nil {
		return nil
	}

	if process.PID <= 0 {
		normalizeProcessContext(process)
		return process
	}

	if cached, ok := t.byPID[process.PID]; ok {
		if process.ParentPID == 0 {
			process.ParentPID = cached.process.ParentPID
		}
		if strings.TrimSpace(process.Username) == "" {
			process.Username = cached.process.Username
		}
		if strings.TrimSpace(process.ParentName) == "" {
			process.ParentName = cached.process.ParentName
		}
		if strings.TrimSpace(process.ParentImage) == "" {
			process.ParentImage = cached.process.ParentImage
		}
		if strings.TrimSpace(process.ParentCommand) == "" {
			process.ParentCommand = cached.process.ParentCommand
		}
	}
	normalizeProcessContext(process)
	t.enrichParentContext(process)
	return process
}

func (t *processTracker) enrichCached(process *model.Process) *model.Process {
	if process == nil {
		return nil
	}

	if process.PID <= 0 {
		normalizeProcessContext(process)
		return process
	}

	cached, ok := t.byPID[process.PID]
	if !ok {
		normalizeProcessContext(process)
		t.enrichParentContext(process)
		return process
	}

	if process.ParentPID == 0 {
		process.ParentPID = cached.process.ParentPID
	}
	if strings.TrimSpace(process.Name) == "" {
		process.Name = cached.process.Name
	}
	if strings.TrimSpace(process.Username) == "" {
		process.Username = cached.process.Username
	}
	if strings.TrimSpace(process.Image) == "" {
		process.Image = cached.process.Image
	}
	if strings.TrimSpace(process.Command) == "" {
		process.Command = cached.process.Command
	}
	if strings.TrimSpace(process.ParentName) == "" {
		process.ParentName = cached.process.ParentName
	}
	if strings.TrimSpace(process.ParentImage) == "" {
		process.ParentImage = cached.process.ParentImage
	}
	if strings.TrimSpace(process.ParentCommand) == "" {
		process.ParentCommand = cached.process.ParentCommand
	}
	normalizeProcessContext(process)
	t.enrichParentContext(process)
	return process
}

func (t *processTracker) enrichParentContext(process *model.Process) {
	if t == nil || process == nil || process.ParentPID <= 0 {
		return
	}

	parent, ok := t.byPID[process.ParentPID]
	if !ok {
		return
	}
	if strings.TrimSpace(process.ParentName) == "" {
		process.ParentName = firstNonEmptyString(parent.process.Name, baseProcessName(parent.process.Image), baseProcessNameFromCommand(parent.process.Command))
	}
	if strings.TrimSpace(process.ParentImage) == "" {
		process.ParentImage = parent.process.Image
	}
	if strings.TrimSpace(process.ParentCommand) == "" {
		process.ParentCommand = parent.process.Command
	}
}

func (t *processTracker) forget(process *model.Process) {
	if t == nil || process == nil || process.PID <= 0 {
		return
	}
	delete(t.byPID, process.PID)
	t.compactOrderIfStale()
}

func (t *processTracker) prune(observedAt time.Time) {
	if t == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	if t.maxEntries > 0 && len(t.byPID) > t.maxEntries {
		t.evictOverflow()
	}

	if t.window > 0 && (t.nextPrune.IsZero() || !observedAt.Before(t.nextPrune)) {
		evicted := false
		for pid, entry := range t.byPID {
			if !entry.lastSeen.IsZero() && observedAt.Sub(entry.lastSeen) > t.window {
				delete(t.byPID, pid)
				evicted = true
			}
		}
		t.nextPrune = observedAt.Add(shortTermContextPruneInterval)
		if evicted {
			t.compactOrder()
		}
	}

	t.compactOrderIfStale()
}

func (t *processTracker) evictOverflow() {
	if t == nil || t.maxEntries <= 0 {
		return
	}
	for len(t.byPID) > t.maxEntries {
		if len(t.order) == 0 {
			t.compactOrder()
			if len(t.order) == 0 {
				return
			}
		}
		candidate := t.order[0]
		t.order = t.order[1:]
		entry, exists := t.byPID[candidate.pid]
		if !exists || entry.sequence != candidate.sequence {
			continue
		}
		delete(t.byPID, candidate.pid)
	}
}

func (t *processTracker) compactOrderIfStale() {
	if t == nil || len(t.order) == 0 {
		return
	}
	threshold := processTrackerOrderCompactThreshold(len(t.byPID), t.maxEntries)
	if threshold <= 0 || len(t.order) <= threshold {
		return
	}
	t.compactOrder()
}

func processTrackerOrderCompactThreshold(activeEntries int, maxEntries int) int {
	threshold := activeEntries*4 + 1024
	if maxEntries > 0 && maxEntries*2 > threshold {
		threshold = maxEntries * 2
	}
	return threshold
}

func (t *processTracker) compactOrder() {
	if t == nil {
		return
	}
	if len(t.byPID) == 0 {
		t.order = nil
		return
	}
	if cap(t.order) < len(t.byPID) {
		t.order = make([]trackedProcessOrder, 0, len(t.byPID))
	} else {
		t.order = t.order[:0]
	}
	for pid, entry := range t.byPID {
		t.order = append(t.order, trackedProcessOrder{
			pid:      pid,
			sequence: entry.sequence,
			lastSeen: entry.lastSeen,
		})
	}
	sort.Slice(t.order, func(left int, right int) bool {
		leftSeen := t.order[left].lastSeen
		rightSeen := t.order[right].lastSeen
		if leftSeen.Equal(rightSeen) {
			return t.order[left].sequence < t.order[right].sequence
		}
		if leftSeen.IsZero() {
			return true
		}
		if rightSeen.IsZero() {
			return false
		}
		return leftSeen.Before(rightSeen)
	})
	if t.maxEntries <= 0 || len(t.byPID) <= t.maxEntries {
		return
	}
	for len(t.byPID) > t.maxEntries && len(t.order) > 0 {
		candidate := t.order[0]
		t.order = t.order[1:]
		entry, exists := t.byPID[candidate.pid]
		if !exists || entry.sequence != candidate.sequence {
			continue
		}
		delete(t.byPID, candidate.pid)
	}
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
	process.ParentImage = strings.TrimSpace(process.ParentImage)
	process.ParentCommand = strings.TrimSpace(process.ParentCommand)

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
