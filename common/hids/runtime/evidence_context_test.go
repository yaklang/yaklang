//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
)

func TestFileEvidenceScanKeepsSlotUntilCanceledExecutorExits(t *testing.T) {
	t.Parallel()
	manager := &Manager{instance: &Instance{pipeline: &pipeline{}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, returned := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	releaseExecutor := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseExecutor)
	var calls atomic.Int32
	execute := func(ctx context.Context, _ model.Event, _ map[string]any) (map[string]any, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 25*time.Second || time.Until(deadline) > 30*time.Second {
			t.Errorf("manual scan lost its 30 second budget: deadline=%v ok=%v", deadline, ok)
		}
		close(started)
		<-release // Simulate a syscall which has not yet returned after cancellation.
		return map[string]any{"complete": true}, nil
	}
	go func() {
		_, err := manager.scanFileEvidenceWithExecutor(ctx, FileEvidenceScanInput{}, execute)
		returned <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not start")
	}
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled scan was reported complete: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled scan did not return")
	}
	// A spec reapply replaces the runtime, but must not replace the occupied slot.
	manager.instance = &Instance{pipeline: &pipeline{}}
	_, err := manager.scanFileEvidenceWithExecutor(context.Background(), FileEvidenceScanInput{}, execute)
	if !errors.Is(err, ErrFileEvidenceScanBusy) || calls.Load() != 1 {
		t.Fatalf("timeout spawned another executor: calls=%d err=%v", calls.Load(), err)
	}
	releaseExecutor()
	deadline := time.After(3 * time.Second)
	for !manager.fileScanMu.TryLock() {
		select {
		case <-deadline:
			t.Fatal("executor did not release scan slot")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	manager.fileScanMu.Unlock()
	result, err := manager.scanFileEvidenceWithExecutor(context.Background(), FileEvidenceScanInput{}, func(context.Context, model.Event, map[string]any) (map[string]any, error) {
		return map[string]any{"complete": true}, nil
	})
	if err != nil || result["complete"] != true {
		t.Fatalf("slot was not reusable after execution stopped: result=%v err=%v", result, err)
	}
}

func TestFileEvidenceScanDoesNotStartCanceledRequest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manager := &Manager{}
	_, err := manager.scanFileEvidenceWithExecutor(ctx, FileEvidenceScanInput{}, func(context.Context, model.Event, map[string]any) (map[string]any, error) {
		t.Error("canceled request started an executor")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected canceled result: %v", err)
	}
}

func TestProcessTreeSelectsChildrenBeforeArtifactIO(t *testing.T) {
	t.Parallel()
	for _, capture := range []bool{false, true} {
		t.Run(fmt.Sprintf("capture_%t", capture), func(t *testing.T) {
			artifacts := newArtifactEnricher(model.EvidencePolicy{CaptureFileHash: true}, shortTermContextConfig{})
			p := &pipeline{processes: newProcessTracker(), artifacts: artifacts, evidencePolicy: model.EvidencePolicy{CaptureProcessTree: capture}}
			root := t.TempDir()
			for index := 0; index < 40; index++ {
				path := filepath.Join(root, fmt.Sprintf("child-%02d", index))
				if err := os.WriteFile(path, []byte("child artifact"), 0600); err != nil {
					t.Fatal(err)
				}
				p.processes.byPID[100+index] = trackedProcess{process: model.Process{PID: 100 + index, ParentPID: 1, Image: path}}
			}
			result, err := p.executeEvidenceRequestContext(context.Background(), model.Event{Process: &model.Process{PID: 1}}, map[string]any{"kind": "process_tree", "target": "process"})
			if err != nil {
				t.Fatal(err)
			}
			tree := result["process_tree"].(map[string]any)
			wantCount := 0
			if capture {
				wantCount = evidenceProcessChildrenLimit
				children := tree["children"].([]map[string]any)
				if len(children) != wantCount || tree["children_truncated"] != true {
					t.Fatalf("children limit changed: %#v", tree)
				}
				for index, child := range children {
					if child["pid"] != 100+index || child["artifact"] == nil {
						t.Fatalf("deterministic selection or artifact was lost: %#v", child)
					}
				}
			} else if _, exists := tree["children"]; exists {
				t.Fatalf("disabled child capture still emitted children: %#v", tree)
			}
			if len(artifacts.cache) != wantCount {
				t.Fatalf("artifact I/O occurred for unselected children: cache=%d want=%d", len(artifacts.cache), wantCount)
			}
		})
	}
}

func TestEvidenceArtifactLimitsRemainErrorsAcrossCacheHits(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "over-budget")
	if err := os.WriteFile(path, []byte("large enough"), 0600); err != nil {
		t.Fatal(err)
	}
	artifacts := newArtifactEnricher(model.EvidencePolicy{CaptureFileHash: true}, shortTermContextConfig{})
	artifacts.options.MaxHashBytes = 1
	p := &pipeline{artifacts: artifacts}
	request := map[string]any{"kind": "file", "target": path}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := p.executeEvidenceRequestContext(context.Background(), model.Event{}, request)
		if !errors.Is(err, enrich.ErrArtifactHashLimit) || result["artifact"] == nil {
			t.Fatalf("incomplete artifact became a successful cache hit: result=%v err=%v", result, err)
		}
	}
	result, err := p.executeEvidenceRequestContext(context.Background(), model.Event{Process: &model.Process{PID: 1, Image: path}}, map[string]any{"kind": "process_tree", "target": "process"})
	if err == nil || result["process_tree"] == nil {
		t.Fatalf("process tree hid incomplete image enrichment: result=%v err=%v", result, err)
	}
	if _, err := p.scanDirectoryContext(context.Background(), filepath.Dir(path), boundedDirectoryScanOptions{MaxEntries: 3, MaxDepth: 1}); !errors.Is(err, enrich.ErrArtifactHashLimit) {
		t.Fatalf("directory scan hid incomplete file enrichment: %v", err)
	}
	alert := p.enrichAlertEvidence(model.Event{}, model.Alert{Detail: map[string]any{"evidence_requests": []map[string]any{request}}})
	issues, ok := alert.Detail["evidence_errors"].([]map[string]any)
	if !ok || len(issues) != 1 || issues[0]["partial_result"] == nil || alert.Detail["evidence_results"] != nil {
		t.Fatalf("partial evidence was not explicitly separated from completed evidence: %#v", alert.Detail)
	}
}

