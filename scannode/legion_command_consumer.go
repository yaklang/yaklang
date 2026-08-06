package scannode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/log"
)

const commandPollInterval = time.Second

type commandConsumer struct {
	sessionID string
	cancel    context.CancelFunc
	conn      *nats.Conn
	sub       *nats.Subscription
}

func (b *legionJobBridge) Run(ctx context.Context) {
	ticker := time.NewTicker(commandPollInterval)
	defer ticker.Stop()
	defer b.stopConsumer()
	defer b.publisher.Close()
	defer b.capabilityPublisher.Close()
	defer b.hidsDryRunPublisher.Close()
	defer b.ruleSyncPublisher.Close()
	if b.aiPublisher != nil {
		defer b.aiPublisher.Close()
	}

	go b.forwardCapabilityAlerts(ctx)
	go b.forwardCapabilityObservations(ctx)

	for {
		if ctx.Err() != nil {
			return
		}
		b.syncConsumer(ctx)
		b.syncCapabilityStatuses(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *legionJobBridge) forwardCapabilityObservations(ctx context.Context) {
	if b == nil || b.agent == nil || b.agent.capabilityManager == nil {
		return
	}

	observations := b.agent.capabilityManager.Observations()
	if observations == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case observation, ok := <-observations:
			if !ok {
				return
			}
			if err := b.capabilityPublisher.PublishObservation(ctx, observation); err != nil {
				log.Errorf(
					"publish hids snapshot observation failed: node_id=%s capability=%s type=%s err=%v",
					b.agent.node.CurrentNodeID(),
					observation.CapabilityKey,
					observation.HIDSEventType,
					err,
				)
			}
		}
	}
}

func (b *legionJobBridge) forwardCapabilityAlerts(ctx context.Context) {
	if b == nil || b.agent == nil || b.agent.capabilityManager == nil {
		return
	}

	alerts := b.agent.capabilityManager.Alerts()
	if alerts == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case alert, ok := <-alerts:
			if !ok {
				return
			}
			if err := b.capabilityPublisher.PublishAlert(ctx, alert); err != nil {
				log.Errorf(
					"publish capability alert failed: node_id=%s capability=%s rule_id=%s err=%v",
					b.agent.node.CurrentNodeID(),
					alert.CapabilityKey,
					alert.RuleID,
					err,
				)
			}
		}
	}
}

func (b *legionJobBridge) syncConsumer(parent context.Context) {
	session, ok := b.agent.node.GetSessionState()
	if !ok {
		b.stopConsumer()
		b.resetCapabilityStatusSync()
		return
	}

	b.mu.Lock()
	current := b.consumer
	b.mu.Unlock()
	if current != nil && current.sessionID == session.SessionID {
		return
	}

	b.stopConsumer()
	consumer, err := b.startConsumer(parent, session.NATSURL, session.SessionID, session.CommandSubject)
	if err != nil {
		log.Errorf("start legion command consumer failed: %v", err)
		return
	}

	b.mu.Lock()
	b.consumer = consumer
	b.mu.Unlock()
}

