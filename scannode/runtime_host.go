package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	AIRuntimeHostCapabilityKey = "ai.runtime.host.v1"
	runtimeHostSpecVersion     = "v1"
	runtimeHostReplyPrefix     = "legion.realtime.ai.runtime."
	runtimeHostCleanupLabel    = "legion.ai.runtime.cleanup_key"
	runtimeHostLeaseLabel      = "legion.ai.runtime.lease_token"
	runtimeHostSessionLabel    = "legion.ai.runtime.session_id"
	runtimeHostReleaseLabel    = "legion.ai.runtime.release_id"
	runtimeHostOwnerLabel      = "legion.ai.runtime.owner"
)

var runtimeHostIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,191}$`)
var runtimeHostSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var runtimeHostReleaseIDPattern = regexp.MustCompile(`^sha256-[a-f0-9]{64}$`)

type RuntimeHostConfig struct {
	Enabled                   bool
	BaseDir                   string
	PlatformAPIBaseURL        string
	RuntimePlatformAPIBaseURL string
	EnrollmentToken           string
	AgentInstallationID       string
	EngineReleaseID           string
	EngineDigest              string
	Network                   string
	HTTPClient                *http.Client
	NodeIDProvider            func() string
	SessionProvider           func() (node.SessionState, bool)
	docker                    runtimeHostDocker
}

type runtimeHostExecutor struct {
	docker                    runtimeHostDocker
	baseDir                   string
	platformAPIBaseURL        string
	runtimePlatformAPIBaseURL string
	enrollmentToken           string
	agentInstallationID       string
	engineReleaseID           string
	engineDigest              string
	network                   string
	httpClient                *http.Client
	nodeIDProvider            func() string
	sessionProvider           func() (node.SessionState, bool)
	operations                map[string]runtimeHostOperationRecord
	images                    map[string]runtimeHostImageRecord
	mu                        sync.Mutex
}

type runtimeHostStatusDetail struct {
	ReasonCode string    `json:"reason_code,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

type runtimeHostCommandRejectedError struct{ cause error }

func (e *runtimeHostCommandRejectedError) Error() string { return e.cause.Error() }
func (e *runtimeHostCommandRejectedError) Unwrap() error { return e.cause }

func rejectRuntimeHostCommand(err error) error {
	return &runtimeHostCommandRejectedError{cause: err}
}

func newRuntimeHostExecutor(cfg RuntimeHostConfig) (*runtimeHostExecutor, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("runtime host is disabled")
	}
	dockerClient := cfg.docker
	var err error
	if dockerClient == nil {
		dockerClient, err = newLocalRuntimeHostDocker()
		if err != nil {
			return nil, err
		}
	}
	network := strings.TrimSpace(cfg.Network)
	if network == "" {
		network = "bridge"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	platformAPIBaseURL := strings.TrimRight(strings.TrimSpace(cfg.PlatformAPIBaseURL), "/")
	runtimePlatformAPIBaseURL := strings.TrimRight(strings.TrimSpace(cfg.RuntimePlatformAPIBaseURL), "/")
	if runtimePlatformAPIBaseURL == "" {
		runtimePlatformAPIBaseURL = platformAPIBaseURL
	}
	executor := &runtimeHostExecutor{
		docker:                    dockerClient,
		baseDir:                   strings.TrimSpace(cfg.BaseDir),
		platformAPIBaseURL:        platformAPIBaseURL,
		runtimePlatformAPIBaseURL: runtimePlatformAPIBaseURL,
		enrollmentToken:           strings.TrimSpace(cfg.EnrollmentToken),
		agentInstallationID:       strings.TrimSpace(cfg.AgentInstallationID),
		engineReleaseID:           strings.TrimSpace(cfg.EngineReleaseID),
		engineDigest:              strings.ToLower(strings.TrimSpace(cfg.EngineDigest)),
		network:                   network,
		httpClient:                client,
		nodeIDProvider:            cfg.NodeIDProvider,
		sessionProvider:           cfg.SessionProvider,
	}
	if executor.baseDir == "" {
		return nil, fmt.Errorf("runtime host base directory is required")
	}
	if !runtimeHostURLHasScheme(executor.platformAPIBaseURL, "http", "https") ||
		!runtimeHostURLHasScheme(executor.runtimePlatformAPIBaseURL, "http", "https") {
		return nil, fmt.Errorf("runtime host platform API URLs must use http or https")
	}
	if executor.enrollmentToken == "" || executor.agentInstallationID == "" ||
		executor.nodeIDProvider == nil || executor.sessionProvider == nil {
		return nil, fmt.Errorf("runtime host authenticated node identity is required")
	}
	if !runtimeHostReleaseIDPattern.MatchString(executor.engineReleaseID) ||
		!runtimeHostSHA256Pattern.MatchString(executor.engineDigest) {
		return nil, fmt.Errorf("runtime host installed release identity is required")
	}
	if err := executor.loadOperationJournal(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := executor.recoverOwnedContainers(ctx); err != nil {
		return nil, err
	}
	return executor, nil
}

func (e *runtimeHostExecutor) Close() error {
	if e == nil || e.docker == nil {
		return nil
	}
	return e.docker.Close()
}

func (e *runtimeHostExecutor) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	if e == nil {
		return CapabilityRuntimeStatus{}, false
	}
	now := time.Now().UTC()
	status := "ready"
	message := "AI Runtime Host is ready"
	reason := ""
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := e.docker.Ping(ctx); err != nil {
		status = "unavailable"
		reason = classifyRuntimeHostDockerError(err)
		message = "Local Docker is unavailable"
	}
	detail, _ := json.Marshal(runtimeHostStatusDetail{ReasonCode: reason, ObservedAt: now})
	return CapabilityRuntimeStatus{
		CapabilityKey: AIRuntimeHostCapabilityKey,
		SpecVersion:   runtimeHostSpecVersion,
		Status:        status,
		Message:       message,
		DetailJSON:    detail,
		ObservedAt:    now,
	}, true
}

