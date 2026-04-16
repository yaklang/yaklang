//go:build hids && linux

package runtime

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
)

type fileTracker struct {
	byPath map[string]trackedFile
}

type trackedFile struct {
	mode  string
	uid   string
	gid   string
	owner string
	group string
}

func newFileTracker() *fileTracker {
	return &fileTracker{
		byPath: make(map[string]trackedFile),
	}
}

func (t *fileTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	switch event.Type {
	case model.EventTypeFileChange:
		event = t.enrichFileChange(event)
		if shouldForgetTrackedFileChange(event.File) {
			t.forget(fileTrackerPathFromFile(event.File))
			return event
		}
		t.rememberFile(event.File)
	case model.EventTypeAudit:
		event = t.enrichAudit(event)
		if shouldForgetTrackedAuditFile(event.Audit, event.File) {
			t.forget(fileTrackerPathFromFile(event.File))
			return event
		}
		t.rememberAudit(event.Audit, event.File)
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

func (t *fileTracker) rememberFile(file *model.File) {
	if t == nil || file == nil {
		return
	}
	path := fileTrackerPathFromFile(file)
	if path == "" {
		return
	}
	current := trackedFile{
		mode:  strings.TrimSpace(file.Mode),
		uid:   strings.TrimSpace(file.UID),
		gid:   strings.TrimSpace(file.GID),
		owner: strings.TrimSpace(file.Owner),
		group: strings.TrimSpace(file.Group),
	}
	if !current.hasIdentity() {
		return
	}
	t.byPath[path] = current
}

func (t *fileTracker) rememberAudit(audit *model.Audit, file *model.File) {
	if t == nil || audit == nil {
		return
	}
	path := fileTrackerPathFromFile(file)
	if path == "" {
		return
	}
	current := trackedFile{
		mode:  strings.TrimSpace(fileModeFromAudit(file, audit)),
		uid:   strings.TrimSpace(audit.FileUID),
		gid:   strings.TrimSpace(audit.FileGID),
		owner: strings.TrimSpace(audit.FileOwner),
		group: strings.TrimSpace(audit.FileGroup),
	}
	if !current.hasIdentity() {
		return
	}
	t.byPath[path] = current
}

func (t *fileTracker) forget(path string) {
	if t == nil || path == "" {
		return
	}
	delete(t.byPath, path)
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
