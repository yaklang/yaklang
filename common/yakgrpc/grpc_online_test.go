package yakgrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDownloadOnlinePluginByUUID(t *testing.T) {
	for _, scenario := range []string{"download and persist", "remote error", "missing UUID"} {
		t.Run(scenario, func(t *testing.T) {
			db := newOnlineTestDB(t, &schema.YakScript{})
			calls := 0
			remote := &stubOnlineService{downloadPlugin: func(token, id string) (*yaklib.OnlinePlugin, error) {
				calls++
				require.Equal(t, "fixture-token", token)
				require.Equal(t, "fixture-uuid", id)
				if scenario == "remote error" {
					return nil, errors.New("remote unavailable")
				}
				return &yaklib.OnlinePlugin{UUID: id, ScriptName: "fixture-plugin", Type: "yak", Content: `println("fixture")`}, nil
			}}
			client, err := newInMemoryClient(&Server{profileDatabase: db, onlineClient: remote})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			req := &ypb.DownloadOnlinePluginByUUIDRequest{UUID: "fixture-uuid", Token: "fixture-token"}
			if scenario == "missing UUID" {
				req.UUID = ""
			}
			result, err := client.DownloadOnlinePluginByUUID(context.Background(), req)
			if scenario != "download and persist" {
				require.Error(t, err)
				require.Nil(t, result)
				if scenario == "missing UUID" {
					require.Zero(t, calls)
				} else {
					require.Equal(t, 1, calls)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, "fixture-plugin", result.GetScriptName())
			saved, err := yakit.GetYakScriptByName(db, "fixture-plugin")
			require.NoError(t, err)
			require.Equal(t, req.UUID, saved.Uuid)
			require.Equal(t, `println("fixture")`, saved.Content)
		})
	}
}
