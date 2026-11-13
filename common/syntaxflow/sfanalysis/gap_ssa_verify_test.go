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

// SSA-mode dataflow rules are checked here. Source and struct rules have their
// own verify suites, and the rule mode comes from desc(mode), never from the
// file name.
func TestGapSSARules_VerifyFilesystem(t *testing.T) {
	root := BuiltinRuleRoot(t)
	local := filesys.NewLocalFs()
	var paths []string
	err := filesys.Recursive(root, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		raw, err := local.ReadFile(path)
		if err != nil {
			return err
		}
		frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
		if err != nil || sfvm.FrameIsSourceMode(frame) || sfvm.FrameIsStructMode(frame) {
			return nil
		}
		// Library rules are sink/source definitions with no fixtures of their
		// own; their importers carry the evidence.
		if rule := frame.GetRule(); rule == nil || rule.AllowIncluded {
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