func (b *legionJobBridge) startConsumer(
	parent context.Context,
	natsURL string,
	sessionID string,
	commandSubject string,
) (*commandConsumer, error) {
	currentNodeID := b.agent.node.CurrentNodeID()
	conn, err := nats.Connect(natsURL, nats.Name("yak-node-commands-"+currentNodeID))
	if err != nil {
		return nil, fmt.Errorf("connect command nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("build command jetstream context: %w", err)
	}

	subscription, err := js.PullSubscribe(
		commandSubjectWildcard(commandSubject),
		consumerNameForNode(currentNodeID),
		nats.BindStream(legionCommandStream),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.MaxAckPending(64),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("pull subscribe commands: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	consumer := &commandConsumer{
		sessionID: sessionID,
		cancel:    cancel,
		conn:      conn,
		sub:       subscription,
	}
	go b.consumeLoop(ctx, consumer)
	log.Infof("started legion command consumer: node_id=%s session_id=%s", currentNodeID, sessionID)
	return consumer, nil
}

func (b *legionJobBridge) stopConsumer() {
	b.mu.Lock()
	consumer := b.consumer
	b.consumer = nil
	b.mu.Unlock()
	if consumer == nil {
		return
	}

	consumer.cancel()
	if consumer.sub != nil {
		_ = consumer.sub.Unsubscribe()
	}
	if consumer.conn != nil {
		consumer.conn.Close()
	}
}

func (b *legionJobBridge) consumeLoop(ctx context.Context, consumer *commandConsumer) {
	for {
		if ctx.Err() != nil {
			return
		}

		messages, err := consumer.sub.Fetch(4, nats.MaxWait(time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || ctx.Err() != nil {
				continue
			}
			if isCommandConsumerResetError(err) {
				log.Errorf(
					"legion command consumer became invalid: node_id=%s session_id=%s err=%v diagnosis=%q",
					b.agent.node.CurrentNodeID(),
					consumer.sessionID,
					err,
					"another process may be running with the same node_id, or the platform session/consumer was replaced",
				)
				b.stopConsumer()
				return
			}
			log.Errorf(
				"fetch legion commands failed: node_id=%s session_id=%s err=%v",
				b.agent.node.CurrentNodeID(),
				consumer.sessionID,
				err,
			)
			continue
		}
		for _, message := range messages {
			if err := b.handleMessage(ctx, message); err != nil {
				log.Errorf("handle legion command failed: %v", err)
				_ = message.Nak()
				continue
			}
			_ = message.Ack()
		}
	}
}

func isCommandConsumerResetError(err error) bool {
	return errors.Is(err, nats.ErrConsumerDeleted) ||
		errors.Is(err, nats.ErrNoResponders) ||
		errors.Is(err, nats.ErrConnectionClosed) ||
		errors.Is(err, nats.ErrDisconnected) ||
		errors.Is(err, nats.ErrBadSubscription) ||
		errors.Is(err, nats.ErrSubscriptionClosed)
}

func (b *legionJobBridge) handleMessage(
	ctx context.Context,
	message *nats.Msg,
) error {
	switch {
	case strings.HasSuffix(message.Subject, "."+legionCommandDispatch):
		return b.handleDispatch(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandCancel):
		return b.handleCancel(message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandCapabilityApply):
		return b.handleCapabilityApply(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandHIDSDesiredSpecDryRun):
		return b.handleHIDSDesiredSpecDryRun(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandHIDSCurrentStateCollect):
		return b.handleHIDSCurrentStateCollect(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandHIDSFileEvidenceCollect):
		return b.handleHIDSFileEvidenceCollect(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandHIDSResponseActionExecute):
		return b.handleHIDSResponseActionExecute(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandSSARuleSyncExport):
		return b.handleSSARuleSyncExport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionBind):
		return b.handleAISessionBind(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionInput):
		return b.handleAISessionInput(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionAppend):
		return b.handleAISessionAppendContext(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionCancel):
		return b.handleAISessionCancel(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionClose):
		return b.handleAISessionClose(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionTitleUpdate):
		return b.handleAISessionTitleUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAISessionDelete):
		return b.handleAISessionDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILogsCheckpointsExport):
		return b.handleAILogsCheckpointsExport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIProviderModelsList):
		return b.handleAIProviderModelsList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIProviderHealthCheck):
		return b.handleAIProviderHealthCheck(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIFocusQuery):
		return b.handleAIFocusQuery(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMaterialsRandomQuery):
		return b.handleAIMaterialsRandomQuery(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIGlobalConfigGet):
		return b.handleAIGlobalConfigGet(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIGlobalConfigSet):
		return b.handleAIGlobalConfigSet(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMCPServersList):
		return b.handleAIMCPServersList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMCPServerCreate):
		return b.handleAIMCPServerCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMCPServerUpdate):
		return b.handleAIMCPServerUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMCPServerDelete):
		return b.handleAIMCPServerDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelsList):
		return b.handleAILocalModelsList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILlamaServerReady):
		return b.handleAILlamaServerReady(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILlamaServerInstall):
		return b.handleAILlamaServerInstall(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelCreate):
		return b.handleAILocalModelCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelUpdate):
		return b.handleAILocalModelUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelDelete):
		return b.handleAILocalModelDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelStart):
		return b.handleAILocalModelStart(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelStop):
		return b.handleAILocalModelStop(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelDownload):
		return b.handleAILocalModelDownload(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelOperationCancel):
		return b.handleAILocalModelOperationCancel(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAILocalModelsClear):
		return b.handleAILocalModelsClear(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgesList):
		return b.handleAIForgesList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgeCreate):
		return b.handleAIForgeCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgeUpdate):
		return b.handleAIForgeUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgeDelete):
		return b.handleAIForgeDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgeExport):
		return b.handleAIForgeExport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIForgeImport):
		return b.handleAIForgeImport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolsList):
		return b.handleAIToolsList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolCreate):
		return b.handleAIToolCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolUpdate):
		return b.handleAIToolUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolMetadataGenerate):
		return b.handleAIToolGenerateMetadata(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolsFavoriteToggle):
		return b.handleAIToolFavoriteToggle(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIToolsDelete):
		return b.handleAIToolsDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBasesList):
		return b.handleAIKnowledgeBasesList(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseCreate):
		return b.handleAIKnowledgeBaseCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseImport):
		return b.handleAIKnowledgeBaseImport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseUpdate):
		return b.handleAIKnowledgeBaseUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseDelete):
		return b.handleAIKnowledgeBaseDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseExport):
		return b.handleAIKnowledgeBaseExport(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseEntriesSearch):
		return b.handleAIKnowledgeBaseEntriesSearch(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseQueryByAI):
		return b.handleAIKnowledgeBaseQueryByAI(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseQueryByAICancel):
		return b.handleAIKnowledgeBaseQueryByAICancel(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseQuestionIndexGenerate):
		return b.handleAIKnowledgeBaseQuestionIndexGenerate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseQuestionIndexCancel):
		return b.handleAIKnowledgeBaseQuestionIndexCancel(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseEntryCreate):
		return b.handleAIKnowledgeBaseEntryCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseEntryUpdate):
		return b.handleAIKnowledgeBaseEntryUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseEntryDelete):
		return b.handleAIKnowledgeBaseEntryDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseVectorIndexBuild):
		return b.handleAIKnowledgeBaseVectorIndexBuild(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIKnowledgeBaseEntryVectorIndexBuild):
		return b.handleAIKnowledgeBaseEntryVectorIndexBuild(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntityCreate):
		return b.handleAIMemoryEntityCreate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntityGet):
		return b.handleAIMemoryEntityGet(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntitiesQuery):
		return b.handleAIMemoryEntitiesQuery(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntityUpdate):
		return b.handleAIMemoryEntityUpdate(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntitiesDelete):
		return b.handleAIMemoryEntitiesDelete(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIMemoryEntityTagsCount):
		return b.handleAIMemoryEntityTagsCount(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIHTTPFlowsQuery):
		return b.handleAIHTTPFlowsQuery(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandAIRisksQuery):
		return b.handleAIRisksQuery(ctx, message.Data)
	default:
		return fmt.Errorf("unsupported legion command subject: %s", message.Subject)
	}
}

func consumerNameForNode(nodeID string) string {
	var builder strings.Builder
	builder.WriteString("legion-node-")
	for _, r := range nodeID {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return builder.String()
}
