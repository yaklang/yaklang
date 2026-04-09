package scannode

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

const legionProgressTotalUnits = 10000
const legionProgressCheckpointInterval = time.Second
const legionProgressSignificantStepUnits = legionProgressTotalUnits / 20

type attemptProgressCheckpoint struct {
	mu                   sync.Mutex
	hasObserved          bool
	hasPublished         bool
	hasPublishedTerminal bool
	lastObservedProcess  float64
	lastObservedUnits    uint32
	lastObservedTotal    uint32
	lastCompletedUnits   uint32
	lastTotalUnits       uint32
	lastPublishedAt      time.Time
}

func (r *ScannerAgentReporter) publishJobProgress(process float64) error {
	r.touchActiveAttempt()
	publisher, ref, ok, err := r.legionPublisher()
	if err != nil || !ok {
		return err
	}
	return publisher.PublishProgress(
		r.agent.node.GetRootContext(),
		*ref,
		"yak_script",
		fmt.Sprintf("%.2f%%", process*100),
		progressUnits(process),
		legionProgressTotalUnits,
	)
}

func (r *ScannerAgentReporter) reportJobProgress(process float64) error {
	if r == nil {
		return nil
	}
	return r.progressCheckpoint.report(
		process,
		time.Now().UTC(),
		r.publishJobProgress,
	)
}

func (r *ScannerAgentReporter) flushLatestJobProgress() error {
	if r == nil {
		return nil
	}
	return r.progressCheckpoint.flushLatest(
		time.Now().UTC(),
		r.publishJobProgress,
	)
}

func (r *ScannerAgentReporter) flushSuccessfulJobProgress() error {
	if r == nil {
		return nil
	}
	return r.reportJobProgress(1.0)
}

func (c *attemptProgressCheckpoint) report(
	process float64,
	now time.Time,
	publish func(float64) error,
) error {
	completedUnits := progressUnits(process)
	totalUnits := uint32(legionProgressTotalUnits)
	terminal := isTerminalProgress(process)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.observeLocked(process, completedUnits, totalUnits)

	if !c.shouldPublishLocked(completedUnits, totalUnits, terminal, now) {
		return nil
	}
	return c.publishLocked(process, completedUnits, totalUnits, terminal, now, publish)
}

func (c *attemptProgressCheckpoint) flushLatest(
	now time.Time,
	publish func(float64) error,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.hasObserved {
		return nil
	}
	terminal := isTerminalProgress(c.lastObservedProcess)
	if c.hasPublished &&
		c.lastObservedUnits == c.lastCompletedUnits &&
		c.lastObservedTotal == c.lastTotalUnits &&
		(!terminal || c.hasPublishedTerminal) {
		return nil
	}
	return c.publishLocked(
		c.lastObservedProcess,
		c.lastObservedUnits,
		c.lastObservedTotal,
		terminal,
		now,
		publish,
	)
}

func (c *attemptProgressCheckpoint) observeLocked(
	process float64,
	completedUnits uint32,
	totalUnits uint32,
) {
	c.hasObserved = true
	c.lastObservedProcess = process
	c.lastObservedUnits = completedUnits
	c.lastObservedTotal = totalUnits
}

func (c *attemptProgressCheckpoint) publishLocked(
	process float64,
	completedUnits uint32,
	totalUnits uint32,
	terminal bool,
	now time.Time,
	publish func(float64) error,
) error {
	if err := publish(process); err != nil {
		return err
	}

	c.hasPublished = true
	c.lastCompletedUnits = completedUnits
	c.lastTotalUnits = totalUnits
	c.lastPublishedAt = now
	if terminal {
		c.hasPublishedTerminal = true
	}
	return nil
}

func (r *ScannerAgentReporter) updateActiveAttemptProgress(process float64) {
	if r == nil || r.agent == nil || r.agent.manager == nil || r.SubTaskId == "" {
		return
	}
	task, err := r.agent.manager.GetTaskById(taskIDForSubtask(r.SubTaskId))
	if err != nil {
		return
	}
	task.UpdateProgressAt(
		progressUnits(process),
		legionProgressTotalUnits,
		time.Now().UTC(),
	)
}