func classifyRuntimeHostDockerError(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "docker_missing"
	}
	message := strings.ToLower(err.Error())
	if errors.Is(err, os.ErrPermission) || strings.Contains(message, "permission denied") {
		return "docker_permission_denied"
	}
	if _, statErr := os.Stat("/var/run/docker.sock"); errors.Is(statErr, os.ErrNotExist) {
		return "docker_missing"
	}
	return "docker_stopped"
}

func (b *legionJobBridge) handleAIRuntimeHostCommand(ctx context.Context, payload []byte) error {
	if b == nil || b.agent == nil || b.agent.runtimeHost == nil {
		return rejectRuntimeHostCommand(fmt.Errorf("AI Runtime Host capability is not enabled"))
	}
	command := &nodev1.AIRuntimeCommand{}
	if err := proto.Unmarshal(payload, command); err != nil {
		return rejectRuntimeHostCommand(fmt.Errorf("decode AI Runtime Host command: %w", err))
	}
	session, ok := b.agent.node.GetSessionState()
	if !ok {
		return rejectRuntimeHostCommand(fmt.Errorf("node session is unavailable"))
	}
	if err := b.agent.runtimeHost.validateCommand(command, session); err != nil {
		// Authentication, expiry, target-session and fixed-spec failures cannot
		// become valid on redelivery. Mark them terminal so an old node-session
		// message cannot occupy the durable consumer after a restart.
		return rejectRuntimeHostCommand(err)
	}
	result := b.agent.runtimeHost.execute(ctx, command, session)
	if err := signRuntimeHostResult(result, b.agent.runtimeHost.enrollmentToken, session.SessionID); err != nil {
		return err
	}
	encoded, err := proto.Marshal(result)
	if err != nil {
		return err
	}
	b.mu.Lock()
	consumer := b.consumer
	b.mu.Unlock()
	if consumer == nil || consumer.conn == nil {
		return fmt.Errorf("runtime host reply transport is unavailable")
	}
	return consumer.conn.Publish(command.ReplySubject, encoded)
}

