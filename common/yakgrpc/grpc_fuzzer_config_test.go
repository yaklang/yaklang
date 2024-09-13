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

	_, err = client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{PageId: "0", DeleteAll: true})
	result, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Limit: 20})
	require.NoError(t, err)
	require.Equal(t, 0, len(result.GetData()))

	saveFuzzerConfig(20)
	_, err = client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{PageId: "1"})
	require.NoError(t, err)
	result, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{})
	require.NoError(t, err)
	require.Equal(t, 19, len(result.GetData()))
}
