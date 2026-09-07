package yakscript

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
)

func TestExecContextMenuScriptPacketResult(t *testing.T) {
	baseCtx := context.Background()
	actionCtx := contextmenu.NewActionContext(baseCtx, contextmenu.ActionContextOptions{
		Scene:      contextmenu.ActionHTTPPacket,
		HttpsState: contextmenu.HttpsStateUnknown,
		HasRequest: true,
	})
	script := &schema.YakScript{
		ScriptName: "packet-context-menu-test",
		Type:       contextmenu.PluginType,
		Uuid:       "packet-context-menu-test-uuid",
		Content: `
handleHTTPPacket = func(ctx, request, response) {
    return context.NewPacketResult(
        context.ReplaceRequest(request),
        context.RequireConfirmation(true),
    )
}
`,
	}
	stream := NewFakeStream(baseCtx, nil)

	result, err := ExecContextMenuScript(
		script,
		contextmenu.HookHTTPPacket,
		actionCtx,
		[]any{[]byte("GET / HTTP/1.1\r\n\r\n"), []byte(nil)},
		stream,
		nil,
		"runtime-test",
		nil,
	)
	require.NoError(t, err)
	packetResult, ok := result.(*contextmenu.PacketActionResult)
	require.True(t, ok)
	require.True(t, packetResult.ReplaceRequest)
	require.True(t, packetResult.RequireConfirmation)
	require.Equal(t, []byte("GET / HTTP/1.1\r\n\r\n"), packetResult.Request)
}
