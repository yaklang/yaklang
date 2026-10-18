package scannode

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/proto"
)

// Byte-identical fixture in Legion's internal/protocol/testdata. Historical
// JSON diagnostics intentionally have no query_id in the response body.
func TestSSALegacyCrossRepositoryWireFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/ssa_legacy_wire.json")
	require.NoError(t, err)
	var fixture map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fixture))
	for name, target := range map[string]any{
		"debug_request": &ssaDebugQueryPayload{}, "debug_response": &ssaDebugQueryResponse{},
		"log_request": &legacySSALogTailCommand{}, "log_response": &legacySSALogTailResponse{},
		"ir_request": &legacySSAIRProgramDeleteCommand{}, "ir_response": &legacySSAIRProgramDeleteResponse{},
	} {
		require.NoError(t, json.Unmarshal(fixture[name], target), name)
		encoded, err := json.Marshal(target)
		require.NoError(t, err)
		require.JSONEq(t, string(fixture[name]), string(encoded), name)
	}
}

func TestSSALegacySubjectsHaveExplicitJSONDecoders(t *testing.T) {
	bridge := &legionJobBridge{}
	for _, tc := range []struct{ subject, missing string }{
		{legionCommandLegacySSADebugQuery, "query_id"},
		{legionCommandLegacySSALogTail, "query_id"},
		{legionCommandLegacySSAIRProgramDelete, "request_id"},
	} {
		err := bridge.handleMessagePayload(context.Background(), "session", &nats.Msg{Subject: "command.node." + tc.subject, Data: []byte(`{}`)})
		require.ErrorContains(t, err, tc.missing+" is required")
		err = bridge.handleMessagePayload(context.Background(), "session", &nats.Msg{Subject: "command.node." + tc.subject, Data: []byte{0x0a, 1, 'q'}})
		require.ErrorContains(t, err, "unmarshal legacy")
	}
}

func TestSSADiagnosticProtobufCommandWireFixtures(t *testing.T) {
	for _, tc := range []struct {
		command proto.Message
		wire    []byte
	}{
		{&ssav1.DebugQueryCommand{QueryId: "q", JobId: "j", AttemptId: "a"}, []byte{0x0a, 1, 'q', 0x12, 1, 'j', 0x1a, 1, 'a'}},
		{&ssav1.LogTailCommand{QueryId: "q", JobId: "j", AttemptId: "a", Offset: 21, MaxBytes: 64, LogKind: "db"}, []byte{0x0a, 1, 'q', 0x12, 1, 'j', 0x1a, 1, 'a', 0x20, 21, 0x28, 64, 0x3a, 2, 'd', 'b'}},
	} {
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(tc.command)
		require.NoError(t, err)
		require.Equal(t, tc.wire, raw)
	}
}

func TestSSAIRDeletionBothCodecsUseIdleHold(t *testing.T) {
	manager := newTaskManager()
	_, _, accepted := manager.LoadOrStoreAttempt(&Task{AttemptID: "running-attempt", Ctx: context.Background()})
	require.True(t, accepted)
	bridge := &legionJobBridge{agent: &ScanNode{manager: manager}}
	_, _, _, err := bridge.ssaIRProgramDeleter()(context.Background(), "store", "program")
	require.ErrorContains(t, err, "扫描节点仍有任务运行")
}
