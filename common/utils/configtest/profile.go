// Package configtest provides read-only preconditions for tests which write
// persisted configuration, without import-time work or a testing dependency.
package configtest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/yakitenv"
)

// TestingT is the part of testing.T used by the guard. Keeping testing itself
// out of this package lets configuration packages expose test helpers without
// linking the Go test runner into normal executables.
type TestingT interface {
	Helper()
	Skip(...any)
	Error(...any)
	Cleanup(func())
}

// BuiltinDefaults permits only an exact match with the public defaults emitted
// by bootstrap code. It never treats arbitrary nonempty AI settings as clean.
type BuiltinDefaults struct {
	Network any
	AI      any
}

// RequireHome must run before a lazy database getter can initialize a database.
func RequireHome(t TestingT) {
	t.Helper()
	if !isWithin(yakitenv.TestRoot(), os.Getenv("YAKIT_HOME")) {
		t.Skip("configuration test refused: Yakit home is outside the private test directory")
	}
}

// RequirePristine skips before any mutation if the DB is not an owned fixture,
// contains user configuration, or cannot be checked. fixtureRoots must come
// directly from t.TempDir(), never from user configuration or environment.
// No configuration getters, migrations or initializers run in this check.
func RequirePristine(t TestingT, db *gorm.DB, builtinNetwork any, fixtureRoots ...string) {
	t.Helper()
	if err := Check(db, builtinNetwork, fixtureRoots...); err != nil {
		// Deliberately exclude keys, values, DSNs and database errors: they may
		// contain API keys or other credentials.
		t.Skip("configuration test refused: " + err.Error())
	}
}

// Check only reads the supplied connection. It does not open another database.
func Check(db *gorm.DB, builtinNetwork any, fixtureRoots ...string) error {
	var builtinAI any
	if defaults, ok := builtinNetwork.(BuiltinDefaults); ok {
		builtinNetwork, builtinAI = defaults.Network, defaults.AI
	}
	if db == nil || db.Dialect().GetName() != "sqlite3" {
		return errors.New("database is not a verifiable SQLite test fixture")
	}
	var databases []struct {
		Name string
		File string
	}
	if err := db.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil || len(databases) == 0 {
		return errors.New("database ownership could not be checked")
	}
	roots := append([]string{yakitenv.TestRoot()}, fixtureRoots...)
	for _, database := range databases {
		if database.File == "" { // SQLite in-memory fixture.
			continue
		}
		owned := false
		for _, root := range roots {
			if isWithin(root, database.File) {
				owned = true
				break
			}
		}
		if !owned {
			return errors.New("database is outside the private test directories")
		}
	}
	// Read-only metadata queries also include soft-deleted rows: secrets left
	// behind by a deleted provider still make a fixture unsuitable for testing.
	for _, model := range configurationModels() {
		if !db.HasTable(model) {
			continue
		}
		var count int64
		if err := db.Unscoped().Model(model).Count(&count).Error; err != nil {
			return errors.New("configuration records could not be checked")
		}
		if count != 0 {
			return errors.New("pre-existing configuration records were detected")
		}
	}
	if !db.HasTable(&schema.GeneralStorage{}) {
		return nil
	}
	var records []schema.GeneralStorage
	if err := db.Unscoped().Find(&records).Error; err != nil {
		return errors.New("stored settings could not be checked")
	}
	for _, record := range records {
		key, value := unquote(record.Key), unquote(record.Value)
		if record.ProcessEnv {
			return errors.New("persisted process environment was detected")
		}
		switch key {
		case "AI_GLOBAL_CONFIG":
			if builtinAI == nil || !sameJSON(value, builtinAI) {
				return errors.New("pre-existing AI configuration was detected")
			}
		case "GLOBAL_NETWORK_CONFIG":
			if builtinNetwork == nil || !sameJSON(value, builtinNetwork) {
				return errors.New("pre-existing network or API configuration was detected")
			}
		case "_YAKLANG_ENGINE_VERSION", "GLOBAL_NETWORK_CONFIG_INIT", "__migrate_database__",
			"cd336beba498c97738c275f6771efca3", "2b709ef7252a06a0c1cfbb952f77f976", "6ef3c850244a2b26ed0b163d1fda9600",
			"core-plugin-ai-fields-version", "yakit.ai_session_meta.migrated.v1",
			"__yaklang_internal_payload_file_flag_repair_v1__", "yak_mcp_builtin_tier_defaults_v1":
			// Internal bootstrap versions/hashes, not user settings.
		default:
			return errors.New("pre-existing user settings were detected")
		}
	}
	return nil
}

