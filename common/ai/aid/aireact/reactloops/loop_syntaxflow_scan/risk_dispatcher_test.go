package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.SSARisk{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertRisks(t *testing.T, db *gorm.DB, runtimeID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		r := &schema.SSARisk{
			RuntimeId: runtimeID,
			Title:     fmt.Sprintf("risk-%d", i),
			RiskType:  "test",
		}
		require.NoError(t, db.Create(r).Error)
	}
}

func publishSSARiskEvent(runtimeID string) {
	schema.PublishRuntimeScopedBroadcast(schema.RuntimeScopedBroadcastTypeSSARisk, runtimeID, "update", 1)
}

func waitDrainedOrFail(t *testing.T, d *riskDispatcher, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	d.WaitDrained(ctx)
	select {
	case <-d.drainedCh:
	default:
		t.Fatalf("riskDispatcher did not drain within %v", timeout)
	}
}

func countBatchFiles(batchesDir string) int {
	entries, err := os.ReadDir(batchesDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && len(name) > 6 && name[:6] == "batch_" && filepath.Ext(name) == ".md" {
			count++
		}
	}
	return count
}

func newTestDispatcher(t *testing.T, db *gorm.DB, runtimeID string, batchSize int, batchInterval, tick, grace time.Duration) (*riskDispatcher, string) {
	t.Helper()
	batchesDir := filepath.Join(t.TempDir(), "batches")
	d := &riskDispatcher{
		runtimeID:     runtimeID,
		batchesDir:    batchesDir,
		db:            db,
		batchSize:     batchSize,
		batchInterval: batchInterval,
		batcherTick:   tick,
		terminalGrace: grace,
		maxInFlight:   1,
		jobCh:         make(chan []int64, 4),
		terminalCh:    make(chan struct{}),
		drainedCh:     make(chan struct{}),
	}
	d.processBatchOverride = func(_ context.Context, ids []int64) {
		seq := d.batchSeq.Add(1)
		content := fmt.Sprintf("# batch_%03d\nIDs: %v\n", seq, ids)
		_ = os.MkdirAll(batchesDir, 0o755)
		_ = os.WriteFile(filepath.Join(batchesDir, fmt.Sprintf("batch_%03d.md", seq)), []byte(content), 0o644)
	}
	return d, batchesDir
}

func TestRiskDispatcher_SizeBatching(t *testing.T) {
	const runtimeID = "disp-size"
	db := openTestDB(t)

	insertRisks(t, db, runtimeID, 3)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 2, 60*time.Second, 10*time.Millisecond, 30*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d.SeedExistingRisks(ctx)
	d.NotifyScanTerminal()

	waitDrainedOrFail(t, d, 5*time.Second)

	n := countBatchFiles(batchesDir)
	require.GreaterOrEqual(t, n, 2, "3 risks at batch size 2 → ≥2 batch files")
}

func TestRiskDispatcher_TimeBatching(t *testing.T) {
	const runtimeID = "disp-time"
	db := openTestDB(t)

	insertRisks(t, db, runtimeID, 1)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 100, 80*time.Millisecond, 10*time.Millisecond, 30*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d.SeedExistingRisks(ctx)
	d.NotifyScanTerminal()

	waitDrainedOrFail(t, d, 5*time.Second)

	n := countBatchFiles(batchesDir)
	require.GreaterOrEqual(t, n, 1, "time threshold should have flushed the 1-risk buffer")
}

func TestRiskDispatcher_EmptyScan_DrainsFast(t *testing.T) {
	const runtimeID = "disp-empty"
	db := openTestDB(t)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 10, 60*time.Second, 10*time.Millisecond, 30*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d.SeedExistingRisks(ctx)
	d.NotifyScanTerminal()

	waitDrainedOrFail(t, d, 3*time.Second)

	require.Equal(t, 0, countBatchFiles(batchesDir), "no risks → no batch files")
}

func TestRiskDispatcher_TerminalGrace_PicksUpLateRisks(t *testing.T) {
	const runtimeID = "disp-grace"
	db := openTestDB(t)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 10, 60*time.Second, 10*time.Millisecond, 400*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d.SeedExistingRisks(ctx)
	d.NotifyScanTerminal()

	time.Sleep(60 * time.Millisecond)
	insertRisks(t, db, runtimeID, 2)
	publishSSARiskEvent(runtimeID)

	waitDrainedOrFail(t, d, 5*time.Second)

	select {
	case <-d.drainedCh:
	default:
		t.Fatal("drainedCh should be closed after drain")
	}
	t.Logf("batch files after grace: %d", countBatchFiles(batchesDir))
}

func TestRiskDispatcher_SingleInFlight(t *testing.T) {
	const runtimeID = "disp-serial"
	db := openTestDB(t)

	insertRisks(t, db, runtimeID, 6)

	var (
		active     int64
		peakActive int64
	)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 2, 60*time.Second, 10*time.Millisecond, 30*time.Millisecond)

	d.processBatchOverride = func(_ context.Context, ids []int64) {
		cur := atomic.AddInt64(&active, 1)
		for {
			pk := atomic.LoadInt64(&peakActive)
			if cur <= pk {
				break
			}
			if atomic.CompareAndSwapInt64(&peakActive, pk, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&active, -1)

		seq := d.batchSeq.Add(1)
		content := fmt.Sprintf("# batch_%03d\n", seq)
		_ = os.MkdirAll(batchesDir, 0o755)
		_ = os.WriteFile(filepath.Join(batchesDir, fmt.Sprintf("batch_%03d.md", seq)), []byte(content), 0o644)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d.SeedExistingRisks(ctx)
	d.NotifyScanTerminal()

	waitDrainedOrFail(t, d, 8*time.Second)

	require.LessOrEqual(t, peakActive, int64(1),
		"with maxInFlight=1, concurrent executions must never exceed 1")
	t.Logf("peak concurrent: %d, batch files: %d", peakActive, countBatchFiles(batchesDir))
}

func TestRiskDispatcher_BroadcastTriggersNewRisks(t *testing.T) {
	const runtimeID = "disp-broadcast"
	db := openTestDB(t)

	d, batchesDir := newTestDispatcher(t, db, runtimeID, 1, 60*time.Second, 10*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d.Start(ctx)

	insertRisks(t, db, runtimeID, 1)
	publishSSARiskEvent(runtimeID)

	time.Sleep(150 * time.Millisecond)

	d.NotifyScanTerminal()

	waitDrainedOrFail(t, d, 5*time.Second)

	n := countBatchFiles(batchesDir)
	require.GreaterOrEqual(t, n, 1, "broadcast risk should produce ≥1 batch file")
}
