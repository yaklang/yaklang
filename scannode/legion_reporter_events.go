package scannode

import (
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/log"
)

const legionProgressTotalUnits = 10000

func (r *ScannerAgentReporter) publishJobProgress(process float64) error {
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

func (r *ScannerAgentReporter) publishJobAsset(
	assetKind string,
	title string,
	target string,
	identityKey string,
	payload []byte,
) error {
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

func logReporterEventError(action string, err error) {
	if err == nil {
		return
	}
	log.Errorf("report legion %s failed: %v", action, err)
}
