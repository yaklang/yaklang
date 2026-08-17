package scannode

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type runtimeHostDockerStub struct {
	pingErr    error
	images     map[string]string
	containers map[string]runtimeHostContainer
	creates    int
	stops      int
}

func (d *runtimeHostDockerStub) Ping(context.Context) error { return d.pingErr }
func (d *runtimeHostDockerStub) ResolveImageID(_ context.Context, selector string) (string, bool, error) {
	image, ok := d.images[selector]
	return image, ok, nil
}
func (d *runtimeHostDockerStub) LoadImage(context.Context, io.Reader) error { return nil }
func (d *runtimeHostDockerStub) FindContainer(_ context.Context, cleanupKey string) (runtimeHostContainer, bool, error) {
	container, ok := d.containers[cleanupKey]
	return container, ok, nil
}
func (d *runtimeHostDockerStub) CreateAndStart(_ context.Context, input runtimeHostContainerInput) (runtimeHostContainer, error) {
	d.creates++
	container := runtimeHostContainer{
		ID: "container-" + input.Labels[runtimeHostSessionLabel], ImageID: input.Image,
		Running: true, Labels: cloneStringMapValue(input.Labels),
	}
	d.containers[input.Labels[runtimeHostCleanupLabel]] = container
	return container, nil
}
func (d *runtimeHostDockerStub) Inspect(_ context.Context, containerID string) (runtimeHostContainer, bool, error) {
	for _, container := range d.containers {
		if container.ID == containerID {
			return container, true, nil
		}
	}
	return runtimeHostContainer{}, false, nil
}
func (d *runtimeHostDockerStub) StopAndRemove(_ context.Context, containerID string) error {
	d.stops++
	for cleanupKey, container := range d.containers {
		if container.ID == containerID {
			delete(d.containers, cleanupKey)
			return nil
		}
	}
	return nil
}
func (d *runtimeHostDockerStub) Close() error { return nil }

func newRuntimeHostTestExecutor(t *testing.T, docker *runtimeHostDockerStub, baseDir string) *runtimeHostExecutor {
	t.Helper()
	executor, err := newRuntimeHostExecutor(RuntimeHostConfig{
		Enabled: true, BaseDir: baseDir, PlatformAPIBaseURL: "http://platform.invalid",
		EnrollmentToken: "enrollment-ticket", AgentInstallationID: "host-installation-1",
		EngineReleaseID: "sha256-" + strings.Repeat("d", 64), EngineDigest: strings.Repeat("a", 64),
		Network: "bridge", docker: docker, NodeIDProvider: func() string { return "node-1" },
		SessionProvider: func() (node.SessionState, bool) {
			return node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}, true
		},
	})
	if err != nil {
		t.Fatalf("newRuntimeHostExecutor() error = %v", err)
	}
	return executor
}

func runtimeHostTestCommand(t *testing.T) *nodev1.AIRuntimeCommand {
	t.Helper()
	imageID := "sha256:" + strings.Repeat("b", 64)
	commandID := uuid.NewString()
	now := time.Now()
	command := &nodev1.AIRuntimeCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: commandID, CommandType: "ai.runtime.container.start", TraceId: "session-1",
			IssuedAt: timestamppb.New(now), ExpireAt: timestamppb.New(now.Add(45 * time.Second)),
		},
		Target:       &nodev1.NodeRef{NodeId: "node-1", NodeSessionId: "node-session-1"},
		Operation:    nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_START,
		ReplySubject: runtimeHostReplyPrefix + commandID, AiSessionId: "session-1",
		CleanupKey: "legion-ai-session-session-1", LeaseToken: "lease-1",
		Release: &nodev1.AIRuntimeRelease{
			ReleaseId: "sha256-" + strings.Repeat("d", 64), EngineDigest: strings.Repeat("a", 64), ImageId: imageID,
			ArchiveSha256: strings.Repeat("c", 64), ArchiveSize: 1,
		},
		Container: &nodev1.AIRuntimeContainerSpec{
			ContainerName: "legion-ai-session-session-1", Network: "bridge",
			Arguments: []string{
				"-api-url", "http://platform.invalid", "-enrollment-token", "short-lived-token",
				"-kind", "ai_session", "-agent-installation-id", "ai-session-session-1",
				"-name", "legion-ai-session-session-1",
			},
			Environment: map[string]string{
				"LEGION_API_URL": "http://platform.invalid", "LEGION_NATS_URL": "nats://platform.invalid",
				"LEGION_ENROLLMENT_TOKEN": "short-lived-token", "LEGION_SESSIONMGR_URL": "http://manager.invalid",
				"LEGION_AI_SESSION_ID": "session-1", "LEGION_AI_RUNTIME": "stateless",
			},
		},
	}
	runtimeHostSignTestCommand(t, command)
	return command
}

