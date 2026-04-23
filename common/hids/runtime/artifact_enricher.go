//go:build hids && linux

package runtime

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
)

type artifactEnricher struct {
	options enrich.ArtifactSnapshotOptions
	window  time.Duration

	maxEntries int
	nextPrune  time.Time

	mu    sync.RWMutex
	cache map[string]artifactCacheEntry
}

type artifactCacheEntry struct {
	signature artifactSignature
	artifact  *model.Artifact
	lastUsed  time.Time
}

type artifactSignature struct {
	sizeBytes int64
	mode      os.FileMode
	modUnixNs int64
}

func newArtifactEnricher(policy model.EvidencePolicy, config shortTermContextConfig) *artifactEnricher {
	if config.window <= 0 {
		config.window = time.Duration(model.DefaultShortTermWindowMinutes) * time.Minute
	}
	if config.maxFiles <= 0 {
		config.maxFiles = defaultShortTermContextMaxFiles
	}
	return &artifactEnricher{
		options: enrich.ArtifactSnapshotOptions{
			CaptureHashes: policy.CaptureFileHash,
		},
		window:     config.window,
		maxEntries: config.maxFiles,
		cache:      make(map[string]artifactCacheEntry),
	}
}

func (e *artifactEnricher) Apply(event model.Event) model.Event {
	if e == nil {
		return event
	}
	if event.Process != nil {
		event.Process = e.enrichProcess(event.Process)
	}
	if event.File != nil {
		event.File = e.enrichFile(event.File)
	}
	return event
}

func (e *artifactEnricher) enrichProcess(process *model.Process) *model.Process {
	if e == nil || process == nil {
		return process
	}
	path := strings.TrimSpace(process.Image)
	if path == "" || !strings.HasPrefix(path, "/") {
		return process
	}

	cloned := *process
	if artifact := e.snapshot(path); artifact != nil {
		cloned.Artifact = artifact
	}
	return &cloned
}

func (e *artifactEnricher) enrichFile(file *model.File) *model.File {
	if e == nil || file == nil {
		return file
	}
	path := strings.TrimSpace(file.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return file
	}

	cloned := *file
	if artifact := e.snapshot(path); artifact != nil {
		cloned.Artifact = artifact
	}
	return &cloned
}

func (e *artifactEnricher) snapshot(path string) *model.Artifact {
	artifact, _ := e.snapshotWithError(path)
	return artifact
}

func (e *artifactEnricher) snapshotWithError(path string) (*model.Artifact, error) {
	signature, ok := currentArtifactSignature(path)
	if !ok {
		return enrich.SnapshotArtifact(path, e.options)
	}

	now := time.Now().UTC()
	e.prune(now)
	cacheKey := artifactCacheKey(path, e.options.CaptureHashes)
	e.mu.RLock()
	cached, ok := e.cache[cacheKey]
	e.mu.RUnlock()
	if ok && cached.signature == signature {
		e.mu.Lock()
		if current, exists := e.cache[cacheKey]; exists && current.signature == signature {
			current.lastUsed = now
			e.cache[cacheKey] = current
		}
		e.mu.Unlock()
		return model.CloneArtifact(cached.artifact), nil
	}

	artifact, err := enrich.SnapshotArtifact(path, e.options)
	if err != nil && artifact == nil {
		return nil, err
	}
	if artifact != nil {
		e.mu.Lock()
		e.cache[cacheKey] = artifactCacheEntry{
			signature: signature,
			artifact:  model.CloneArtifact(artifact),
			lastUsed:  now,
		}
		e.pruneLocked(now)
		e.enforceCapacityLocked()
		e.mu.Unlock()
	}
	return artifact, err
}

func (e *artifactEnricher) prune(observedAt time.Time) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pruneLocked(observedAt)
	e.enforceCapacityLocked()
}

func (e *artifactEnricher) pruneLocked(observedAt time.Time) {
	if e == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if len(e.cache) <= e.maxEntries && !e.nextPrune.IsZero() && observedAt.Before(e.nextPrune) {
		return
	}
	if e.window > 0 {
		for key, entry := range e.cache {
			if !entry.lastUsed.IsZero() && observedAt.Sub(entry.lastUsed) > e.window {
				delete(e.cache, key)
			}
		}
	}
	e.nextPrune = observedAt.Add(shortTermContextPruneInterval)
}

func (e *artifactEnricher) enforceCapacityLocked() {
	if e == nil || e.maxEntries <= 0 {
		return
	}
	for len(e.cache) > e.maxEntries {
		oldestKey := ""
		var oldestUsed time.Time
		for key, entry := range e.cache {
			if oldestKey == "" || entry.lastUsed.Before(oldestUsed) {
				oldestKey = key
				oldestUsed = entry.lastUsed
			}
		}
		if oldestKey == "" {
			return
		}
		delete(e.cache, oldestKey)
	}
}

func currentArtifactSignature(path string) (artifactSignature, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return artifactSignature{}, false
	}
	return artifactSignature{
		sizeBytes: info.Size(),
		mode:      info.Mode(),
		modUnixNs: info.ModTime().UTC().UnixNano(),
	}, true
}

func artifactCacheKey(path string, captureHashes bool) string {
	if captureHashes {
		return "hash:" + path
	}
	return "plain:" + path
}
