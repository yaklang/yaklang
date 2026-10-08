package scannode

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newCompanyRenewalTestBridge(t *testing.T) (*legionJobBridge, node.SessionState, context.Context) {
	t.Helper()
	now := time.Now().UTC()
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1", SessionToken: "token", CompanyID: "company-a", SessionStartedAt: now.Add(-time.Minute), NATSURL: "tls://node.test:4222", NATSCredentials: "test-creds", NATSCredentialsExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Hour), CommandStream: "LEGION_COMMANDS", CommandConsumer: "session-1", InboxPrefix: "_INBOX.node.session-1", CommandSubject: "legion.command.node.node-1.session.node-session-1", EventSubjectPrefix: "legion.event.node.node-session-1"}
	base, err := node.NewNodeBase(node.BaseConfig{NodeID: session.NodeID, BaseDir: t.TempDir(), EnrollmentToken: "enroll", PlatformAPIBaseURL: "http://platform.test", TransportClient: &aiBootstrapSessionTransport{session: session}, HeartbeatInterval: time.Hour, TickerInterval: time.Hour, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go base.Serve()
	t.Cleanup(base.Shutdown)
	waitForAINodeSession(t, base)
	bridge := newLegionJobBridge(&ScanNode{node: base, ruleSyncClient: NewRuleSyncClient(&RuleSyncConfig{ServerURL: "http://platform.test"}), httpClient: &http.Client{Timeout: time.Second}})
	bridge.companyBound.Store(true)
	bridge.companyExecutionCtx, bridge.companyExecutionCancel = context.WithCancel(context.Background())
	bridge.companyExecutionSession = session.SessionID
	bridge.switchDispatchSession(session.SessionID)
	transportCtx, stopTransport := context.WithCancel(context.Background())
	bridge.consumer = &commandConsumer{sessionID: session.SessionID, connectionKey: natsSessionConnectionKey(session), cancel: stopTransport}
	bridge.aiPublisher.conn = newReconnectingEventTestConn(t)
	bridge.aiPublisher.js = &aiFakeJetStreamContext{}
	bridge.aiPublisher.sessionKey = natsSessionConnectionKey(session)
	t.Cleanup(bridge.invalidateCompanyExecution)
	return bridge, session, transportCtx
}

func waitCompanyRenewalStage(t *testing.T, stage <-chan struct{}) {
	t.Helper()
	select {
	case <-stage:
	case <-time.After(3 * time.Second):
		t.Fatal("command did not reach preparation")
	}
}

func TestResilienceCompanyCredentialRenewalPreservesPendingAIBindAndScan(t *testing.T) {
	bridge, session, transportCtx := newCompanyRenewalTestBridge(t)
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{recorder: recorder, blockOn: 1, started: make(chan struct{}), release: make(chan struct{})}
	bridge.aiRuntime = newAISessionRuntimeManager(driver)
	command := validAISessionBindCommand()
	command.TargetNodeId = session.NodeID
	command.Metadata.CompanyId = session.CompanyID
	command.Metadata.IssuedAt = timestamppb.Now()
	command.Metadata.ExpireAt = timestamppb.New(time.Now().Add(time.Minute))
	scanCtx, scanCancel := context.WithCancel(context.Background())
	defer scanCancel()
	registry := bridge.dispatchAdmissions
	reservation, result := registry.Reserve(session.SessionID, jobExecutionRef{CommandID: "scan-command", AttemptID: "scan-attempt"}, "identity", time.Now().Add(time.Hour))
	if result != dispatchReserved {
		t.Fatal(result)
	}
	registry.AttachClaim(reservation, &Task{AttemptID: "scan-attempt", Ctx: scanCtx, Cancel: scanCancel}, nil)
	reservation.state = dispatchAdmissionRunning
	done := make(chan error, 1)
	go func() {
		_, err := bridge.handleMessageWithDisposition(transportCtx, session.SessionID, &nats.Msg{Subject: session.CommandSubject + "." + legionCommandAISessionBind, Data: mustMarshalProto(t, command)})
		done <- err
	}()
	waitCompanyRenewalStage(t, driver.started)
	bridge.stopConsumer()
	bridge.switchDispatchSession(session.SessionID)
	if transportCtx.Err() == nil || scanCtx.Err() != nil {
		t.Fatal("rotation did not isolate transport and scan lifetimes")
	}
	select {
	case err := <-done:
		t.Fatalf("rotation canceled pending bind: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(driver.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bind did not finish after renewal")
	}
	bridge.aiRuntime.mu.Lock()
	installed := bridge.aiRuntime.sessions[command.Session.SessionId] != nil
	bridge.aiRuntime.mu.Unlock()
	if !installed {
		t.Fatal("binding was not installed")
	}
	bridge.invalidateCompanyExecution()
	if scanCtx.Err() == nil || bridge.companyExecutionCtx.Err() == nil {
		t.Fatal("transport revocation did not cancel execution")
	}
	bridge.aiRuntime.mu.Lock()
	remaining := len(bridge.aiRuntime.sessions)
	bridge.aiRuntime.mu.Unlock()
	if remaining != 0 {
		t.Fatal("AI binding survived revocation")
	}
}

type companyRenewalDocker struct {
	*runtimeHostDockerStub
	started  chan struct{}
	release  chan struct{}
	observed chan error
}

func (d *companyRenewalDocker) ResolveImageID(ctx context.Context, selector string) (string, bool, error) {
	close(d.started)
	select {
	case <-d.release:
	case <-ctx.Done():
		d.observed <- ctx.Err()
		return "", false, ctx.Err()
	}
	d.observed <- ctx.Err()
	return selector, true, nil
}

func TestResilienceCompanyCredentialRenewalPreservesRuntimeHostPreparation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		name := "renewal"
		if revoke {
			name = "revocation"
		}
		t.Run(name, func(t *testing.T) {
			bridge, session, transportCtx := newCompanyRenewalTestBridge(t)
			stub := &runtimeHostDockerStub{images: map[string]string{}, containers: map[string]runtimeHostContainer{}}
			docker := &companyRenewalDocker{runtimeHostDockerStub: stub, started: make(chan struct{}), release: make(chan struct{}), observed: make(chan error, 1)}
			executor := newRuntimeHostTestExecutor(t, stub, t.TempDir())
			executor.docker = docker
			bridge.agent.runtimeHost = executor
			command := runtimeHostTestCommand(t)
			command.Metadata.CompanyId = session.CompanyID
			command.Operation = nodev1.AIRuntimeOperation_AI_RUNTIME_OPERATION_ENSURE_IMAGE
			command.Metadata.CommandType = legionCommandAIRuntimeImageEnsure
			command.ReplySubject, _ = sessionScopedOutboundSubject(session, runtimeHostReplyPrefix+command.Metadata.CommandId)
			command.Container = nil
			runtimeHostSignTestCommand(t, command)
			done := make(chan error, 1)
			go func() {
				_, err := bridge.handleMessageWithDisposition(transportCtx, session.SessionID, &nats.Msg{Subject: session.CommandSubject + "." + legionCommandAIRuntimeImageEnsure, Data: mustMarshalProto(t, command)})
				done <- err
			}()
			waitCompanyRenewalStage(t, docker.started)
			if revoke {
				bridge.invalidateCompanyExecution()
			} else {
				bridge.stopConsumer()
				bridge.switchDispatchSession(session.SessionID)
			}
			if !revoke {
				select {
				case err := <-docker.observed:
					t.Fatalf("rotation canceled image preparation: %v", err)
				case <-time.After(30 * time.Millisecond):
				}
				close(docker.release)
			}
			select {
			case err := <-docker.observed:
				if revoke && !errors.Is(err, context.Canceled) {
					t.Fatalf("revoked preparation: %v", err)
				}
				if !revoke && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("image preparation did not settle")
			}
			// The deliberately absent broker affects the reply only, after execution.
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "reply transport is unavailable") {
					t.Fatalf("reply: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("runtime host command did not settle")
			}
		})
	}
}