func (e *runtimeHostExecutor) validateCommand(command *nodev1.AIRuntimeCommand, session node.SessionState) error {
	if err := verifyRuntimeHostCommand(command, e.enrollmentToken, session.SessionID); err != nil {
		return err
	}
	if command.Metadata == nil || strings.TrimSpace(command.Metadata.CommandId) == "" {
		return fmt.Errorf("runtime host command metadata is required")
	}
	expectedCommandType := runtimeHostCommandType(command.Operation)
	if expectedCommandType == "" || command.Metadata.CommandType != expectedCommandType ||
		command.Metadata.TraceId != command.AiSessionId {
		return fmt.Errorf("runtime host command type does not match its operation")
	}
	now := time.Now().UTC()
	if command.Metadata.IssuedAt == nil || command.Metadata.ExpireAt == nil ||
		!command.Metadata.ExpireAt.AsTime().After(command.Metadata.IssuedAt.AsTime()) ||
		command.Metadata.ExpireAt.AsTime().Sub(command.Metadata.IssuedAt.AsTime()) > time.Minute ||
		now.Before(command.Metadata.IssuedAt.AsTime().Add(-10*time.Second)) ||
		now.After(command.Metadata.ExpireAt.AsTime()) {
		return fmt.Errorf("runtime host command validity window is invalid")
	}
	if command.Target == nil || command.Target.NodeId != strings.TrimSpace(e.nodeIDProvider()) || command.Target.NodeSessionId != session.SessionID {
		return fmt.Errorf("runtime host command targets another node session")
	}
	if command.ReplySubject != runtimeHostReplyPrefix+command.Metadata.CommandId ||
		!runtimeHostIdentifierPattern.MatchString(command.Metadata.CommandId) {
		return fmt.Errorf("runtime host reply subject is invalid")
	}
	if !runtimeHostIdentifierPattern.MatchString(command.AiSessionId) || !runtimeHostIdentifierPattern.MatchString(command.CleanupKey) || !runtimeHostIdentifierPattern.MatchString(command.LeaseToken) {
		return fmt.Errorf("runtime host command identity is invalid")
	}
	if err := validateRuntimeHostRelease(command.Release); err != nil {
		return err
	}
	if command.Release.ReleaseId != e.engineReleaseID ||
		!strings.EqualFold(command.Release.EngineDigest, e.engineDigest) {
		return fmt.Errorf("runtime release does not match the installed node release")
	}
	if command.Operation == nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_START {
		return e.validateContainerSpec(command.Container, command.AiSessionId)
	}
	return nil
}

func validateRuntimeHostRelease(release *nodev1.AIRuntimeRelease) error {
	if release == nil || !runtimeHostReleaseIDPattern.MatchString(release.ReleaseId) {
		return fmt.Errorf("runtime release identity is invalid")
	}
	if !runtimeHostSHA256Pattern.MatchString(strings.ToLower(release.EngineDigest)) ||
		!runtimeHostSHA256Pattern.MatchString(strings.ToLower(release.ArchiveSha256)) ||
		release.ArchiveSize <= 0 {
		return fmt.Errorf("runtime release digest is invalid")
	}
	if !strings.HasPrefix(strings.ToLower(release.ImageId), "sha256:") ||
		!runtimeHostSHA256Pattern.MatchString(strings.TrimPrefix(strings.ToLower(release.ImageId), "sha256:")) {
		return fmt.Errorf("runtime image identity is invalid")
	}
	return nil
}

func runtimeHostCommandType(operation nodev1.AIRuntimeOperation) string {
	switch operation {
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_ENSURE_IMAGE:
		return "ai.runtime.image.ensure"
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_START:
		return "ai.runtime.container.start"
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_INSPECT:
		return "ai.runtime.container.inspect"
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_STOP:
		return "ai.runtime.container.stop"
	default:
		return ""
	}
}

