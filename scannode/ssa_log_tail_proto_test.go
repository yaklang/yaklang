package scannode

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/proto"
)

func TestSSALogTailRejectsLegacyJSON(t *testing.T) {
	bridge := &legionJobBridge{}
	err := bridge.handleSSALogTail(context.Background(), []byte(`{"query_id":"q","job_id":"j","attempt_id":"a"}`))
	require.ErrorContains(t, err, "unmarshal ssa log tail")
}

func TestSSALogTailAcceptsProtobufBeforeValidation(t *testing.T) {
	command, err := proto.Marshal(&ssav1.LogTailCommand{QueryId: "q", JobId: "j", AttemptId: "a", Offset: -1})
	require.NoError(t, err)
	bridge := &legionJobBridge{}
	err = bridge.handleSSALogTail(context.Background(), command)
	require.ErrorContains(t, err, "offset must be >= 0")
}

func TestSSALogTailProtobufReplyFitsNATSPayload(t *testing.T) {
	result := &ssav1.LogTailResult{QueryId: "q", Found: true, Content: strings.Repeat("x", ssaLogTailMaxBytesLimit)}
	require.Less(t, proto.Size(result), 900*1024)
	require.Equal(t, "legion.realtime.ssa.log.tail.v2.q", ssaLogTailResultSubject("q"))
}
