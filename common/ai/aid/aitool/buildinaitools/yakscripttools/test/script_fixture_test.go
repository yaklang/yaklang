package test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Execute the embedded production script with invocation-local library mocks.
// No process-wide export maps are mutated, and CLI parsing/result construction
// still run in the real Yak VM. Tool conversion is covered separately.
func executeFixtureScript(t *testing.T, path string, params aitool.InvokeParams, libraries map[string]any) (any, string) {
	t.Helper()
	content, err := yakscripttools.GetEmbedFS().ReadFile(path)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var out strings.Builder
	client := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		out.WriteString(yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result, false))
		out.WriteByte('\n')
		return nil
	})
	engine := yak.NewYakitVirtualClientScriptEngine(client)
	args := []string{}
	for key, value := range params {
		args = append(args, "--"+key, fmt.Sprint(value))
	}
	engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
		yak.BindYakitPluginContextToEngine(ae, yak.CreateYakitPluginContext("fixture").WithContext(ctx).WithContextCancel(cancel).WithCliApp(yak.GetHookCliApp(args)).WithYakitClient(client))
		ae.SetVars(libraries)
		return nil
	})
	executed, err := engine.ExecuteExWithContext(ctx, string(content), map[string]any{"CTX": ctx, "RUNTIME_ID": "fixture"})
	require.NoError(t, err, out.String())
	result, _ := executed.GetVar("RESULT")
	return result, out.String()
}