// Guard additionally restores the clean starting state after the test. Cleanup
// is registered only after the read-only precondition has passed; skipped tests
// never delete, migrate or restore anything in the rejected database.
func Guard(t TestingT, db *gorm.DB, builtinNetwork any) {
	t.Helper()
	RequirePristine(t, db, builtinNetwork)
	var baseline []schema.GeneralStorage
	if db.HasTable(&schema.GeneralStorage{}) {
		if err := db.Unscoped().Find(&baseline).Error; err != nil {
			t.Skip("configuration test refused: clean baseline could not be read")
		}
	}
	t.Cleanup(func() {
		tx := db.Begin()
		if tx.Error != nil {
			t.Error("could not restore isolated configuration fixture")
			return
		}
		defer tx.Rollback()
		for _, model := range append([]any{&schema.GeneralStorage{}}, configurationModels()...) {
			if tx.HasTable(model) {
				if err := tx.Unscoped().Where("1 = 1").Delete(model).Error; err != nil {
					t.Error("could not restore isolated configuration fixture")
					return
				}
			}
		}
		for _, record := range baseline {
			if err := tx.Create(&record).Error; err != nil {
				t.Error("could not restore isolated configuration fixture")
				return
			}
		}
		if err := tx.Commit().Error; err != nil {
			t.Error("could not restore isolated configuration fixture")
		}
	})
}

func configurationModels() []any {
	return []any{
		&schema.AIThirdPartyConfig{}, &schema.PluginEnv{},
		&schema.MCPServer{}, &schema.MCPClientToolConfig{}, &schema.MCPServerToolConfig{},
		&schema.WebFuzzerConfig{}, &schema.ProxyEndpoint{}, &schema.ProxyRoute{},
		&schema.HotPatchTemplate{}, &schema.NavigationBar{}, &schema.MenuItem{},
	}
}

// RequirePublicRuntime also catches credentials loaded into a process-local AI
// cache independently of SQLite. Only the exact public builtin provider key is
// accepted; no private credential values are printed.
func RequirePublicRuntime(t TestingT, configs ...any) {
	t.Helper()
	for _, config := range configs {
		raw, err := json.Marshal(config)
		var value any
		if err != nil || json.Unmarshal(raw, &value) != nil || hasPrivateCredential(value) {
			t.Skip("configuration test refused: pre-existing runtime credentials were detected")
		}
	}
}

func hasPrivateCredential(value any) bool {
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			if hasPrivateCredential(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range value {
			name := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if text, ok := item.(string); ok && text != "" {
				switch name {
				case "apikey":
					if text != "free-user" || value["Type"] != "aibalance" || value["Domain"] != "aibalance.yaklang.com" {
						return true
					}
				case "usersecret", "password", "authorization", "accesstoken", "secretkey":
					return true
				case "key":
					if strings.EqualFold(text, "Authorization") && value["Value"] != "" {
						return true
					}
				}
			}
			if hasPrivateCredential(item) {
				return true
			}
		}
	}
	return false
}

func isWithin(root, path string) bool {
	if root == "" {
		return false
	}
	root, rootErr := filepath.EvalSymlinks(root)
	path, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func unquote(value string) string {
	if decoded, err := strconv.Unquote(value); err == nil {
		return decoded
	}
	return value
}

func sameJSON(raw string, expected any) bool {
	encoded, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	var a, b any
	return json.Unmarshal([]byte(raw), &a) == nil && json.Unmarshal(encoded, &b) == nil && reflect.DeepEqual(a, b)
}
