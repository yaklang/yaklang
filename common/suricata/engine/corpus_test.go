package engine

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func TestETCorpusHTTPAndUDPStatefulCoverage(t *testing.T) {
	data, err := os.ReadFile("../rules/emerging-scan.rules")
	require.NoError(t, err)
	rules, err := rule.Parse(string(data))
	require.NoError(t, err)
	executable, rejected := 0, 0
	for _, current := range rules {
		if current == nil {
			continue
		}
		if _, err := CompileStatefulRule(current); err == nil {
			executable++
		} else {
			rejected++
		}
	}
	require.Equal(t, 275, executable)
	require.Equal(t, 9, rejected)
}

func TestETTCPCorpusStatefulCompilation(t *testing.T) {
	data, err := os.ReadFile("../rules/emerging-scan.rules")
	require.NoError(t, err)
	rules, err := rule.Parse(string(data))
	require.NoError(t, err)
	var tcpRules, executable int
	rejections := map[string]int{}
	for _, current := range rules {
		if current.Protocol != "tcp" {
			continue
		}
		tcpRules++
		if _, err := CompileStatefulRule(current); err != nil {
			rejections[err.Error()]++
			continue
		}
		executable++
	}
	require.Equal(t, 55, tcpRules)
	require.Equal(t, 52, executable)
	require.Len(t, rejections, 3)
	for reason, count := range rejections {
		t.Logf("rejected=%d reason=%s", count, reason)
	}
	t.Logf("ET TCP rules=%d executable=%d rejected=%d", tcpRules, executable, tcpRules-executable)
}
