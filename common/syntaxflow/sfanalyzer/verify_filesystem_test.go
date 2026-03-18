package sfanalyzer

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	commonlog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestEvaluateVerifyFilesystemWithFrame_NoTestcases_NoErrorLog(t *testing.T) {
	var buf bytes.Buffer

	prevLevel := commonlog.DefaultLogger.Level
	commonlog.DefaultLogger.Level = commonlog.ErrorLevel
	commonlog.DefaultLogger.SetOutput(&buf)
	t.Cleanup(func() {
		commonlog.DefaultLogger.Level = prevLevel
		commonlog.DefaultLogger.SetOutput(os.Stdout)
	})

	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile("\n")
	require.NoError(t, err)
	require.NoError(t, EvaluateVerifyFilesystemWithFrame(frame))

	require.NotContains(t, buf.String(), "no positive filesystem found", "should not log errors when rule has no testcases")
}
