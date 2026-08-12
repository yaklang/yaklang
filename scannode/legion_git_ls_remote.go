package scannode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
)

type gitLsRemoteCommandRef struct {
	CommandID string
	NodeID    string
}

type gitLsRemoteEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newGitLsRemoteEventPublisher(base *node.NodeBase) *gitLsRemoteEventPublisher {
	return &gitLsRemoteEventPublisher{node: base}
}

func (p *gitLsRemoteEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func (b *legionJobBridge) handleSSAGitLsRemote(ctx context.Context, raw []byte) error {
	var command ssav1.GitLsRemoteCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal SSA git ls-remote command: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	ref := gitLsRemoteCommandRefFromCommand(currentNodeID, &command)
	if err := validateSSAGitLsRemoteCommand(currentNodeID, &command); err != nil {
		return b.gitLsRemotePublisher.PublishFailed(
			ctx,
			ref,
			"invalid_ssa_git_ls_remote_command",
			err.Error(),
		)
	}

	result, err := executeSSAGitLsRemote(ctx, &command)
	if err != nil {
		return b.gitLsRemotePublisher.PublishFailed(
			ctx,
			ref,
			"ssa_git_ls_remote_failed",
			err.Error(),
		)
	}
	return b.gitLsRemotePublisher.PublishCompleted(ctx, ref, result)
}

func validateSSAGitLsRemoteCommand(nodeID string, command *ssav1.GitLsRemoteCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ssa git ls-remote metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ssa git ls-remote command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ssa git ls-remote target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ssa git ls-remote target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetSourceLocator()) == "":
		return fmt.Errorf("ssa git ls-remote source_locator is required")
	case !command.GetIncludeHead() && !command.GetIncludeTags():
		return fmt.Errorf("ssa git ls-remote requires include_head and/or include_tags")
	default:
		return nil
	}
}

func gitLsRemoteCommandRefFromCommand(nodeID string, command *ssav1.GitLsRemoteCommand) gitLsRemoteCommandRef {
	return gitLsRemoteCommandRef{
		CommandID: strings.TrimSpace(command.GetMetadata().GetCommandId()),
		NodeID:    strings.TrimSpace(nodeID),
	}
}

type gitLsRemoteResult struct {
	HeadSHA string
	Tags    map[string]string
	Success bool
	Message string
}

func executeSSAGitLsRemote(ctx context.Context, command *ssav1.GitLsRemoteCommand) (gitLsRemoteResult, error) {
	opts, err := yakgitAuthOptions(command.GetAuth())
	if err != nil {
		return gitLsRemoteResult{}, err
	}
	opts = append(opts, yakgit.WithContext(ctx))
	if branch := strings.TrimSpace(command.GetBranch()); branch != "" {
		opts = append(opts, yakgit.WithBranch(branch))
	}

	refs, err := yakgit.ListRemote(strings.TrimSpace(command.GetSourceLocator()), opts...)
	if err != nil {
		return gitLsRemoteResult{}, err
	}

	out := gitLsRemoteResult{
		Success: true,
		Tags:    map[string]string{},
	}
	if command.GetIncludeHead() {
		out.HeadSHA = strings.TrimSpace(refs.HeadSHA)
		if out.HeadSHA == "" {
			return gitLsRemoteResult{}, fmt.Errorf("remote HEAD SHA not resolved")
		}
	}
	if command.GetIncludeTags() {
		out.Tags = refs.Tags
		if out.Tags == nil {
			out.Tags = map[string]string{}
		}
	}

	sourcePath := strings.TrimSpace(command.GetSourcePath())
	if sourcePath != "" {
		if !command.GetIncludeHead() {
			return gitLsRemoteResult{}, fmt.Errorf("source_path validation requires include_head")
		}
		if err := validateRemoteSourcePath(ctx, command, opts, sourcePath); err != nil {
			out.Success = false
			out.Message = err.Error()
			return out, nil
		}
		out.Message = "连接成功，仓库、分支和子路径可访问。"
	} else if command.GetIncludeHead() {
		if strings.TrimSpace(command.GetBranch()) != "" {
			out.Message = "连接成功，仓库和分支可访问。"
		} else {
			out.Message = "连接成功，仓库可访问。"
		}
	} else {
		out.Message = "连接成功，仓库标签可访问。"
	}
	return out, nil
}

