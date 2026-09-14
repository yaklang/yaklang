package aiskillloader

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func builtinSyncLoader(t *testing.T, revision string) *AutoSkillLoader {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("skills/review/SKILL.md", buildTestSkillMD("review", "Review "+revision, "# Review\n"+revision))
	vfs.AddFile("skills/review/references/guide.txt", strings.Repeat(revision, 12000))
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	require.NoError(t, err)
	return loader
}

func requireBuiltinSync(t *testing.T, db *gorm.DB, revision string, count int) {
	t.Helper()
	actual, err := SyncBuiltinAISkillsToDB(db, builtinSyncLoader(t, revision))
	require.NoError(t, err)
	require.Equal(t, count, actual)
}

func requireSkillForge(t *testing.T, db *gorm.DB, name string) *schema.AIForge {
	t.Helper()
	forge, err := yakit.GetAIForgeByNameAndTypes(db, name, schema.FORGE_TYPE_SkillMD)
	require.NoError(t, err)
	return forge
}

func TestSyncBuiltinAISkills_UnmodifiedAndNew(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	requireBuiltinSync(t, db, "v1", 1)
	original := requireSkillForge(t, db, "review")
	require.True(t, original.IsBuiltin)
	require.Equal(t, schema.AIResourceAuthorBuiltin, original.Author)
	require.NotEmpty(t, original.BuiltinSkillHash)
	requireBuiltinSync(t, db, "v1", 0)
	require.Equal(t, original.UpdatedAt, requireSkillForge(t, db, "review").UpdatedAt)
	requireBuiltinSync(t, db, "v2", 1)
	updated := requireSkillForge(t, db, "review")
	require.Equal(t, original.ID, updated.ID)
	require.Equal(t, original.CreatedAt, updated.CreatedAt)
	require.Contains(t, updated.InitPrompt, "v2")
	loaded, err := AIForgeToLoadedSkill(updated)
	require.NoError(t, err)
	resource, err := loaded.FileSystem.ReadFile("references/guide.txt")
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("v2", 12000), string(resource))
	_, err = yakit.GetAIForgeByName(db, "review-legacy")
	require.True(t, gorm.IsRecordNotFoundError(err))

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("new/SKILL.md", buildTestSkillMD("new-skill", "New builtin", "New content"))
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	require.NoError(t, err)
	count, err := SyncBuiltinAISkillsToDB(db, loader)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.True(t, requireSkillForge(t, db, "new-skill").IsBuiltin)
}

func TestSyncBuiltinAISkills_UserEditsAndSingleLegacy(t *testing.T) {
	for _, field := range []string{"body", "description", "tags", "resources", "preferences"} {
		t.Run(field, func(t *testing.T) {
			db := newSearchTestDB(t)
			defer db.Close()
			requireBuiltinSync(t, db, "v1", 1)
			custom := requireSkillForge(t, db, "review")
			switch field {
			case "body":
				custom.InitPrompt = "User's review workflow"
			case "description":
				custom.Description = "Custom review description"
			case "tags":
				custom.Tags = "team:custom"
			case "resources":
				resources := filesys.NewVirtualFs()
				resources.AddFile("references/custom.txt", strings.Repeat("user", 12000))
				var err error
				custom.FSBytes, err = filesys.SerializeFileSystemToGzipBytes(resources)
				require.NoError(t, err)
			case "preferences":
				custom.UserPersistentData = `{"team":"custom"}`
			}
			// Exercise the real user update helper, including a request without the
			// internal revision field (as happens with GRPC2AIForge).
			baseline := custom.BuiltinSkillHash
			custom.BuiltinSkillHash = ""
			require.NoError(t, yakit.UpdateAIForge(db, custom))
			require.Equal(t, baseline, requireSkillForge(t, db, "review").BuiltinSkillHash)
			requireBuiltinSync(t, db, "v1", 0)
			requireBuiltinSync(t, db, "v2", 1)
			legacy := requireSkillForge(t, db, "review-legacy")
			require.Equal(t, custom.InitPrompt, legacy.InitPrompt)
			require.Equal(t, custom.Description, legacy.Description)
			require.Equal(t, custom.Tags, legacy.Tags)
			require.Equal(t, custom.FSBytes, legacy.FSBytes)
			require.Equal(t, custom.UserPersistentData, legacy.UserPersistentData)
			require.False(t, legacy.IsBuiltin)
			require.Empty(t, legacy.BuiltinSkillHash)
			loaded, err := AIForgeToLoadedSkill(legacy)
			require.NoError(t, err)
			require.Equal(t, "review-legacy", loaded.Meta.Name)
			requireBuiltinSync(t, db, "v3", 1)
			require.Equal(t, legacy.UpdatedAt, requireSkillForge(t, db, "review-legacy").UpdatedAt)
			current := requireSkillForge(t, db, "review")
			current.InitPrompt = "Second user edit"
			require.NoError(t, yakit.UpdateAIForge(db, current))
			requireBuiltinSync(t, db, "v4", 1)
			replacement := requireSkillForge(t, db, "review-legacy")
			require.Equal(t, legacy.ID, replacement.ID)
			require.Equal(t, "Second user edit", replacement.InitPrompt)
			var count int
			require.NoError(t, db.Unscoped().Model(&schema.AIForge{}).Where("forge_name LIKE ?", "review-legacy%").Count(&count).Error)
			require.Equal(t, 1, count)
		})
	}
}

