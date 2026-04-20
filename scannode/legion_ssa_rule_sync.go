package scannode

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
)

const yaklangRuleArchiveFormat = "yaklang_syntaxflow_archive.zip.v1"

type ssaRuleSyncCommandRef struct {
	CommandID string
	NodeID    string
}

type exportedRuleArchive struct {
	Data          []byte
	ContentSHA256 string
	RawSizeBytes  uint64
	RuleCount     int
	GroupCount    int
	RiskTypeCount int
	ExportedAt    time.Time
}

type ssaRuleSyncEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newSSARuleSyncEventPublisher(base *node.NodeBase) *ssaRuleSyncEventPublisher {
	return &ssaRuleSyncEventPublisher{node: base}
}

func (p *ssaRuleSyncEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func (b *legionJobBridge) handleSSARuleSyncExport(ctx context.Context, raw []byte) error {
	var command ssav1.ExportRuleCatalogCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal SSA rule sync export command: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	ref := ssaRuleSyncCommandRefFromCommand(currentNodeID, &command)
	if err := validateSSARuleSyncExportCommand(currentNodeID, &command); err != nil {
		return b.ruleSyncPublisher.PublishFailed(
			ctx,
			ref,
			"invalid_ssa_rule_sync_command",
			err.Error(),
		)
	}

	archive, err := exportNodeRuleArchive(ctx)
	if err != nil {
		log.Errorf(
			"export SSA rule archive failed: node_id=%s command_id=%s err=%v",
			ref.NodeID,
			ref.CommandID,
			err,
		)
		return b.ruleSyncPublisher.PublishFailed(
			ctx,
			ref,
			"ssa_rule_archive_export_failed",
			err.Error(),
		)
	}

	return b.ruleSyncPublisher.PublishReady(ctx, ref, &command, archive)
}

func (p *ssaRuleSyncEventPublisher) PublishReady(
	ctx context.Context,
	ref ssaRuleSyncCommandRef,
	command *ssav1.ExportRuleCatalogCommand,
	archive exportedRuleArchive,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	objectStoreBucket := strings.TrimSpace(command.GetObjectStoreBucket())
	objectStoreKey := strings.TrimSpace(command.GetObjectStoreKey())
	if objectStoreBucket == "" || objectStoreKey == "" {
		return fmt.Errorf("SSA rule sync object store bucket/key is required")
	}

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	store, err := js.ObjectStore(objectStoreBucket)
	if err != nil {
		return fmt.Errorf("load object store %s: %w", objectStoreBucket, err)
	}
	if _, err := store.PutBytes(objectStoreKey, archive.Data); err != nil {
		return fmt.Errorf("put SSA rule sync archive %s/%s: %w", objectStoreBucket, objectStoreKey, err)
	}

	eventID := ref.CommandID + ":ready"
	event := &ssav1.RuleCatalogExportReady{
		ObjectStoreBucket: objectStoreBucket,
		ObjectStoreKey:    objectStoreKey,
		ArchiveFormat:     yaklangRuleArchiveFormat,
		ContentSha256:     archive.ContentSHA256,
		RawSizeBytes:      archive.RawSizeBytes,
		RuleCount:         uint32(archive.RuleCount),
		GroupCount:        uint32(archive.GroupCount),
		RiskTypeCount:     uint32(archive.RiskTypeCount),
		ExportedAt:        timestamppb.New(archive.ExportedAt),
		ActivateSnapshot:  command.GetActivateSnapshot(),
	}
	return p.publish(ctx, session, ref, eventID, legionEventSSARuleSyncReady, event)
}

func (p *ssaRuleSyncEventPublisher) PublishFailed(
	ctx context.Context,
	ref ssaRuleSyncCommandRef,
	errorCode string,
	errorMessage string,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	eventID := ref.CommandID + ":failed"
	event := &ssav1.RuleCatalogExportFailed{
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}
	return p.publish(ctx, session, ref, eventID, legionEventSSARuleSyncFailed, event)
}

func (p *ssaRuleSyncEventPublisher) publish(
	ctx context.Context,
	session node.SessionState,
	ref ssaRuleSyncCommandRef,
	eventID string,
	eventType string,
	message proto.Message,
) error {
	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.NodeID + ":ssa-rule-sync",
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachSSARuleSyncMetadata(message, metadata); err != nil {
		return err
	}
	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal SSA rule sync event: %w", err)
	}

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish SSA rule sync event %s: %w", eventType, err)
	}
	log.Infof("published SSA rule sync event: type=%s node_id=%s", eventType, ref.NodeID)
	return nil
}

