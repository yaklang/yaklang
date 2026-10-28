package aicommon

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TrajectoryEvaluator receives persisted revisions on a background worker. It
// must enqueue model work without retaining a live session or its UI consumer.
type TrajectoryEvaluator func(*gorm.DB, bool, *schema.AITrajectory)

// TrajectoryResponseReceipt registers a nonblocking notification for the complete
// response. An unfinished provider must not occupy a database writer or expire
// an otherwise successful compression's receipt.
type TrajectoryResponseReceipt func(receive func(string))

var trajectoryEvaluatorRegistry struct {
	sync.RWMutex
	evaluator TrajectoryEvaluator
}

func RegisterTrajectoryEvaluator(evaluator TrajectoryEvaluator) {
	trajectoryEvaluatorRegistry.Lock()
	defer trajectoryEvaluatorRegistry.Unlock()
	trajectoryEvaluatorRegistry.evaluator = evaluator
}

// RecoverTrajectoryCollection is only used as a defer at collection boundaries.
// Recovery itself does no logging, IO or user callbacks on the foreground path.
func RecoverTrajectoryCollection() {
	if recover() != nil {
		trajectoryCollectionDropped.Add(1)
	}
}

var trajectoryCollectionDropped atomic.Uint64

type trajectoryCompressionMetadata struct {
	ThroughID     int64  `json:"through_id"`
	RetainedRange string `json:"retained_range,omitempty"`
	InputTokens   int    `json:"input_tokens"`
	SummaryTokens int    `json:"summary_tokens"`
}

type trajectoryWrite struct {
	db          *gorm.DB
	singleModel bool
	record      schema.AITrajectory
	compression *trajectoryCompressionMetadata
	resume      bool
}

type trajectoryWritePool struct {
	queue   chan trajectoryWrite
	pending atomic.Int64
}

// Fixed workers and a bounded queue keep collection independent of slow or
// unavailable databases. Queue saturation discards telemetry, never user work.
var trajectoryWrites = &trajectoryWritePool{queue: make(chan trajectoryWrite, 64)}

func init() {
	for range 2 {
		go trajectoryWrites.worker()
	}
}

func (p *trajectoryWritePool) enqueue(job trajectoryWrite) bool {
	p.pending.Add(1)
	select {
	case p.queue <- job:
		return true
	default:
		p.pending.Add(-1)
		trajectoryCollectionDropped.Add(1)
		return false
	}
}

func (p *trajectoryWritePool) worker() {
	for job := range p.queue {
		func() {
			defer p.pending.Add(-1)
			defer RecoverTrajectoryCollection()
			if job.resume {
				records, err := yakit.ResumeAITrajectoryEvaluations(job.db, job.record.PersistentSessionID)
				if err != nil {
					log.Warnf("resume trajectory evaluations: %v", err)
					return
				}
				for _, record := range records {
					submitTrajectoryEvaluation(job.db, job.singleModel, record)
				}
				return
			}
			if job.compression != nil {
				raw, err := json.Marshal(job.compression)
				if err != nil {
					log.Warnf("serialize compression trajectory: %v", err)
					return
				}
				job.record.TrajectoryKey = uuid.NewString()
				job.record.CompressionJSON = string(raw)
				if job.record.Response == "" {
					return // An incomplete response is not a successful compression receipt.
				}
			}
			changed, err := yakit.SaveAITrajectory(job.db, &job.record)
			if err != nil {
				log.Warnf("save session trajectory: %v", err)
				return
			}
			if changed {
				submitTrajectoryEvaluation(job.db, job.singleModel, &job.record)
			}
		}()
	}
}

func submitTrajectoryEvaluation(db *gorm.DB, singleModel bool, record *schema.AITrajectory) {
	defer RecoverTrajectoryCollection()
	trajectoryEvaluatorRegistry.RLock()
	evaluator := trajectoryEvaluatorRegistry.evaluator
	trajectoryEvaluatorRegistry.RUnlock()
	if evaluator != nil && record != nil {
		evaluator(db, singleModel, record)
	}
}

func (c *Config) trajectoryPersistenceEnabled() bool {
	if c == nil || c.DisableCreateDBRuntime || c.BaseCheckpointableStorage == nil || c.GetDB() == nil || c.PersistentSessionId == "" || c.Timeline == nil {
		return false
	}
	// The branch identity is an atomic snapshot: unrelated timeline writes must
	// neither block nor cause the only terminal receipt to be discarded.
	return !c.Timeline.branchTimeline.Load()
}