func (e *runtimeHostExecutor) validateContainerSpec(spec *nodev1.AIRuntimeContainerSpec, sessionID string) error {
	if spec == nil || spec.ContainerName != "legion-ai-session-"+sessionID {
		return fmt.Errorf("runtime container name is invalid")
	}
	if strings.TrimSpace(spec.Network) != e.network {
		return fmt.Errorf("runtime container network is not allowed")
	}
	allowedEnvironment := map[string]struct{}{
		"LEGION_NATS_URL": {}, "LEGION_API_URL": {}, "LEGION_ENROLLMENT_TOKEN": {},
		"LEGION_SESSIONMGR_URL": {}, "LEGION_AI_SESSION_ID": {}, "LEGION_AI_RUNTIME": {},
	}
	for key, value := range spec.Environment {
		if _, ok := allowedEnvironment[key]; !ok || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("runtime container environment is not allowed")
		}
	}
	if spec.Environment["LEGION_AI_SESSION_ID"] != sessionID || strings.TrimSpace(spec.Environment["LEGION_ENROLLMENT_TOKEN"]) == "" {
		return fmt.Errorf("runtime container startup credential is missing")
	}
	runtimePlatformAPIBaseURL := e.runtimePlatformAPIBaseURL
	if runtimePlatformAPIBaseURL == "" {
		runtimePlatformAPIBaseURL = e.platformAPIBaseURL
	}
	if strings.TrimRight(spec.Environment["LEGION_API_URL"], "/") != runtimePlatformAPIBaseURL ||
		spec.Environment["LEGION_AI_RUNTIME"] != "stateless" ||
		!runtimeHostURLHasScheme(spec.Environment["LEGION_SESSIONMGR_URL"], "http", "https") ||
		!runtimeHostURLHasScheme(spec.Environment["LEGION_NATS_URL"], "nats") {
		return fmt.Errorf("runtime container endpoints do not match the fixed startup contract")
	}
	allowedFlags := map[string]struct{}{
		"-api-url": {}, "-enrollment-token": {}, "-kind": {}, "-agent-installation-id": {}, "-name": {},
	}
	if len(spec.Arguments)%2 != 0 || len(spec.Arguments) > len(allowedFlags)*2 {
		return fmt.Errorf("runtime container arguments are invalid")
	}
	seen := map[string]struct{}{}
	for index := 0; index < len(spec.Arguments); index += 2 {
		flag, value := spec.Arguments[index], spec.Arguments[index+1]
		if _, ok := allowedFlags[flag]; !ok || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("runtime container argument is not allowed")
		}
		if _, duplicate := seen[flag]; duplicate {
			return fmt.Errorf("runtime container argument is duplicated")
		}
		seen[flag] = struct{}{}
	}
	if !runtimeHostArgumentsMatch(spec.Arguments, "-kind", "ai_session") ||
		!runtimeHostArgumentsMatch(spec.Arguments, "-name", spec.ContainerName) ||
		!runtimeHostArgumentsMatch(spec.Arguments, "-enrollment-token", spec.Environment["LEGION_ENROLLMENT_TOKEN"]) ||
		!runtimeHostArgumentsMatch(spec.Arguments, "-agent-installation-id", "ai-session-"+sessionID) ||
		!runtimeHostArgumentsMatch(spec.Arguments, "-api-url", spec.Environment["LEGION_API_URL"]) {
		return fmt.Errorf("runtime container arguments do not match the fixed startup contract")
	}
	return nil
}

func runtimeHostURLHasScheme(value string, schemes ...string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return true
		}
	}
	return false
}

func runtimeHostArgumentsMatch(arguments []string, flag, expected string) bool {
	for index := 0; index+1 < len(arguments); index += 2 {
		if arguments[index] == flag {
			return arguments[index+1] == expected
		}
	}
	return false
}

