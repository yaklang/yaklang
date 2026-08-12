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
	agent                  *ScanNode
	publisher              *jobEventPublisher
	capabilityPublisher    capabilityEventReporter
	hidsDryRunPublisher    hidsDesiredSpecDryRunReporter
	ruleSyncPublisher      *ssaRuleSyncEventPublisher
	gitLsRemotePublisher   *gitLsRemoteEventPublisher
	aiPublisher            *aiSessionEventPublisher
	aiRuntime              *aiSessionRuntimeManager
	aiLocalModelOps        *aiLocalModelOperationManager
	aiKnowledgeBaseQueries *aiKnowledgeBaseQueryManager
	aiKnowledgeBaseQuestionIndexes *aiKnowledgeBaseQueryManager

	mu       sync.Mutex
	consumer *commandConsumer

	statusMu            sync.Mutex
	lastStatusSessionID string
	lastStatusSync      time.Time
}

func newLegionJobBridge(agent *ScanNode) *legionJobBridge {
	capabilityPublisher := newCapabilityEventPublisher(agent.node)
	return &legionJobBridge{
		agent:                  agent,
		publisher:              newJobEventPublisher(agent.node),
		capabilityPublisher:    capabilityPublisher,
		hidsDryRunPublisher:    capabilityPublisher,
		ruleSyncPublisher:      newSSARuleSyncEventPublisher(agent.node),
		gitLsRemotePublisher:   newGitLsRemoteEventPublisher(agent.node),
		aiPublisher:            newAISessionEventPublisher(agent.node),
		aiRuntime:              newAISessionRuntimeManager(selectAISessionRuntimeDriver()),
		aiLocalModelOps:        newAILocalModelOperationManager(),
		aiKnowledgeBaseQueries: newAIKnowledgeBaseQueryManager(),
		aiKnowledgeBaseQuestionIndexes: newAIKnowledgeBaseQueryManager(),
	}
}
