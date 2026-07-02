//go:build manual

package preprocess

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func opensslRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("OPENSSL_SRC")
	if root == "" {
		root = `C:\Users\13766\work\测试\openssl`
	}
	if _, err := os.Stat(filepath.Join(root, "apps", "configutl.c")); err != nil {
		t.Skipf("openssl tree not found: %v", err)
	}
	return root
}

func TestOpenSSL_CollectHasStackOf(t *testing.T) {
	root := opensslRoot(t)
	fs := filesys.NewRelLocalFs(root)
	raw, err := fs.ReadFile("apps/configutl.c")
	require.NoError(t, err)

	project := BuildProject(fs, DefaultConfig())
	tables := project.collectMacroEnvironment("apps/configutl.c", string(raw))
	_, ok := tables.Function["STACK_OF"]
	require.True(t, ok, "STACK_OF must be collected from OpenSSL headers")
}

func TestOpenSSL_SafestackDirectCollect(t *testing.T) {
	root := opensslRoot(t)
	fs := filesys.NewRelLocalFs(root)
	project := BuildProject(fs, DefaultConfig())

	stored, found := project.resolver.Resolve("openssl/safestack.h", true, "apps/configutl.c")
	require.True(t, found, "must resolve openssl/safestack.h")
	t.Logf("safestack stored at %s", stored)

	content, ok := project.ReadHeader(stored)
	require.True(t, ok)
	tables := project.collectMacroEnvironment(stored, string(content))
	_, has := tables.Function["STACK_OF"]
	require.True(t, has, "STACK_OF must be in safestack.h")
}

func TestOpenSSL_ScanSafestackDefine(t *testing.T) {
	root := opensslRoot(t)
	fs := filesys.NewRelLocalFs(root)
	data, err := fs.ReadFile("include/openssl/safestack.h.in")
	require.NoError(t, err)
	tables := ScanMacroTablesFromSource(string(data))
	_, has := tables.Function["STACK_OF"]
	require.True(t, has, "ScanMacroTablesFromSource must find STACK_OF in safestack.h.in")
}