func (c *Config) newTrajectory(kind string) schema.AITrajectory {
	return schema.AITrajectory{PersistentSessionID: c.PersistentSessionId, Kind: kind,
		RuntimeID: c.GetRuntimeId(), CapturedAt: time.Now()}
}

// Session startup only enqueues recovery. No trajectory database operation is
// allowed to delay opening or restoring the user's session.
func (c *Config) ResumeTrajectoryEvaluations() {
	defer RecoverTrajectoryCollection()
	if c.trajectoryPersistenceEnabled() {
		trajectoryWrites.enqueue(trajectoryWrite{db: c.GetDB(), singleModel: c.singleAIModelMode,
			record: c.newTrajectory(schema.AITrajectoryTerminal), resume: true})
	}
}

// RecordTaskTrajectoryPrompt retains only the last constructed prompt on the
// owning task. Nested loops can change task.GetReActLoop(), so that reference
// cannot identify the latest request. This path does no serialization or IO.
func RecordTaskTrajectoryPrompt(task AIStatefulTask, prompt string) {
	defer RecoverTrajectoryCollection()
	if target, ok := task.(interface{ SetTrajectoryPrompt(string) }); ok {
		target.SetTrajectoryPrompt(prompt)
	}
}

func (s *AIStatefulTaskBase) SetTrajectoryPrompt(prompt string) {
	if s != nil {
		s.trajectoryPrompt.Store(&prompt)
	}
}

func (s *AIStatefulTaskBase) GetTrajectoryPrompt() string {
	if s != nil {
		if prompt := s.trajectoryPrompt.Load(); prompt != nil {
			return *prompt
		}
	}
	return ""
}

// CollectTerminalTrajectory records only the last constructed decision prompt
// on finish/cancel. No emitted result, task result or Timeline dump is involved.
func (c *Config) CollectTerminalTrajectory(task AIStatefulTask) bool {
	defer RecoverTrajectoryCollection()
	if task == nil || task.IsSubAgent() || !c.trajectoryPersistenceEnabled() {
		return false
	}
	state, ok := task.(interface{ TryGetStatus() (AITaskState, bool) })
	if !ok {
		return false
	}
	status, available := state.TryGetStatus()
	if !available || !isFinishedTaskStatus(status) {
		return false
	}
	source, ok := task.(interface{ GetTrajectoryPrompt() string })
	if !ok {
		return false
	}
	prompt := source.GetTrajectoryPrompt()
	if prompt == "" {
		return false
	}
	record := c.newTrajectory(schema.AITrajectoryTerminal)
	record.TaskID, record.UserQuery, record.Status = task.GetId(), task.GetOriginUserInput(), string(status)
	record.Prompt = prompt
	return trajectoryWrites.enqueue(trajectoryWrite{db: c.GetDB(), singleModel: c.singleAIModelMode, record: record})
}

// Capture only immutable strings and fixed-size metadata while the compression
// state is locked. The notification enqueues after a successful commit; even
// capture, queue, database or evaluator failures cannot reject compression.
func (c *Config) compressionTrajectoryNotification(prompt string, result *TimelineCompressionResult, response TrajectoryResponseReceipt) (notify func()) {
	defer RecoverTrajectoryCollection()
	if c == nil || c.DisableCreateDBRuntime || c.BaseCheckpointableStorage == nil || c.GetDB() == nil || c.PersistentSessionId == "" || prompt == "" || response == nil {
		return nil
	}
	record := c.newTrajectory(schema.AITrajectoryCompression)
	record.Prompt, record.Status = prompt, "committed"
	metadata := &trajectoryCompressionMetadata{ThroughID: result.ThroughID, RetainedRange: result.RetainedRange,
		InputTokens: result.InputTokens, SummaryTokens: result.SummaryTokens}
	job := trajectoryWrite{db: c.GetDB(), singleModel: c.singleAIModelMode, record: record, compression: metadata}
	var received atomic.Bool
	return func() {
		defer RecoverTrajectoryCollection()
		response(func(fullResponse string) {
			defer RecoverTrajectoryCollection()
			if fullResponse == "" || !received.CompareAndSwap(false, true) {
				return
			}
			ready := job
			ready.record.Response = fullResponse
			trajectoryWrites.enqueue(ready)
		})
	}
}
