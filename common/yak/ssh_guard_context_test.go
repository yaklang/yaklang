package yak

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/sshguard"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func TestSSHGuardFollowsDynamicImports(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested.yak")
	require.NoError(t, os.WriteFile(file, []byte("connector = ssh.ConnectWithPasswd"), 0600))
	ctx := sshguard.WithGuard(context.Background(), nil)
	code := `connector = dyn.Import(getParam("file"), "connector")~`
	engine, err := NewScriptEngine(1).ExecuteExWithContext(ctx, code, map[string]any{"file": file})
	require.NoError(t, err)
	value, ok := engine.GetVar("connector")
	require.True(t, ok)
	require.IsType(t, yaklib.SSHConnectWithPasswd, value)
	require.NotEqual(t, reflect.ValueOf(yaklib.SSHConnectWithPasswd).Pointer(), reflect.ValueOf(value).Pointer())
}

func TestManagedSSHLibraryContainsOnlyBoundConnectors(t *testing.T) {
	ctx := sshguard.WithGuard(context.Background(), nil)
	guarded := yaklang.NewWithContext(ctx)
	standalone := yaklang.New()

	guardedExports, ok := guarded.GetVM().GetGlobalVar().Load("ssh")
	require.True(t, ok)
	standaloneExports, ok := standalone.GetVM().GetGlobalVar().Load("ssh")
	require.True(t, ok)
	readConnector := func(module any) uintptr {
		return reflect.ValueOf(module.(map[string]interface{})["ConnectWithPasswd"]).Pointer()
	}
	require.Equal(t, reflect.ValueOf(yaklib.SSHConnectWithPasswd).Pointer(), readConnector(standaloneExports))
	require.NotEqual(t, readConnector(standaloneExports), readConnector(guardedExports))
}

func TestManagedSSHHookRetainsRuntimeContext(t *testing.T) {
	ctx := sshguard.WithGuard(context.Background(), nil)
	engine, err := NewScriptEngine(1).ExecuteExWithContext(ctx, "manager = hook.NewManager()", nil)
	require.NoError(t, err)
	value, ok := engine.GetVar("manager")
	require.True(t, ok)
	manager := value.(*YakToCallerManager)
	require.True(t, sshguard.SameScope(ctx, manager.managedSSHContext))
	require.True(t, sshguard.SameScope(ctx, manager.getYakitPluginContext(context.Background()).Ctx))
}