func TestSyncBuiltinAISkills_Migration(t *testing.T) {
	for _, customized := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "unknown-differing"}[customized], func(t *testing.T) {
			db := newSearchTestDB(t)
			defer db.Close()
			loader := builtinSyncLoader(t, "v1")
			_, err := ImportAISkillsToDB(db, loader)
			require.NoError(t, err)
			if customized {
				record := requireSkillForge(t, db, "review")
				record.InitPrompt = "Previous installation contents"
				require.NoError(t, yakit.UpdateAIForge(db, record))
			}
			requireBuiltinSync(t, db, "v1", 1)
			requireBuiltinSync(t, db, "v1", 0)
			_, err = yakit.GetAIForgeByName(db, "review-legacy")
			if customized {
				require.NoError(t, err)
			} else {
				require.True(t, gorm.IsRecordNotFoundError(err))
			}
		})
	}
}

func TestSyncBuiltinAISkills_RollsBackBackupAndRelease(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	requireBuiltinSync(t, db, "v1", 1)
	custom := requireSkillForge(t, db, "review")
	custom.InitPrompt = "Keep this user edit"
	require.NoError(t, yakit.UpdateAIForge(db, custom))
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_builtin_update BEFORE UPDATE ON ai_forges WHEN NEW.forge_name = 'review' BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END`).Error)
	_, err := SyncBuiltinAISkillsToDB(db, builtinSyncLoader(t, "v2"))
	require.ErrorContains(t, err, "simulated write failure")
	current := requireSkillForge(t, db, "review")
	require.Equal(t, custom.InitPrompt, current.InitPrompt)
	require.Equal(t, custom.BuiltinSkillHash, current.BuiltinSkillHash)
	_, err = yakit.GetAIForgeByName(db, "review-legacy")
	require.True(t, gorm.IsRecordNotFoundError(err))
	require.NoError(t, db.Exec("DROP TRIGGER fail_builtin_update").Error)
	requireBuiltinSync(t, db, "v2", 1)
	require.Equal(t, custom.InitPrompt, requireSkillForge(t, db, "review-legacy").InitPrompt)
}

func TestSyncBuiltinAISkills_ResourceOnlyRevision(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("review/SKILL.md", buildTestSkillMD("review", "Review", "Unchanged body"))
	vfs.AddFile("review/old.txt", strings.Repeat("old", 12000))
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	require.NoError(t, err)
	_, err = SyncBuiltinAISkillsToDB(db, loader)
	require.NoError(t, err)
	require.NoError(t, vfs.Delete("review/old.txt"))
	vfs.AddFile("review/new.txt", strings.Repeat("new", 12000))
	count, err := SyncBuiltinAISkillsToDB(db, loader)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	updated, err := AIForgeToLoadedSkill(requireSkillForge(t, db, "review"))
	require.NoError(t, err)
	_, err = updated.FileSystem.ReadFile("old.txt")
	require.Error(t, err)
	raw, err := updated.FileSystem.ReadFile("new.txt")
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("new", 12000), string(raw))
	_, err = yakit.GetAIForgeByName(db, "review-legacy")
	require.True(t, gorm.IsRecordNotFoundError(err))
}

func TestSyncBuiltinAISkills_ConcurrentUpgradeAndSearch(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	requireBuiltinSync(t, db, "v1", 1)
	custom := requireSkillForge(t, db, "review")
	custom.Description = "Customized corpus marker"
	require.NoError(t, yakit.UpdateAIForge(db, custom))
	loader := builtinSyncLoader(t, "v2")
	var wg sync.WaitGroup
	counts := make(chan int, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := SyncBuiltinAISkillsToDB(db, loader)
			counts <- count
			errors <- err
		}()
	}
	wg.Wait()
	close(counts)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	total := 0
	for count := range counts {
		total += count
	}
	require.Equal(t, 1, total)
	results, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{Keywords: []string{"Customized corpus"}}, 10, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "review-legacy", results[0].ForgeName)
	dbLoader, err := NewAutoSkillLoader(WithAutoLoad_Database(db))
	require.NoError(t, err)
	legacy, err := dbLoader.LoadSkill("review-legacy")
	require.NoError(t, err)
	require.Equal(t, custom.Description, legacy.Meta.Description)
}

func TestSyncBuiltinAISkills_LegacyWriteFailure(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	requireBuiltinSync(t, db, "v1", 1)
	custom := requireSkillForge(t, db, "review")
	custom.InitPrompt = "First customization"
	require.NoError(t, yakit.UpdateAIForge(db, custom))
	requireBuiltinSync(t, db, "v2", 1)
	custom = requireSkillForge(t, db, "review")
	custom.InitPrompt = "Second customization"
	require.NoError(t, yakit.UpdateAIForge(db, custom))
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_legacy_update BEFORE UPDATE ON ai_forges WHEN NEW.forge_name = 'review-legacy' BEGIN SELECT RAISE(ABORT, 'legacy write failure'); END`).Error)
	_, err := SyncBuiltinAISkillsToDB(db, builtinSyncLoader(t, "v3"))
	require.ErrorContains(t, err, "legacy write failure")
	require.Equal(t, "First customization", requireSkillForge(t, db, "review-legacy").InitPrompt)
	require.Equal(t, "Second customization", requireSkillForge(t, db, "review").InitPrompt)
}

