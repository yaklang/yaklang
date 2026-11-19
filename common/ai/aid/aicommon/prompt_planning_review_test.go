package aicommon

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise the published examples through the shared schema and real review
// response parser, with a scripted model rather than a network call.
func TestCodingPlanningReviewExamples(t *testing.T) {
	prompts := AgentPrompts(ypb.AgentModeCoding)
	for _, tc := range []struct {
		name      string
		heading   string
		examples  string
		schema    string
		eventType schema.EventType
		review    PlanningReviewControl
	}{
		{"plan", "# Coding Plan Review", prompts.PlanReviewOutputExample, aiPlanReviewSchemaTemplate, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, DefaultAIPlanReviewControl},
		{"task", "# Coding Task Review", prompts.TaskReviewOutputExample, aiTaskReviewSchemaTemplate, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, DefaultAITaskReviewControl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var definition any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &definition))
			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("review.json", definition))
			compiled, err := compiler.Compile("review.json")
			require.NoError(t, err)
			examples := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(tc.examples, -1)
			require.NotEmpty(t, examples)
			for _, example := range examples {
				var expected map[string]any
				require.NoError(t, json.Unmarshal([]byte(example[1]), &expected))
				require.NoError(t, compiled.Validate(expected))
				t.Run(expected["suggestion"].(string), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					calls := 0
					cfg := newDynamicPlanningTestConfig(ctx, func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
						calls++
						require.Contains(t, req.GetPrompt(), tc.heading)
						require.Contains(t, req.GetPrompt(), `"working_directory": "/coding-checkout"`)
						require.NotContains(t, req.GetPrompt(), "sqlmap")
						rsp := NewUnboundAIResponse()
						rsp.EmitOutputStream(strings.NewReader(example[1]))
						rsp.Close()
						return rsp, nil
					}, WithAgentMode(ypb.AgentModeCoding))
					cfg.Coding.PrimaryProjectDir = "/coding-checkout"
					ep := cfg.Epm.CreateEndpointWithEventType(tc.eventType)
					ep.SetReviewMaterials(aitool.InvokeParams{"short_summary": "parser updated"})
					result, err := tc.review(ctx, cfg, ep)
					require.NoError(t, err)
					require.Equal(t, 1, calls)
					require.Equal(t, expected["suggestion"], result["suggestion"])
					if deltas, ok := expected["task_deltas"]; ok {
						require.Equal(t, deltas, result["task_deltas"])
						require.Equal(t, expected["task_delta_summary"], result["task_delta_summary"])
					}
				})
			}
		})
	}
}

func TestPlanningReviewPromptCacheBoundaries(t *testing.T) {
	for _, mode := range []string{ypb.AgentModeSecurity, ypb.AgentModeCoding} {
		for name, build := range map[string]func(*Config, aitool.InvokeParams) (string, error){
			"plan": generatePlanReviewPrompt,
			"task": generateTaskReviewPrompt,
		} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				cfg := &Config{AgentMode: mode, Workdir: "/artifacts", Language: "zh",
					Coding: CodingConfig{PrimaryProjectDir: "/checkout"}}
				sections := func(summary string) map[string][]*aiprojection.Chunk {
					prompt, err := build(cfg, aitool.InvokeParams{"short_summary": summary})
					require.NoError(t, err)
					chunks := promptBuilderChunksBySection(t, prompt)
					require.Empty(t, chunks[aiprojection.SectionRaw])
					return chunks
				}
				first, second := sections("first result"), sections("second result")
				for _, section := range []string{aiprojection.SectionHighStatic, aiprojection.SectionSemiDynamic1, aiprojection.SectionSemiDynamic2} {
					require.NotEmpty(t, first[section])
					require.Equal(t, first[section], second[section])
				}
				require.NotEqual(t, first[aiprojection.SectionDynamic], second[aiprojection.SectionDynamic])
				cfg.Workdir = "/other-artifacts"
				cfg.Coding.PrimaryProjectDir = "/other-checkout"
				third := sections("second result")
				require.Equal(t, second[aiprojection.SectionHighStatic], third[aiprojection.SectionHighStatic])
				require.Equal(t, second[aiprojection.SectionSemiDynamic2], third[aiprojection.SectionSemiDynamic2])
				require.NotEqual(t, second[aiprojection.SectionSemiDynamic1], third[aiprojection.SectionSemiDynamic1])
			})
		}
	}
}
