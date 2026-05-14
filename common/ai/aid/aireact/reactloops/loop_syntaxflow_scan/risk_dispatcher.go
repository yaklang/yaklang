package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	riskBatchSize     = 80
	riskBatchInterval = 1 * time.Minute
	batcherTickPeriod = 2 * time.Minute
	terminalGrace     = 30 * time.Second
)

func readMaxInFlight() int {
	v := strings.TrimSpace(os.Getenv("YAK_SF_SCAN_OVERVIEW_PARALLEL"))
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

type riskDispatcher struct {
	r          aicommon.AIInvokeRuntime
	loop       *reactloops.ReActLoop
	task       aicommon.AIStatefulTask
	db         *gorm.DB
	runtimeID  string
	batchesDir string

	batchSize     int
	batchInterval time.Duration
	batcherTick   time.Duration
	terminalGrace time.Duration
	maxInFlight   int

	processBatchOverride func(ctx context.Context, ids []int64)

	mu              sync.Mutex
	buf             []int64
	lastSeenID      int64
	firstEnqueuedAt time.Time
	hasEnqueued     bool

	scanPending atomic.Int32

	jobCh chan []int64
	wg    sync.WaitGroup

	terminalOnce sync.Once
	terminalCh   chan struct{}

	drainedCh   chan struct{}
	drainedOnce sync.Once

	batchSeq atomic.Int64
}

func newRiskDispatcher(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	db *gorm.DB,
	runtimeID string,
) *riskDispatcher {
	mif := readMaxInFlight()
	var batchesDir string
	if loop != nil {
		batchesDir = filepath.Join(loop.GetLoopContentDir("syntaxflow_scan"), "batches")
	}
	return &riskDispatcher{
		r:             r,
		loop:          loop,
		task:          task,
		db:            db,
		runtimeID:     runtimeID,
		batchesDir:    batchesDir,
		batchSize:     riskBatchSize,
		batchInterval: riskBatchInterval,
		batcherTick:   batcherTickPeriod,
		terminalGrace: terminalGrace,
		maxInFlight:   mif,
		jobCh:         make(chan []int64, mif*4),
		terminalCh:    make(chan struct{}),
		drainedCh:     make(chan struct{}),
	}
}

func (d *riskDispatcher) Start(ctx context.Context) {
	if d == nil {
		return
	}
	cancel := d.subscribeSSARiskWakeup()

	d.sweepNewRisks()

	for i := 0; i < d.maxInFlight; i++ {
		go d.workerLoop(ctx)
	}

	go d.batcherLoop(ctx, cancel)
}

func (d *riskDispatcher) SeedExistingRisks(ctx context.Context) {
	if d == nil {
		return
	}
	cancel := d.subscribeSSARiskWakeup()

	d.sweepNewRisks()

	for i := 0; i < d.maxInFlight; i++ {
		go d.workerLoop(ctx)
	}

	go d.batcherLoop(ctx, cancel)
}

func (d *riskDispatcher) subscribeSSARiskWakeup() func() {
	return schema.SubscribeRuntimeScopedBroadcast(d.runtimeID, func(event *schema.RuntimeScopedBroadcastEvent) {
		if event == nil || event.Type != schema.RuntimeScopedBroadcastTypeSSARisk {
			return
		}
		if d.scanPending.CompareAndSwap(0, 1) {
			go func() {
				defer d.scanPending.Store(0)
				d.sweepNewRisks()
			}()
		}
	})
}

func (d *riskDispatcher) NotifyScanTerminal() {
	if d == nil {
		return
	}
	d.terminalOnce.Do(func() {
		close(d.terminalCh)
	})
}

func (d *riskDispatcher) WaitDrained(ctx context.Context) {
	if d == nil {
		return
	}
	select {
	case <-d.drainedCh:
	case <-ctx.Done():
	}
}

func (d *riskDispatcher) sweepNewRisks() {
	db := d.db
	if db == nil {
		db = sfu.GetSSADB()
	}
	if db == nil {
		return
	}

	d.mu.Lock()
	afterID := d.lastSeenID
	d.mu.Unlock()

	var ids []int64
	err := db.Model(&schema.SSARisk{}).
		Where("runtime_id = ? AND id > ?", d.runtimeID, afterID).
		Order("id asc").
		Pluck("id", &ids).Error
	if err != nil {
		log.Debugf("[risk_dispatcher] sweepNewRisks: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	d.mu.Lock()
	d.buf = append(d.buf, ids...)
	d.lastSeenID = ids[len(ids)-1]
	if !d.hasEnqueued {
		d.hasEnqueued = true
		d.firstEnqueuedAt = time.Now()
	}
	d.mu.Unlock()
}

func (d *riskDispatcher) batcherLoop(ctx context.Context, cancelSubscribe func()) {
	defer func() {
		if cancelSubscribe != nil {
			cancelSubscribe()
		}
	}()

	ticker := time.NewTicker(d.batcherTick)
	defer ticker.Stop()

	var terminalAt time.Time
	terminalSeen := false

	for {
		select {
		case <-ctx.Done():
			d.flushAll()
			close(d.jobCh)
			d.wg.Wait()
			d.signalDrained()
			return

		case <-d.terminalCh:
			if !terminalSeen {
				terminalSeen = true
				terminalAt = time.Now()
			}

		case <-ticker.C:
			d.tryFlush()

			if terminalSeen && time.Since(terminalAt) >= d.terminalGrace {
				d.sweepNewRisks()
				d.flushAll()
				close(d.jobCh)
				d.wg.Wait()
				d.signalDrained()
				return
			}
		}
	}
}

func (d *riskDispatcher) tryFlush() {
	d.mu.Lock()
	if len(d.buf) == 0 {
		d.mu.Unlock()
		return
	}
	sizeMet := len(d.buf) >= d.batchSize
	timeMet := d.hasEnqueued && time.Since(d.firstEnqueuedAt) >= d.batchInterval
	if !sizeMet && !timeMet {
		d.mu.Unlock()
		return
	}
	batch := d.buf[:d.batchSize]
	if len(d.buf) < d.batchSize {
		batch = d.buf
	}
	d.buf = d.buf[len(batch):]
	if len(d.buf) > 0 {
		d.firstEnqueuedAt = time.Now()
	} else {
		d.hasEnqueued = false
	}
	d.mu.Unlock()

	d.dispatch(batch)
}

func (d *riskDispatcher) flushAll() {
	for {
		d.mu.Lock()
		if len(d.buf) == 0 {
			d.mu.Unlock()
			return
		}
		size := d.batchSize
		if len(d.buf) < size {
			size = len(d.buf)
		}
		batch := make([]int64, size)
		copy(batch, d.buf[:size])
		d.buf = d.buf[size:]
		d.mu.Unlock()

		d.dispatch(batch)
	}
}

func (d *riskDispatcher) dispatch(ids []int64) {
	if len(ids) == 0 {
		return
	}
	cp := make([]int64, len(ids))
	copy(cp, ids)
	d.wg.Add(1)
	select {
	case d.jobCh <- cp:
	default:
		d.wg.Done()
		log.Debugf("[risk_dispatcher] jobCh full or closed, dropping batch of %d", len(ids))
	}
}

func (d *riskDispatcher) workerLoop(ctx context.Context) {
	for ids := range d.jobCh {
		if d.processBatchOverride != nil {
			d.processBatchOverride(ctx, ids)
		} else {
			d.processOneBatch(ctx, ids)
		}
		d.wg.Done()
	}
}

func (d *riskDispatcher) processOneBatch(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	seq := d.batchSeq.Add(1)
	batchLabel := fmt.Sprintf("batch_%03d", seq)

	AppendSFPipelineLine(d.loop, fmt.Sprintf("【dispatcher·第%d批】共 %d 条 id，启动 ssa_risk_overview 子环", seq, len(ids)))
	if d.r != nil {
		d.r.AddToTimeline("syntaxflow_scan",
			fmt.Sprintf("[risk_dispatcher] %s: %d ids, running ssa_risk_overview sub-loop", batchLabel, len(ids)))
	}

	filter := &ypb.SSARisksFilter{ID: ids}
	riskDB := sfu.GetSSADB()

	tempLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW,
		d.r,
		reactloops.WithMaxIterations(capOverviewSubLoopMaxIter(d.r)),
	)
	if err != nil {
		log.Warnf("[risk_dispatcher] %s: CreateLoopByName: %v", batchLabel, err)
		d.writeFallbackBatch(seq, ids, fmt.Sprintf("overview 子环创建失败: %v", err))
		return
	}
	tempLoop.Set(sfu.LoopVarSyntaxFlowTaskID, d.runtimeID)
	PersistEffectiveOverviewFilter(tempLoop, filter)

	sub := NewSyntaxflowSubTask(d.task, fmt.Sprintf("risk_dispatcher_%s", batchLabel))
	if sub == nil {
		log.Warnf("[risk_dispatcher] %s: nil subtask", batchLabel)
		d.writeFallbackBatch(seq, ids, "subtask 创建失败")
		return
	}

	if err := tempLoop.ExecuteWithExistedTask(sub); err != nil {
		log.Warnf("[risk_dispatcher] %s: overview execute: %v", batchLabel, err)
	}
	_ = ApplySSARiskOverviewDB(tempLoop, d.r, riskDB, sub, filter, int64(len(ids)))

	preface := tempLoop.Get("ssa_risk_overview_preface")
	listSummary := tempLoop.Get("ssa_risk_list_summary")
	totalHint := tempLoop.Get("ssa_risk_total_hint")

	var md strings.Builder
	fmt.Fprintf(&md, "# SSA Risk Overview – %s\n\n", batchLabel)
	fmt.Fprintf(&md, "> task_id: `%s` | IDs in batch: %d | approx DB count: %s\n\n", d.runtimeID, len(ids), totalHint)
	fmt.Fprintf(&md, "## 本批 Risk ID 列表\n\n")
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}
	md.WriteString(strings.Join(idStrs, ", "))
	md.WriteString("\n\n## Overview 分析（ssa_risk_overview_preface）\n\n")
	md.WriteString(utils.ShrinkTextBlock(preface, 12000))
	md.WriteString("\n\n## 风险列表（ssa_risk_list_summary）\n\n")
	md.WriteString(utils.ShrinkTextBlock(listSummary, 12000))
	md.WriteString("\n")

	d.writeBatchFile(seq, batchLabel, md.String(), len(ids))
}

func (d *riskDispatcher) writeBatchFile(seq int64, batchLabel, content string, idCount int) {
	if err := os.MkdirAll(d.batchesDir, 0o755); err != nil {
		log.Warnf("[risk_dispatcher] mkdir %s: %v", d.batchesDir, err)
		return
	}
	path := filepath.Join(d.batchesDir, batchLabel+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Warnf("[risk_dispatcher] write %s: %v", path, err)
		return
	}

	AppendSFPipelineLine(d.loop, fmt.Sprintf("【dispatcher·第%d批】已写入: %s (%d ids)", seq, path, idCount))
	AppendSfScanInterpretLog(d.loop, d.r, d.runtimeID,
		fmt.Sprintf("risk_dispatcher: %s 已完成 (%d ids) → %s", batchLabel, idCount, path))

	parentID := ""
	if d.task != nil {
		parentID = d.task.GetId()
	}
	EmitSyntaxFlowUserStageMarkdown(d.loop, parentID,
		fmt.Sprintf("risk_batch_%03d", seq),
		fmt.Sprintf("# SSA Risk 分析·第 %d 批\n\n**本批 ID 数**: %d\n\n%s",
			seq, idCount, utils.ShrinkTextBlock(content, 8000)))
}

func (d *riskDispatcher) writeFallbackBatch(seq int64, ids []int64, reason string) {
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}
	content := fmt.Sprintf("# SSA Risk Overview – batch_%03d (降级)\n\n> 子环执行失败: %s\n\n本批 IDs: %s\n",
		seq, reason, strings.Join(idStrs, ", "))
	d.writeBatchFile(seq, fmt.Sprintf("batch_%03d", seq), content, len(ids))
}

func (d *riskDispatcher) signalDrained() {
	d.drainedOnce.Do(func() {
		if d.loop != nil {
			d.loop.Set(sfu.LoopVarSFRiskConverged, "1")
			AppendSFPipelineLine(d.loop, "【dispatcher·完成】所有批次已处理，sf_scan_risk_converged=1")
			if d.r != nil {
				d.r.AddToTimeline("syntaxflow_scan", "[risk_dispatcher] all batches drained, risk converged")
			}
		}
		close(d.drainedCh)
	})
}

func (d *riskDispatcher) BatchesDir() string {
	if d == nil {
		return ""
	}
	return d.batchesDir
}
