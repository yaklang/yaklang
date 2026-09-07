package contextmenu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActionContextHttpsStateAndParams(t *testing.T) {
	originalParams := map[string]any{"name": "demo", "enabled": "true", "limit": "12"}
	ctx := NewActionContext(context.Background(), ActionContextOptions{
		HttpsState: HttpsStateHTTPS,
		Params:     originalParams,
	})
	originalParams["name"] = "changed"

	require.True(t, ctx.HasHttpsInfo())
	require.True(t, ctx.IsHttps())
	require.True(t, ctx.ContainsHttps())
	require.Equal(t, "demo", ctx.ParamString("name"))
	require.True(t, ctx.ParamBool("enabled"))
	require.EqualValues(t, 12, ctx.ParamInt("limit"))

	copyOfParams := ctx.Params()
	copyOfParams["name"] = "changed again"
	require.Equal(t, "demo", ctx.ParamString("name"))
}

func TestActionContextUnknownAndMixedHttpsState(t *testing.T) {
	unknown := NewActionContext(nil, ActionContextOptions{HttpsState: "invalid"})
	require.Equal(t, "unknown", unknown.HttpsState())
	require.False(t, unknown.HasHttpsInfo())
	require.False(t, unknown.IsHttps())

	mixed := NewActionContext(nil, ActionContextOptions{HttpsState: HttpsStateMixed})
	require.True(t, mixed.HasHttpsInfo())
	require.False(t, mixed.IsHttps())
	require.True(t, mixed.ContainsHttps())
}

func TestPacketResultCopiesPackets(t *testing.T) {
	request := []byte("request")
	result := NewPacketResult(WithRequest(request), WithConfirmation(true))
	request[0] = 'X'

	require.Equal(t, []byte("request"), result.Request)
	require.True(t, result.ReplaceRequest)
	require.False(t, result.ReplaceResponse)
	require.True(t, result.RequireConfirmation)
}