func TestArtifactEnricherCanceledContextDoesNotPoisonCacheOrMutateInput(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sample")
	if err := os.WriteFile(path, []byte("sample"), 0600); err != nil {
		t.Fatal(err)
	}
	artifacts := newArtifactEnricher(model.EvidencePolicy{CaptureFileHash: true}, shortTermContextConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := map[string]any{"original": true}
	event := model.Event{File: &model.File{Path: path}, Data: data}
	result := artifacts.ApplyContext(ctx, event)
	if result.Data["artifact_errors"] == nil || data["artifact_errors"] != nil || len(artifacts.cache) != 0 {
		t.Fatalf("cancellation was hidden or polluted input/cache: event=%#v cache=%v", result, artifacts.cache)
	}
	artifact, err := artifacts.snapshotWithErrorContext(context.Background(), path)
	if err != nil || artifact.Hashes == nil || len(artifacts.cache) != 1 {
		t.Fatalf("canceled snapshot poisoned subsequent full request: artifact=%#v err=%v", artifact, err)
	}
}

func TestProcessEvidenceRetainsErrorFromAutomaticPartialArtifact(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "over-budget")
	if err := os.WriteFile(path, []byte("large enough"), 0600); err != nil {
		t.Fatal(err)
	}
	artifacts := newArtifactEnricher(model.EvidencePolicy{CaptureFileHash: true}, shortTermContextConfig{})
	artifacts.options.MaxHashBytes = 1
	event := artifacts.Apply(model.Event{Process: &model.Process{PID: 1, Image: path}})
	if event.Process.Artifact == nil || event.Data["artifact_errors"] == nil {
		t.Fatalf("fixture did not retain a partial snapshot: %#v", event)
	}
	p := &pipeline{artifacts: artifacts}
	result, err := p.executeEvidenceRequestContext(context.Background(), event, map[string]any{"kind": "process_tree", "target": "process"})
	if err == nil || result["process_tree"] == nil {
		t.Fatalf("known partial image became completed process evidence: result=%v err=%v", result, err)
	}
	// An unrelated PID must not inherit this event's image failure.
	p.processes = newProcessTracker()
	p.processes.byPID[2] = trackedProcess{process: model.Process{PID: 2}}
	result, err = p.executeEvidenceRequestContext(context.Background(), event, map[string]any{"kind": "process_tree", "metadata": map[string]any{"pid": 2}})
	if err != nil || result["resolved_target"] != 2 {
		t.Fatalf("event image error leaked to another PID: result=%v err=%v", result, err)
	}
}

