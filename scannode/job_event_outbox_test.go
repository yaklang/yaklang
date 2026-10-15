package scannode

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/node"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func outboxTestEvent(t *testing.T, kind string) durableJobEvent {
	t.Helper()
	ref := jobExecutionRef{NodeSessionID: "old-session", CommandID: "command-a", JobID: "job-a", SubtaskID: "sub-a", AttemptID: "attempt-a"}
	id := ref.AttemptID + ":" + kind
	m := &nodev1.EventMetadata{EventId: id, EventType: "job." + kind, CausationId: ref.CommandID, CorrelationId: ref.AttemptID, EmittedAt: timestamppb.Now(), Node: &nodev1.NodeRef{NodeId: "node-a", NodeSessionId: ref.NodeSessionID}}
	j := &jobv1.JobRef{JobId: ref.JobID, SubtaskId: ref.SubtaskID, AttemptId: ref.AttemptID}
	var message proto.Message
	terminal := false
	switch kind {
	case "asset":
		message = &jobv1.JobAsset{Metadata: m, Job: j, AssetKind: "port", AssetJson: []byte(`{"port":80}`)}
	case "risk":
		message = &jobv1.JobRisk{Metadata: m, Job: j, RiskKind: "fixture", RiskJson: []byte(`{"severity":"low"}`)}
	case "succeeded":
		terminal = true
		message = &jobv1.JobSucceeded{Metadata: m, Job: j, FinishedAt: timestamppb.Now(), ResultJson: []byte(`{"ok":true}`)}
	default:
		t.Fatal("unknown test event")
	}
	raw, err := proto.Marshal(message)
	require.NoError(t, err)
	return durableJobEvent{NodeID: "node-a", Ref: ref, EventID: id, Subject: "legion.event.job." + kind, Payload: raw, CreatedAt: time.Now().UTC(), Terminal: terminal}
}

func TestJobOutboxRetainsTerminalUntilProjectionAndFencesDuplicate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	for _, kind := range []string{"asset", "risk", "succeeded"} {
		require.NoError(t, box.enqueue(outboxTestEvent(t, kind)))
	}
	before := box.snapshot()
	require.Len(t, before, 3)
	retry := outboxTestEvent(t, "succeeded")
	require.NoError(t, box.enqueue(retry))
	require.True(t, bytes.Equal(before[2].Payload, box.snapshot()[2].Payload), "retry must preserve the original envelope")
	for _, e := range before {
		require.NoError(t, box.brokerAck(e.EventID))
	}
	require.Len(t, box.snapshot(), 1, "a broker ack is not a platform projection receipt")
	box, err = openJobEventOutbox(dir)
	require.NoError(t, err)
	require.True(t, box.snapshot()[0].BrokerAck)
	manifest := box.pendingDeliveries()
	require.Len(t, manifest, 1)
	require.Error(t, box.acceptRecovery(node.SessionState{SessionID: "new-session"}), "old platforms must not silently lose recovery")
	require.NoError(t, box.acceptRecovery(node.SessionState{NodeID: "node-a", SessionID: "new-session", JobDeliveryRecoverySupported: true, AcceptedJobDeliveryAttempts: []string{"attempt-a"}}))
	require.NoError(t, box.applyReceipts([]node.JobDeliveryReceipt{{AttemptID: "wrong-attempt", TerminalEventID: manifest[0].TerminalEventID, Disposition: "projected"}}))
	require.Len(t, box.snapshot(), 1)
	require.NoError(t, box.applyReceipts([]node.JobDeliveryReceipt{{AttemptID: "attempt-a", TerminalEventID: manifest[0].TerminalEventID, Disposition: "projected"}}))
	require.Empty(t, box.snapshot())
	box, err = openJobEventOutbox(dir)
	require.NoError(t, err)
	ref := before[2].Ref
	ref.NodeSessionID = "new-session"
	found, matches := box.completedCommand(ref)
	require.True(t, found)
	require.True(t, matches)
	ref.CommandID = "different-command"
	found, matches = box.completedCommand(ref)
	require.True(t, found)
	require.False(t, matches)
}

func TestJobOutboxCrashWriter(t *testing.T) {
	dir := os.Getenv("LEGION_JOB_OUTBOX_CRASH_HELPER")
	if dir == "" {
		return
	}
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	for _, kind := range []string{"asset", "risk", "succeeded"} {
		require.NoError(t, box.enqueue(outboxTestEvent(t, kind)))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600))
	select {}
}

func TestJobOutboxSurvivesAbruptProcessDeath(t *testing.T) {
	if os.Getenv("LEGION_JOB_OUTBOX_CRASH_HELPER") != "" {
		return
	}
	dir := filepath.Join(t.TempDir(), "outbox")
	cmd := exec.Command(os.Args[0], "-test.run=^TestJobOutboxCrashWriter$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "LEGION_JOB_OUTBOX_CRASH_HELPER="+dir)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, "ready")); return err == nil }, 15*time.Second, 20*time.Millisecond)
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err, output.String())
	entries := box.snapshot()
	require.Len(t, entries, 3)
	require.Equal(t, []string{"attempt-a:asset", "attempt-a:risk", "attempt-a:succeeded"}, []string{entries[0].EventID, entries[1].EventID, entries[2].EventID})
	for _, entry := range entries {
		require.NoError(t, validateDurableJobEvent(entry))
		require.Equal(t, "old-session", entry.Ref.NodeSessionID)
	}
}

