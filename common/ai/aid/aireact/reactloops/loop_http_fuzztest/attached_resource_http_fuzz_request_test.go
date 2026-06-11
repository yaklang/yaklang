package loop_http_fuzztest

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestBindAttachedHTTPFuzzRequestToLoop(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
	)
	require.NoError(t, err)

	packet := "POST /login HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"user\":\"admin\"}"
	task := aicommon.NewStatefulTaskBase("attached-http-packet-test", "fuzz attached packet", context.Background(), loop.GetEmitter(), true)
	task.SetAttachedDatas([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(
			"httppacket",
			aicommon.AttachedResourceKeyContent,
			fmt.Sprintf(`{"http_packet":%q,"is_https":true}`, packet),
		),
	})
	loop.SetCurrentTask(task)

	resources := reactloops.RunAttachedExtraResourcesInit(invoker, loop, task.GetAttachedDatas())
	require.NotEmpty(t, resources)
	require.IsType(t, &aicommon.AttachedHTTPFuzzRequestData{}, resources[0])

	require.True(t, bindAttachedHTTPFuzzRequestToLoop(loop, invoker, task, resources))
	require.NotNil(t, loop.GetVariable("fuzz_request"))
	require.Contains(t, loop.Get("original_request"), "POST /login HTTP/1.1")
	require.Contains(t, loop.Get("current_request"), "Host: example.com")
	require.Equal(t, "true", loop.Get("is_https"))
	require.Equal(t, "attached_http_packet", loop.Get("bootstrap_source"))
	require.Contains(t, loop.Get("original_request_summary"), "example.com")
}