func (p *ssaRuleSyncEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	if p.conn != nil {
		p.conn.Close()
	}
	conn, err := nats.Connect(natsURL, nats.Name("yak-node-ssa-rule-sync-"+p.node.CurrentNodeID()))
	if err != nil {
		return fmt.Errorf("connect SSA rule sync nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build SSA rule sync jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func exportNodeRuleArchive(ctx context.Context) (exportedRuleArchive, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return exportedRuleArchive{}, fmt.Errorf("local profile database is not available")
	}
	if err := sfbuildin.SyncEmbedRule(); err != nil {
		return exportedRuleArchive{}, fmt.Errorf("sync embedded syntax flow rules: %w", err)
	}

	data, result, err := sfdb.ExportRulesToBytes(ctx, db)
	if err != nil {
		return exportedRuleArchive{}, err
	}
	inventory, err := inspectExportedRuleArchive(data)
	if err != nil {
		return exportedRuleArchive{}, err
	}
	if result != nil && result.Count > 0 {
		inventory.RuleCount = result.Count
	}
	sum := sha256.Sum256(data)
	inventory.Data = data
	inventory.ContentSHA256 = hex.EncodeToString(sum[:])
	inventory.RawSizeBytes = uint64(len(data))
	inventory.ExportedAt = time.Now().UTC()
	return inventory, nil
}

type exportArchiveMetadata struct {
	Relationship []exportArchiveRelationship `json:"relationship"`
}

type exportArchiveRelationship struct {
	RuleID     string   `json:"rule_id"`
	GroupNames []string `json:"group_names"`
}

type exportArchiveRuleRecord struct {
	RiskType string `json:"RiskType"`
}

func inspectExportedRuleArchive(data []byte) (exportedRuleArchive, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return exportedRuleArchive{}, fmt.Errorf("open exported SSA rule archive: %w", err)
	}

	groupSet := map[string]struct{}{}
	riskTypeSet := map[string]struct{}{}
	ruleCount := 0

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		payload, err := readRuleArchiveFile(file)
		if err != nil {
			return exportedRuleArchive{}, err
		}
		if filepath.Base(file.Name) == "meta.json" {
			var metadata exportArchiveMetadata
			if err := json.Unmarshal(payload, &metadata); err != nil {
				return exportedRuleArchive{}, fmt.Errorf("decode SSA rule archive metadata: %w", err)
			}
			for _, relationship := range metadata.Relationship {
				for _, groupName := range relationship.GroupNames {
					trimmed := strings.TrimSpace(groupName)
					if trimmed == "" {
						continue
					}
					groupSet[trimmed] = struct{}{}
				}
			}
			continue
		}

		var rule exportArchiveRuleRecord
		if err := json.Unmarshal(payload, &rule); err != nil {
			return exportedRuleArchive{}, fmt.Errorf("decode SSA rule archive rule %s: %w", file.Name, err)
		}
		ruleCount++
		if trimmed := strings.TrimSpace(rule.RiskType); trimmed != "" {
			riskTypeSet[trimmed] = struct{}{}
		}
	}

	return exportedRuleArchive{
		RuleCount:     ruleCount,
		GroupCount:    len(groupSet),
		RiskTypeCount: len(riskTypeSet),
	}, nil
}

func readRuleArchiveFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open exported SSA rule archive file %s: %w", file.Name, err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read exported SSA rule archive file %s: %w", file.Name, err)
	}
	return payload, nil
}

func validateSSARuleSyncExportCommand(nodeID string, command *ssav1.ExportRuleCatalogCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("SSA rule sync metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("SSA rule sync command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("SSA rule sync target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("SSA rule sync target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetObjectStoreBucket()) == "":
		return fmt.Errorf("SSA rule sync object_store_bucket is required")
	case strings.TrimSpace(command.GetObjectStoreKey()) == "":
		return fmt.Errorf("SSA rule sync object_store_key is required")
	default:
		return nil
	}
}

func ssaRuleSyncCommandRefFromCommand(
	nodeID string,
	command *ssav1.ExportRuleCatalogCommand,
) ssaRuleSyncCommandRef {
	ref := ssaRuleSyncCommandRef{NodeID: nodeID}
	if command == nil {
		return ref
	}
	if command.GetMetadata() != nil {
		ref.CommandID = command.GetMetadata().GetCommandId()
	}
	if targetNodeID := strings.TrimSpace(command.GetTargetNodeId()); targetNodeID != "" {
		ref.NodeID = targetNodeID
	}
	return ref
}

func attachSSARuleSyncMetadata(
	message proto.Message,
	metadata *nodev1.EventMetadata,
) error {
	switch value := message.(type) {
	case *ssav1.RuleCatalogExportReady:
		value.Metadata = metadata
	case *ssav1.RuleCatalogExportFailed:
		value.Metadata = metadata
	default:
		return fmt.Errorf("unsupported SSA rule sync message: %T", message)
	}
	return nil
}
