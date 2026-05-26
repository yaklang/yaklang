package scannode

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

type aiSessionCommandRef struct {
	CommandID   string
	SessionID   string
	RunID       string
	OwnerUserID string
}

type aiProviderCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiFocusCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiMaterialsCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiForgeCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiToolCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiMCPCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiLocalModelCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiKnowledgeBaseCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiMemoryCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiRuntimeQueryCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiLogsCommandRef struct {
	CommandID   string
	OwnerUserID string
	SessionID   string
}

type aiSessionEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newAISessionEventPublisher(base *node.NodeBase) *aiSessionEventPublisher {
	return &aiSessionEventPublisher{node: base}
}

func (p *aiSessionEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *aiSessionEventPublisher) PublishReady(ctx context.Context, ref aiSessionCommandRef) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionReady, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "ready"), &aiv1.AISessionReady{
		Session:        aiSessionProtoRef(ref),
		RuntimeName:    "yak-ai-runtime",
		RuntimeVersion: "nats-bridge-v1",
		ReadyAt:        timestamppb.New(now),
	})
}

func (p *aiSessionEventPublisher) PublishEvent(
	ctx context.Context,
	ref aiSessionCommandRef,
	seq uint64,
	eventType string,
	payloadJSON []byte,
) error {
	if eventType == "" {
		eventType = legionEventAISessionEvent
	}
	suffix := "event"
	if seq > 0 {
		suffix = fmt.Sprintf("event-%d", seq)
	}
	return p.publish(ctx, legionEventAISessionEvent, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, suffix), &aiv1.AISessionEvent{
		Session:     aiSessionProtoRef(ref),
		Seq:         seq,
		EventType:   eventType,
		PayloadJson: cloneBytes(payloadJSON),
	})
}

func (p *aiSessionEventPublisher) PublishDone(
	ctx context.Context,
	ref aiSessionCommandRef,
	resultJSON []byte,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionDone, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "done"), &aiv1.AISessionDone{
		Session:    aiSessionProtoRef(ref),
		FinishedAt: timestamppb.New(now),
		ResultJson: cloneBytes(resultJSON),
	})
}

func (p *aiSessionEventPublisher) PublishFailed(
	ctx context.Context,
	ref aiSessionCommandRef,
	errorCode string,
	errorMessage string,
	errorDetailJSON []byte,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionFailed, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "failed"), &aiv1.AISessionFailed{
		Session:         aiSessionProtoRef(ref),
		FinishedAt:      timestamppb.New(now),
		ErrorCode:       errorCode,
		ErrorMessage:    errorMessage,
		ErrorDetailJson: cloneBytes(errorDetailJSON),
	})
}

func (p *aiSessionEventPublisher) PublishCancelled(
	ctx context.Context,
	ref aiSessionCommandRef,
	reason string,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionCancelled, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "cancelled"), &aiv1.AISessionCancelled{
		Session:    aiSessionProtoRef(ref),
		FinishedAt: timestamppb.New(now),
		Reason:     reason,
	})
}