func runtimeHostSignTestCommand(t *testing.T, command *nodev1.AIRuntimeCommand) {
	t.Helper()
	command.AuthTag = nil
	key, err := runtimeHostSessionKey("enrollment-ticket", "node-session-1")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	command.AuthTag = mac.Sum(nil)
}

func TestRuntimeHostCommandAuthenticationRejectsTampering(t *testing.T) {
	command := runtimeHostTestCommand(t)
	executor := &runtimeHostExecutor{
		enrollmentToken: "enrollment-ticket", network: "bridge", platformAPIBaseURL: "http://platform.invalid",
		engineReleaseID: command.Release.ReleaseId, engineDigest: command.Release.EngineDigest,
		nodeIDProvider: func() string { return "node-1" },
	}
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	if err := executor.validateCommand(command, session); err != nil {
		t.Fatalf("validateCommand() rejected signed command: %v", err)
	}
	command.LeaseToken = "tampered-lease"
	if err := executor.validateCommand(command, session); err == nil {
		t.Fatal("validateCommand() accepted tampered signed command")
	}
}

func TestRuntimeHostCommandRejectionIsTerminallyTyped(t *testing.T) {
	cause := errors.New("targets another node session")
	err := rejectRuntimeHostCommand(cause)
	var rejected *runtimeHostCommandRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, cause) {
		t.Fatalf("runtime command rejection lost its terminal type: %v", err)
	}
}

func TestRuntimeHostCannotEnableWithoutAuthenticatedNodeIdentity(t *testing.T) {
	_, err := newRuntimeHostExecutor(RuntimeHostConfig{
		Enabled: true, BaseDir: t.TempDir(), PlatformAPIBaseURL: "http://platform.invalid",
		AgentInstallationID: "host-installation-1", docker: &runtimeHostDockerStub{},
		NodeIDProvider: func() string { return "node-1" },
		SessionProvider: func() (node.SessionState, bool) {
			return node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}, true
		},
	})
	if err == nil || !strings.Contains(err.Error(), "authenticated node identity") {
		t.Fatalf("newRuntimeHostExecutor() error = %v", err)
	}
}

func TestRuntimeHostRejectsChangedFixedContainerIdentity(t *testing.T) {
	command := runtimeHostTestCommand(t)
	executor := &runtimeHostExecutor{
		enrollmentToken: "enrollment-ticket", network: "bridge", platformAPIBaseURL: "http://platform.invalid",
		engineReleaseID: command.Release.ReleaseId, engineDigest: command.Release.EngineDigest,
		nodeIDProvider: func() string { return "node-1" },
	}
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	command.Container.Arguments[7] = "ai-session-another-session"
	runtimeHostSignTestCommand(t, command)
	if err := executor.validateCommand(command, session); err == nil {
		t.Fatal("validateCommand() accepted changed Runtime agent identity")
	}
}

