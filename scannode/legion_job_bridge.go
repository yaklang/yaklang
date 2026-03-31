package scannode

import "sync"

type legionJobBridge struct {
	agent               *ScanNode
	publisher           *jobEventPublisher
	capabilityPublisher *capabilityEventPublisher

	mu       sync.Mutex
	consumer *commandConsumer
}

func newLegionJobBridge(agent *ScanNode) *legionJobBridge {
	return &legionJobBridge{
		agent:               agent,
		publisher:           newJobEventPublisher(agent.node),
		capabilityPublisher: newCapabilityEventPublisher(agent.node),
	}
}
