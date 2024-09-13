package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"testing"
)

func TestGRPCMUSTPASS_FuzzerConfig(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	saveFuzzerConfig := func(num int) {
		var Data []*ypb.FuzzerConfig
		for i := 0; i < num; i++ {
			Data = append(Data, &ypb.FuzzerConfig{
				PageId: strconv.Itoa(i),
				Type:   "group",
				Config: "{\"isHTTPS\":true",
			})
			req := &ypb.SaveFuzzerConfigRequest{
				Data: Data,
			}
			_, err = client.SaveFuzzerConfig(context.Background(), req)
			require.NoError(t, err)
		}
	}

	saveFuzzerConfig(100)
	result, err := client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Limit: 20})
	require.NoError(t, err)
	require.Equal(t, 20, len(result.GetData()))

	msg, err := client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{DeleteAll: true})
	require.Contains(t, msg.ExtraMessage, "Delete all webFuzzerConfig")
	result, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Limit: 20})
	require.NoError(t, err)
	require.Equal(t, 0, len(result.GetData()))

	saveFuzzerConfig(20)
	msg, err = client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{PageId: []string{"0", "1"}, DeleteAll: false})
	require.Contains(t, msg.ExtraMessage, "Delete webFuzzerConfig with pageId")
	require.NoError(t, err)
	result, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Limit: -1})
	require.NoError(t, err)
	require.Equal(t, 18, len(result.GetData()))
}
