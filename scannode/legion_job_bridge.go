package scannode

import (
	"context"
	"sync"
	"sync/atomic"
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

type dispatchEventReporter interface {
	PublishClaimed(context.Context, jobExecutionRef) error
	PublishStarted(context.Context, jobExecutionRef) error
	PublishSucceeded(context.Context, jobExecutionRef, any) error
	PublishFailed(context.Context, jobExecutionRef, string, string, map[string]string) error
	PublishCancelled(context.Context, jobExecutionRef, string) error
}

type dispatchExecutor func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error)

type legionJobBridge struct {
	agent                          *ScanNode
	publisher                      *jobEventPublisher
	dispatchEvents                 dispatchEventReporter
	dispatchExecutor               dispatchExecutor
	dispatchAdmissions             *dispatchAdmissionRegistry
	dispatchAdmissionsOnce         sync.Once
	nodeIDProvider                 func() string
	rootContextProvider            func() context.Context
	capabilityPublisher            capabilityEventReporter
	hidsDryRunPublisher            hidsDesiredSpecDryRunReporter
	ruleSyncPublisher              *ssaRuleSyncEventPublisher
	aiPublisher                    *aiSessionEventPublisher
	aiRuntime                      *aiSessionRuntimeManager
	aiLocalModelOps                *aiLocalModelOperationManager
	aiKnowledgeBaseQueries         *aiKnowledgeBaseQueryManager
	aiKnowledgeBaseQuestionIndexes *aiKnowledgeBaseQueryManager

	mu           sync.Mutex
	consumer     *commandConsumer
	shuttingDown atomic.Bool

	statusMu            sync.Mutex
	lastStatusSessionID string
	lastStatusSync      time.Time
}

func newLegionJobBridge(agent *ScanNode) *legionJobBridge {
	capabilityPublisher := newCapabilityEventPublisher(agent.node)
	bridge := &legionJobBridge{
		agent:                          agent,
		publisher:                      newJobEventPublisher(agent.node),
		capabilityPublisher:            capabilityPublisher,
		hidsDryRunPublisher:            capabilityPublisher,
		ruleSyncPublisher:              newSSARuleSyncEventPublisher(agent.node),
		aiPublisher:                    newAISessionEventPublisher(agent.node),
		aiRuntime:                      newAISessionRuntimeManager(selectAISessionRuntimeDriver()),
		aiLocalModelOps:                newAILocalModelOperationManager(),
		aiKnowledgeBaseQueries:         newAIKnowledgeBaseQueryManager(),
		aiKnowledgeBaseQuestionIndexes: newAIKnowledgeBaseQueryManager(),
		dispatchAdmissions:             newDispatchAdmissionRegistry(),
	}
	bridge.dispatchEvents = bridge.publisher
	bridge.dispatchExecutor = agent.executeScriptTask
	bridge.nodeIDProvider = agent.node.CurrentNodeID
	bridge.rootContextProvider = agent.node.GetRootContext
	return bridge
}

func (b *legionJobBridge) dispatchReporter() dispatchEventReporter {
	if b.dispatchEvents != nil {
		return b.dispatchEvents
	}
	return b.publisher
}

func (b *legionJobBridge) admissions() *dispatchAdmissionRegistry {
	b.dispatchAdmissionsOnce.Do(func() {
		if b.dispatchAdmissions == nil {
			b.dispatchAdmissions = newDispatchAdmissionRegistry()
		}
	})
	return b.dispatchAdmissions
}

func (b *legionJobBridge) currentNodeID() string {
	if b.nodeIDProvider != nil {
		return b.nodeIDProvider()
	}
	if b.agent != nil && b.agent.node != nil {
		return b.agent.node.CurrentNodeID()
	}
	return ""
}

func (b *legionJobBridge) rootContext() context.Context {
	if b.rootContextProvider != nil {
		return b.rootContextProvider()
	}
	if b.agent != nil && b.agent.node != nil {
		return b.agent.node.GetRootContext()
	}
	return context.Background()
}
