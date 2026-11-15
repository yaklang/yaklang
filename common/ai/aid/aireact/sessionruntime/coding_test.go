package sessionruntime

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCodingRuntimeAttachKeepsProfileAndBinding(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workspace, err := yakit.CreateAICodingWorkspace(ctx, db, "runtime", t.TempDir())
	require.NoError(t, err)
	project, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer project.Close()
	require.NoError(t, project.AutoMigrate(&schema.AISession{}).Error)
	runtime := New(func() *gorm.DB { return project }, func() *gorm.DB { return db }).(*reActSessionRuntime)
	runtime.newReAct = aireact.NewTestReAct
	manager, _ := aicommon.NewMockEKManagerAndToken()
	id := "coding-" + uuid.NewString()
	params := &ypb.AIStartParams{TimelineSessionID: id, CodingMode: true, CodingWorkspaceID: workspace.WorkspaceID, DisableAISearchForge: true, DisableToolUse: true}
	owner, err := runtime.connectWithOptions(ctx, ConnectRequest{StartParams: params}, nil, false, aicommon.WithEnhanceKnowledgeManager(manager))
	require.NoError(t, err)
	defer owner.Close()
	require.True(t, owner.CreatedRuntime())
	// A second client can attach with no coding fields, without erasing the durable binding.
	attachParams := &ypb.AIStartParams{TimelineSessionID: id, Attach: true}
	attached, err := runtime.connectWithOptions(ctx, ConnectRequest{StartParams: attachParams}, nil, false)
	require.NoError(t, err)
	defer attached.Close()
	require.False(t, attached.CreatedRuntime())
	require.False(t, attachParams.CodingMode, "the caller's request must not be mutated")
	saved, err := yakit.GetAISessionMetaStartParamsBySessionID(project, id)
	require.NoError(t, err)
	require.True(t, saved.CodingMode)
	require.Equal(t, workspace.WorkspaceID, saved.CodingWorkspaceID)
	require.Equal(t, workspace.RootPath, saved.CodingWorkingDirectory)
	require.Equal(t, []string{workspace.RootPath}, saved.CodingWorkspaceRoots)
}

func TestCodingRuntimeFailureAndConcurrentRestartReuseWorktree(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	profile, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer profile.Close()
	project, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer project.Close()
	require.NoError(t, profile.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}).Error)
	require.NoError(t, project.AutoMigrate(&schema.AISession{}).Error)
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workspace, err := yakit.CreateAICodingWorkspace(ctx, profile, "runtime", root, t.TempDir())
	require.NoError(t, err)
	runtime := New(func() *gorm.DB { return project }, func() *gorm.DB { return profile }).(*reActSessionRuntime)
	id := "coding-" + uuid.NewString()
	params := &ypb.AIStartParams{TimelineSessionID: id, CodingMode: true, CodingWorkspaceID: workspace.WorkspaceID,
		CodingCreateWorktree: true, DisableAISearchForge: true, DisableToolUse: true}
	runtime.newReAct = func(...aicommon.ConfigOption) (*aireact.ReAct, error) {
		return nil, fmt.Errorf("injected initialization failure")
	}
	_, err = runtime.connectWithOptions(ctx, ConnectRequest{StartParams: params}, nil, false)
	require.ErrorContains(t, err, "injected initialization failure")
	trees, err := yakit.ListAICodingWorktrees(profile, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	checkout := trees[0].Path
	require.True(t, params.CodingCreateWorktree, "startup must not mutate caller's params")
	require.Empty(t, params.CodingWorktreeID)

	// Restart from a fresh runtime, racing two clients for the same persistent ID.
	runtime = New(func() *gorm.DB { return project }, func() *gorm.DB { return profile }).(*reActSessionRuntime)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	runtime.newReAct = func(options ...aicommon.ConfigOption) (*aireact.ReAct, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return aireact.NewTestReAct(options...)
	}
	manager, _ := aicommon.NewMockEKManagerAndToken()
	type result struct {
		connection ReActConnection
		err        error
	}
	results := make(chan result, 2)
	connect := func() {
		connection, err := runtime.connectWithOptions(ctx, ConnectRequest{StartParams: params}, nil, false, aicommon.WithEnhanceKnowledgeManager(manager))
		results <- result{connection, err}
	}
	go connect()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go connect()
	close(release)
	created := 0
	for i := 0; i < 2; i++ {
		var result result
		select {
		case result = <-results:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		require.NoError(t, result.err)
		defer result.connection.Close()
		if result.connection.CreatedRuntime() {
			created++
		}
	}
	require.Equal(t, 1, created)
	require.Equal(t, int32(1), calls.Load())
	trees, err = yakit.ListAICodingWorktrees(profile, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	require.Equal(t, checkout, trees[0].Path)
	saved, err := yakit.GetAISessionMetaStartParamsBySessionID(project, id)
	require.NoError(t, err)
	require.Equal(t, trees[0].WorktreeID, saved.CodingWorktreeID)
	require.Equal(t, checkout, saved.CodingWorkspaceRoots[0])
	require.Equal(t, workspace.RootPaths[1], saved.CodingWorkspaceRoots[1])
	actual, err := coding.ListWorktrees(ctx, workspace.RootPath, true)
	require.NoError(t, err)
	require.Len(t, actual, 2)
	require.Equal(t, filepath.Clean(checkout), saved.CodingWorkingDirectory)
}