func (e *runtimeHostExecutor) execute(ctx context.Context, command *nodev1.AIRuntimeCommand, session node.SessionState) *nodev1.AIRuntimeResult {
	result := &nodev1.AIRuntimeResult{
		Metadata: &nodev1.EventMetadata{
			EventId: uuid.NewString(), EventType: "ai.runtime.result", CausationId: command.Metadata.CommandId,
			CorrelationId: command.AiSessionId, EmittedAt: timestamppb.Now(),
			Node: &nodev1.NodeRef{NodeId: command.Target.NodeId, NodeSessionId: session.SessionID},
		},
		CommandId: command.Metadata.CommandId, Operation: command.Operation,
		AiSessionId: command.AiSessionId, CleanupKey: command.CleanupKey, LeaseToken: command.LeaseToken,
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	switch command.Operation {
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_ENSURE_IMAGE:
		result.ResolvedImage, err = e.ensureImage(ctx, command.Release)
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_START:
		result.ContainerId, result.Running, err = e.start(ctx, command)
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_INSPECT:
		result.ContainerId, result.Running, err = e.inspect(ctx, command)
	case nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_STOP:
		err = e.stop(ctx, command)
	default:
		err = fmt.Errorf("unsupported runtime host operation")
	}
	result.Success = err == nil
	if err != nil {
		result.ErrorCode = runtimeHostOperationErrorCode(err)
		result.ErrorMessage = err.Error()
	}
	return result
}

func (e *runtimeHostExecutor) ensureImage(ctx context.Context, release *nodev1.AIRuntimeRelease) (string, error) {
	if imageID, exists, err := e.resolveRuntimeImage(ctx, release); err != nil || exists {
		return imageID, err
	}
	archive, cleanup, err := e.downloadRuntimeArchive(ctx, release)
	if err != nil {
		return "", err
	}
	defer cleanup()
	imageTag, err := inspectRuntimeImageArchive(archive, release)
	if err != nil {
		return "", err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err := e.docker.LoadImage(ctx, archive); err != nil {
		return "", fmt.Errorf("load verified Runtime image: %w", err)
	}
	imageID, exists, err := e.docker.ResolveImageID(ctx, imageTag)
	if err != nil {
		return "", err
	}
	if !exists || !runtimeHostImageIDPattern.MatchString(strings.ToLower(strings.TrimSpace(imageID))) {
		return "", fmt.Errorf("loaded Runtime image has no target-local identity")
	}
	if err := e.recordRuntimeImage(release, imageTag, imageID); err != nil {
		return "", fmt.Errorf("persist target-local Runtime image identity: %w", err)
	}
	return imageID, nil
}

func (e *runtimeHostExecutor) downloadRuntimeArchive(ctx context.Context, release *nodev1.AIRuntimeRelease) (io.ReadSeeker, func(), error) {
	endpoint, err := url.Parse(e.platformAPIBaseURL + "/v1/node-engine-control/runtime")
	if err != nil {
		return nil, func() {}, err
	}
	query := endpoint.Query()
	query.Set("agent_installation_id", e.agentInstallationID)
	query.Set("release_id", release.ReleaseId)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, func() {}, err
	}
	request.Header.Set("Authorization", "Enrollment "+e.enrollmentToken)
	response, err := e.httpClient.Do(request)
	if err != nil {
		return nil, func() {}, fmt.Errorf("download Runtime archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, func() {}, fmt.Errorf("download Runtime archive returned HTTP %d", response.StatusCode)
	}
	dir := filepath.Join(e.baseDir, "legion", "runtime-host")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, func() {}, err
	}
	file, err := os.CreateTemp(dir, "runtime-*.docker.tar")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = file.Close(); _ = os.Remove(file.Name()) }
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, release.ArchiveSize+1))
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if written != release.ArchiveSize || hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(release.ArchiveSha256) {
		cleanup()
		return nil, func() {}, fmt.Errorf("downloaded Runtime archive does not match the pinned digest")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return file, cleanup, nil
}

func (e *runtimeHostExecutor) start(ctx context.Context, command *nodev1.AIRuntimeCommand) (string, bool, error) {
	if record, ok := e.operations[command.CleanupKey]; ok {
		if record.LeaseToken == command.LeaseToken && record.State == "stopped" {
			return "", false, fmt.Errorf("runtime container generation was already stopped")
		}
		if record.LeaseToken != command.LeaseToken && record.State != "stopped" && record.State != "missing" {
			return "", false, fmt.Errorf("runtime container ownership does not match the active generation")
		}
	}
	imageID, exists, err := e.resolveRuntimeImage(ctx, command.Release)
	if err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, fmt.Errorf("pinned Runtime image is not ready")
	}
	if existing, found, err := e.docker.FindContainer(ctx, command.CleanupKey); err != nil {
		return "", false, err
	} else if found {
		if err := validateOwnedRuntimeContainer(existing, command, e.agentInstallationID); err != nil {
			return "", false, err
		}
		record := runtimeHostRecordFromCommand(command, existing.ID, runtimeHostContainerState(existing.Running))
		if err := e.recordOperation(command.Metadata.CommandId, &record); err != nil {
			return "", false, err
		}
		return existing.ID, existing.Running, nil
	}
	environment := make([]string, 0, len(command.Container.Environment))
	keys := make([]string, 0, len(command.Container.Environment))
	for key := range command.Container.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+command.Container.Environment[key])
	}
	created, err := e.docker.CreateAndStart(ctx, runtimeHostContainerInput{
		Name: command.Container.ContainerName, Image: imageID, Network: e.network,
		Args: append([]string(nil), command.Container.Arguments...), Env: environment,
		Labels: map[string]string{
			runtimeHostCleanupLabel: command.CleanupKey,
			runtimeHostLeaseLabel:   command.LeaseToken,
			runtimeHostSessionLabel: command.AiSessionId,
			runtimeHostReleaseLabel: command.Release.ReleaseId,
			runtimeHostOwnerLabel:   strings.TrimSpace(e.agentInstallationID),
		},
	})
	if err != nil {
		return "", false, err
	}
	record := runtimeHostRecordFromCommand(command, created.ID, runtimeHostContainerState(created.Running))
	if err := e.recordOperation(command.Metadata.CommandId, &record); err != nil {
		_ = e.docker.StopAndRemove(context.Background(), created.ID)
		return "", false, fmt.Errorf("persist Runtime container operation: %w", err)
	}
	return created.ID, created.Running, nil
}

