package loop_ssa_api_discovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeParsedPreferBase(t *testing.T) {
	a := &ParsedUserInput{CodePath: "/a", TargetRaw: ""}
	b := &ParsedUserInput{CodePath: "/b", TargetRaw: "http://x/"}
	out := MergeParsedPreferBase(a, b)
	require.Equal(t, "/a", out.CodePath)
	require.Equal(t, NormalizeTargetString("http://x/"), out.TargetRaw)

	baseStage := &ParsedUserInput{CodePath: "/a", PipelineMaxStage: 0}
	overlayStage := &ParsedUserInput{PipelineMaxStage: 4}
	require.Equal(t, 4, MergeParsedPreferBase(baseStage, overlayStage).PipelineMaxStage)
	keep := &ParsedUserInput{CodePath: "/a", PipelineMaxStage: 3}
	require.Equal(t, 3, MergeParsedPreferBase(keep, overlayStage).PipelineMaxStage)
}

func TestPipelineIntentHeuristic(t *testing.T) {
	require.True(t, pipelineIntentHeuristic("请对 Code path: /tmp/x 做全流程", nil))
	require.True(t, pipelineIntentHeuristic("扫描攻击面发现", &ParsedUserInput{}))
	p, _ := ParseUserInputLenient("Code path: /tmp/p\n")
	require.True(t, pipelineIntentHeuristic("x", p))
}

func TestQAIntentHeuristic(t *testing.T) {
	p, _ := ParseUserInputLenient("什么是代码审计中的污点分析？")
	require.True(t, qaIntentHeuristic("什么是代码审计中的污点分析？", p))
	require.False(t, qaIntentHeuristic("Code path: /a\n全流程扫描", nil))
}

func TestClassifyHeuristicWithoutInvoker(t *testing.T) {
	p, _ := ParseUserInputLenient("解释一下 Spring 鉴权原理")
	mode, used := ClassifySsaDiscoveryRoute(context.Background(), nil, "解释一下 Spring 鉴权原理", p)
	require.Equal(t, SsaDiscoveryModeQAReview, mode)
	require.False(t, used)
}