func TestRuntimeHostRejectsReplySubjectOutsideCommandIdentity(t *testing.T) {
	command := runtimeHostTestCommand(t)
	executor := &runtimeHostExecutor{
		enrollmentToken: "enrollment-ticket", network: "bridge", platformAPIBaseURL: "http://platform.invalid",
		engineReleaseID: command.Release.ReleaseId, engineDigest: command.Release.EngineDigest,
		nodeIDProvider: func() string { return "node-1" },
	}
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	command.ReplySubject = runtimeHostReplyPrefix + uuid.NewString()
	runtimeHostSignTestCommand(t, command)
	if err := executor.validateCommand(command, session); err == nil {
		t.Fatal("validateCommand() accepted a reply subject for another command")
	}
}

func TestRuntimeHostRejectsReleaseDifferentFromInstalledNode(t *testing.T) {
	command := runtimeHostTestCommand(t)
	executor := &runtimeHostExecutor{
		enrollmentToken: "enrollment-ticket", network: "bridge", platformAPIBaseURL: "http://platform.invalid",
		engineReleaseID: command.Release.ReleaseId, engineDigest: command.Release.EngineDigest,
		nodeIDProvider: func() string { return "node-1" },
	}
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	command.Release.EngineDigest = strings.Repeat("e", 64)
	runtimeHostSignTestCommand(t, command)
	if err := executor.validateCommand(command, session); err == nil {
		t.Fatal("validateCommand() accepted a release different from the installed node")
	}
}

func TestRuntimeHostStatusUsesSafeDockerReason(t *testing.T) {
	docker := &runtimeHostDockerStub{pingErr: errors.New("permission denied")}
	executor := &runtimeHostExecutor{docker: docker}
	status, ok := executor.CurrentStatus()
	if !ok || status.Status != "unavailable" || !strings.Contains(string(status.DetailJSON), "docker_permission_denied") {
		t.Fatalf("CurrentStatus() = %+v, %v", status, ok)
	}
	docker.pingErr = nil
	status, ok = executor.CurrentStatus()
	if !ok || status.Status != "ready" || strings.Contains(string(status.DetailJSON), "docker_") {
		t.Fatalf("CurrentStatus() did not recover with Docker: %+v, %v", status, ok)
	}
}

func TestResilienceRuntimeHostReplayRestartAndStopDoNotDuplicateContainer(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("b", 64)
	docker := &runtimeHostDockerStub{
		images: map[string]string{imageID: imageID}, containers: make(map[string]runtimeHostContainer),
	}
	baseDir := t.TempDir()
	executor := newRuntimeHostTestExecutor(t, docker, baseDir)
	command := runtimeHostTestCommand(t)
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	first := executor.execute(context.Background(), command, session)
	second := executor.execute(context.Background(), command, session)
	if !first.Success || !second.Success || first.ContainerId != second.ContainerId || docker.creates != 1 {
		t.Fatalf("replayed start results = %+v / %+v, creates=%d", first, second, docker.creates)
	}

	restarted := newRuntimeHostTestExecutor(t, docker, baseDir)
	third := restarted.execute(context.Background(), command, session)
	if !third.Success || third.ContainerId != first.ContainerId || docker.creates != 1 {
		t.Fatalf("restart adoption result = %+v, creates=%d", third, docker.creates)
	}

	stop := proto.Clone(command).(*nodev1.AIRuntimeCommand)
	stop.Operation = nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_STOP
	stop.Metadata.CommandId = uuid.NewString()
	stop.Metadata.CommandType = "ai.runtime.container.stop"
	stop.ReplySubject = runtimeHostReplyPrefix + stop.Metadata.CommandId
	stop.AuthTag = nil
	stopResult := restarted.execute(context.Background(), stop, session)
	duplicateStop := restarted.execute(context.Background(), stop, session)
	if !stopResult.Success || !duplicateStop.Success || docker.stops != 1 {
		t.Fatalf("stop results = %+v / %+v, stops=%d", stopResult, duplicateStop, docker.stops)
	}

	afterStopRestart := newRuntimeHostTestExecutor(t, docker, baseDir)
	replayedStart := afterStopRestart.execute(context.Background(), command, session)
	if replayedStart.Success || docker.creates != 1 {
		t.Fatalf("stopped generation was recreated: result=%+v creates=%d", replayedStart, docker.creates)
	}
}
