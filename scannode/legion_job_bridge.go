package scannode

import (
	"context"
	"sync"
	"time"
)

type capabilityEventReporter interface {
	Close()
	PublishStatus(context.Context, capabilityCommandRef, CapabilityApplyResult) error
	PublishFailed(context.Context, capabilityCommandRef, string, string) error
	PublishAlert(context.Context, CapabilityRuntimeAlert) error
	PublishObservation(context.Context, CapabilityRuntimeObservation) error
	PublishResponseActionResult(context.Context, HIDSResponseActionResultInput) error
}

type hidsDesiredSpecDryRunReporter interface {
	Close()
	PublishDesiredSpecDryRunResult(context.Context, capabilityCommandRef, CapabilityDryRunResult) error
}

type legionJobBridge struct {
	agent               *ScanNode
	publisher           *jobEventPublisher
	capabilityPublisher capabilityEventReporter
	hidsDryRunPublisher hidsDesiredSpecDryRunReporter
	ruleSyncPublisher   *ssaRuleSyncEventPublisher
	aiPublisher         *aiSessionEventPublisher
	aiRuntime           *aiSessionRuntimeManager

	mu       sync.Mutex
	consumer *commandConsumer

	statusMu            sync.Mutex
	lastStatusSessionID string
	lastStatusSync      time.Time
}

func newLegionJobBridge(agent *ScanNode) *legionJobBridge {
	capabilityPublisher := newCapabilityEventPublisher(agent.node)
	return &legionJobBridge{
		agent:               agent,
		publisher:           newJobEventPublisher(agent.node),
		capabilityPublisher: capabilityPublisher,
		hidsDryRunPublisher: capabilityPublisher,
		ruleSyncPublisher:   newSSARuleSyncEventPublisher(agent.node),
		aiPublisher:         newAISessionEventPublisher(agent.node),
		aiRuntime:           newAISessionRuntimeManager(newYakAIEngineRuntimeDriver()),
	}
}
