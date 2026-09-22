package sfanalysis

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// New dataflow rules are named ssa-*.sf. Pending-language rules are source-only
// and are skipped here the same way builtin verify skips that directory.
func TestGapSSARules_VerifyFilesystem(t *testing.T) {
	root := BuiltinRuleRoot(t)
	local := filesys.NewLocalFs()
	var paths []string
	err := filesys.Recursive(root, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		slash := filepath.ToSlash(path)
		if strings.Contains(slash, "/pending-language/") {
			return nil
		}
		if !strings.HasPrefix(info.Name(), "ssa-") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := local.ReadFile(path)
			require.NoError(t, err)
			frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
			require.NoError(t, err)
			require.True(t, !sfvm.FrameIsSourceMode(frame) && !sfvm.FrameIsStructMode(frame))
			require.NoError(t, EvaluateVerifyFilesystemWithFrame(frame, WithStrictEmbeddedVerify()))
		})
	}
}
