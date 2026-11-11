package yak

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/sshguard"
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
