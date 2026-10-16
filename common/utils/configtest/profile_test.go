package configtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakitenv"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}, &schema.AIThirdPartyConfig{}).Error)
	return db
}

func TestGuardSkipsDirtyFixturesWithoutChangingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(*gorm.DB) error
	}{
		{"ai settings", func(db *gorm.DB) error {
			return db.Create(&schema.GeneralStorage{Key: strconv.Quote("AI_GLOBAL_CONFIG"), Value: strconv.Quote(`{"APIKey":"private-test-sentinel"}`)}).Error
		}},
		{"arbitrary settings", func(db *gorm.DB) error {
			return db.Create(&schema.GeneralStorage{Key: "user-setting", Value: "private-test-sentinel"}).Error
		}},
		{"network credentials", func(db *gorm.DB) error {
			return db.Create(&schema.GeneralStorage{Key: "GLOBAL_NETWORK_CONFIG", Value: `{"AppConfigs":[{"APIKey":"private-test-sentinel"}]}`}).Error
		}},
		{"provider", func(db *gorm.DB) error {
			return db.Create(&schema.AIThirdPartyConfig{Type: "openai", APIKey: "private-test-sentinel"}).Error
		}},
		{"deleted provider", func(db *gorm.DB) error {
			provider := &schema.AIThirdPartyConfig{Type: "openai", APIKey: "private-test-sentinel"}
			if err := db.Create(provider).Error; err != nil {
				return err
			}
			return db.Delete(provider).Error
		}},
		{"process environment", func(db *gorm.DB) error {
			return db.Create(&schema.GeneralStorage{Key: "_YAKLANG_ENGINE_VERSION", Value: "private-test-sentinel", ProcessEnv: true}).Error
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			require.NoError(t, tc.seed(db))
			snapshot := func() string {
				var settings []schema.GeneralStorage
				var providers []schema.AIThirdPartyConfig
				require.NoError(t, db.Unscoped().Find(&settings).Error)
				require.NoError(t, db.Unscoped().Find(&providers).Error)
				raw, err := json.Marshal([]any{settings, providers})
				require.NoError(t, err)
				return string(raw)
			}
			before := snapshot()
			var skipped bool
			t.Run("refused before write", func(st *testing.T) {
				defer func() { skipped = st.Skipped() }()
				Guard(st, db, nil)
				st.Fatal("dirty fixture was allowed to run")
			})
			require.True(t, skipped)
			// Do not print the snapshots on failure: they contain synthetic keys.
			require.True(t, before == snapshot(), "skipping must leave stored credentials byte-for-byte unchanged")
			err := Check(db, nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-test-sentinel")
		})
	}
}

func TestGuardRestoresCleanFixtureAfterMutatingTest(t *testing.T) {
	db := testDB(t)
	baseline := &schema.GeneralStorage{Key: strconv.Quote("_YAKLANG_ENGINE_VERSION"), Value: strconv.Quote("test-version")}
	require.NoError(t, db.Create(baseline).Error)
	t.Run("configuration writer", func(st *testing.T) {
		Guard(st, db, nil)
		require.NoError(st, db.Create(&schema.GeneralStorage{Key: "AI_GLOBAL_CONFIG", Value: "test-key"}).Error)
		require.NoError(st, db.Create(&schema.AIThirdPartyConfig{Type: "openai", APIKey: "test-key"}).Error)
	})
	require.NoError(t, Check(db, nil), "a subsequent test must see a clean fixture")
	var rows []schema.GeneralStorage
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, baseline.ID, rows[0].ID)
	require.Equal(t, baseline.Key, rows[0].Key)
	require.Equal(t, baseline.Value, rows[0].Value)
	require.True(t, baseline.CreatedAt.Equal(rows[0].CreatedAt))
	require.True(t, baseline.UpdatedAt.Equal(rows[0].UpdatedAt))
}

func TestPristineCheckOnlyReadsAndDoesNotMigrate(t *testing.T) {
	db := testDB(t)
	changes := func() int {
		var count int
		require.NoError(t, db.DB().QueryRow("SELECT total_changes()").Scan(&count))
		return count
	}
	before := changes()
	for i := 0; i < 20; i++ {
		require.NoError(t, Check(db, nil))
	}
	require.Equal(t, before, changes())
	require.False(t, db.HasTable(&schema.WebFuzzerConfig{}), "the guard must not create missing tables")
}

func TestOnlyExactPublicBootstrapSettingsAreAllowed(t *testing.T) {
	db := testDB(t)
	public := map[string]any{"Enabled": true, "Provider": map[string]any{"Type": "aibalance", "Domain": "aibalance.yaklang.com", "APIKey": "free-user"}}
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	row := &schema.GeneralStorage{Key: "AI_GLOBAL_CONFIG", Value: string(raw)}
	require.NoError(t, db.Create(row).Error)
	defaults := BuiltinDefaults{AI: public}
	require.NoError(t, Check(db, defaults))
	require.NoError(t, db.Model(row).Update("value", `{"Enabled":true,"Provider":{"Type":"openai","APIKey":"private-test-sentinel"}}`).Error)
	require.Error(t, Check(db, defaults))
	require.NoError(t, db.Model(row).Update("value", `{"Enabled":true,"Provider":{"Type":"aibalance","Domain":"aibalance.yaklang.com","APIKey":"free-user"},"AIPresetPrompt":"user preference"}`).Error)
	require.Error(t, Check(db, defaults), "even public credentials do not authorize replacing a customized configuration")
}

func TestPristineCheckRejectsUnownedAndSymlinkedDatabases(t *testing.T) {
	dir := t.TempDir()
	db, err := gorm.Open("sqlite3", filepath.Join(dir, "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Error(t, Check(db, nil))
	require.NoError(t, Check(db, nil, dir), "an explicitly created temporary fixture is allowed")
	link := filepath.Join(yakitenv.TestRoot(), "outside-fixture-link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	require.False(t, isWithin(yakitenv.TestRoot(), filepath.Join(link, "profile.db")))
}

func TestRuntimeCredentialsAreRefusedWithoutPrintingValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		bad   bool
	}{
		{"empty", nil, false},
		{"public builtin", map[string]any{"Type": "aibalance", "Domain": "aibalance.yaklang.com", "APIKey": "free-user"}, false},
		{"private provider", map[string]any{"Type": "openai", "APIKey": "private-test-sentinel"}, true},
		{"public key on other provider", map[string]any{"Type": "openai", "APIKey": "free-user"}, true},
		{"nested provider", []any{map[string]any{"Provider": map[string]any{"APIKey": "private-test-sentinel"}}}, true},
		{"authorization header", map[string]any{"Key": "Authorization", "Value": "private-test-sentinel"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.bad, hasPrivateCredential(tc.value))
		})
	}
}

func BenchmarkPristineProfileCheck(b *testing.B) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := db.AutoMigrate(&schema.GeneralStorage{}, &schema.AIThirdPartyConfig{}).Error; err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Check(db, nil); err != nil {
			b.Fatal(strings.TrimSpace(err.Error()))
		}
	}
}

func TestBuiltinCorePluginHashesDoNotMaskUserSettings(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Create(&schema.GeneralStorage{Key: strconv.Quote("core-plugin-item-hashes"), Value: strconv.Quote(`{"builtin-plugin":"hash"}`)}).Error)
	require.NoError(t, Check(db, nil))
	require.NoError(t, db.Create(&schema.GeneralStorage{Key: "user-setting", Value: "private-test-sentinel"}).Error)
	require.Error(t, Check(db, nil), "builtin metadata must not permit unrelated user configuration")
}
