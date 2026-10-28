package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestServiceScanParseProtocolFieldsExport(t *testing.T) {
	for _, raw := range []string{`"SSH-2.0-OpenSSH_9.6\r\n"`, `b"SSH-2.0-OpenSSH_9.6\r\n"`} {
		var output map[string]any
		engine := antlr4yak.New()
		engine.ImportLibs(map[string]any{
			"servicescan": FingerprintScanExports,
			"capture":     func(fields map[string]any) { output = fields },
		})
		require.NoError(t, engine.SafeEval(context.Background(), `capture(servicescan.ParseProtocolFields(`+raw+`, "tcp", 22))`))
		require.Contains(t, output, "ssh")
		message := output["ssh"].([]any)[0].(map[string]any)
		require.Equal(t, "SSH-2.0-OpenSSH_9.6", message["fields"].(map[string]any)["Identification"])
	}
}
