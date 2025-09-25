package ssa_bootstrapping

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestBuildInRule(t *testing.T) {
	testCase := []RuleChecker{{
		Name: "test git repo",
		ConfigInfo: &ssaconfig.CodeSourceInfo{
			Kind:   ssaconfig.CodeSourceGit,
			URL:    "https://github.com/digininja/DVWA",
			Branch: "master",
		},
		Language:        string(consts.PHP),
		RequiredExclude: true,
		RiskInfo: []*RiskInfo{
			{
				Kind:         Checked,
				RuleName:     "重定向漏洞",
				FileName:     "/vulnerabilities/open_redirect/source/low.php",
				Line:         4,
				StartLine:    4,
				EndLine:      4,
				StartColumn:  25,
				EndColumn:    42,
				Severity:     schema.SFR_SEVERITY_HIGH,
				RiskVariable: "high",
			},
		},
	},
	}
	require.NoError(t, startCase(testCase))
}
