//go:build hids && linux

package runtime

import (
	"context"
	"errors"
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
	err       error
}

const automaticArtifactTimeout = 2 * time.Second

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
	return e.ApplyContext(context.Background(), event)
}

func (e *artifactEnricher) ApplyContext(ctx context.Context, event model.Event) model.Event {
	if e == nil {
		return event
	}
	// Automatic event enrichment shares this short budget. Explicit scans pass
	// their own request context to snapshotWithErrorContext and keep 30 seconds.
	ctx, cancel := context.WithTimeout(ctx, automaticArtifactTimeout)
	defer cancel()
	var issues []map[string]any
	if event.Process != nil {
		var err error
		event.Process, err = e.enrichProcess(ctx, event.Process)
		if err != nil {
			issues = append(issues, map[string]any{"target": "process.image", "error": err.Error()})
		}
	}
	if event.File != nil {
		var err error
		event.File, err = e.enrichFile(ctx, event.File)
		if err != nil {
			issues = append(issues, map[string]any{"target": "file.path", "error": err.Error()})
		}
	}
	if len(issues) > 0 {
		event.Data = cloneAnyMap(event.Data)
		if event.Data == nil {
			event.Data = make(map[string]any)
		}
		event.Data["artifact_errors"] = issues
	}
	return event
}

func (e *artifactEnricher) enrichProcess(ctx context.Context, process *model.Process) (*model.Process, error) {
	if e == nil || process == nil {
		return process, nil
	}
	path := strings.TrimSpace(process.Image)
	if path == "" || !strings.HasPrefix(path, "/") {
		return process, nil
	}

	cloned := *process
	artifact, err := e.snapshotWithErrorContext(ctx, path)
	if artifact != nil {
		cloned.Artifact = artifact
	}
	return &cloned, err
}

func (e *artifactEnricher) enrichFile(ctx context.Context, file *model.File) (*model.File, error) {
	if e == nil || file == nil {
		return file, nil
	}
	path := strings.TrimSpace(file.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return file, nil
	}

	cloned := *file
	artifact, err := e.snapshotWithErrorContext(ctx, path)
	if artifact != nil {
		cloned.Artifact = artifact
	}
	return &cloned, err
}

func (e *artifactEnricher) snapshot(path string) *model.Artifact {
	artifact, _ := e.snapshotWithError(path)
	return artifact
}

func (e *artifactEnricher) snapshotWithError(path string) (*model.Artifact, error) {
	return e.snapshotWithErrorContext(context.Background(), path)
}

func (e *artifactEnricher) snapshotWithErrorContext(ctx context.Context, path string) (*model.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	signature, ok := currentArtifactSignature(path)
	if !ok {
		return enrich.SnapshotArtifactContext(ctx, path, e.options)
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return model.CloneArtifact(cached.artifact), cached.err
	}

	artifact, err := enrich.SnapshotArtifactContext(ctx, path, e.options)
	if err != nil && artifact == nil {
		return nil, err
	}
	if artifact != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		e.mu.Lock()
		e.cache[cacheKey] = artifactCacheEntry{
			signature: signature,
			artifact:  model.CloneArtifact(artifact),
			lastUsed:  now,
			err:       err,
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
