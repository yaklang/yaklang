package sfpattern_test

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestBuiltinSourceRules_VerifyFilesystem(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "sfbuildin", "buildin", "source")
	root, err := filepath.Abs(root)
	require.NoError(t, err)

	var rules []string
	err = filesys.Recursive(root, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		rules = append(rules, path)
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, rules, "expected source/*.sf under %s", root)

	local := filesys.NewLocalFs()
	for _, path := range rules {
		path := path
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			raw, err := local.ReadFile(path)
			require.NoError(t, err)
			frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
			require.NoError(t, err)
			require.True(t, sfvm.FrameIsSourceMode(frame), "rule must be mode=source")
			err = sfanalysis.EvaluateVerifyFilesystemWithFrame(frame, sfanalysis.WithStrictEmbeddedVerify())
			require.NoError(t, err)
		})
	}
}