func (e *runtimeHostExecutor) inspect(ctx context.Context, command *nodev1.AIRuntimeCommand) (string, bool, error) {
	container, found, err := e.docker.FindContainer(ctx, command.CleanupKey)
	if err != nil || !found {
		return "", false, err
	}
	if err := validateOwnedRuntimeContainer(container, command, e.agentInstallationID); err != nil {
		return "", false, err
	}
	record := runtimeHostRecordFromCommand(command, container.ID, runtimeHostContainerState(container.Running))
	if err := e.recordOperation(command.Metadata.CommandId, &record); err != nil {
		return "", false, err
	}
	return container.ID, container.Running, nil
}

func (e *runtimeHostExecutor) stop(ctx context.Context, command *nodev1.AIRuntimeCommand) error {
	container, found, err := e.docker.FindContainer(ctx, command.CleanupKey)
	if err != nil {
		return err
	}
	if !found {
		record := runtimeHostRecordFromCommand(command, "", "stopped")
		return e.recordOperation(command.Metadata.CommandId, &record)
	}
	if err := validateOwnedRuntimeContainer(container, command, e.agentInstallationID); err != nil {
		return err
	}
	if err := e.docker.StopAndRemove(ctx, container.ID); err != nil {
		return err
	}
	record := runtimeHostRecordFromCommand(command, container.ID, "stopped")
	return e.recordOperation(command.Metadata.CommandId, &record)
}

func runtimeHostRecordFromCommand(command *nodev1.AIRuntimeCommand, containerID, state string) runtimeHostOperationRecord {
	return runtimeHostOperationRecord{
		CleanupKey: command.CleanupKey, LeaseToken: command.LeaseToken,
		SessionID: command.AiSessionId, ReleaseID: command.Release.ReleaseId,
		ContainerID: containerID, State: state,
	}
}

func runtimeHostContainerState(running bool) string {
	if running {
		return "running"
	}
	return "stopped_container"
}

func validateOwnedRuntimeContainer(container runtimeHostContainer, command *nodev1.AIRuntimeCommand, owner string) error {
	if container.Labels[runtimeHostCleanupLabel] != command.CleanupKey ||
		container.Labels[runtimeHostLeaseLabel] != command.LeaseToken ||
		container.Labels[runtimeHostSessionLabel] != command.AiSessionId ||
		container.Labels[runtimeHostReleaseLabel] != command.Release.ReleaseId ||
		container.Labels[runtimeHostOwnerLabel] != strings.TrimSpace(owner) {
		return fmt.Errorf("runtime container ownership does not match the requested generation")
	}
	return nil
}

func runtimeHostOperationErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "authentication"), strings.Contains(message, "not allowed"), strings.Contains(message, "ownership"):
		return "runtime_command_rejected"
	case strings.Contains(message, "image") || strings.Contains(message, "digest"):
		return "runtime_release_mismatch"
	default:
		return classifyRuntimeHostDockerError(err)
	}
}