func TestFileEvidenceContextReachesTraversalAndPostFilter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := newPipeline(nil)
	if _, err := p.scanDirectoryContext(ctx, "/absent", boundedDirectoryScanOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("traversal ignored cancellation: %v", err)
	}
	if _, err := p.applyScanPostFiltersContext(ctx, map[string]any{"metadata": map[string]any{"scan_match": "true"}}, map[string]any{"path": "/sample"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("postfilter ignored cancellation: %v", err)
	}
}

func TestFileEvidencePostFilterStopsRunningExpressionAtDeadline(t *testing.T) {
	t.Parallel()
	p := newPipeline(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := p.applyScanPostFiltersContext(ctx, map[string]any{
			"metadata": map[string]any{"scan_match": "for { }; true"},
		}, map[string]any{"path": "/sample"})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("running scan expression ignored deadline: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("running scan expression did not stop at deadline")
	}
	result, err := p.applyScanPostFiltersContext(context.Background(), map[string]any{
		"metadata": map[string]any{"scan_match": "true"},
	}, map[string]any{"path": "/sample"})
	if err != nil || result["scan_match"].(map[string]any)["matched"] != true {
		t.Fatalf("sandbox was not reusable after cancellation: result=%v err=%v", result, err)
	}
}

func TestConcurrentEvidencePostFiltersKeepIndependentDeadlines(t *testing.T) {
	t.Parallel()
	p := newPipeline(nil)
	started, release, longDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	releaseLong := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseLong)
	go func() {
		_, err := p.applyScanPostFiltersContext(context.Background(), map[string]any{
			"metadata": map[string]any{"scan_match": "scan.block(); true"},
		}, map[string]any{"block": func() { close(started); <-release }})
		longDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("long filter did not enter its expression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	shortDone := make(chan error, 1)
	go func() {
		_, err := p.applyScanPostFiltersContext(ctx, map[string]any{
			"metadata": map[string]any{"scan_match": "for { }; true"},
		}, map[string]any{"path": "/sample"})
		shortDone <- err
	}()
	select {
	case err := <-shortDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("short filter ignored its deadline: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("short filter waited for the unrelated long filter")
	}
	releaseLong()
	select {
	case err := <-longDone:
		if err != nil {
			t.Fatalf("independent long filter failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long filter did not finish after release")
	}
}

func TestDirectoryEvidenceStopsAtEntryBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for index := 0; index < 40; index++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("dir-%02d", index)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	p := &pipeline{artifacts: newArtifactEnricher(model.EvidencePolicy{}, shortTermContextConfig{})}
	result, err := p.scanDirectoryContext(context.Background(), root, boundedDirectoryScanOptions{MaxEntries: 3, MaxDepth: 1})
	if err != nil || result.ScannedCount != 3 || !result.Truncated || len(p.artifacts.cache) != 4 {
		t.Fatalf("directory limit did not stop artifact traversal: result=%#v cache=%d err=%v", result, len(p.artifacts.cache), err)
	}
}
