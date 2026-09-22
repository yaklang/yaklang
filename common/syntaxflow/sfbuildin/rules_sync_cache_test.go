//go:build !irify_exclude

package sfbuildin

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func collectTestRuleFiles(t *testing.T) []ruleFile {
	t.Helper()
	var files []ruleFile
	require.NoError(t, filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(s)
		if err != nil {
			return err
		}
		dirName, name := ruleFSWithHash.PathSplit(s)
		files = append(files, ruleFile{path: s, name: name, content: string(raw), tags: ruleTagsFromDir(dirName)})
		return nil
	})))
	return files
}

func TestBuiltinSnapshotReplacement(t *testing.T) {
	db, err := consts.CreateProfileDatabase(filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	userFS := filesys.NewVirtualFs()
	userFS.AddFile("user.sf", `desc(title: "user"); * as $a; alert $a;`)
	require.NoError(t, SyncRuleFromFileSystemToDB(db, userFS, false))
	get := func(name string) schema.SyntaxFlowRule {
		var row schema.SyntaxFlowRule
		require.NoError(t, db.Where("rule_name = ?", name).First(&row).Error)
		return row
	}
	userID := get("user.sf").ID
	fs := filesys.NewVirtualFs()
	fs.AddFile("keep.sf", `desc(
 title: "keep"
)
* as $a; alert $a;`)
	fs.AddFile("remove.sf", `desc(
 title: "remove"
)
* as $a; alert $a;`)
	syncSnapshot := func(fs filesys_interface.FileSystem) error {
		return syncRuleFromFileSystemToDB(db, fs, true, true)
	}
	require.NoError(t, syncSnapshot(fs))
	old := get("keep")
	removed := get("remove")
	require.NoError(t, syncSnapshot(fs))
	require.Equal(t, old.ID, get("keep").ID, "unchanged snapshot must not rewrite rules")

	// Removing a file alone is also a snapshot change, even when no remaining
	// rule needs recompilation according to the per-rule cache.
	next := filesys.NewVirtualFs()
	next.AddFile("keep.sf", `desc(
 title: "keep"
)
* as $a; alert $a;`)
	require.NoError(t, syncSnapshot(next))
	require.NotEqual(t, old.ID, get("keep").ID)
	var count int64
	require.NoError(t, db.Unscoped().Model(&schema.SyntaxFlowRule{}).Where("id IN (?)", []uint{old.ID, removed.ID}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Table("syntax_flow_rule_and_group").Where("syntax_flow_rule_id IN (?)", []uint{old.ID, removed.ID}).Count(&count).Error)
	require.Zero(t, count)
	require.Equal(t, userID, get("user.sf").ID)

	updated := filesys.NewVirtualFs()
	updated.AddFile("keep.sf", `desc(
 title: "keep"
)
* as $b; alert $b;`)
	require.NoError(t, syncSnapshot(updated))
	require.Contains(t, get("keep").Content, "$b")
	beforeFailure := get("keep")
	invalid := filesys.NewVirtualFs()
	invalid.AddFile("broken.sf", "desc(")
	require.Error(t, syncSnapshot(invalid))
	require.Equal(t, beforeFailure.ID, get("keep").ID, "compile failure must preserve installed rules")

	require.NoError(t, syncSnapshot(filesys.NewVirtualFs()))
	require.NoError(t, db.Unscoped().Model(&schema.SyntaxFlowRule{}).Where("is_build_in_rule = ?", true).Count(&count).Error)
	require.Zero(t, count)
	require.Equal(t, userID, get("user.sf").ID)
}

func TestSyncRuleFromFileSystemToDB_ReusesLegacyMode(t *testing.T) {
	db, err := consts.CreateProfileDatabase(filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	const content = `desc(title: "legacy probe"); * as $a; alert $a;`
	fs := filesys.NewVirtualFs()
	fs.AddFile("legacy.sf", content)
	require.NoError(t, SyncRuleFromFileSystemToDB(db, fs, false))
	var before schema.SyntaxFlowRule
	require.NoError(t, db.Where("rule_name = ?", "legacy.sf").First(&before).Error)
	require.NoError(t, db.Model(&schema.SyntaxFlowRule{}).Where("rule_name = ?", "legacy.sf").UpdateColumn("mode", nil).Error)
	require.NoError(t, SyncRuleFromFileSystemToDB(db, fs, false))
	var after schema.SyntaxFlowRule
	require.NoError(t, db.Where("rule_name = ?", "legacy.sf").First(&after).Error)
	require.Equal(t, before.ID, after.ID, "the complete stored rule should be reused")
}

func TestSyncRuleFromFileSystemToDB_RepairsMissingOpcodes(t *testing.T) {
	db, err := consts.CreateProfileDatabase(filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	fs := filesys.NewVirtualFs()
	fs.AddFile("repair.sf", `desc(title: "repair probe"); * as $a; alert $a;`)
	require.NoError(t, SyncRuleFromFileSystemToDB(db, fs, false))
	require.NoError(t, db.Model(&schema.SyntaxFlowRule{}).Where("rule_name = ?", "repair.sf").UpdateColumn("op_codes", "").Error)
	require.NoError(t, SyncRuleFromFileSystemToDB(db, fs, false))
	var repaired schema.SyntaxFlowRule
	require.NoError(t, db.Where("rule_name = ?", "repair.sf").First(&repaired).Error)
	require.NotEmpty(t, repaired.OpCodes, "unchanged content must not prevent repairing missing opcodes")
}

// TestSyncRuleFromFileSystemToDB_ReusesStoredRules is the regression guard for
// the cold-start sync cost: a second sync over unchanged embed rules must not
// recompile them, and a rule whose content changed must still be re-imported.
func TestSyncRuleFromFileSystemToDB_ReusesStoredRules(t *testing.T) {
	oldProfileDB := consts.GetGormProfileDatabase()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	profileDB, err := consts.CreateProfileDatabase(dbPath)
	require.NoError(t, err)
	consts.BindProfileDatabase(profileDB, dbPath)
	t.Cleanup(func() {
		consts.BindProfileDatabase(oldProfileDB, "")
		_ = profileDB.Close()
	})

	// First sync: every embed rule is compiled and stored with opcodes.
	firstStart := time.Now()
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, ruleFSWithHash, true))
	firstCost := time.Since(firstStart)

	var stored int64
	require.NoError(t, profileDB.Table("syntax_flow_rules").
		Where("op_codes IS NOT NULL AND op_codes != ''").Count(&stored).Error)
	require.Greater(t, stored, int64(0), "first sync must persist compiled opcodes")

	storedRules, err := sfdb.LoadStoredRulesForSync(profileDB, true)
	require.NoError(t, err)
	require.Len(t, storedRules, int(stored), "every compiled rule must be reusable")
	storedByContent := make(map[string]*schema.SyntaxFlowRule, len(storedRules))
	for _, rule := range storedRules {
		require.NotNil(t, rule)
		require.NotEmpty(t, rule.Content)
		require.NotEmpty(t, rule.OpCodes)
		storedByContent[rule.Content] = rule
	}

	// Every embed rule must resolve to a complete AST-derived stored rule.
	files := collectTestRuleFiles(t)
	require.NotEmpty(t, files)
	for _, f := range files {
		rule := storedByContent[f.content]
		require.NotNil(t, rule, "rule %s was not recognized by exact content", f.name)
	}

	// Second sync: unchanged rules are reused from the database, so the pass
	// must be dramatically cheaper than the first compile.
	secondStart := time.Now()
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, ruleFSWithHash, true))
	secondCost := time.Since(secondStart)
	require.Less(t, secondCost, firstCost/2,
		"second sync reused nothing: first=%v second=%v", firstCost, secondCost)

	// The forced re-import escape hatch rewrites the snapshot while reusing
	// already parsed rule structures.
	t.Setenv("YAK_SYNTAXFLOW_FORCE_RULE_SYNC", "true")
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, ruleFSWithHash, true))
	t.Setenv("YAK_SYNTAXFLOW_FORCE_RULE_SYNC", "")

	// A changed rule must not be silently skipped: rewrite one embed rule in a
	// scratch FS and confirm its row is updated.
	const ruleName = "cache-probe.sf"
	const original = `desc(
    mode: "ssa",
    language: golang,
    title: "cache probe one",
); * as $a; alert $a;`
	const updated = `desc(
    mode: "ssa",
    language: golang,
    title: "cache probe two",
); * as $b; alert $b;`

	scratch := filesys.NewVirtualFs()
	scratch.AddFile(ruleName, original)
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, scratch, true))

	var row schema.SyntaxFlowRule
	require.NoError(t, profileDB.Where("rule_name = ?", "cache probe one").First(&row).Error)
	require.Equal(t, original, row.Content)

	scratch2 := filesys.NewVirtualFs()
	scratch2.AddFile(ruleName, updated)
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, scratch2, true))

	var updatedRow schema.SyntaxFlowRule
	require.NoError(t, profileDB.Where("rule_name = ?", "cache probe two").First(&updatedRow).Error)
	require.Equal(t, updated, updatedRow.Content, "changed rule content must be re-imported")
	require.NotEmpty(t, updatedRow.OpCodes, "re-imported rule must carry its compiled opcodes")
}
