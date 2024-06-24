package sfvm

import (
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *SFFrameResult) Sarif() (*sarif.Report, error) {
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}
	report.AddRun(
		sarif.NewRun(
			*sarif.NewSimpleTool("syntaxflow"),
		).WithDefaultSourceLanguage(
			"java",
		).WithDefaultEncoding(
			"utf-8",
		).WithArtifacts([]*sarif.Artifact{
			sarif.NewArtifact(),
		}).WithResults([]*sarif.Result{
			sarif.NewRuleResult(
				r.Rule,
			).WithCodeFlows([]*sarif.CodeFlow{
				sarif.NewCodeFlow(),
			}).WithLocations([]*sarif.Location{
				sarif.NewLocation().WithPhysicalLocation(
					sarif.NewPhysicalLocation().WithArtifactLocation(
						sarif.NewArtifactLocation(), // .WithIndex(),
					),
				),
			}).WithRelatedLocations([]*sarif.Location{}),
		}),
	)
	return report, nil
}
