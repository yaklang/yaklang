package loop_syntaxflow_scan

import (
	"context"

	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// BuildCodeScanJSONForLocalPath builds a minimal code-scan JSON for a local file or directory.
// Implementation lives in syntaxflow_utils ([sfu.BuildCodeScanJSONForLocalPath]).
func BuildCodeScanJSONForLocalPath(localPath string) (string, error) {
	return sfu.BuildCodeScanJSONForLocalPath(localPath)
}

// LoadProgramsFromCodeScanJSON parses code-scan JSON and loads SSA Programs ([sfu.LoadProgramsFromCodeScanJSON]).
func LoadProgramsFromCodeScanJSON(ctx context.Context, jsonRaw []byte) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	return sfu.LoadProgramsFromCodeScanJSON(ctx, jsonRaw)
}

// CodeScanToSyntaxFlowRuleOptions aligns extra StartScan options ([sfu.CodeScanToSyntaxFlowRuleOptions]).
func CodeScanToSyntaxFlowRuleOptions(cfg *ssaconfig.Config) []ssaconfig.Option {
	return sfu.CodeScanToSyntaxFlowRuleOptions(cfg)
}
