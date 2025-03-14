package ssa_bootstrapping

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
	"time"
)

func TestBuildInRule(t *testing.T) {
	testCase := []RuleChecker{
		{
			Name: "test git repo",
			ConfigInfo: &ssaapi.ConfigInfo{
				Kind:   ssaapi.Git,
				URL:    "https://github.com/digininja/DVWA",
				Branch: "master",
			},
			RuleNames:       []string{},
			Language:        string(consts.PHP),
			RequiredExclude: true,
			RiskInfo: []*RiskInfo{
				{
					Kind:         Checked,
					RuleName:     "sql(mysql)注入漏洞检测",
					FileName:     "/vulnerabilities/open_redirect/low.php",
					Line:         4,
					StartLine:    4,
					EndLine:      4,
					StartColumn:  25,
					EndColumn:    40,
					Severity:     schema.SFR_SEVERITY_HIGH,
					RiskVariable: "high_risk",
				},
			},
		},
	}
	for _, Case := range testCase {
		fmt.Println(time.Now().Format("2006-01-02 15:04:05"))
		err := Case.run()
		fmt.Println(time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Error(err)
		}
	}
}
