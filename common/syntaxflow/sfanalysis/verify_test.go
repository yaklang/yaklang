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

func TestEvaluateVerifyFilesystemWithRule_DefaultSkipsNegativeFilesystem(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	language: yaklang,
	'file://unsafe.yak': <<<EOF
a = 1;
EOF
	'safe://safe.yak': <<<EOF
a = 1;
EOF
)

a as $output;
alert $output;
`}

	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()), "alert symbol table not empty")
}

func TestEvaluateVerifyFilesystemWithRule_DefaultAllowsAlertHighOverflow(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	language: yaklang,
	alert_high: 1,
	'file://unsafe.yak': <<<EOF
a = 1;
EOF
	'safe://safe.yak': <<<EOF
b = 1;
EOF
)

a as $first;
a as $second;
alert $first for {level: "high"};
alert $second for {level: "high"};
`}

	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()), "alert symbol table is less than alert_high config")
}
