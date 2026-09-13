package aireact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func useBuiltinSkillRevision(t *testing.T, revision string) *filesys.VirtualFS {
	t.Helper()
	previous := builtinSkillsFS
	t.Cleanup(func() { builtinSkillsFS = previous })
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("skills/code-review/SKILL.md", "---\nname: code-review\ndescription: Review "+revision+"\nmetadata:\n  display_name_zh-CN: 代码安全审计\n---\n\n# Review "+revision+"\n")
	vfs.AddFile("skills/code-review/references/guide.txt", strings.Repeat(revision, 12000))
	builtinSkillsFS = vfs
	return vfs
}

func readBuiltinTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	require.NoError(t, err)
	return string(content)
}

func TestBuiltinSkillUpgrade_UntouchedDefaults(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	skillsDir := t.TempDir()
	useBuiltinSkillRevision(t, "v1")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	canonical := filepath.Join(skillsDir, "builtin", "code-review")
	before, err := os.Stat(filepath.Join(canonical, "SKILL.md"))
	require.NoError(t, err)
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	after, err := os.Stat(filepath.Join(canonical, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
	useBuiltinSkillRevision(t, "v2")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, canonical, "SKILL.md"), "Review v2")
	require.Equal(t, strings.Repeat("v2", 12000), readBuiltinTestFile(t, canonical, "references/guide.txt"))
	_, err = os.Stat(canonical + "-legacy")
	require.True(t, os.IsNotExist(err))
	forge, err := yakit.GetAIForgeByName(builtinSkillReleaseDB(), "code-review")
	require.NoError(t, err)
	require.Contains(t, forge.InitPrompt, "Review v2")
}

func TestBuiltinSkillUpgrade_PreservesOneLoadableLegacy(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	useTempYakitHome(t)
	skillsDir := consts.GetDefaultAISkillsDir()
	useBuiltinSkillRevision(t, "v1")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	canonical := filepath.Join(skillsDir, "builtin", "code-review")
	_, err := updateRecommendedBuiltinSkillAtDir(skillsDir, "code-review", "# User review instructions")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(canonical, "custom.txt"), []byte("User resource"), 0o644))
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, canonical, "SKILL.md"), "User review instructions")

	useBuiltinSkillRevision(t, "v2")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	legacy := canonical + "-legacy"
	require.Contains(t, readBuiltinTestFile(t, canonical, "SKILL.md"), "Review v2")
	require.Contains(t, readBuiltinTestFile(t, legacy, "SKILL.md"), "User review instructions")
	require.Equal(t, "User resource", readBuiltinTestFile(t, legacy, "custom.txt"))
	_, err = os.Stat(filepath.Join(canonical, "custom.txt"))
	require.True(t, os.IsNotExist(err))
	meta, err := aiskillloader.ParseSkillMeta(readBuiltinTestFile(t, legacy, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "code-review-legacy", meta.Name)
	forge, err := yakit.GetAIForgeByName(builtinSkillReleaseDB(), "code-review-legacy")
	require.NoError(t, err)
	require.Contains(t, forge.InitPrompt, "User review instructions")

	// The unchanged canonical v2 must not replace the saved user copy at v3.
	useBuiltinSkillRevision(t, "v3")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, legacy, "SKILL.md"), "User review instructions")
	_, err = updateRecommendedBuiltinSkillAtDir(skillsDir, "code-review", "# Second user revision")
	require.NoError(t, err)
	useBuiltinSkillRevision(t, "v4")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, legacy, "SKILL.md"), "Second user revision")
	_, err = os.Stat(filepath.Join(legacy, "custom.txt"))
	require.True(t, os.IsNotExist(err), "legacy replacement must remove resources absent from the new backup")
	entries, err := os.ReadDir(filepath.Dir(canonical))
	require.NoError(t, err)
	require.Len(t, entries, 2)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	react, err := NewReAct(
		aicommon.WithContext(ctx),
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
	)
	require.NoError(t, err)
	current, err := react.config.GetSkillLoader().LoadSkill("code-review")
	require.NoError(t, err)
	require.Contains(t, current.Meta.Body, "Review v4")
	backup, err := react.config.GetSkillLoader().LoadSkill("code-review-legacy")
	require.NoError(t, err)
	require.Contains(t, backup.Meta.Body, "Second user revision")
}

func TestBuiltinSkillUpgrade_ResourceOnlyChangeAndNewSkill(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	skillsDir := t.TempDir()
	vfs := useBuiltinSkillRevision(t, "v1")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	canonical := filepath.Join(skillsDir, "builtin", "code-review")
	// Editing a large resource without touching SKILL.md still needs a backup.
	require.NoError(t, os.WriteFile(filepath.Join(canonical, "references", "guide.txt"), []byte(strings.Repeat("user", 12000)), 0o644))
	require.NoError(t, vfs.Delete("skills/code-review/references/guide.txt"))
	vfs.AddFile("skills/code-review/references/new.txt", "New release resource")
	vfs.AddFile("skills/new-skill/SKILL.md", "---\nname: new-skill\ndescription: New skill\n---\nNew instructions\n")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Equal(t, strings.Repeat("user", 12000), readBuiltinTestFile(t, canonical+"-legacy", "references/guide.txt"))
	require.Equal(t, "New release resource", readBuiltinTestFile(t, canonical, "references/new.txt"))
	_, err := os.Stat(filepath.Join(canonical, "references", "guide.txt"))
	require.True(t, os.IsNotExist(err))
	require.Contains(t, readBuiltinTestFile(t, filepath.Dir(canonical), "new-skill/SKILL.md"), "New instructions")
	forge, err := yakit.GetAIForgeByName(builtinSkillReleaseDB(), "new-skill")
	require.NoError(t, err)
	require.True(t, forge.IsBuiltin)
}

func TestBuiltinSkillUpgrade_BackupFailureKeepsUserCopy(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	skillsDir := t.TempDir()
	useBuiltinSkillRevision(t, "v1")
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	canonical := filepath.Join(skillsDir, "builtin", "code-review")
	_, err := updateRecommendedBuiltinSkillAtDir(skillsDir, "code-review", "# Keep my work")
	require.NoError(t, err)
	// A file occupying the reserved backup directory must cause a safe failure.
	require.NoError(t, os.WriteFile(canonical+"-legacy", []byte("occupied"), 0o644))
	useBuiltinSkillRevision(t, "v2")
	require.Error(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, canonical, "SKILL.md"), "Keep my work")
	require.NoError(t, os.Remove(canonical+"-legacy"))
	require.NoError(t, ExtractBuiltinSkillsToDir(skillsDir))
	require.Contains(t, readBuiltinTestFile(t, canonical+"-legacy", "SKILL.md"), "Keep my work")
	require.Contains(t, readBuiltinTestFile(t, canonical, "SKILL.md"), "Review v2")
}
