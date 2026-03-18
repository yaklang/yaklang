package sfanalysis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEvaluateVerifyFilesystemWithRule_BlankIsNoop(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: " \n\t "}
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()))
}
