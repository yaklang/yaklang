package yakgrpc

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRequestYakURL(t *testing.T) {
	t.Run("fs", func(t *testing.T) {
		p := "/"
		if runtime.GOOS == "windows" {
			p = "C:\\"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)

		resources, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   p,
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "list",
					},
				},
			},
		})
		require.NoError(t, err)
		t.Logf("resources len: %d", resources.Total)
		require.Greater(t, int(resources.Total), 0, "resources should not be empty")
	})
}