func (c *attemptProgressCheckpoint) shouldPublishLocked(
	completedUnits uint32,
	totalUnits uint32,
	terminal bool,
	now time.Time,
) bool {
	if !c.hasPublished {
		return true
	}
	if c.hasPublishedTerminal &&
		completedUnits <= c.lastCompletedUnits &&
		totalUnits == c.lastTotalUnits {
		return false
	}
	if totalUnits != c.lastTotalUnits {
		return true
	}
	if terminal && !c.hasPublishedTerminal {
		return true
	}
	if completedUnits <= c.lastCompletedUnits {
		return now.Sub(c.lastPublishedAt) >= legionProgressCheckpointInterval
	}
	if completedUnits-c.lastCompletedUnits >= significantProgressStepUnits(totalUnits) {
		return true
	}
	return now.Sub(c.lastPublishedAt) >= legionProgressCheckpointInterval
}

func significantProgressStepUnits(totalUnits uint32) uint32 {
	if totalUnits == 0 {
		return legionProgressSignificantStepUnits
	}
	stepUnits := totalUnits / 20
	if stepUnits == 0 {
		return 1
	}
	return stepUnits
}

func (r *ScannerAgentReporter) publishJobAsset(
	assetKind string,
	title string,
	target string,
	identityKey string,
	payload []byte,
) error {
	r.touchActiveAttempt()
	publisher, ref, ok, err := r.legionPublisher()
	if err != nil || !ok {
		return err
	}
	return publisher.PublishAsset(
		r.agent.node.GetRootContext(),
		*ref,
		assetKind,
		title,
		target,
		identityKey,
		payload,
	)
}

func (r *ScannerAgentReporter) publishJobRisk(
	riskKind string,
	title string,
	target string,
	severity string,
	dedupeKey string,
	payload []byte,
) error {
	r.touchActiveAttempt()
	publisher, ref, ok, err := r.legionPublisher()
	if err != nil || !ok {
		return err
	}
	return publisher.PublishRisk(
		r.agent.node.GetRootContext(),
		*ref,
		riskKind,
		title,
		target,
		severity,
		dedupeKey,
		payload,
	)
}

func (r *ScannerAgentReporter) publishJobReport(
	reportKind string,
	payload []byte,
) error {
	r.touchActiveAttempt()
	publisher, ref, ok, err := r.legionPublisher()
	if err != nil || !ok {
		return err
	}
	return publisher.PublishReport(
		r.agent.node.GetRootContext(),
		*ref,
		reportKind,
		payload,
	)
}

func (r *ScannerAgentReporter) PublishSSAArtifactReady(
	event *SSAArtifactReadyEvent,
) error {
	if event == nil {
		return nil
	}

	metricsJSON, err := buildSSAArtifactMetricsPayload(event)
	if err != nil {
		return fmt.Errorf("marshal ssa artifact metrics: %w", err)
	}

	publisher, ref, ok, err := r.legionPublisher()
	if err != nil || !ok {
		return err
	}
	r.touchActiveAttempt()
	return publisher.PublishArtifactReady(
		r.agent.node.GetRootContext(),
		*ref,
		legionArtifactKindSSAResultV1,
		event.ArtifactFormat,
		event.ObjectKey,
		event.Codec,
		event.SHA256,
		ssaSizeToUint64(event.UncompressedSize),
		ssaSizeToUint64(event.CompressedSize),
		metricsJSON,
	)
}

func (r *ScannerAgentReporter) legionPublisher() (
	*jobEventPublisher,
	*jobExecutionRef,
	bool,
	error,
) {
	if r == nil || r.executionRef == nil {
		return nil, nil, false, nil
	}
	if r.agent == nil || r.agent.bridge == nil || r.agent.bridge.publisher == nil {
		return nil, nil, false, fmt.Errorf("legion job publisher is not ready")
	}
	return r.agent.bridge.publisher, r.executionRef, true, nil
}

func progressUnits(process float64) uint32 {
	clamped := process
	switch {
	case clamped < 0:
		clamped = 0
	case clamped > 1:
		clamped = 1
	}
	return uint32(math.Round(clamped * legionProgressTotalUnits))
}

func isTerminalProgress(process float64) bool {
	return process >= 1
}

func logReporterEventError(action string, err error) {
	if err == nil {
		return
	}
	log.Errorf("report legion %s failed: %v", action, err)
}

func ssaSizeToUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func (r *ScannerAgentReporter) touchActiveAttempt() {
	if r == nil || r.agent == nil || r.agent.manager == nil || r.SubTaskId == "" {
		return
	}
	r.agent.manager.Touch(taskIDForSubtask(r.SubTaskId))
}
