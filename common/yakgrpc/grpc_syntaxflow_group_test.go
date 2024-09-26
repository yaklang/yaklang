package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Rule_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var ruleNames []string
	language := []string{"java", "php", "go", "yak"}
	purpose := []string{"vuln", "audit", "config", "security"}
	severity := []string{"info", "middle", "critical", "high"}
	groupName := make(map[string]struct{})
	for _, l := range language {
		groupName[l] = struct{}{}
	}
	for _, p := range purpose {
		groupName[p] = struct{}{}
	}
	for _, s := range severity {
		groupName[s] = struct{}{}
	}
	saveSyntaxFlowRule := func(num int) []string {
		ruleNames, reqs := generateSyntaxFlowRule(num, language, purpose, severity)
		for _, req := range reqs {
			_, err = client.SaveSyntaxFlowRule(context.Background(), req)
			require.NoError(t, err)
		}
		return ruleNames
	}

	checkSfGroup := func(originRsp, newRsp *ypb.QuerySyntaxFlowRuleGroupResponse, handler func(groupName string, n1, n2 int)) {
		for _, n := range newRsp.GetGroup() {
			for _, o := range originRsp.GetGroup() {
				_, ok := groupName[n.GetGroupName()]
				if n.GetGroupName() == o.GetGroupName() && ok {
					log.Infof("group name :%s,new count:%v,old count:%v,gap:%v", n.GetGroupName(), n.GetCount(), o.GetCount(), n.GetCount()-o.GetCount())
					handler(n.GetGroupName(), int(n.GetCount()), int(o.GetCount()))
				}
			}
		}
	}

	originRsp, err := client.QuerySyntaxFlowRuleGroup(context.Background(), &ypb.QuerySyntaxFlowRuleGroupRequest{
		All:           true,
		IsBuiltinRule: false,
	})
	require.NoError(t, err)
	ruleNames = saveSyntaxFlowRule(100)
	newRsp1, err := client.QuerySyntaxFlowRuleGroup(context.Background(), &ypb.QuerySyntaxFlowRuleGroupRequest{
		All:           true,
		IsBuiltinRule: false,
	})
	require.NoError(t, err)
	checkSfGroup(originRsp, newRsp1, func(groupName string, n1, n2 int) {
		require.Equal(t, 25, n1-n2)
	})

	deletCount := 0
	for _, ruleName := range ruleNames {
		msg, err := client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{RuleName: ruleName},
		})
		require.NoError(t, err)
		deletCount += int(msg.EffectRows)
	}
	require.Equal(t, 100, deletCount)
}

func generateSyntaxFlowRule(num int, language []string, purpose []string, severity []string) (ruleName []string, req []*ypb.SaveSyntaxFlowRuleRequest) {
	for i := 0; i < num; i++ {
		l := language[i%len(language)]
		p := purpose[i%len(purpose)]
		s := severity[i%len(severity)]
		name := fmt.Sprintf("test_%s.sf", uuid.NewString())
		ruleName = append(ruleName, name)
		content := fmt.Sprintf(`desc(
							type:  %s,
							level: %s,
						)
						check $a%d;
						`, p, s, rand.Int())
		req = append(req, &ypb.SaveSyntaxFlowRuleRequest{
			RuleName: name,
			Content:  content,
			Language: l,
			Tags:     nil,
		})
	}
	return ruleName, req
}