func yakgitAuthOptions(auth *ssav1.GitAuthSnapshot) ([]yakgit.Option, error) {
	if auth == nil {
		return nil, nil
	}
	kind := strings.TrimSpace(auth.GetKind())
	if kind == "" || kind == "none" {
		return nil, nil
	}
	username := strings.TrimSpace(auth.GetUsername())
	switch kind {
	case "password":
		password := auth.GetPassword()
		if username == "" || password == "" {
			return nil, fmt.Errorf("password auth requires username and password")
		}
		return []yakgit.Option{yakgit.WithUsernamePassword(username, password)}, nil
	case "token":
		token := auth.GetToken()
		if token == "" {
			return nil, fmt.Errorf("token auth requires token")
		}
		if username == "" {
			username = "git"
		}
		return []yakgit.Option{yakgit.WithUsernamePassword(username, token)}, nil
	case "ssh_key":
		key := auth.GetPrivateKey()
		if key == "" {
			return nil, fmt.Errorf("ssh_key auth requires private_key")
		}
		if username == "" {
			username = "git"
		}
		return []yakgit.Option{
			yakgit.WithPrivateKeyContent(username, key, ""),
			yakgit.WithInsecureIgnoreHostKey(),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported auth kind: %s", kind)
	}
}

func validateRemoteSourcePath(
	ctx context.Context,
	command *ssav1.GitLsRemoteCommand,
	authOpts []yakgit.Option,
	sourcePath string,
) error {
	workDir, err := os.MkdirTemp("", "legion-git-ls-remote-path-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	repoDir := filepath.Join(workDir, "repo")
	opts := append([]yakgit.Option{}, authOpts...)
	opts = append(opts, yakgit.WithContext(ctx), yakgit.WithDepth(1))
	if branch := strings.TrimSpace(command.GetBranch()); branch != "" {
		opts = append(opts, yakgit.WithBranch(branch))
	}
	if err := yakgit.Clone(strings.TrimSpace(command.GetSourceLocator()), repoDir, opts...); err != nil {
		return fmt.Errorf("clone for path validation: %w", err)
	}

	checkPath := filepath.Join(repoDir, filepath.FromSlash(sourcePath))
	if _, err := os.Stat(checkPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("仓库子路径不存在，或当前分支下无法访问该路径")
		}
		return fmt.Errorf("check source path: %w", err)
	}
	return nil
}

func (p *gitLsRemoteEventPublisher) PublishCompleted(
	ctx context.Context,
	ref gitLsRemoteCommandRef,
	result gitLsRemoteResult,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}
	eventID := ref.CommandID + ":completed"
	event := &ssav1.GitLsRemoteCompleted{
		HeadSha: result.HeadSHA,
		Tags:    result.Tags,
		Success: result.Success,
		Message: result.Message,
	}
	return p.publish(ctx, session, ref, eventID, legionEventSSAGitLsRemoteCompleted, event)
}

func (p *gitLsRemoteEventPublisher) PublishFailed(
	ctx context.Context,
	ref gitLsRemoteCommandRef,
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
	event := &ssav1.GitLsRemoteFailed{
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}
	return p.publish(ctx, session, ref, eventID, legionEventSSAGitLsRemoteFailed, event)
}

func (p *gitLsRemoteEventPublisher) publish(
	ctx context.Context,
	session node.SessionState,
	ref gitLsRemoteCommandRef,
	eventID string,
	eventType string,
	message proto.Message,
) error {
	_ = ctx
	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.NodeID + ":ssa-git-ls-remote",
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachGitLsRemoteMetadata(message, metadata); err != nil {
		return err
	}
	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal SSA git ls-remote event: %w", err)
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
		return fmt.Errorf("publish SSA git ls-remote event %s: %w", eventType, err)
	}
	log.Infof("published SSA git ls-remote event: type=%s node_id=%s", eventType, ref.NodeID)
	return nil
}

func (p *gitLsRemoteEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	if p.conn != nil {
		p.conn.Close()
	}
	conn, err := nats.Connect(natsURL, nats.Name("yak-node-ssa-git-ls-remote-"+p.node.CurrentNodeID()))
	if err != nil {
		return fmt.Errorf("connect SSA git ls-remote nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build SSA git ls-remote jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func attachGitLsRemoteMetadata(message proto.Message, metadata *nodev1.EventMetadata) error {
	switch event := message.(type) {
	case *ssav1.GitLsRemoteCompleted:
		event.Metadata = metadata
	case *ssav1.GitLsRemoteFailed:
		event.Metadata = metadata
	default:
		return fmt.Errorf("unsupported SSA git ls-remote event type %T", message)
	}
	return nil
}