func (p *aiSessionEventPublisher) PublishAISessionTitleUpdated(
	ctx context.Context,
	ref aiSessionCommandRef,
	title string,
	message string,
) error {
	return p.publish(
		ctx,
		legionEventAISessionTitleUpdated,
		ref,
		eventIDWithSuffix(ref.CommandID, ref.SessionID, "title-updated"),
		&aiv1.AISessionTitleUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Session:     aiSessionProtoRef(ref),
			Title:       strings.TrimSpace(title),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAISessionTitleUpdateFailed(
	ctx context.Context,
	ref aiSessionCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publish(
		ctx,
		legionEventAISessionTitleUpdateFailed,
		ref,
		eventIDWithSuffix(ref.CommandID, ref.SessionID, "title-update-failed"),
		&aiv1.AISessionTitleUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			Session:      aiSessionProtoRef(ref),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAISessionDeleteCompleted(
	ctx context.Context,
	ref aiSessionCommandRef,
	message string,
) error {
	return p.publish(
		ctx,
		legionEventAISessionDeleteCompleted,
		ref,
		eventIDWithSuffix(ref.CommandID, ref.SessionID, "delete-completed"),
		&aiv1.AISessionDeleteCompleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Session:     aiSessionProtoRef(ref),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAISessionDeleteFailed(
	ctx context.Context,
	ref aiSessionCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publish(
		ctx,
		legionEventAISessionDeleteFailed,
		ref,
		eventIDWithSuffix(ref.CommandID, ref.SessionID, "delete-failed"),
		&aiv1.AISessionDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			Session:      aiSessionProtoRef(ref),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILogsCheckpointsExported(
	ctx context.Context,
	ref aiLogsCommandRef,
	checkpointsJSON []byte,
	total int64,
) error {
	return p.publishLogs(
		ctx,
		legionEventAILogsCheckpointsExported,
		ref,
		logsEventIDWithSuffix(ref.CommandID, ref.SessionID, "checkpoints-exported"),
		&aiv1.AILogsCheckpointsExported{
			OwnerUserId:     strings.TrimSpace(ref.OwnerUserID),
			SessionId:       strings.TrimSpace(ref.SessionID),
			CheckpointsJson: append([]byte(nil), checkpointsJSON...),
			Total:           total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILogsCheckpointsExportFailed(
	ctx context.Context,
	ref aiLogsCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLogs(
		ctx,
		legionEventAILogsCheckpointsExportFailed,
		ref,
		logsEventIDWithSuffix(ref.CommandID, ref.SessionID, "checkpoints-export-failed"),
		&aiv1.AILogsCheckpointsExportFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			SessionId:    strings.TrimSpace(ref.SessionID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) publish(
	ctx context.Context,
	eventType string,
	ref aiSessionCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.SessionID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai session event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai session event %s: %w", eventType, err)
	}
	logPublishedAISessionEvent(eventType, ref.SessionID, message)
	return nil
}

func (p *aiSessionEventPublisher) PublishProviderModelsListed(
	ctx context.Context,
	ref aiProviderCommandRef,
	items []*aiv1.AIProviderPreviewModel,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderModelsListed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "models-listed"),
		&aiv1.AIProviderModelsListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
		},
	)
}

func (p *aiSessionEventPublisher) PublishProviderModelsFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderModelsFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "models-failed"),
		&aiv1.AIProviderModelsFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishProviderHealthCheckCompleted(
	ctx context.Context,
	ref aiProviderCommandRef,
	result *aiv1.AIProviderHealthCheckCompleted,
) error {
	if result == nil {
		result = &aiv1.AIProviderHealthCheckCompleted{}
	}
	result.OwnerUserId = strings.TrimSpace(ref.OwnerUserID)
	return p.publishProvider(
		ctx,
		legionEventAIProviderHealthCheckCompleted,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "health-check-completed"),
		result,
	)
}

func (p *aiSessionEventPublisher) PublishProviderHealthCheckFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderHealthCheckFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "health-check-failed"),
		&aiv1.AIProviderHealthCheckFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIFocusQueried(
	ctx context.Context,
	ref aiFocusCommandRef,
	items []*aiv1.AIFocus,
) error {
	return p.publishFocus(
		ctx,
		legionEventAIFocusQueried,
		ref,
		focusEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "queried"),
		&aiv1.AIFocusQueried{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIFocusQueryFailed(
	ctx context.Context,
	ref aiFocusCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishFocus(
		ctx,
		legionEventAIFocusQueryFailed,
		ref,
		focusEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-failed"),
		&aiv1.AIFocusQueryFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMaterialsRandomQueried(
	ctx context.Context,
	ref aiMaterialsCommandRef,
	knowledgeBaseEntries []*aiv1.AIKnowledgeBaseEntryRecord,
	tools []*aiv1.AIToolRecord,
	forges []*aiv1.AIForgeRecord,
) error {
	return p.publishMaterials(
		ctx,
		legionEventAIMaterialsRandomQueried,
		ref,
		materialsEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "queried"),
		&aiv1.AIMaterialsRandomQueried{
			OwnerUserId:          strings.TrimSpace(ref.OwnerUserID),
			KnowledgeBaseEntries: knowledgeBaseEntries,
			AiTools:              tools,
			AiForges:             forges,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMaterialsRandomQueryFailed(
	ctx context.Context,
	ref aiMaterialsCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMaterials(
		ctx,
		legionEventAIMaterialsRandomQueryFailed,
		ref,
		materialsEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-failed"),
		&aiv1.AIMaterialsRandomQueryFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIGlobalConfigFetched(
	ctx context.Context,
	ref aiProviderCommandRef,
	config *aiv1.AIGlobalConfigSnapshot,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIGlobalConfigFetched,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "global-config-fetched"),
		&aiv1.AIGlobalConfigFetched{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Config:      config,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIGlobalConfigFetchFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIGlobalConfigFetchFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "global-config-fetch-failed"),
		&aiv1.AIGlobalConfigFetchFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIGlobalConfigUpdated(
	ctx context.Context,
	ref aiProviderCommandRef,
	config *aiv1.AIGlobalConfigSnapshot,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIGlobalConfigUpdated,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "global-config-updated"),
		&aiv1.AIGlobalConfigUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Config:      config,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIGlobalConfigUpdateFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIGlobalConfigUpdateFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "global-config-update-failed"),
		&aiv1.AIGlobalConfigUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServersListed(
	ctx context.Context,
	ref aiMCPCommandRef,
	items []*aiv1.AIMCPServerRecord,
	pagination *aiv1.AIMCPServerPagination,
	total int64,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServersListed,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list"),
		&aiv1.AIMCPServersListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServersListFailed(
	ctx context.Context,
	ref aiMCPCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServersListFailed,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list-failed"),
		&aiv1.AIMCPServersListFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerCreated(
	ctx context.Context,
	ref aiMCPCommandRef,
	item *aiv1.AIMCPServerRecord,
	message string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerCreated,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AIMCPServerCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerCreateFailed(
	ctx context.Context,
	ref aiMCPCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerCreateFailed,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AIMCPServerCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerUpdated(
	ctx context.Context,
	ref aiMCPCommandRef,
	item *aiv1.AIMCPServerRecord,
	message string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerUpdated,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AIMCPServerUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerUpdateFailed(
	ctx context.Context,
	ref aiMCPCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerUpdateFailed,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AIMCPServerUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerDeleted(
	ctx context.Context,
	ref aiMCPCommandRef,
	item *aiv1.AIMCPServerRecord,
	message string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerDeleted,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AIMCPServerDeleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMCPServerDeleteFailed(
	ctx context.Context,
	ref aiMCPCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMCP(
		ctx,
		legionEventAIMCPServerDeleteFailed,
		ref,
		mcpEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AIMCPServerDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelsListed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	items []*aiv1.AILocalModelRecord,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelsListed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list"),
		&aiv1.AILocalModelsListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelsListFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelsListFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list-failed"),
		&aiv1.AILocalModelsListFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILlamaServerReadyChecked(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	ok bool,
	reason string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILlamaServerReadyChecked,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "llama-server-ready"),
		&aiv1.AILlamaServerReadyChecked{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Ok:          ok,
			Reason:      strings.TrimSpace(reason),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILlamaServerReadyCheckFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILlamaServerReadyCheckFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "llama-server-ready-failed"),
		&aiv1.AILlamaServerReadyCheckFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILlamaServerInstalled(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	ok bool,
	reason string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILlamaServerInstalled,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "llama-server-installed"),
		&aiv1.AILlamaServerInstalled{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Ok:          ok,
			Reason:      strings.TrimSpace(reason),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILlamaServerInstallFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILlamaServerInstallFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "llama-server-install-failed"),
		&aiv1.AILlamaServerInstallFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelOperationAccepted(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelOperationAccepted,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "operation-accepted"),
		&aiv1.AILocalModelOperationAccepted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Operation:   operation,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelOperationProgressed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	progress float32,
	message string,
	rawMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelOperationProgressed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "operation-progressed"),
		&aiv1.AILocalModelOperationProgressed{
			OwnerUserId:     strings.TrimSpace(ref.OwnerUserID),
			Operation:       operation,
			ProgressPercent: progress,
			Message:         strings.TrimSpace(message),
			IsMessage:       strings.TrimSpace(message) != "",
			RawMessage:      rawMessage,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelOperationCompleted(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	reason string,
	item *aiv1.AILocalModelRecord,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelOperationCompleted,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "operation-completed"),
		&aiv1.AILocalModelOperationCompleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Operation:   operation,
			Ok:          true,
			Reason:      strings.TrimSpace(reason),
			Item:        item,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelOperationCancelled(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	reason string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelOperationCancelled,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "operation-cancelled"),
		&aiv1.AILocalModelOperationCancelled{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Operation:   operation,
			Reason:      strings.TrimSpace(reason),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelOperationFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	operation *aiv1.AILocalModelOperation,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelOperationFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "operation-failed"),
		&aiv1.AILocalModelOperationFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			Operation:    operation,
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelCreated(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelCreated,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AILocalModelCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelCreateFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelCreateFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AILocalModelCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelUpdated(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelUpdated,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AILocalModelUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelUpdateFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelUpdateFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AILocalModelUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelDeleted(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelDeleted,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AILocalModelDeleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelDeleteFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelDeleteFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AILocalModelDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelStarted(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelStarted,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "started"),
		&aiv1.AILocalModelStarted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelStartFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelStartFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "start-failed"),
		&aiv1.AILocalModelStartFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelStopped(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelStopped,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "stopped"),
		&aiv1.AILocalModelStopped{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelStopFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelStopFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "stop-failed"),
		&aiv1.AILocalModelStopFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelDownloaded(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	item *aiv1.AILocalModelRecord,
	message string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelDownloaded,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "downloaded"),
		&aiv1.AILocalModelDownloaded{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelDownloadFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelDownloadFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "download-failed"),
		&aiv1.AILocalModelDownloadFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelsCleared(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	ok bool,
	reason string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelsCleared,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "cleared"),
		&aiv1.AILocalModelsCleared{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Ok:          ok,
			Reason:      strings.TrimSpace(reason),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAILocalModelsClearFailed(
	ctx context.Context,
	ref aiLocalModelCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishLocalModel(
		ctx,
		legionEventAILocalModelsClearFailed,
		ref,
		localModelEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "clear-failed"),
		&aiv1.AILocalModelsClearFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgesListed(
	ctx context.Context,
	ref aiForgeCommandRef,
	items []*aiv1.AIForgeRecord,
	pagination *aiv1.AIForgePagination,
	total int64,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgesListed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list"),
		&aiv1.AIForgesListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgesListFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgesListFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list-failed"),
		&aiv1.AIForgesListFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeCreated(
	ctx context.Context,
	ref aiForgeCommandRef,
	item *aiv1.AIForgeRecord,
	message string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeCreated,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AIForgeCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeCreateFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeCreateFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AIForgeCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeUpdated(
	ctx context.Context,
	ref aiForgeCommandRef,
	item *aiv1.AIForgeRecord,
	message string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeUpdated,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AIForgeUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeUpdateFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeUpdateFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AIForgeUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeDeleted(
	ctx context.Context,
	ref aiForgeCommandRef,
	forgeID string,
	message string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeDeleted,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AIForgeDeleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			ForgeId:     strings.TrimSpace(forgeID),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeDeleteFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeDeleteFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AIForgeDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeExportProgressed(
	ctx context.Context,
	ref aiForgeCommandRef,
	percent float64,
	message string,
	messageType string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeExportProgressed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, fmt.Sprintf("export-progress-%d", int(percent))),
		&aiv1.AIForgeExportProgressed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Percent:     percent,
			Message:     strings.TrimSpace(message),
			MessageType: strings.TrimSpace(messageType),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeExported(
	ctx context.Context,
	ref aiForgeCommandRef,
	fileName string,
	contentType string,
	objectStoreBucket string,
	objectStoreKey string,
	sizeBytes int64,
	message string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeExported,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "exported"),
		&aiv1.AIForgeExported{
			OwnerUserId:       strings.TrimSpace(ref.OwnerUserID),
			FileName:          strings.TrimSpace(fileName),
			ContentType:       strings.TrimSpace(contentType),
			ObjectStoreBucket: strings.TrimSpace(objectStoreBucket),
			ObjectStoreKey:    strings.TrimSpace(objectStoreKey),
			SizeBytes:         uint64(sizeBytes),
			Message:           strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeExportFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeExportFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "export-failed"),
		&aiv1.AIForgeExportFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeImportProgressed(
	ctx context.Context,
	ref aiForgeCommandRef,
	percent float64,
	message string,
	messageType string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeImportProgressed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, fmt.Sprintf("import-progress-%d", int(percent))),
		&aiv1.AIForgeImportProgressed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Percent:     percent,
			Message:     strings.TrimSpace(message),
			MessageType: strings.TrimSpace(messageType),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeImported(
	ctx context.Context,
	ref aiForgeCommandRef,
	created int64,
	updated int64,
	skipped int64,
	items []*aiv1.AIForgeImportItem,
	message string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeImported,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "imported"),
		&aiv1.AIForgeImported{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Created:     created,
			Updated:     updated,
			Skipped:     skipped,
			Items:       items,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIForgeImportFailed(
	ctx context.Context,
	ref aiForgeCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishForge(
		ctx,
		legionEventAIForgeImportFailed,
		ref,
		forgeEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "import-failed"),
		&aiv1.AIForgeImportFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolsListed(
	ctx context.Context,
	ref aiToolCommandRef,
	items []*aiv1.AIToolRecord,
	pagination *aiv1.AIToolPagination,
	total int64,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsListed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list"),
		&aiv1.AIToolsListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolsListFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsListFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list-failed"),
		&aiv1.AIToolsListFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolCreated(
	ctx context.Context,
	ref aiToolCommandRef,
	item *aiv1.AIToolRecord,
	message string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolCreated,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AIToolCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolCreateFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolCreateFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AIToolCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolUpdated(
	ctx context.Context,
	ref aiToolCommandRef,
	item *aiv1.AIToolRecord,
	message string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolUpdated,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AIToolUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolUpdateFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolUpdateFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AIToolUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolMetadataGenerated(
	ctx context.Context,
	ref aiToolCommandRef,
	name string,
	description string,
	keywords []string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolMetadataGenerated,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "metadata-generated"),
		&aiv1.AIToolMetadataGenerated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Name:        strings.TrimSpace(name),
			Description: strings.TrimSpace(description),
			Keywords:    append([]string(nil), keywords...),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolMetadataGenerateFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolMetadataGenerateFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "metadata-generate-failed"),
		&aiv1.AIToolMetadataGenerateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolFavoriteToggled(
	ctx context.Context,
	ref aiToolCommandRef,
	toolID int64,
	isFavorite bool,
	message string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsFavoriteToggled,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "favorite-toggled"),
		&aiv1.AIToolFavoriteToggled{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			ToolId:      toolID,
			IsFavorite:  isFavorite,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolFavoriteToggleFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsFavoriteToggleFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "favorite-toggle-failed"),
		&aiv1.AIToolFavoriteToggleFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolsDeleted(
	ctx context.Context,
	ref aiToolCommandRef,
	toolIDs []int64,
	message string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsDeleted,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AIToolsDeleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			ToolIds:     append([]int64(nil), toolIDs...),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIToolsDeleteFailed(
	ctx context.Context,
	ref aiToolCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishTool(
		ctx,
		legionEventAIToolsDeleteFailed,
		ref,
		toolEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AIToolsDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBasesListed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	items []*aiv1.AIKnowledgeBaseRecord,
	pagination *aiv1.AIKnowledgeBasePagination,
	total int64,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBasesListed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list"),
		&aiv1.AIKnowledgeBasesListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBasesListFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBasesListFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "list-failed"),
		&aiv1.AIKnowledgeBasesListFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseCreated(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseCreated,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AIKnowledgeBaseCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseCreateFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseCreateFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AIKnowledgeBaseCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseImported(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseImported,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "imported"),
		&aiv1.AIKnowledgeBaseImported{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseImportFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseImportFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "import-failed"),
		&aiv1.AIKnowledgeBaseImportFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseUpdated(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseUpdated,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AIKnowledgeBaseUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseUpdateFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseUpdateFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AIKnowledgeBaseUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseDeleted(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseDeleted,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AIKnowledgeBaseDeleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseDeleteFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseDeleteFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AIKnowledgeBaseDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseExported(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseRecord,
	message string,
	fileName string,
	contentType string,
	objectStoreBucket string,
	objectStoreKey string,
	sizeBytes int64,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseExported,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "exported"),
		&aiv1.AIKnowledgeBaseExported{
			OwnerUserId:       strings.TrimSpace(ref.OwnerUserID),
			Item:              item,
			Message:           strings.TrimSpace(message),
			FileName:          strings.TrimSpace(fileName),
			ContentType:       strings.TrimSpace(contentType),
			ObjectStoreBucket: strings.TrimSpace(objectStoreBucket),
			ObjectStoreKey:    strings.TrimSpace(objectStoreKey),
			SizeBytes:         uint64(sizeBytes),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseExportFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseExportFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "export-failed"),
		&aiv1.AIKnowledgeBaseExportFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntriesSearched(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	items []*aiv1.AIKnowledgeBaseEntryRecord,
	pagination *aiv1.AIKnowledgeBasePagination,
	total int64,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntriesSearched,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entries-searched"),
		&aiv1.AIKnowledgeBaseEntriesSearched{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntriesSearchFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntriesSearchFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entries-search-failed"),
		&aiv1.AIKnowledgeBaseEntriesSearchFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQueryByAIChunk(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	message string,
	messageType string,
	data string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQueryByAIChunk,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-by-ai-chunk"),
		&aiv1.AIKnowledgeBaseQueryByAIChunk{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Message:     strings.TrimSpace(message),
			MessageType: strings.TrimSpace(messageType),
			Data:        data,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQueryByAICompleted(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQueryByAICompleted,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-by-ai-completed"),
		&aiv1.AIKnowledgeBaseQueryByAICompleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQueryByAIFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQueryByAIFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-by-ai-failed"),
		&aiv1.AIKnowledgeBaseQueryByAIFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQuestionIndexProgress(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	percent float64,
	message string,
	messageType string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQuestionIndexProgress,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "question-index-progress"),
		&aiv1.AIKnowledgeBaseQuestionIndexProgress{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Percent:     percent,
			Message:     strings.TrimSpace(message),
			MessageType: strings.TrimSpace(messageType),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQuestionIndexCompleted(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQuestionIndexCompleted,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "question-index-completed"),
		&aiv1.AIKnowledgeBaseQuestionIndexCompleted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseQuestionIndexFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseQuestionIndexFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "question-index-failed"),
		&aiv1.AIKnowledgeBaseQuestionIndexFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryCreated(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseEntryRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryCreated,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-created"),
		&aiv1.AIKnowledgeBaseEntryCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryCreateFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryCreateFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-create-failed"),
		&aiv1.AIKnowledgeBaseEntryCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryUpdated(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	item *aiv1.AIKnowledgeBaseEntryRecord,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryUpdated,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-updated"),
		&aiv1.AIKnowledgeBaseEntryUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryUpdateFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryUpdateFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-update-failed"),
		&aiv1.AIKnowledgeBaseEntryUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryDeleted(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	knowledgeBaseID int64,
	entryID int64,
	hiddenIndex string,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryDeleted,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-deleted"),
		&aiv1.AIKnowledgeBaseEntryDeleted{
			OwnerUserId:                   strings.TrimSpace(ref.OwnerUserID),
			KnowledgeBaseId:               knowledgeBaseID,
			KnowledgeBaseEntryId:          entryID,
			KnowledgeBaseEntryHiddenIndex: strings.TrimSpace(hiddenIndex),
			Message:                       strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryDeleteFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryDeleteFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-delete-failed"),
		&aiv1.AIKnowledgeBaseEntryDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseVectorIndexBuilt(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	knowledgeBaseID int64,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseVectorIndexBuilt,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "vector-index-built"),
		&aiv1.AIKnowledgeBaseVectorIndexBuilt{
			OwnerUserId:     strings.TrimSpace(ref.OwnerUserID),
			KnowledgeBaseId: knowledgeBaseID,
			Message:         strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseVectorIndexBuildFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseVectorIndexBuildFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "vector-index-build-failed"),
		&aiv1.AIKnowledgeBaseVectorIndexBuildFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryVectorIndexBuilt(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	knowledgeBaseID int64,
	entryID int64,
	hiddenIndex string,
	message string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryVectorIndexBuilt,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-vector-index-built"),
		&aiv1.AIKnowledgeBaseEntryVectorIndexBuilt{
			OwnerUserId:                   strings.TrimSpace(ref.OwnerUserID),
			KnowledgeBaseId:               knowledgeBaseID,
			KnowledgeBaseEntryId:          entryID,
			KnowledgeBaseEntryHiddenIndex: strings.TrimSpace(hiddenIndex),
			Message:                       strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIKnowledgeBaseEntryVectorIndexBuildFailed(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishKnowledgeBase(
		ctx,
		legionEventAIKnowledgeBaseEntryVectorIndexBuildFailed,
		ref,
		knowledgeBaseEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "entry-vector-index-build-failed"),
		&aiv1.AIKnowledgeBaseEntryVectorIndexBuildFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityCreated(
	ctx context.Context,
	ref aiMemoryCommandRef,
	sessionID string,
	message string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityCreated,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "created"),
		&aiv1.AIMemoryEntityCreated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			SessionId:   strings.TrimSpace(sessionID),
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityCreateFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	sessionID string,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityCreateFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "create-failed"),
		&aiv1.AIMemoryEntityCreateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			SessionId:    strings.TrimSpace(sessionID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityFetched(
	ctx context.Context,
	ref aiMemoryCommandRef,
	item *aiv1.AIMemoryEntityRecord,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityFetched,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "fetched"),
		&aiv1.AIMemoryEntityFetched{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityFetchFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityFetchFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "fetch-failed"),
		&aiv1.AIMemoryEntityFetchFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntitiesQueried(
	ctx context.Context,
	ref aiMemoryCommandRef,
	pagination *aiv1.AIMemoryPagination,
	items []*aiv1.AIMemoryEntityRecord,
	total int64,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntitiesQueried,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "queried"),
		&aiv1.AIMemoryEntitiesQueried{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Pagination:  pagination,
			Items:       items,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntitiesQueryFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntitiesQueryFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "query-failed"),
		&aiv1.AIMemoryEntitiesQueryFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityUpdated(
	ctx context.Context,
	ref aiMemoryCommandRef,
	item *aiv1.AIMemoryEntityRecord,
	message string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityUpdated,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "updated"),
		&aiv1.AIMemoryEntityUpdated{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Item:        item,
			Message:     strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityUpdateFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityUpdateFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "update-failed"),
		&aiv1.AIMemoryEntityUpdateFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntitiesDeleted(
	ctx context.Context,
	ref aiMemoryCommandRef,
	affectedCount int64,
	message string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntitiesDeleted,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "deleted"),
		&aiv1.AIMemoryEntitiesDeleted{
			OwnerUserId:   strings.TrimSpace(ref.OwnerUserID),
			AffectedCount: affectedCount,
			Message:       strings.TrimSpace(message),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntitiesDeleteFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntitiesDeleteFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "delete-failed"),
		&aiv1.AIMemoryEntitiesDeleteFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityTagsCounted(
	ctx context.Context,
	ref aiMemoryCommandRef,
	sessionID string,
	tagsCount []*aiv1.AIMemoryTagCount,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityTagsCounted,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "tags-counted"),
		&aiv1.AIMemoryEntityTagsCounted{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			SessionId:   strings.TrimSpace(sessionID),
			TagsCount:   tagsCount,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIMemoryEntityTagsCountFailed(
	ctx context.Context,
	ref aiMemoryCommandRef,
	sessionID string,
	errorCode string,
	errorMessage string,
) error {
	return p.publishMemory(
		ctx,
		legionEventAIMemoryEntityTagsCountFailed,
		ref,
		memoryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "tags-count-failed"),
		&aiv1.AIMemoryEntityTagsCountFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			SessionId:    strings.TrimSpace(sessionID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIHTTPFlowsQueried(
	ctx context.Context,
	ref aiRuntimeQueryCommandRef,
	items []*aiv1.AIHTTPFlowRecord,
	pagination *aiv1.AIRuntimePagination,
	total int64,
) error {
	return p.publishRuntimeQuery(
		ctx,
		legionEventAIHTTPFlowsQueried,
		ref,
		runtimeQueryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "http-flows-queried"),
		&aiv1.AIHTTPFlowsQueried{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIHTTPFlowsQueryFailed(
	ctx context.Context,
	ref aiRuntimeQueryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishRuntimeQuery(
		ctx,
		legionEventAIHTTPFlowsQueryFailed,
		ref,
		runtimeQueryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "http-flows-query-failed"),
		&aiv1.AIHTTPFlowsQueryFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIRisksQueried(
	ctx context.Context,
	ref aiRuntimeQueryCommandRef,
	items []*aiv1.AIRiskRecord,
	pagination *aiv1.AIRuntimePagination,
	total int64,
) error {
	return p.publishRuntimeQuery(
		ctx,
		legionEventAIRisksQueried,
		ref,
		runtimeQueryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "risks-queried"),
		&aiv1.AIRisksQueried{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
			Pagination:  pagination,
			Total:       total,
		},
	)
}

func (p *aiSessionEventPublisher) PublishAIRisksQueryFailed(
	ctx context.Context,
	ref aiRuntimeQueryCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishRuntimeQuery(
		ctx,
		legionEventAIRisksQueryFailed,
		ref,
		runtimeQueryEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "risks-query-failed"),
		&aiv1.AIRisksQueryFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) publishProvider(
	ctx context.Context,
	eventType string,
	ref aiProviderCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai provider event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai provider event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishLogs(
	ctx context.Context,
	eventType string,
	ref aiLogsCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	correlationID := strings.TrimSpace(ref.SessionID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(ref.OwnerUserID)
	}
	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: correlationID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai logs event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai logs event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishFocus(
	ctx context.Context,
	eventType string,
	ref aiFocusCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai focus event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai focus event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishMaterials(
	ctx context.Context,
	eventType string,
	ref aiMaterialsCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai materials event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai materials event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishForge(
	ctx context.Context,
	eventType string,
	ref aiForgeCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai forge event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai forge event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishTool(
	ctx context.Context,
	eventType string,
	ref aiToolCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai tool event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai tool event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishMCP(
	ctx context.Context,
	eventType string,
	ref aiMCPCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai mcp event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai mcp event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishLocalModel(
	ctx context.Context,
	eventType string,
	ref aiLocalModelCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal local model event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish local model event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishKnowledgeBase(
	ctx context.Context,
	eventType string,
	ref aiKnowledgeBaseCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai knowledge base event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai knowledge base event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishMemory(
	ctx context.Context,
	eventType string,
	ref aiMemoryCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai memory event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai memory event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) publishRuntimeQuery(
	ctx context.Context,
	eventType string,
	ref aiRuntimeQueryCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai runtime query event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai runtime query event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) putObjectBytes(
	ctx context.Context,
	bucket string,
	key string,
	data []byte,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}

	store, err := js.ObjectStore(strings.TrimSpace(bucket))
	if err != nil {
		return fmt.Errorf("load object store %s: %w", bucket, err)
	}
	if _, err := store.PutBytes(strings.TrimSpace(key), data); err != nil {
		return fmt.Errorf("put ai object %s/%s: %w", bucket, key, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (p *aiSessionEventPublisher) getObjectBytes(
	ctx context.Context,
	bucket string,
	key string,
) ([]byte, error) {
	session, ok := p.node.GetSessionState()
	if !ok {
		return nil, ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return nil, err
	}

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return nil, fmt.Errorf("jetstream context is not ready")
	}

	store, err := js.ObjectStore(strings.TrimSpace(bucket))
	if err != nil {
		return nil, fmt.Errorf("load object store %s: %w", bucket, err)
	}
	data, err := store.GetBytes(strings.TrimSpace(key), nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("get ai object %s/%s: %w", bucket, key, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (p *aiSessionEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	p.closeLocked()

	conn, err := nats.Connect(natsURL, nats.Name("yak-node-ai-events-"+p.node.CurrentNodeID()))
	if err != nil {
		return fmt.Errorf("connect ai event nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build ai event jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func (p *aiSessionEventPublisher) closeLocked() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func attachAIEventMetadata(message proto.Message, metadata *nodev1.EventMetadata) error {
	switch value := message.(type) {
	case *aiv1.AISessionReady:
		value.Metadata = metadata
	case *aiv1.AISessionEvent:
		value.Metadata = metadata
	case *aiv1.AISessionDone:
		value.Metadata = metadata
	case *aiv1.AISessionFailed:
		value.Metadata = metadata
	case *aiv1.AISessionCancelled:
		value.Metadata = metadata
	case *aiv1.AISessionTitleUpdated:
		value.Metadata = metadata
	case *aiv1.AISessionTitleUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AISessionDeleteCompleted:
		value.Metadata = metadata
	case *aiv1.AISessionDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIProviderModelsListed:
		value.Metadata = metadata
	case *aiv1.AIProviderModelsFailed:
		value.Metadata = metadata
	case *aiv1.AIProviderHealthCheckCompleted:
		value.Metadata = metadata
	case *aiv1.AIProviderHealthCheckFailed:
		value.Metadata = metadata
	case *aiv1.AIFocusQueried:
		value.Metadata = metadata
	case *aiv1.AIFocusQueryFailed:
		value.Metadata = metadata
	case *aiv1.AIMaterialsRandomQueried:
		value.Metadata = metadata
	case *aiv1.AIMaterialsRandomQueryFailed:
		value.Metadata = metadata
	case *aiv1.AIGlobalConfigFetched:
		value.Metadata = metadata
	case *aiv1.AIGlobalConfigFetchFailed:
		value.Metadata = metadata
	case *aiv1.AIGlobalConfigUpdated:
		value.Metadata = metadata
	case *aiv1.AIGlobalConfigUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIMCPServersListed:
		value.Metadata = metadata
	case *aiv1.AIMCPServersListFailed:
		value.Metadata = metadata
	case *aiv1.AIMCPServerCreated:
		value.Metadata = metadata
	case *aiv1.AIMCPServerCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIMCPServerUpdated:
		value.Metadata = metadata
	case *aiv1.AIMCPServerUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIMCPServerDeleted:
		value.Metadata = metadata
	case *aiv1.AIMCPServerDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelsListed:
		value.Metadata = metadata
	case *aiv1.AILocalModelsListFailed:
		value.Metadata = metadata
	case *aiv1.AILlamaServerReadyChecked:
		value.Metadata = metadata
	case *aiv1.AILlamaServerReadyCheckFailed:
		value.Metadata = metadata
	case *aiv1.AILlamaServerInstalled:
		value.Metadata = metadata
	case *aiv1.AILlamaServerInstallFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelOperationAccepted:
		value.Metadata = metadata
	case *aiv1.AILocalModelOperationProgressed:
		value.Metadata = metadata
	case *aiv1.AILocalModelOperationCompleted:
		value.Metadata = metadata
	case *aiv1.AILocalModelOperationCancelled:
		value.Metadata = metadata
	case *aiv1.AILocalModelOperationFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelCreated:
		value.Metadata = metadata
	case *aiv1.AILocalModelCreateFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelUpdated:
		value.Metadata = metadata
	case *aiv1.AILocalModelUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelDeleted:
		value.Metadata = metadata
	case *aiv1.AILocalModelDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelStarted:
		value.Metadata = metadata
	case *aiv1.AILocalModelStartFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelStopped:
		value.Metadata = metadata
	case *aiv1.AILocalModelStopFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelDownloaded:
		value.Metadata = metadata
	case *aiv1.AILocalModelDownloadFailed:
		value.Metadata = metadata
	case *aiv1.AILocalModelsCleared:
		value.Metadata = metadata
	case *aiv1.AILocalModelsClearFailed:
		value.Metadata = metadata
	case *aiv1.AIForgesListed:
		value.Metadata = metadata
	case *aiv1.AIForgesListFailed:
		value.Metadata = metadata
	case *aiv1.AIForgeCreated:
		value.Metadata = metadata
	case *aiv1.AIForgeCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIForgeUpdated:
		value.Metadata = metadata
	case *aiv1.AIForgeUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIForgeDeleted:
		value.Metadata = metadata
	case *aiv1.AIForgeDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIForgeExportProgressed:
		value.Metadata = metadata
	case *aiv1.AIForgeExported:
		value.Metadata = metadata
	case *aiv1.AIForgeExportFailed:
		value.Metadata = metadata
	case *aiv1.AIForgeImportProgressed:
		value.Metadata = metadata
	case *aiv1.AIForgeImported:
		value.Metadata = metadata
	case *aiv1.AIForgeImportFailed:
		value.Metadata = metadata
	case *aiv1.AIToolsListed:
		value.Metadata = metadata
	case *aiv1.AIToolsListFailed:
		value.Metadata = metadata
	case *aiv1.AIToolCreated:
		value.Metadata = metadata
	case *aiv1.AIToolCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIToolUpdated:
		value.Metadata = metadata
	case *aiv1.AIToolUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIToolMetadataGenerated:
		value.Metadata = metadata
	case *aiv1.AIToolMetadataGenerateFailed:
		value.Metadata = metadata
	case *aiv1.AIToolFavoriteToggled:
		value.Metadata = metadata
	case *aiv1.AIToolFavoriteToggleFailed:
		value.Metadata = metadata
	case *aiv1.AIToolsDeleted:
		value.Metadata = metadata
	case *aiv1.AIToolsDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBasesListed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBasesListFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseCreated:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseImported:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseImportFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseUpdated:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseDeleted:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseExported:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseExportFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntriesSearched:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntriesSearchFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQueryByAIChunk:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQueryByAICompleted:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQueryByAIFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQuestionIndexProgress:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQuestionIndexCompleted:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseQuestionIndexFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryCreated:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryUpdated:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryDeleted:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseVectorIndexBuilt:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseVectorIndexBuildFailed:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryVectorIndexBuilt:
		value.Metadata = metadata
	case *aiv1.AIKnowledgeBaseEntryVectorIndexBuildFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityCreated:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityCreateFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityFetched:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityFetchFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntitiesQueried:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntitiesQueryFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityUpdated:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityUpdateFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntitiesDeleted:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntitiesDeleteFailed:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityTagsCounted:
		value.Metadata = metadata
	case *aiv1.AIMemoryEntityTagsCountFailed:
		value.Metadata = metadata
	case *aiv1.AIHTTPFlowsQueried:
		value.Metadata = metadata
	case *aiv1.AIHTTPFlowsQueryFailed:
		value.Metadata = metadata
	case *aiv1.AIRisksQueried:
		value.Metadata = metadata
	case *aiv1.AIRisksQueryFailed:
		value.Metadata = metadata
	case *aiv1.AILogsCheckpointsExported:
		value.Metadata = metadata
	case *aiv1.AILogsCheckpointsExportFailed:
		value.Metadata = metadata
	default:
		return fmt.Errorf("unsupported ai event message: %T", message)
	}
	return nil
}

func aiSessionProtoRef(ref aiSessionCommandRef) *aiv1.AISessionRef {
	return &aiv1.AISessionRef{
		SessionId: ref.SessionID,
		RunId:     ref.RunID,
	}
}

func eventIDWithSuffix(commandID string, sessionID string, suffix string) string {
	if commandID != "" {
		return commandID + ":" + suffix
	}
	if sessionID != "" {
		return sessionID + ":" + suffix + ":" + uuid.NewString()
	}
	return uuid.NewString()
}

func providerEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "provider"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func focusEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "focus"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func materialsEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "materials"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func forgeEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "forge"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func toolEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "tool"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func mcpEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "mcp"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func localModelEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "local-model"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func knowledgeBaseEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "knowledge-base"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func memoryEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "memory"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func runtimeQueryEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "runtime-query"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func logsEventIDWithSuffix(commandID string, sessionID string, suffix string) string {
	base := strings.TrimSpace(sessionID)
	if base == "" {
		base = "logs"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func logPublishedAISessionEvent(eventType string, sessionID string, message proto.Message) {
	runtimeType, seq := extractAISessionRuntimeLogFields(message)
	if shouldDebugAISessionRuntimeEvent(runtimeType) {
		log.Debugf(
			"published legion ai session event: type=%s runtime_type=%s seq=%d session_id=%s",
			eventType,
			runtimeType,
			seq,
			sessionID,
		)
		return
	}
	if runtimeType != "" {
		log.Infof(
			"published legion ai session event: type=%s runtime_type=%s seq=%d session_id=%s",
			eventType,
			runtimeType,
			seq,
			sessionID,
		)
		return
	}
	log.Infof("published legion ai session event: type=%s session_id=%s", eventType, sessionID)
}

func extractAISessionRuntimeLogFields(message proto.Message) (string, uint64) {
	event, ok := message.(*aiv1.AISessionEvent)
	if !ok {
		return "", 0
	}
	return strings.TrimSpace(event.GetEventType()), event.GetSeq()
}

func shouldDebugAISessionRuntimeEvent(runtimeType string) bool {
	switch strings.TrimSpace(runtimeType) {
	case aiSessionRuntimeEventDelta,
		aiSessionRuntimeEventThought,
		aiSessionRuntimeEventMessage,
		aiSessionRuntimeEventToolCall,
		aiSessionRuntimeEventToolResult,
		"consumption":
		return true
	default:
		return false
	}
}
