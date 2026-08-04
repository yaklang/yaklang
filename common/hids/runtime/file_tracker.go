//go:build hids && linux

package runtime

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type fileTracker struct {
	byPath     map[string]trackedFile
	window     time.Duration
	maxEntries int
	nextPrune  time.Time
}

type trackedFile struct {
	mode     string
	uid      string
	gid      string
	owner    string
	group    string
	lastSeen time.Time
}

func newFileTracker() *fileTracker {
	return newFileTrackerWithConfig(shortTermContextConfigFromSpec(model.DesiredSpec{}))
}

func newFileTrackerWithConfig(config shortTermContextConfig) *fileTracker {
	if config.window <= 0 {
		config.window = time.Duration(model.DefaultShortTermWindowMinutes) * time.Minute
	}
	if config.maxFiles <= 0 {
		config.maxFiles = defaultShortTermContextMaxFiles
	}
	return &fileTracker{
		byPath:     make(map[string]trackedFile),
		window:     config.window,
		maxEntries: config.maxFiles,
	}
}

func (t *fileTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	observedAt := normalizedEventTimestamp(event.Timestamp)
	t.prune(observedAt)
	switch event.Type {
	case model.EventTypeFileChange:
		event = t.enrichFileChange(event)
		if shouldForgetTrackedFileChange(event.File) {
			t.forget(fileTrackerPathFromFile(event.File))
			return event
		}
		t.rememberFile(event.File, observedAt)
	case model.EventTypeAudit:
		event = t.enrichAudit(event)
		if shouldForgetTrackedAuditFile(event.Audit, event.File) {
			t.forget(fileTrackerPathFromFile(event.File))
			return event
		}
		t.rememberAudit(event.Audit, event.File, observedAt)
	}
	return event
}

func (t *fileTracker) enrichFileChange(event model.Event) model.Event {
	path := fileTrackerPathFromFile(event.File)
	if path == "" {
		return event
	}
	previous, ok := t.byPath[path]
	if !ok {
		return event
	}
	event.Data = mergeEventData(
		map[string]any{
			"previous_file_mode":  previous.mode,
			"previous_file_uid":   previous.uid,
			"previous_file_gid":   previous.gid,
			"previous_file_owner": previous.owner,
			"previous_file_group": previous.group,
		},
		event.Data,
	)
	return event
}

func (t *fileTracker) enrichAudit(event model.Event) model.Event {
	if event.Audit == nil {
		return event
	}
	path := fileTrackerPathFromFile(event.File)
	if path == "" {
		return event
	}
	previous, ok := t.byPath[path]
	if !ok {
		return event
	}
	if strings.TrimSpace(event.Audit.PreviousFileMode) == "" {
		event.Audit.PreviousFileMode = previous.mode
	}
	if strings.TrimSpace(event.Audit.PreviousFileUID) == "" {
		event.Audit.PreviousFileUID = previous.uid
	}
	if strings.TrimSpace(event.Audit.PreviousFileGID) == "" {
		event.Audit.PreviousFileGID = previous.gid
	}
	if strings.TrimSpace(event.Audit.PreviousFileOwner) == "" {
		event.Audit.PreviousFileOwner = previous.owner
	}
	if strings.TrimSpace(event.Audit.PreviousFileGroup) == "" {
		event.Audit.PreviousFileGroup = previous.group
	}
	event.Data = mergeEventData(
		map[string]any{
			"previous_file_mode":  event.Audit.PreviousFileMode,
			"previous_file_uid":   event.Audit.PreviousFileUID,
			"previous_file_gid":   event.Audit.PreviousFileGID,
			"previous_file_owner": event.Audit.PreviousFileOwner,
			"previous_file_group": event.Audit.PreviousFileGroup,
		},
		event.Data,
	)
	return event
}

func (t *fileTracker) rememberFile(file *model.File, observedAt time.Time) {
	if t == nil || file == nil {
		return
	}
	path := fileTrackerPathFromFile(file)
	if path == "" {
		return
	}
	current := trackedFile{
		mode:     strings.TrimSpace(file.Mode),
		uid:      strings.TrimSpace(file.UID),
		gid:      strings.TrimSpace(file.GID),
		owner:    strings.TrimSpace(file.Owner),
		group:    strings.TrimSpace(file.Group),
		lastSeen: observedAt,
	}
	if !current.hasIdentity() {
		return
	}
	t.byPath[path] = current
	t.prune(observedAt)
}

func (t *fileTracker) rememberAudit(audit *model.Audit, file *model.File, observedAt time.Time) {
	if t == nil || audit == nil {
		return
	}
	path := fileTrackerPathFromFile(file)
	if path == "" {
		return
	}
	current := trackedFile{
		mode:     strings.TrimSpace(fileModeFromAudit(file, audit)),
		uid:      strings.TrimSpace(audit.FileUID),
		gid:      strings.TrimSpace(audit.FileGID),
		owner:    strings.TrimSpace(audit.FileOwner),
		group:    strings.TrimSpace(audit.FileGroup),
		lastSeen: observedAt,
	}
	if !current.hasIdentity() {
		return
	}
	t.byPath[path] = current
	t.prune(observedAt)
}

func (t *fileTracker) forget(path string) {
	if t == nil || path == "" {
		return
	}
	delete(t.byPath, path)
}

func (t *fileTracker) prune(observedAt time.Time) {
	if t == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if len(t.byPath) <= t.maxEntries && !t.nextPrune.IsZero() && observedAt.Before(t.nextPrune) {
		return
	}
	if t.window > 0 {
		for path, entry := range t.byPath {
			if !entry.lastSeen.IsZero() && observedAt.Sub(entry.lastSeen) > t.window {
				delete(t.byPath, path)
			}
		}
	}
	for t.maxEntries > 0 && len(t.byPath) > t.maxEntries {
		oldestPath := ""
		var oldestSeen time.Time
		for path, entry := range t.byPath {
			if oldestPath == "" || entry.lastSeen.Before(oldestSeen) {
				oldestPath = path
				oldestSeen = entry.lastSeen
			}
		}
		if oldestPath == "" {
			break
		}
		delete(t.byPath, oldestPath)
	}
	t.nextPrune = observedAt.Add(shortTermContextPruneInterval)
}

func (value trackedFile) hasIdentity() bool {
	return value.mode != "" || value.uid != "" || value.gid != "" || value.owner != "" || value.group != ""
}

func shouldForgetTrackedFileChange(file *model.File) bool {
	if file == nil {
		return false
	}
	operation := strings.ToUpper(strings.TrimSpace(file.Operation))
	return strings.Contains(operation, "REMOVE") || strings.Contains(operation, "RENAME")
}

func shouldForgetTrackedAuditFile(audit *model.Audit, file *model.File) bool {
	if audit == nil {
		return false
	}
	action := strings.ToLower(strings.TrimSpace(audit.Action))
	if action == "" && file != nil {
		action = strings.ToLower(strings.TrimSpace(file.Operation))
	}
	for _, token := range []string{"unlink", "remove", "delete", "rename", "rmdir"} {
		if strings.Contains(action, token) {
			return true
		}
	}
	return false
}

func fileTrackerPathFromFile(file *model.File) string {
	if file == nil {
		return ""
	}
	value := strings.TrimSpace(file.Path)
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." {
		return ""
	}
	return value
}

func fileModeFromAudit(file *model.File, audit *model.Audit) string {
	if audit != nil && strings.TrimSpace(audit.FileMode) != "" {
		return audit.FileMode
	}
	if file != nil {
		return file.Mode
	}
	return ""
}
