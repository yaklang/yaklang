package yakgrpc

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFlow_LargeRequest_Spill(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := utils.RandStringBytes(12)
	body := strings.Repeat("X", 300*1024) // 300KB > 200KB spill threshold
	reqRaw := []byte("POST /" + token + " HTTP/1.1\r\nHost: spill.test\r\n\r\n" + body)
	rspRaw := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")

	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false, reqRaw, rspRaw, "mitm-test", "http://spill.test/"+token, "127.0.0.1:80",
	)
	require.NoError(t, err)
	require.True(t, flow.IsTooLargeRequest)
	require.NotEmpty(t, flow.TooLargeRequestBodyFile)
	defer os.Remove(flow.TooLargeRequestBodyFile)
	defer os.Remove(flow.TooLargeRequestHeaderFile)

	flow.CalcHash()
	db := consts.GetGormProjectDatabase()
	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	defer yakit.DeleteHTTPFlowByID(db, int64(flow.ID))

	list, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Keyword:    token,
		SourceType: "mitm-test",
	})
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	require.True(t, list.Data[0].IsTooLargeRequest)
	require.True(t, list.Data[0].IsRequestOversize)
	require.Empty(t, list.Data[0].Request)
	require.Equal(t, int64(len(body)), list.Data[0].RequestLength)

	byID, err := client.GetHTTPFlowById(context.Background(), &ypb.GetHTTPFlowByIdRequest{Id: int64(list.Data[0].Id)})
	require.NoError(t, err)
	require.True(t, byID.IsTooLargeRequest)
	require.NotEmpty(t, byID.TooLargeRequestBodyFile)
	require.Contains(t, string(byID.Request), "request too large")
	require.Less(t, len(byID.Request), len(body))

	stream, err := client.GetHTTPFlowBodyById(context.Background(), &ypb.GetHTTPFlowBodyByIdRequest{
		Id:        int64(flow.ID),
		IsRequest: true,
	})
	require.NoError(t, err)
	var chunks []byte
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		if msg == nil {
			break
		}
		chunks = append(chunks, msg.GetData()...)
		if msg.GetEOF() {
			break
		}
	}
	require.Equal(t, body, string(chunks))
}
