package yakit

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestHTTPFlowObservabilityRegistryIsBoundedAndDatabaseScoped(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)

	databaseIdentity := HTTPFlowDatabaseIdentity("project-a.db")
	const projectGeneration = uint64(1)
	flow := &schema.HTTPFlow{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:        databaseIdentity,
			ProjectGeneration:       projectGeneration,
			RequestHijackAtUnixMs:   10,
			ResponseMirrorAtUnixMs:  20,
			FlowBuiltAtUnixMs:       30,
			PersistEnqueuedAtUnixMs: 40,
			PersistStartedAtUnixMs:  50,
			PersistedAtUnixMs:       60,
		},
	}
	flow.ID = 42
	RecordHTTPFlowPersisted(flow)

	timing, ok := GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, 42)
	require.True(t, ok)
	require.Equal(t, int64(50), timing.PersistStartedAtUnixMs)
	require.Equal(t, uint64(42), SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration).LatestPersistedID)
	_, ok = GetHTTPFlowPersistTiming(HTTPFlowDatabaseIdentity("project-b.db"), projectGeneration, 42)
	require.False(t, ok)

	detectedAt := time.UnixMilli(75)
	RecordHTTPFlowChangeDetected(databaseIdentity, projectGeneration, 41, 42, detectedAt)
	timing, ok = GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, 42)
	require.True(t, ok)
	require.Equal(t, detectedAt.UnixMilli(), timing.DatabaseChangeDetectedAtUnixMs)
	highWater := SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration)
	require.Equal(t, uint64(42), highWater.LatestDetectedID)
	require.Equal(t, detectedAt.UnixMilli(), highWater.LatestDetectedAtUnixMs)

	// An ID maps to a fixed slot. A newer ID in the same slot must overwrite
	// the old sample instead of growing process memory.
	newer := &schema.HTTPFlow{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:  databaseIdentity,
			ProjectGeneration: projectGeneration,
			PersistedAtUnixMs: 80,
		},
	}
	newer.ID = uint(42 + HTTPFlowTimingRegistrySize)
	RecordHTTPFlowPersisted(newer)
	_, ok = GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, 42)
	require.False(t, ok)
	_, ok = GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, uint64(newer.ID))
	require.True(t, ok)

	RecordHTTPFlowChangeDetected(databaseIdentity, projectGeneration, uint64(newer.ID), 0, time.UnixMilli(90))
	highWater = SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration)
	require.Zero(t, highWater.LatestPersistedID)
	require.Zero(t, highWater.LatestDetectedID)
}

func TestHTTPFlowObservabilityDoesNotCountPluginFlowInMITMHighWater(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)

	databaseIdentity := HTTPFlowDatabaseIdentity("project.db")
	const projectGeneration = uint64(1)
	mitmFlow := &schema.HTTPFlow{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:  databaseIdentity,
			ProjectGeneration: projectGeneration,
			PersistedAtUnixMs: 10,
		},
	}
	mitmFlow.ID = 7
	RecordHTTPFlowPersisted(mitmFlow)

	pluginFlow := &schema.HTTPFlow{
		SourceType: schema.HTTPFlow_SourceType_SCAN,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:  databaseIdentity,
			ProjectGeneration: projectGeneration,
			PersistedAtUnixMs: 20,
		},
	}
	pluginFlow.ID = 99
	RecordHTTPFlowPersisted(pluginFlow)

	require.Equal(t, uint64(7), SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration).LatestPersistedID)
	_, ok := GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, 99)
	require.True(t, ok, "the bounded registry may diagnose non-table V2 flows without inflating MITM backlog")
}

func TestInsertHTTPFlowRecordsPersistStagesWithoutChangingSchema(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	databaseIdentity := HTTPFlowDatabaseIdentity("insert-project.db")
	const projectGeneration = uint64(1)
	flow := &schema.HTTPFlow{
		Hash:       "observability-insert",
		SourceType: schema.HTTPFlow_SourceType_MITM,
		RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
			DatabaseIdentity:  databaseIdentity,
			ProjectGeneration: projectGeneration,
			FlowBuiltAtUnixMs: time.Now().Add(-time.Second).UnixMilli(),
		},
	}
	require.NoError(t, InsertHTTPFlow(db, flow))
	require.NotZero(t, flow.ID)
	require.Positive(t, flow.RuntimeTiming.PersistEnqueuedAtUnixMs)
	require.GreaterOrEqual(t, flow.RuntimeTiming.PersistStartedAtUnixMs, flow.RuntimeTiming.PersistEnqueuedAtUnixMs)
	require.GreaterOrEqual(t, flow.RuntimeTiming.PersistedAtUnixMs, flow.RuntimeTiming.PersistStartedAtUnixMs)

	timing, ok := GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, uint64(flow.ID))
	require.True(t, ok)
	require.Equal(t, flow.RuntimeTiming.PersistedAtUnixMs, timing.PersistedAtUnixMs)
}

func TestHTTPFlowObservabilityConcurrentRecordAndRead(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)

	databaseIdentity := HTTPFlowDatabaseIdentity("concurrent-project.db")
	const projectGeneration = uint64(1)
	const (
		workers    = 4
		iterations = 250
	)
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for iteration := 1; iteration <= iterations; iteration++ {
				id := uint(worker*iterations + iteration)
				flow := &schema.HTTPFlow{
					SourceType: schema.HTTPFlow_SourceType_MITM,
					RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
						DatabaseIdentity:  databaseIdentity,
						ProjectGeneration: projectGeneration,
						PersistedAtUnixMs: int64(id),
					},
				}
				flow.ID = id
				RecordHTTPFlowPersisted(flow)
				RecordHTTPFlowChangeDetected(databaseIdentity, projectGeneration, uint64(id-1), uint64(id), time.UnixMilli(int64(id+1)))
				_, _ = GetHTTPFlowPersistTiming(databaseIdentity, projectGeneration, uint64(id))
				_ = SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration)
			}
		}()
	}
	waitGroup.Wait()

	highWater := SnapshotHTTPFlowPipelineHighWater(databaseIdentity, projectGeneration)
	require.Equal(t, uint64(workers*iterations), highWater.LatestPersistedID)
	require.Equal(t, uint64(workers*iterations), highWater.LatestDetectedID)
}

func TestHTTPFlowObservabilitySeparatesReopenedDatabaseGeneration(t *testing.T) {
	resetHTTPFlowObservabilityForTest()
	t.Cleanup(resetHTTPFlowObservabilityForTest)

	databaseIdentity := HTTPFlowDatabaseIdentity("reopened-project.db")
	for generation, id := range map[uint64]uint{1: 90, 2: 3} {
		flow := &schema.HTTPFlow{
			SourceType: schema.HTTPFlow_SourceType_MITM,
			RuntimeTiming: &schema.HTTPFlowRuntimeTiming{
				DatabaseIdentity:  databaseIdentity,
				ProjectGeneration: generation,
				PersistedAtUnixMs: int64(id),
			},
		}
		flow.ID = id
		RecordHTTPFlowPersisted(flow)
	}

	require.Equal(t, uint64(90), SnapshotHTTPFlowPipelineHighWater(databaseIdentity, 1).LatestPersistedID)
	require.Equal(t, uint64(3), SnapshotHTTPFlowPipelineHighWater(databaseIdentity, 2).LatestPersistedID)
	_, oldFound := GetHTTPFlowPersistTiming(databaseIdentity, 1, 3)
	_, currentFound := GetHTTPFlowPersistTiming(databaseIdentity, 2, 3)
	require.False(t, oldFound)
	require.True(t, currentFound)
}
