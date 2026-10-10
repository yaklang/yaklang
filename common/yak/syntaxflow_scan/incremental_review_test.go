package syntaxflow_scan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// An incremental project reviews the current source view, including unchanged
// vulnerable files. The emitted evidence must belong to the new program, while
// removed/overridden base files cannot reappear after a process cache reset.
func TestResilienceIncrementalReviewStreamsCurrentOverlay(t *testing.T) {
	for _, noSaveRisk := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory-%t", noSaveRisk), func(t *testing.T) {
			checkIncrementalReviewStreamsCurrentOverlay(t, noSaveRisk)
		})
	}
}

func checkIncrementalReviewStreamsCurrentOverlay(t *testing.T, noSaveRisk bool) {
	oldDB := ssadb.GetDB()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "incremental.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() { ssadb.SetDB(oldDB); _ = db.Close() })
	rule, err := os.ReadFile("../../syntaxflow/sfbuildin/buildin/java/cwe-327-weak-crypto/struct-java-aes-ecb.sf")
	require.NoError(t, err)
	dir := t.TempDir()
	crypt := `import javax.crypto.Cipher;
class Crypt { void bad() throws Exception { Cipher.getInstance("AES/ECB/PKCS5Padding"); } }`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Crypt.java"), []byte(crypt), 0600))
	var names []string
	for version, wantRisks := range []int{1, 1, 0, 1, 0} {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			if version == 2 {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Crypt.java"), []byte(`class Crypt { void safe() {} }`), 0600))
			}
			if version == 3 {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Crypt.java"), []byte(crypt), 0600))
			}
			if version == 4 {
				require.NoError(t, os.Remove(filepath.Join(dir, "Crypt.java")))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "FixtureVersion.java"), []byte(fmt.Sprintf(`class FixtureVersion { String label() { return "v%d"; } }`, version)), 0600))
			for _, name := range names {
				ssaapi.ProgramCache.Remove(name)
				ssadb.GetIrCodeCache(name).Purge()
				ssadb.GetIrTypeCache(name).Purge()
			}
			name := fmt.Sprintf("incremental-review-%d", version)
			opts := []ssaconfig.Option{
				ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal), ssaconfig.WithCodeSourceLocalFile(dir), ssaconfig.WithProjectRawLanguage("java"),
				ssaconfig.WithSetProgramName(name), syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
				ssaconfig.WithNoSaveRisk(noSaveRisk), ssaconfig.WithNoSaveTask(true),
				ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: string(rule), Language: "java"}),
			}
			if version > 0 {
				opts = append(opts, ssaconfig.WithBaseProgramName(names[version-1]))
			}
			var parts []*sfreport.SSAResultParts
			opts = append(opts, syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
				if r == nil || r.Result == nil {
					return
				}
				if r.Result.RiskCount() > 0 {
					require.Equal(t, !noSaveRisk, r.Result.IsDatabase(), "ordinary scans retain persisted results; no_save_risk stays in memory")
				}
				part, err := sfreport.ConvertSingleResultToSSAResultParts(r.Result, sfreport.NewStreamPartsOptions())
				require.NoError(t, err)
				if part != nil {
					parts = append(parts, part)
				}
			}))
			result, err := syntaxflow_scan.ScanProject(context.Background(), opts...)
			require.NoError(t, err)
			require.True(t, result.Succeeded)
			require.Equal(t, name, result.ProgramName)
			var review syntaxflow_scan.StageOutcome
			for _, stage := range result.Stages {
				if stage.Stage == syntaxflow_scan.StageReview {
					review = stage
				}
			}
			require.True(t, review.Succeeded())
			require.Positive(t, review.RuleCount, "review success requires actual rule execution")
			var risks, files, flows int
			for _, part := range parts {
				require.Equal(t, name, part.ProgramName, "base IR evidence belongs to this scan's new program")
				risks += len(part.Risks)
				files += len(part.Files)
				flows += len(part.Dataflows)
			}
			require.Equal(t, wantRisks, risks)
			if wantRisks > 0 {
				require.Positive(t, files)
				require.Positive(t, flows)
			}
		})
		names = append(names, fmt.Sprintf("incremental-review-%d", version))
		if t.Failed() {
			break
		}
	}
	for _, name := range names {
		ssaapi.ProgramCache.Remove(name)
	}
}
