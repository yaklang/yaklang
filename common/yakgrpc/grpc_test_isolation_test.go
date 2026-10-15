package yakgrpc

import (
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDefaultLocalClientDoesNotModifyInheritedInstallation(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	if os.Getenv("YAK_TEST_ISOLATION_CHILD") == "1" {
		checkDefaultLocalClientIsolation(t)
		return
	}

	// A disposable installation, never the developer's actual databases. Also
	// covers paths outside YAKIT_HOME and startup hooks which run before tests.
	installation := t.TempDir()
	home := filepath.Join(installation, "yakit-projects")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "temp"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "temp", "keep"), []byte("production cache"), 0o600))
	profilePath := filepath.Join(installation, "profile.db")
	projectPath := filepath.Join(installation, "project.db")
	ssaPath := filepath.Join(installation, "ssa.db")
	profile, err := consts.CreateProfileDatabase(profilePath)
	require.NoError(t, err)
	for _, key := range []string{consts.AI_GLOBAL_CONFIG_KEY, consts.GLOBAL_NETWORK_CONFIG, "production-setting"} {
		require.NoError(t, yakit.SetKey(profile, key, "production-value"))
	}
	require.NoError(t, profile.Close())
	project, err := consts.CreateProjectDatabase(projectPath)
	require.NoError(t, err)
	require.NoError(t, yakit.SaveHTTPFlow(project, &schema.HTTPFlow{Url: "https://production.invalid/", Hash: "production-flow"}))
	_, err = yakit.CreateOrUpdateAISessionMeta(project, "production-session", "production")
	require.NoError(t, err)
	require.NoError(t, project.Close())
	ssa, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, ssaPath)
	require.NoError(t, err)
	require.NoError(t, ssa.Close())

	snapshot := func() map[string][32]byte {
		files := make(map[string][32]byte)
		require.NoError(t, filepath.WalkDir(installation, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = sha256.Sum256(data)
			}
			return nil
		}))
		return files
	}
	before := snapshot()
	result := filepath.Join(t.TempDir(), "test-home")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDefaultLocalClientDoesNotModifyInheritedInstallation$", "-test.count=1")
	cmd.Env = append(os.Environ(), "YAK_TEST_ISOLATION_CHILD=1", "YAK_TEST_ISOLATION_RESULT="+result,
		"YAKIT_HOME="+home,
		"YAK_DEFAULT_PROFILE_DATABASE_NAME="+profilePath,
		"YAK_DEFAULT_PROJECT_DATABASE_NAME="+projectPath,
		"SSA_DATABASE_RAW="+ssaPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Equal(t, before, snapshot(), "test startup and writes must not change any production file")
	childHome, err := os.ReadFile(result)
	require.NoError(t, err)
	// Cleanup only after the entire child process (including async writers) exits.
	require.NoError(t, os.RemoveAll(filepath.Dir(string(childHome))))
}

func checkDefaultLocalClientIsolation(t *testing.T) {
	t.Helper()
	home := consts.GetDefaultYakitBaseDir()
	require.Equal(t, "yakit-projects", filepath.Base(home))
	require.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(home)), "yaklang-test-"), "unexpected test home: %s", home)
	assertUnderHome := func(path string) {
		t.Helper()
		rel, err := filepath.Rel(home, path)
		require.NoError(t, err)
		require.False(t, filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)), "%s is outside test home %s", path, home)
	}
	assertUnderHome(consts.GetCurrentProfileDatabasePath())
	assertUnderHome(consts.GetCurrentProjectDatabasePath())
	_, ssaPath := consts.GetSSADataBaseInfo()
	assertUnderHome(ssaPath)
	profile := consts.GetGormProfileDatabase()
	project := consts.GetGormProjectDatabase()
	require.Empty(t, yakit.GetKey(profile, "production-setting"))
	for _, model := range []any{&schema.HTTPFlow{}, &schema.AISession{}} {
		var count int64
		require.NoError(t, project.Model(model).Count(&count).Error)
		require.Zero(t, count, "production traffic and sessions must not be loaded")
	}

	// Exercise the shared default client, rather than the opt-in temporary DB
	// helper. Its package-level initialization is the original pollution path.
	client, err := NewLocalClient()
	require.NoError(t, err)
	require.Same(t, profile, client.(*Client).GetProfileDatabase())
	require.Same(t, project, client.(*Client).GetProjectDatabase())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.SetAIGlobalConfig(ctx, &ypb.AIGlobalConfig{AIPresetPrompt: "isolated-test-prompt"})
	require.NoError(t, err)
	aiConfig, err := client.GetAIGlobalConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "isolated-test-prompt", aiConfig.GetAIPresetPrompt())
	_, err = client.SetGlobalNetworkConfig(ctx, &ypb.GlobalNetworkConfig{CallPluginTimeout: 123})
	require.NoError(t, err)
	network, err := client.GetGlobalNetworkConfig(ctx, &ypb.GetGlobalNetworkConfigRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 123, network.GetCallPluginTimeout())
	require.NoError(t, yakit.SetKey(profile, "production-setting", "test-value"))

	_, err = yakit.CreateOrUpdateAISessionMeta(project, "test-session", "test")
	require.NoError(t, err)
	saved := make(chan error, 1)
	require.NoError(t, yakit.EnqueueDBSave(func(db *gorm.DB) error {
		err := yakit.SaveHTTPFlow(db, &schema.HTTPFlow{Url: "https://test.invalid/"})
		saved <- err
		return err
	}))
	select {
	case err := <-saved:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async test traffic")
	}
	var count int64
	require.NoError(t, project.Model(&schema.HTTPFlow{}).Where("url = ?", "https://test.invalid/").Count(&count).Error)
	require.EqualValues(t, 1, count)
	for _, create := range []func(string) (*os.File, error){consts.TempFile, consts.TempAIFile} {
		f, err := create("isolation-*")
		require.NoError(t, err)
		assertUnderHome(f.Name())
		require.NoError(t, f.Close())
	}
	require.NoError(t, os.WriteFile(os.Getenv("YAK_TEST_ISOLATION_RESULT"), []byte(home), 0o600))
}