func TestJobOutboxRejectsCorruptionAndPrivateFileViolations(t *testing.T) {
	for _, kind := range []string{"payload", "binding", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "outbox")
			box, err := openJobEventOutbox(dir)
			require.NoError(t, err)
			require.NoError(t, box.enqueue(outboxTestEvent(t, "succeeded")))
			e := box.snapshot()[0]
			path := filepath.Join(dir, e.filename())
			switch kind {
			case "payload":
				e.Payload[0] ^= 1
			case "binding":
				e.Ref.NodeSessionID = "forged-session"
			case "permissions":
				require.NoError(t, os.Chmod(path, 0644))
			}
			if kind != "permissions" {
				raw, err := json.Marshal(e)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, raw, 0600))
			}
			_, err = openJobEventOutbox(dir)
			require.Error(t, err, "corruption must not start an empty queue")
		})
	}
}

func TestJobOutboxRetiresStalePayloadsWithoutRebinding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	require.NoError(t, box.enqueue(outboxTestEvent(t, "asset")))
	require.NoError(t, box.enqueue(outboxTestEvent(t, "succeeded")))
	require.NoError(t, box.acceptRecovery(node.SessionState{NodeID: "node-a", SessionID: "new-session", JobDeliveryRecoverySupported: true, JobDeliveryReceipts: []node.JobDeliveryReceipt{{AttemptID: "attempt-a", TerminalEventID: "attempt-a:succeeded", Disposition: "retired"}}}))
	require.Empty(t, box.snapshot())
	files, err := os.ReadDir(filepath.Join(dir, "retired"))
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestJobOutboxReplaysOriginalEnvelopeAfterNewSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	for _, kind := range []string{"asset", "risk", "succeeded"} {
		require.NoError(t, box.enqueue(outboxTestEvent(t, kind)))
	}
	entries := box.snapshot()
	session := node.SessionState{NodeID: "node-a", SessionID: "new-session", SessionToken: "fixture", NATSURL: "nats://unused.test", JobDeliveryRecoverySupported: true, AcceptedJobDeliveryAttempts: []string{"attempt-a"}}
	base, err := node.NewNodeBase(node.BaseConfig{NodeID: "node-a", BaseDir: t.TempDir(), EnrollmentToken: "fixture", PlatformAPIBaseURL: "http://fixture.test", TransportClient: &bootstrapSessionTransport{session: session}, HeartbeatInterval: time.Hour, TickerInterval: time.Hour})
	require.NoError(t, err)
	go base.Serve()
	defer base.Shutdown()
	waitForNodeSession(t, base)
	fake := &fakeJetStreamContext{}
	publisher := newJobEventPublisher(base)
	publisher.outbox = box
	publisher.js = fake
	publisher.natsURL = session.NATSURL
	require.Error(t, publisher.deliverPending(context.Background(), entries), "terminal replay needs a recovery grant")
	require.NoError(t, box.acceptRecovery(session))
	fake.mu.Lock()
	fake.publish = nil
	fake.mu.Unlock()
	require.NoError(t, publisher.deliverPending(context.Background(), entries))
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.publish, 3)
	for i, e := range entries {
		require.Equal(t, e.Payload, fake.publish[i].Data, "replay cannot relabel the original session or change the event id")
	}
	require.Len(t, box.pendingDeliveries(), 1)
}

func TestJobOutboxCompletedDispatchCannotExecuteAfterRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	require.NoError(t, box.enqueue(outboxTestEvent(t, "succeeded")))
	require.NoError(t, box.applyReceipts([]node.JobDeliveryReceipt{{AttemptID: "attempt-a", TerminalEventID: "attempt-a:succeeded", Disposition: "projected"}}))
	box, err = openJobEventOutbox(dir)
	require.NoError(t, err)
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		t.Error("completed command executed again")
		return &ScriptExecutionResult{}, nil
	})
	bridge.publisher = &jobEventPublisher{outbox: box}
	cmd := admissionTestCommand("command-a", "job-a", "sub-a", "attempt-a")
	disposition, err := bridge.handleDispatch(context.Background(), "new-session", marshalAdmissionCommand(t, cmd))
	require.NoError(t, err)
	require.Equal(t, messageAckSync, disposition.kind)
	require.Nil(t, disposition.afterAck)
	require.Zero(t, reporter.count("claimed", "attempt-a"))
}

func TestJobOutboxStorageFailureStopsNewExecution(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	box, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	require.NoError(t, os.Rename(dir, dir+"-offline"))
	require.Error(t, box.enqueue(outboxTestEvent(t, "succeeded")), "a failed fsync path cannot acknowledge enqueue")
	bridge, reporter := newAdmissionTestBridge(1, nil)
	bridge.publisher = &jobEventPublisher{outbox: box}
	cmd := admissionTestCommand("new-command", "new-job", "new-sub", "new-attempt")
	disposition, err := bridge.handleDispatch(context.Background(), "session", marshalAdmissionCommand(t, cmd))
	require.Error(t, err)
	require.Equal(t, messageNakDelayed, disposition.kind)
	require.Zero(t, reporter.count("claimed", "new-attempt"))
	require.NoError(t, os.Rename(dir+"-offline", dir))
	require.Error(t, box.admissionError(), "I/O failure stays closed until storage is recovered and reopened")
	reopened, err := openJobEventOutbox(dir)
	require.NoError(t, err)
	require.NoError(t, reopened.admissionError())
}

func TestJobOutboxRefusesAChangedNodeIdentity(t *testing.T) {
	box, err := openJobEventOutbox(filepath.Join(t.TempDir(), "outbox"))
	require.NoError(t, err)
	require.NoError(t, box.enqueue(outboxTestEvent(t, "asset")))
	require.Error(t, box.acceptRecovery(node.SessionState{NodeID: "another-node", SessionID: "new-session", JobDeliveryRecoverySupported: true}))
	require.Len(t, box.snapshot(), 1, "unowned partial results must be preserved")
}