func TestSyncBuiltinAISkills_PreservesMalformedArchive(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	requireBuiltinSync(t, db, "v1", 1)
	custom := requireSkillForge(t, db, "review")
	custom.FSBytes = []byte("damaged custom archive")
	require.NoError(t, yakit.UpdateAIForge(db, custom))
	requireBuiltinSync(t, db, "v2", 1)
	require.Equal(t, custom.FSBytes, requireSkillForge(t, db, "review-legacy").FSBytes)
	require.Contains(t, requireSkillForge(t, db, "review").InitPrompt, "v2")
}

func TestSkillDocumentRenamePreservesFrontmatter(t *testing.T) {
	document, err := ParseSkillDocument("---\nname: review\ndescription: Review code\nlicense: MIT\ncustom-options:\n  enabled: true\nmetadata:\n  display_name_zh-CN: 代码审计\n---\n\n# Customized instructions\n")
	require.NoError(t, err)
	renamed, err := document.Rename("review-legacy")
	require.NoError(t, err)
	meta, err := ParseSkillMeta(renamed)
	require.NoError(t, err)
	require.Equal(t, "review-legacy", meta.Name)
	require.Equal(t, "MIT", meta.License)
	require.Equal(t, "代码审计", meta.GetDisplayName(SkillLocaleZhCN))
	require.Contains(t, renamed, "custom-options:")
	require.Contains(t, renamed, "enabled: true")
	require.Equal(t, document.Meta.Body, meta.Body)
}
