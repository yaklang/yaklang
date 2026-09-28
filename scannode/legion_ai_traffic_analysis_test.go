package scannode

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func trafficAnalysisTestBinding() aiSessionBinding {
	return aiSessionBinding{Ref: aiSessionCommandRef{SessionID: "analysis"}, TrafficAnalysis: &aiv1.AITrafficAnalysisContext{SourceSessionId: "source", FlowIds: []string{"flow-a"}, ReadOnly: true}}
}
func trafficAnalysisTestInput() aiSessionInput {
	return aiSessionInput{InputType: "message", ContextPackage: &aiv1.ContextPackage{TrafficAnalysis: &aiv1.AITrafficAnalysisContext{SourceSessionId: "source", FlowIds: []string{"flow-a"}, EvidenceText: "[flow:flow-a] HTTP/1.1 200 OK", ReadOnly: true}}}
}
func TestAITrafficAnalysisPolicyRejectsExecutableBinding(t *testing.T) {
	cases := []struct {
		name    string
		options yakRuntimeOptions
	}{
		{"forge", yakRuntimeOptions{ForgeName: "shell"}},
		{"focus", yakRuntimeOptions{Focus: "arbitrary"}},
		{"workspace", yakRuntimeOptions{SourceWorkspace: &legionCodeWorkspaceSpec{}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTrafficAnalysisBinding(trafficAnalysisTestBinding(), test.options); err == nil {
				t.Fatal("executable evidence-analysis binding admitted")
			}
		})
	}
	if err := validateTrafficAnalysisBinding(trafficAnalysisTestBinding(), yakRuntimeOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestAITrafficAnalysisRejectsMissingEvidenceAndControlOverrides(t *testing.T) {
	binding := trafficAnalysisTestBinding()
	if err := validateTrafficAnalysisInput(binding, trafficAnalysisTestInput()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"hotpatch", "sync_event", "interactive_response"} {
		input := trafficAnalysisTestInput()
		input.InputType = kind
		if err := validateTrafficAnalysisInput(binding, input); err == nil {
			t.Fatalf("accepted %s override", kind)
		}
	}
	missing := trafficAnalysisTestInput()
	missing.ContextPackage = nil
	if err := validateTrafficAnalysisInput(binding, missing); err == nil {
		t.Fatal("later turn shed server-owned evidence policy")
	}
	foreign := trafficAnalysisTestInput()
	foreign.ContextPackage.TrafficAnalysis.FlowIds = []string{"foreign-flow"}
	if err := validateTrafficAnalysisInput(binding, foreign); err == nil {
		t.Fatal("foreign evidence flow admitted")
	}
}
func TestAITrafficAnalysisClampsProviderAndMessageOptions(t *testing.T) {
	config := aiengine.NewAIEngineConfig(aiengine.WithDisableToolUse(false), aiengine.WithDisableAIForge(false), aiengine.WithDisableMCPServers(false), aiengine.WithFocus("network-focus"), trafficAnalysisEnginePolicy())
	if !config.DisableToolUse || !config.DisableAIForge || !config.DisableMCPServers || config.EnableAISearchTool || config.EnableForgeSearchTool || config.Focus != "" {
		t.Fatal("provider settings escaped final evidence-only policy")
	}
	common := aicommon.NewConfig(context.Background(), config.ExtOptions...)
	if !common.IsReadOnlyEvidence() || !common.DisableToolUse || !common.DisableWebSearch {
		t.Fatal("execution-level policy was not applied")
	}
}
