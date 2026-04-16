//go:build hids && linux

package auditd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

type sensitiveFileState struct {
	IsDir bool
	Mode  string
	UID   string
	GID   string
	Owner string
	Group string
}

type sensitiveFileStateCache struct {
	mu        sync.RWMutex
	snapshots map[string]sensitiveFileState
}

func newSensitiveFileStateCache() *sensitiveFileStateCache {
	return &sensitiveFileStateCache{
		snapshots: make(map[string]sensitiveFileState),
	}
}

func (c *sensitiveFileStateCache) Seed() {
	if c == nil {
		return
	}
	for _, path := range policy.SensitiveAuditSeedPaths() {
		c.seedPath(path)
	}
}

func (c *sensitiveFileStateCache) seedPath(root string) {
	if c == nil {
		return
	}
	normalizedRoot := policy.NormalizePath(root)
	if normalizedRoot == "" {
		return
	}

	info, err := os.Lstat(normalizedRoot)
	if err != nil {
		return
	}

	c.store(normalizedRoot, fileStateFromIdentity(enrich.FileIdentityFromFileInfo(info)))
	if !info.IsDir() {
		return
	}

	_ = filepath.WalkDir(normalizedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		normalizedPath := policy.NormalizePath(path)
		if normalizedPath == "" || !policy.IsSensitiveAuditPath(normalizedPath) {
			return nil
		}
		if d == nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		c.store(normalizedPath, fileStateFromIdentity(enrich.FileIdentityFromFileInfo(info)))
		return nil
	})
}

func (c *sensitiveFileStateCache) Enrich(event model.Event) model.Event {
	if c == nil || event.Audit == nil {
		return event
	}

	filePath := policy.NormalizePath(firstNonEmpty(
		valueOrEmpty(event.File, func(file *model.File) string { return file.Path }),
		valueOrEmpty(event.Audit, func(audit *model.Audit) string { return audit.ObjectPrimary }),
		valueOrEmpty(event.Audit, func(audit *model.Audit) string { return audit.ObjectSecondary }),
	))
	if !policy.IsSensitiveAuditPath(filePath) {
		return event
	}

	previous, hasPrevious := c.load(filePath)
	current, hasCurrent := snapshotSensitiveFileState(filePath)

	if hasPrevious {
		event.Audit.PreviousFileMode = previous.Mode
		event.Audit.PreviousFileUID = previous.UID
		event.Audit.PreviousFileGID = previous.GID
		event.Audit.PreviousFileOwner = previous.Owner
		event.Audit.PreviousFileGroup = previous.Group
		if event.File != nil {
			if strings.TrimSpace(event.File.Mode) == "" {
				event.File.Mode = previous.Mode
			}
			if strings.TrimSpace(event.File.UID) == "" {
				event.File.UID = previous.UID
			}
			if strings.TrimSpace(event.File.GID) == "" {
				event.File.GID = previous.GID
			}
			if strings.TrimSpace(event.File.Owner) == "" {
				event.File.Owner = previous.Owner
			}
			if strings.TrimSpace(event.File.Group) == "" {
				event.File.Group = previous.Group
			}
		}
	}

	if hasCurrent {
		event = applySensitiveFileState(event, current)
		c.store(filePath, current)
	} else {
		c.delete(filePath)
	}

	return event
}

func snapshotSensitiveFileState(path string) (sensitiveFileState, bool) {
	identity, err := enrich.SnapshotFileIdentity(path)
	if err != nil {
		return sensitiveFileState{}, false
	}
	return fileStateFromIdentity(identity), true
}

func fileStateFromIdentity(identity enrich.FileIdentity) sensitiveFileState {
	return sensitiveFileState{
		IsDir: identity.IsDir,
		Mode:  identity.Mode,
		UID:   identity.UID,
		GID:   identity.GID,
		Owner: identity.Owner,
		Group: identity.Group,
	}
}

func applySensitiveFileState(event model.Event, state sensitiveFileState) model.Event {
	if event.File == nil {
		event.File = &model.File{}
	}
	event.File.IsDir = state.IsDir
	event.File.Mode = state.Mode
	event.File.UID = state.UID
	event.File.GID = state.GID
	event.File.Owner = state.Owner
	event.File.Group = state.Group

	if event.Audit != nil {
		event.Audit.FileMode = state.Mode
		event.Audit.FileUID = state.UID
		event.Audit.FileGID = state.GID
		event.Audit.FileOwner = state.Owner
		event.Audit.FileGroup = state.Group
	}
	return event
}

func (c *sensitiveFileStateCache) load(path string) (sensitiveFileState, bool) {
	if c == nil {
		return sensitiveFileState{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, ok := c.snapshots[path]
	return state, ok
}

func (c *sensitiveFileStateCache) store(path string, state sensitiveFileState) {
	if c == nil || path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshots[path] = state
}

func (c *sensitiveFileStateCache) delete(path string) {
	if c == nil || path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.snapshots, path)
}
