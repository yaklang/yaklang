package sfanalysis

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// BuiltinVerifyFilter selects which desc(mode) values the filesystem walker runs.
type BuiltinVerifyFilter struct {
	Source bool
	Struct bool
	SSA    bool
}

func matchBuiltinVerifyFilter(frame *sfvm.SFFrame, filter BuiltinVerifyFilter) (mode string, ok bool) {
	switch {
	case sfvm.FrameIsSourceMode(frame):
		return "source", filter.Source
	case sfvm.FrameIsStructMode(frame):
		return "struct", filter.Struct
	default:
		return "ssa", filter.SSA
	}
}

// BuiltinRuleRoot is the embeddable SyntaxFlow rule tree (buildin/).
func BuiltinRuleRoot(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "sfbuildin", "buildin")
	root, err := filepath.Abs(root)
	require.NoError(t, err)
	return root
}

// RunBuiltinRuleVerify walks builtin .sf files and checks each matching rule
// through EvaluateVerifyFilesystemWithFrame. source/struct always require
// embedded POS/NEG; SSA keeps historical non-strict behavior unless the caller
// passed WithStrictEmbeddedVerify via EvaluateVerifyFilesystemWithFrame.
func RunBuiltinRuleVerify(t *testing.T, filter BuiltinVerifyFilter) {
	t.Helper()
	root := BuiltinRuleRoot(t)
	local := filesys.NewLocalFs()
	type item struct {
		path string
		mode string
	}
	var rules []item
	err := filesys.Recursive(root, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		raw, err := local.ReadFile(path)
		if err != nil {
			return err
		}
		frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
		if err != nil {
			return nil
		}
		mode, keep := matchBuiltinVerifyFilter(frame, filter)
		if !keep {
			return nil
		}
		rules = append(rules, item{path: path, mode: mode})
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, rules, "expected builtin rules under %s", root)

	for _, it := range rules {
		it := it
		t.Run(it.mode+"/"+filepath.Base(it.path), func(t *testing.T) {
			raw, err := local.ReadFile(it.path)
			require.NoError(t, err)
			frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
			require.NoError(t, err)
			if sfvm.FrameIsSourceMode(frame) || sfvm.FrameIsStructMode(frame) {
				require.NotEmpty(t, frame.VerifyFsInfo, "source/struct rule must embed POS/NEG filesystems")
			}
			if len(frame.VerifyFsInfo) == 0 {
				t.Skip("no embedded verify filesystem")
			}
			require.NoError(t, EvaluateVerifyFilesystemWithFrame(frame))
		})
	}
}
