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
}

type legionJobBridge struct {
	agent               *ScanNode
	publisher           *jobEventPublisher
	capabilityPublisher capabilityEventReporter
	ruleSyncPublisher   *ssaRuleSyncEventPublisher

	mu       sync.Mutex
	consumer *commandConsumer

	statusMu            sync.Mutex
	lastStatusSessionID string
	lastStatusSync      time.Time

	observationMu             sync.Mutex
	suppressedObservationDrop int
}

func newLegionJobBridge(agent *ScanNode) *legionJobBridge {
	return &legionJobBridge{
		agent:               agent,
		publisher:           newJobEventPublisher(agent.node),
		capabilityPublisher: newCapabilityEventPublisher(agent.node),
		ruleSyncPublisher:   newSSARuleSyncEventPublisher(agent.node),
	}
}
