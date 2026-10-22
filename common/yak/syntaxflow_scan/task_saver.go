package syntaxflow_scan

import (
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const (
	// taskSnapshotBatchSize is the SQL upsert size. A chunk must not repeat
	// task_id: SQLite would keep the later row, PostgreSQL rejects it.
	// dbcache.WithSaveSize cannot go below its own floor (900); saveBatch still
	// hands SaveInBatches this smaller size after keeping the latest row.
	taskSnapshotBatchSize = 64
)

// taskSnapshotSaver keeps the newest SyntaxFlowScanTask per task_id. There is
// one writer: dbcache.Save has a single consumer goroutine, and WithSaveParallelism
// stays at its default of 1, so batches never write concurrently. The mutex only
// stops Enqueue from calling Save while Flush runs; dbcache forbids that pair.
type taskSnapshotSaver struct {
	db func() *gorm.DB

	mu     sync.Mutex
	writer *dbcache.Save[*schema.SyntaxFlowScanTask]

	stopped  bool
	stopOnce sync.Once
}

var defaultTaskSnapshotSaver = newTaskSnapshotSaver(ssadb.GetDB, time.Second)

func newTaskSnapshotSaver(db func() *gorm.DB, flushEvery time.Duration) *taskSnapshotSaver {
	s := &taskSnapshotSaver{db: db}
	// flushEvery <= 0 disables the timer. dbcache treats a zero timeout as
	// "fire immediately", so tests that want an explicit Flush use a long one.
	timeout := flushEvery
	if timeout <= 0 {
		timeout = 24 * time.Hour
	}
	s.writer = dbcache.NewSave(s.saveBatch,
		dbcache.WithName("syntaxflow-scan-task"),
		dbcache.WithSaveTimeout(timeout),
		dbcache.WithSaveSize(taskSnapshotBatchSize),
	)
	return s
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

func (s *taskSnapshotSaver) stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopped = true
		writer := s.writer
		s.mu.Unlock()
		if writer != nil {
			_ = writer.Close()
		}
	})
}

// Enqueue queues a snapshot without waiting for the database. It blocks only
// while Flush holds the barrier.
func (s *taskSnapshotSaver) Enqueue(task *schema.SyntaxFlowScanTask) {
	if s == nil || task == nil || task.TaskId == "" {
		return
	}
	cloned := cloneSyntaxFlowScanTask(task)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.writer == nil {
		return
	}
	s.writer.Save(cloned)
}

// Save queues this snapshot and waits until the single writer has persisted
// every row queued so far. The caller can read the task back before publishing
// a terminal state.
func (s *taskSnapshotSaver) Save(task *schema.SyntaxFlowScanTask) error {
	if s == nil || task == nil || task.TaskId == "" {
		return nil
	}
	cloned := cloneSyntaxFlowScanTask(task)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.writer == nil {
		return nil
	}
	s.writer.Save(cloned)
	return s.writer.Flush()
}

func (s *taskSnapshotSaver) Flush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.writer == nil {
		return nil
	}
	return s.writer.Flush()
}

func (s *taskSnapshotSaver) saveBatch(tasks []*schema.SyntaxFlowScanTask) error {
	return saveSyntaxFlowScanTaskBatch(s.db(), tasks)
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
