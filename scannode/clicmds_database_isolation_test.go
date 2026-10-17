package scannode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/urfavecli"
)

func TestDistYakCommandAppliesLocalDatabasePathsBeforeScript(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		name := "node home defaults"
		if explicit {
			name = "per-execution absolute paths"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			nodeHome := filepath.Join(root, "node", "yakit-home")
			t.Setenv("HOME", filepath.Join(root, "unrelated-home"))
			t.Setenv("YAKIT_HOME", nodeHome)
			t.Setenv(consts.ENV_SSA_DATABASE_COMPANY_ID, "")
			t.Setenv(consts.ENV_SSA_DATABASE_RAW, filepath.Join(root, "execution-ir.db"))
			t.Setenv(consts.ENV_SSA_DB_SKIP_MIGRATE, "1")
			preserveDistYakDatabaseSettings(t)
			// Stale process globals must not override the current execution's env.
			consts.SetDefaultYakitProjectDatabaseName(filepath.Join(root, "stale-project.db"))
			consts.SetDefaultYakitProfileDatabaseName(filepath.Join(root, "stale-profile.db"))
			project, profile := "", ""
			if explicit {
				project = filepath.Join(root, "node", "task-project.db")
				profile = filepath.Join(nodeHome, "task-profile.db")
			}
			t.Setenv(consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, project)
			t.Setenv(consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, profile)
			app := cli.NewApp()
			app.Commands = []cli.Command{DistYakCommand}
			// Stop at file validation: configuring database paths must not open or
			// migrate any database before the dispatched script is evaluated.
			err := app.Run([]string{"yak", "distyak", filepath.Join(root, "missing.yak")})
			if err == nil || !os.IsNotExist(err) {
				t.Fatalf("expected missing script, got %v", err)
			}
			if !explicit {
				project = filepath.Join(nodeHome, "default-yakit.db")
				profile = filepath.Join(nodeHome, "yakit-profile-plugin.db")
			}
			if got := consts.GetDefaultYakitProjectDatabase(consts.GetDefaultYakitBaseDir()); got != project {
				t.Fatalf("project path = %q, want %q", got, project)
			}
			if got := consts.GetDefaultYakitPluginDatabase(consts.GetDefaultYakitBaseDir()); got != profile {
				t.Fatalf("profile path = %q, want %q", got, profile)
			}
			_, raw := consts.GetSSADataBaseInfo()
			if raw != filepath.Join(root, "execution-ir.db") || !consts.SSADatabaseSkipMigrate() {
				t.Fatal("local path configuration changed the SSA connection/migration policy")
			}
			assertNoDistYakDatabaseFiles(t, root)
		})
	}
}

func TestDistYakCommandRejectsInvalidCompanyIRBeforeLocalDatabaseOpen(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "unrelated-home"))
	t.Setenv("YAKIT_HOME", filepath.Join(root, "node-home"))
	t.Setenv(consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, filepath.Join(root, "project.db"))
	t.Setenv(consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, filepath.Join(root, "profile.db"))
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, filepath.Join(root, "forbidden-company-ir.db"))
	t.Setenv(consts.ENV_SSA_DB_SKIP_MIGRATE, "1")
	t.Setenv(consts.ENV_SSA_DATABASE_COMPANY_ID, "company-test")
	preserveDistYakDatabaseSettings(t)
	app := cli.NewApp()
	app.Commands = []cli.Command{DistYakCommand}
	err := app.Run([]string{"yak", "distyak", filepath.Join(root, "missing.yak")})
	if err == nil || !strings.Contains(err.Error(), "company SSA database requires PostgreSQL and disabled migration") {
		t.Fatalf("company IR validation did not precede script/local DB access: %v", err)
	}
	for _, key := range []string{consts.ENV_SSA_DATABASE_RAW, consts.ENV_SSA_DB_SKIP_MIGRATE, consts.ENV_SSA_DATABASE_COMPANY_ID} {
		if os.Getenv(key) != "" {
			t.Fatalf("trusted SSA setting %s remained in the child environment", key)
		}
	}
	assertNoDistYakDatabaseFiles(t, root)
}

func TestDistYakCommandRuleSnapshotProfileOverridesNodeProfile(t *testing.T) {
	root := t.TempDir()
	nodeHome := filepath.Join(root, "node-home")
	taskHome := filepath.Join(root, "snapshot-home")
	project := filepath.Join(root, "execution-project.db")
	t.Setenv("HOME", filepath.Join(root, "unrelated-home"))
	t.Setenv("YAKIT_HOME", nodeHome)
	t.Setenv(consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, project)
	t.Setenv(consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, filepath.Join(nodeHome, "yakit-profile-plugin.db"))
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, filepath.Join(root, "execution-ir.db"))
	t.Setenv(consts.ENV_SSA_DB_SKIP_MIGRATE, "1")
	t.Setenv(consts.ENV_SSA_DATABASE_COMPANY_ID, "")
	preserveDistYakDatabaseSettings(t)
	for _, entry := range ruleSnapshotDatabaseEnv(taskHome) {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid snapshot environment entry: %q", entry)
		}
		t.Setenv(key, value)
	}
	app := cli.NewApp()
	app.Commands = []cli.Command{DistYakCommand}
	err := app.Run([]string{"yak", "distyak", filepath.Join(root, "missing.yak")})
	if !os.IsNotExist(err) {
		t.Fatalf("expected missing script after path configuration, got %v", err)
	}
	if got := consts.GetDefaultYakitPluginDatabase(consts.GetDefaultYakitBaseDir()); got != filepath.Join(taskHome, "yakit-profile-plugin.db") {
		t.Fatalf("snapshot uses a shared node profile: %q", got)
	}
	if got := consts.GetDefaultYakitProjectDatabase(consts.GetDefaultYakitBaseDir()); got != project {
		t.Fatalf("snapshot replaced the per-execution project database: %q", got)
	}
	_, ir := consts.GetSSADataBaseInfo()
	if ir != filepath.Join(root, "execution-ir.db") || !consts.SSADatabaseSkipMigrate() {
		t.Fatal("snapshot profile changed company SSA configuration")
	}
	assertNoDistYakDatabaseFiles(t, root)
}

func preserveDistYakDatabaseSettings(t *testing.T) {
	t.Helper()
	project, profile := consts.YAK_PROJECT_DATA_DB_NAME, consts.YAK_PROFILE_PLUGIN_DB_NAME
	_, raw := consts.GetSSADataBaseInfo()
	t.Cleanup(func() {
		consts.SetDefaultYakitProjectDatabaseName(project)
		consts.SetDefaultYakitProfileDatabaseName(profile)
		consts.SetSSADatabaseInfo(raw)
		consts.SetSSADatabaseCompanyID("")
		consts.SetSSADatabaseSkipMigrate(false)
	})
}

func assertNoDistYakDatabaseFiles(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("configuration opened local files before script execution: %v", entries)
	}
}
