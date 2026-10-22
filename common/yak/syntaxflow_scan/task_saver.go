package syntaxflow_scan

import (
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const (
	// taskSnapshotBatchSize is the multi-row upsert size. A chunk must not
	// repeat task_id: SQLite would keep the later row, PostgreSQL rejects it.
	taskSnapshotBatchSize = 64
	// taskSnapshotFlushEvery coalesces per-rule progress into one write. Stage
	// end, pause, error, and completion call Save and do not wait for this.
	taskSnapshotFlushEvery = 32
)

// taskSnapshotSaver keeps the newest SyntaxFlowScanTask per task_id and writes
// those rows with one SaveInBatches upsert. It belongs to the process, while a
// test can supply its own database; rule execution never shares its frame.
type taskSnapshotSaver struct {
	db         func() *gorm.DB
	flushEvery time.Duration

	mu         sync.Mutex
	pending    map[string]*schema.SyntaxFlowScanTask
	sinceFlush int
	err        error
	stopped    bool

	wake      chan struct{}
	flushCh   chan chan error
	closeCh   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

var defaultTaskSnapshotSaver = newTaskSnapshotSaver(ssadb.GetDB, time.Second)

func newTaskSnapshotSaver(db func() *gorm.DB, flushEvery time.Duration) *taskSnapshotSaver {
	return &taskSnapshotSaver{
		db:         db,
		flushEvery: flushEvery,
		pending:    make(map[string]*schema.SyntaxFlowScanTask),
		wake:       make(chan struct{}, 1),
		flushCh:    make(chan chan error),
		closeCh:    make(chan struct{}),
	}
}

func cloneSyntaxFlowScanTask(task *schema.SyntaxFlowScanTask) *schema.SyntaxFlowScanTask {
	if task == nil {
		return nil
	}
	cloned := *task
	if task.Config != nil {
		cloned.Config = append([]byte(nil), task.Config...)
	}
	return &cloned
}

func (s *taskSnapshotSaver) start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.worker()
	})
}

func (s *taskSnapshotSaver) stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		close(s.closeCh)
	})
	s.start()
	s.wg.Wait()
}

// Enqueue keeps only the latest snapshot for the task. It does not wait for
// the database, so a rule finishing cannot queue behind a commit.
func (s *taskSnapshotSaver) Enqueue(task *schema.SyntaxFlowScanTask) {
	if s == nil || task == nil || task.TaskId == "" {
		return
	}
	s.start()
	cloned := cloneSyntaxFlowScanTask(task)
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.pending[cloned.TaskId] = cloned
	s.sinceFlush++
	wake := s.sinceFlush >= taskSnapshotFlushEvery
	s.mu.Unlock()
	if wake {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// Save merges this snapshot and waits until this saver's pending rows are
// written. The caller can read the task back before publishing a terminal state.
func (s *taskSnapshotSaver) Save(task *schema.SyntaxFlowScanTask) error {
	if s == nil || task == nil || task.TaskId == "" {
		return nil
	}
	s.Enqueue(task)
	return s.Flush()
}

func (s *taskSnapshotSaver) Flush() error {
	if s == nil {
		return nil
	}
	s.start()
	done := make(chan error, 1)
	select {
	case s.flushCh <- done:
		return <-done
	case <-s.closeCh:
		return s.drain()
	}
}

func (s *taskSnapshotSaver) worker() {
	defer s.wg.Done()
	var tick <-chan time.Time
	if s.flushEvery > 0 {
		ticker := time.NewTicker(s.flushEvery)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case <-s.closeCh:
			_ = s.drain()
			return
		case done := <-s.flushCh:
			done <- s.drain()
		case <-s.wake:
			_ = s.drain()
		case <-tick:
			_ = s.drain()
		}
	}
}

func (s *taskSnapshotSaver) drain() error {
	s.mu.Lock()
	batch := s.pending
	s.pending = make(map[string]*schema.SyntaxFlowScanTask, len(batch))
	s.sinceFlush = 0
	s.mu.Unlock()
	if len(batch) == 0 {
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		return err
	}
	rows := make([]*schema.SyntaxFlowScanTask, 0, len(batch))
	for _, row := range batch {
		rows = append(rows, row)
	}
	err := saveSyntaxFlowScanTaskBatch(s.db(), rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		for id, row := range batch {
			if _, exists := s.pending[id]; !exists {
				s.pending[id] = row
			}
		}
		s.err = utils.JoinErrors(s.err, err)
		return s.err
	}
	s.err = nil
	return nil
}

func saveSyntaxFlowScanTaskBatch(db *gorm.DB, tasks []*schema.SyntaxFlowScanTask) error {
	rows := dedupeSyntaxFlowScanTasks(tasks)
	if len(rows) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save syntaxflow scan tasks: database is nil")
	}
	return db.SaveInBatches(rows, taskSnapshotBatchSize, "task_id").Error
}

func dedupeSyntaxFlowScanTasks(tasks []*schema.SyntaxFlowScanTask) []*schema.SyntaxFlowScanTask {
	latest := make(map[string]*schema.SyntaxFlowScanTask, len(tasks))
	order := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.TaskId == "" {
			continue
		}
		if _, ok := latest[task.TaskId]; !ok {
			order = append(order, task.TaskId)
		}
		latest[task.TaskId] = task
	}
	rows := make([]*schema.SyntaxFlowScanTask, 0, len(order))
	for _, id := range order {
		rows = append(rows, latest[id])
	}
	return rows
}
