package syntaxflow_forges

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SelectProjectsForCampaignResult is structured output for campaign project selection.
type SelectProjectsForCampaignResult struct {
	SelectorJSON string `json:"selector_json"`
	Reason       string `json:"reason"`
}

// SelectProjectsForCampaign maps user goals to a {"project_paths":[...]} selector for [syntaxflow_services.RunRuleAcrossProjects].
func SelectProjectsForCampaign(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string, catalogHint string) (*SelectProjectsForCampaignResult, error) {
	promptTpl := `You output a JSON campaign selector for SyntaxFlow bulk scans.

## User goal
<|USER_GOAL_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_GOAL_END|>

## Known projects (from list_ssa_projects or similar; lines of id/path)
<|CAT_{{ .Nonce }}|>
{{ .Catalog }}
<|CAT_END|>

Return project_paths as **absolute local paths** only when clearly inferable; otherwise use empty array and explain in reason.
`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Nonce":     utils.RandStringBytes(4),
		"UserInput": userInput,
		"Catalog":   strings.TrimSpace(catalogHint),
	})
	if err != nil {
		return nil, utils.Wrap(err, "render select projects prompt")
	}

	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"select-projects-for-syntaxflow-campaign",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("selector_json",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description(`JSON body like {"project_paths":["/abs/a"]}`)),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Why these paths")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("campaign", "reason"),
	)
	if err != nil {
		return nil, err
	}
	out := &SelectProjectsForCampaignResult{
		SelectorJSON: strings.TrimSpace(result.GetString("selector_json")),
		Reason:       result.GetString("reason"),
	}
	log.Infof("[syntaxflow_forges] SelectProjectsForCampaign: selector=%s reason=%q", out.SelectorJSON, out.Reason)
	return out, nil
}

// ClusterSSARisksSummary groups risk descriptions for compaction.
type ClusterSSARisksSummary struct {
	GroupsMarkdown string `json:"groups_md"`
	Reason         string `json:"reason"`
}

// ClusterSSARisks summarizes raw risk bullet text into thematic groups (Markdown).
func ClusterSSARisks(ctx context.Context, r aicommon.AIInvokeRuntime, riskBulletsText string) (*ClusterSSARisksSummary, error) {
	promptTpl := `Group the following SSA risk lines by rule/title/theme. Output Markdown bullet sections.

## Risks (verbatim lines)
{{ .Bullets }}

Return concise group headings and representative lines.`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Bullets": strings.TrimSpace(riskBulletsText),
	})
	if err != nil {
		return nil, err
	}
	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"cluster-ssa-risks-lite",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("groups_md", aitool.WithParam_Description("Markdown grouped summary")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Notes")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("cluster", "groups_md"),
	)
	if err != nil {
		return nil, err
	}
	return &ClusterSSARisksSummary{
		GroupsMarkdown: result.GetString("groups_md"),
		Reason:         result.GetString("reason"),
	}, nil
}

// SummarizeCampaignResultsResult is a short executive summary for multi-project runs.
type SummarizeCampaignResultsResult struct {
	SummaryMarkdown string `json:"summary_md"`
	Reason          string `json:"reason"`
}

// SummarizeCampaignResults compresses per-project run lines into a campaign report blurb.
func SummarizeCampaignResults(ctx context.Context, r aicommon.AIInvokeRuntime, perProjectLines string) (*SummarizeCampaignResultsResult, error) {
	promptTpl := `Summarize this multi-project SyntaxFlow campaign outcome for engineers.

## Per-project lines
{{ .Lines }}
`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Lines": strings.TrimSpace(perProjectLines),
	})
	if err != nil {
		return nil, err
	}
	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"summarize-syntaxflow-campaign-results",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("summary_md", aitool.WithParam_Description("Short Markdown summary")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Caveats")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("campaign_summary", "summary_md"),
	)
	if err != nil {
		return nil, err
	}
	return &SummarizeCampaignResultsResult{
		SummaryMarkdown: result.GetString("summary_md"),
		Reason:          result.GetString("reason"),
	}, nil
}
