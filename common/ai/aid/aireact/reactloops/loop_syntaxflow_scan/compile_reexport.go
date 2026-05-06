package loop_syntaxflow_scan

import (
	"context"

	sfs "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_services"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// BuildCodeScanJSONForLocalPath builds a minimal code-scan JSON for a local file or directory.
// Implementation lives in syntaxflow_services ([sfs.BuildCodeScanJSONForLocalPath]).
func BuildCodeScanJSONForLocalPath(localPath string) (string, error) {
	return sfs.BuildCodeScanJSONForLocalPath(localPath)
}

// LoadProgramsFromCodeScanJSON parses code-scan JSON and loads SSA Programs ([sfs.LoadProgramsFromCodeScanJSON]).
func LoadProgramsFromCodeScanJSON(ctx context.Context, jsonRaw []byte) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	return sfs.LoadProgramsFromCodeScanJSON(ctx, jsonRaw)
}

// CodeScanToSyntaxFlowRuleOptions aligns extra StartScan options ([sfs.CodeScanToSyntaxFlowRuleOptions]).
func CodeScanToSyntaxFlowRuleOptions(cfg *ssaconfig.Config) []ssaconfig.Option {
	return sfs.CodeScanToSyntaxFlowRuleOptions(cfg)
}
