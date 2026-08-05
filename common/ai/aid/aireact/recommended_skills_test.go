package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRecommendedBuiltinSkills(t *testing.T) {
	skills, err := GetRecommendedBuiltinSkills()
	require.NoError(t, err)
	require.Len(t, skills, 2)
	require.Equal(t, []string{
		"pentest-task-design",
		"code-review",
	}, []string{skills[0].Name, skills[1].Name})
	require.Equal(t, []string{
		"渗透测试",
		"代码安全审计",
	}, []string{skills[0].DisplayNameZhCN, skills[1].DisplayNameZhCN})
	for _, skill := range skills {
		require.NotEmpty(t, skill.Description)
	}
}
