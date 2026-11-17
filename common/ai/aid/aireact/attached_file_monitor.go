package aireact

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

const (
	attachedFileMonitorHashLimit    = 10 * 1024 * 1024
	attachedFileMonitorPollInterval = time.Second
)

type attachedFileFingerprint struct {
	exists  bool
	size    int64
	modTime time.Time
	sha256  string
}

func readAttachedFileFingerprint(path string) (attachedFileFingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return attachedFileFingerprint{}, nil
		}
		return attachedFileFingerprint{}, err
	}
	if info.IsDir() {
		return attachedFileFingerprint{}, fmt.Errorf("attached file path %q is a directory", path)
	}

	fingerprint := attachedFileFingerprint{
		exists:  true,
		size:    info.Size(),
		modTime: info.ModTime(),
	}
	if info.Size() > attachedFileMonitorHashLimit {
		return fingerprint, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return attachedFileFingerprint{}, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return attachedFileFingerprint{}, err
	}
	fingerprint.sha256 = fmt.Sprintf("%x", hasher.Sum(nil))
	return fingerprint, nil
}

func attachedFileFingerprintChanged(previous, current attachedFileFingerprint) bool {
	if previous.exists != current.exists {
		return true
	}
	if !current.exists {
		return false
	}
	if previous.sha256 != "" && current.sha256 != "" {
		return previous.sha256 != current.sha256
	}
	return previous.size != current.size || !previous.modTime.Equal(current.modTime)
}

type attachedFileMonitor struct {
	path    string
	watcher *filesys.YakFileMonitor
	stopped atomic.Bool

	mu          sync.Mutex
	fingerprint attachedFileFingerprint
}

func (m *attachedFileMonitor) check(runtime *ReAct) {
	if m == nil || runtime == nil || m.stopped.Load() {
		return
	}

	current, err := readAttachedFileFingerprint(m.path)
	if err != nil {
		log.Warnf("attached file monitor failed to inspect %s: %v", m.path, err)
		return
	}

	m.mu.Lock()
	previous := m.fingerprint
	if !attachedFileFingerprintChanged(previous, current) {
		m.fingerprint = current
		m.mu.Unlock()
		return
	}
	m.fingerprint = current
	m.mu.Unlock()

	runtime.recordAttachedFileChange(m.path, previous, current)
}

func (r *ReAct) ensureAttachedFileMonitors(resources []*aicommon.AttachedResource, opts ...filesys.WatchPathOption) {
	if r == nil {
		return
	}
	for _, resource := range resources {
		if !isAttachedFilePathResource(resource) {
			continue
		}
		if err := r.ensureAttachedFilePathMonitor(resource.Value, opts...); err != nil {
			log.Warnf("failed to monitor attached file %s: %v", resource.Value, err)
		}
	}
}

func isAttachedFilePathResource(resource *aicommon.AttachedResource) bool {
	if resource == nil || !resource.HasType(aicommon.AttachedResourceTypeFile) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(resource.Key)) {
	case aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "filepath", "file-path":
		return true
	default:
		return false
	}
}

func (r *ReAct) ensureAttachedFilePathMonitor(path string, opts ...filesys.WatchPathOption) error {
	if r == nil || r.config == nil || r.config.Timeline == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolutePath = filepath.Clean(absolutePath)

	r.attachedFileMonitorsMu.Lock()
	defer r.attachedFileMonitorsMu.Unlock()
	if r.attachedFileMonitors == nil {
		r.attachedFileMonitors = make(map[string]*attachedFileMonitor)
	}
	if _, exists := r.attachedFileMonitors[absolutePath]; exists {
		return nil
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	initial, err := readAttachedFileFingerprint(absolutePath)
	if err != nil {
		return err
	}

	state := &attachedFileMonitor{
		path:        absolutePath,
		fingerprint: initial,
	}
	if len(opts) == 0 {
		opts = append(opts, filesys.WithPollInterval(attachedFileMonitorPollInterval))
	}
	watcher, err := filesys.WatchPath(
		r.config.GetContext(),
		absolutePath,
		func(*filesys.EventSet) {
			state.check(r)
		},
		opts...,
	)
	if err != nil {
		return err
	}
	state.watcher = watcher
	r.attachedFileMonitors[absolutePath] = state
	return nil
}

func (r *ReAct) recordAttachedFileChange(path string, previous, current attachedFileFingerprint) {
	if r == nil || r.config == nil || r.config.Timeline == nil {
		return
	}

	if path == "" {
		return
	}

	var message string
	switch {
	case previous.exists && !current.exists:
		message = fmt.Sprintf(
			"The user-pinned file `%s` was removed after it was attached. The earlier attachment preview is stale.",
			path,
		)
	case !previous.exists && current.exists:
		message = fmt.Sprintf(
			"The user-pinned file `%s` reappeared after it was attached. The earlier attachment preview is stale; re-read the file before relying on its content.",
			path,
		)
	default:
		message = fmt.Sprintf(
			"The user-pinned file `%s` changed on disk after it was attached (fingerprint %s -> %s). The earlier attachment preview is stale; re-read the file before relying on its content.",
			path,
			shortAttachedFileFingerprint(previous),
			shortAttachedFileFingerprint(current),
		)
	}
	r.AddToTimeline("attached_file_changed", message)
	r.SaveTimeline()
}

func shortAttachedFileFingerprint(fingerprint attachedFileFingerprint) string {
	if !fingerprint.exists {
		return "missing"
	}
	if fingerprint.sha256 == "" {
		return fmt.Sprintf("size=%d,mtime=%d", fingerprint.size, fingerprint.modTime.UnixNano())
	}
	if len(fingerprint.sha256) <= 12 {
		return fingerprint.sha256
	}
	return fingerprint.sha256[:12]
}

// CloseAttachedFileMonitors stops session-scoped pinned-file monitors. Normal
// AIEngine shutdown also stops them by cancelling the ReAct config context.
func (r *ReAct) CloseAttachedFileMonitors() {
	if r == nil {
		return
	}
	r.attachedFileMonitorsMu.Lock()
	monitors := r.attachedFileMonitors
	r.attachedFileMonitors = make(map[string]*attachedFileMonitor)
	r.attachedFileMonitorsMu.Unlock()

	for _, monitor := range monitors {
		if monitor == nil {
			continue
		}
		monitor.stopped.Store(true)
		if monitor.watcher != nil && monitor.watcher.CancelFunc != nil {
			monitor.watcher.CancelFunc()
		}
	}
}
