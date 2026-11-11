package yak

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/sshguard"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
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
	load, cancel := context.WithCancel(ctx)
	defer cancel()
	engine, err := NewScriptEngine(1).ExecuteExWithContext(load, `
manager = hook.NewManager()
run = func() {
    manager.Set(context.Background(), "get = func() { return ssh.ConnectWithPasswd }", {}, nil, "get")~
    return manager.Call("get")[0]
}
`, nil)
	require.NoError(t, err)
	cancel()
	value, err := engine.SafeCallYakFunction(ctx, "run", nil)
	require.NoError(t, err)
	require.IsType(t, yaklib.SSHConnectWithPasswd, value)
	require.NotEqual(t, reflect.ValueOf(yaklib.SSHConnectWithPasswd).Pointer(), reflect.ValueOf(value).Pointer())
}

func TestManagedSSHHookPassesInvocationCancellation(t *testing.T) {
	host := sshguard.WithGuard(context.Background(), nil)
	request, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewYakToCallerManager()
	manager.managedSSHContext = func() context.Context { return host }
	entered := make(chan context.Context, 1)
	manager.table.Store("handle", []*Caller{{Id: "fixture", Core: &YakFunctionCaller{
		callWithContext: func(ctx context.Context, _ func(*yakvm.Frame), _ ...any) any {
			entered <- ctx
			<-ctx.Done()
			return ctx.Err()
		},
	}}})
	done := make(chan []any, 1)
	go func() {
		done <- manager.Call("handle", WithCallConfigRuntimeCtx(request), WithCallConfigForceSync(true))
	}()
	select {
	case ctx := <-entered:
		require.True(t, sshguard.SameScope(host, ctx))
	case <-time.After(5 * time.Second):
		t.Fatal("managed hook did not receive its invocation context")
	}
	cancel()
	select {
	case result := <-done:
		require.Equal(t, []any{context.Canceled}, result)
	case <-time.After(5 * time.Second):
		t.Fatal("managed hook did not receive cancellation")
	}
}

func TestManagedSSHForgeRetainsRuntimeContext(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)
	fixture := &schema.AIForge{
		ForgeName: "ssh-guard-" + uuid.NewString(), ForgeType: schema.FORGE_TYPE_YAK,
		ForgeContent: "forgeHandle = func(items) { return ssh.ConnectWithPasswd }",
	}
	require.NoError(t, db.Create(fixture).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(fixture).Error) })

	plainConnector := reflect.ValueOf(yaklib.SSHConnectWithPasswd).Pointer()
	for _, test := range []struct {
		name string
		ctx  context.Context
	}{
		{"standalone", context.Background()},
		{"managed", sshguard.WithGuard(context.Background(), nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, err := NewScriptEngine(1).ExecuteExWithContext(test.ctx,
				`connector = aiagent.ExecuteForge(getParam("fixture"), {}, aiagent.context(context.Background()))~`,
				map[string]any{"fixture": fixture.ForgeName})
			require.NoError(t, err)
			value, ok := engine.GetVar("connector")
			require.True(t, ok)
			require.IsType(t, yaklib.SSHConnectWithPasswd, value)
			_, managed := sshguard.FromContext(test.ctx)
			require.Equal(t, managed, reflect.ValueOf(value).Pointer() != plainConnector)
		})
	}
}
