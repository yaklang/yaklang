package aireact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/consts"
)

func TestGetRecommendedBuiltinSkills(t *testing.T) {
	skills, err := GetRecommendedBuiltinSkills()
	require.NoError(t, err)
	require.Len(t, skills, 3)
	require.Equal(t, []string{
		"security-engineering",
		"code-review",
		"pentest-task-design",
	}, []string{skills[0].Name, skills[1].Name, skills[2].Name})
	require.Equal(t, []string{
		"安全领域",
		"代码安全审计",
		"渗透测试",
	}, []string{skills[0].DisplayNameZhCN, skills[1].DisplayNameZhCN, skills[2].DisplayNameZhCN})
	for _, skill := range skills {
		require.NotEmpty(t, skill.Description)
		require.NotEmpty(t, skill.Content)
	}
}

func TestRecommendedBuiltinSkillUpdateAndReset(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	skillsDir := t.TempDir()

	// 编辑只替换正文，预期内置 name 和展示元数据保持不变。
	updated, err := updateRecommendedBuiltinSkillAtDir(skillsDir, "security-engineering", "# 自定义安全框架\n\n只使用低影响检查。")
	require.NoError(t, err)
	require.True(t, updated.IsModified)
	require.Contains(t, updated.Content, "自定义安全框架")

	localDocument, err := os.ReadFile(filepath.Join(skillsDir, "builtin", "security-engineering", "SKILL.md"))
	require.NoError(t, err)
	meta, err := aiskillloader.ParseSkillMeta(string(localDocument))
	require.NoError(t, err)
	require.Equal(t, "security-engineering", meta.Name)
	require.Equal(t, "安全领域", meta.GetDisplayName(aiskillloader.SkillLocaleZhCN))

	// 恢复默认会重写完整内置文档，预期修改标记清除。
	reset, err := resetRecommendedBuiltinSkillAtDir(skillsDir, "security-engineering")
	require.NoError(t, err)
	require.False(t, reset.IsModified)
	require.NotContains(t, reset.Content, "自定义安全框架")
}

func TestRecommendedBuiltinSkillUpdateRejectsInvalidInput(t *testing.T) {
	// 仅固定推荐列表可写且正文不能为空，预期拒绝越界名称和空内容。
	_, err := updateRecommendedBuiltinSkillAtDir(t.TempDir(), "../../other-skill", "custom")
	require.Error(t, err)
	_, err = updateRecommendedBuiltinSkillAtDir(t.TempDir(), "code-review", "  ")
	require.Error(t, err)
}

func TestRecommendedBuiltinSkillCanonicalCopyWinsNameCollision(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	useTempYakitHome(t)
	skillsDir := consts.GetDefaultAISkillsDir()
	legacyPath := filepath.Join(skillsDir, "code-review", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte("---\nname: code-review\ndescription: legacy\n---\n\n# legacy copy\n"), 0o644))
	_, err := updateRecommendedBuiltinSkillAtDir(skillsDir, "code-review", "# canonical edited copy")
	require.NoError(t, err)

	// 同名历史副本存在时，预期新会话仍加载推荐接口维护的 builtin 版本。
	react, err := NewReAct(
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
	)
	require.NoError(t, err)
	loaded, err := react.config.GetSkillLoader().LoadSkill("code-review")
	require.NoError(t, err)
	require.Contains(t, loaded.SkillMDContent, "canonical edited copy")
	require.NotContains(t, loaded.SkillMDContent, "legacy copy")
}
