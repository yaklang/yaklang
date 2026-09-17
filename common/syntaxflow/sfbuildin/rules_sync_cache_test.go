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

	hashes, err := sfdb.LoadStoredContentHashesByRuleName(profileDB, true)
	require.NoError(t, err)
	require.Len(t, hashes, int(stored), "every stored rule must be discoverable by name")
	for name, fingerprint := range hashes {
		require.NotEmpty(t, fingerprint.ContentHash, "fingerprint for %s must carry a content hash", name)
	}

	// Every embed rule must be recognized as already stored. The fingerprint
	// compares content, the directory-derived tag string and the compiler's
	// mode tag, so this also guards the tag/mode reconstruction the fast path
	// relies on: a mismatch here would silently re-compile the whole embed.
	files := collectTestRuleFiles(t)
	require.NotEmpty(t, files)
	for _, f := range files {
		require.True(t, syncRuleAlreadyStored(hashes, f, true),
			"rule %s was not recognized as stored (stored fingerprint missing or tag/mode mismatch)", f.name)
	}

	// Second sync: unchanged rules are reused from the database, so the pass
	// must be dramatically cheaper than the first compile.
	secondStart := time.Now()
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, ruleFSWithHash, true))
	secondCost := time.Since(secondStart)
	require.Less(t, secondCost, firstCost/2,
		"second sync reused nothing: first=%v second=%v", firstCost, secondCost)

	// The forced re-import escape hatch must bypass the cache entirely.
	t.Setenv("YAK_SYNTAXFLOW_FORCE_RULE_SYNC", "true")
	forceStart := time.Now()
	require.NoError(t, SyncRuleFromFileSystemToDB(profileDB, ruleFSWithHash, true))
	require.Greater(t, time.Since(forceStart), secondCost*10,
		"force re-sync reused the cache instead of re-importing")
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
