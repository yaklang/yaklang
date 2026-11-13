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
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
// embedded POS/NEG, and any rule that can emit high/critical gets the same
// treatment through applyHighSeverityVerifyDefaults.
func RunBuiltinRuleVerify(t *testing.T, filter BuiltinVerifyFilter) {
	t.Helper()
	root := BuiltinRuleRoot(t)
	local := filesys.NewLocalFs()
	type item struct {
		path  string
		mode  string
		frame *sfvm.SFFrame
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
		rules = append(rules, item{path: path, mode: mode, frame: frame})
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, rules, "expected builtin rules under %s", root)

	for _, it := range rules {
		it := it
		t.Run(it.mode+"/"+filepath.Base(it.path), func(t *testing.T) {
			// The selection pass already compiled this exact source. Use its frame
			// for POS/NEG verification rather than parsing the same rule twice.
			frame := it.frame
			if sfvm.FrameIsSourceMode(frame) || sfvm.FrameIsStructMode(frame) {
				require.NotEmpty(t, frame.VerifyFsInfo, "source/struct rule must embed POS/NEG filesystems")
			}
			if len(frame.VerifyFsInfo) == 0 {
				t.Skip("no embedded verify filesystem")
			}
			require.NoError(t, EvaluateVerifyFilesystemWithFrame(frame))
			if it.mode == "source" {
				requireSourceNegativeIsReachable(t, frame)
				requireNegativeIsNearMiss(t, frame)
			}
		})
	}
}

// placeholderNegative is the fixture text that used to stand in for a negative
// sample. It is prose, not a near miss, so it cannot catch a pattern that grew
// too loose.
const placeholderNegative = "# safe sample\nno secrets here\njust regular code"

// requireNegativeIsNearMiss rejects the prose placeholder as a negative fixture.
// A negative sample has to look like the thing the rule looks for, otherwise
// "no alert" says nothing: an empty file also produces no alert.
func requireNegativeIsNearMiss(t *testing.T, frame *sfvm.SFFrame) {
	t.Helper()
	negFs, err := frame.ExtractNegativeFilesystemAndLanguage()
	require.NoError(t, err)
	for _, f := range negFs {
		body := strings.TrimSpace(firstFixtureBody(t, f))
		require.NotEqual(t, placeholderNegative, body,
			"negative fixture is the prose placeholder; write a near miss instead")
		require.NotEmpty(t, body, "negative fixture is empty")
	}
}

// requireSourceNegativeIsReachable proves the negative fixture is not vacuous.
// A source rule selects files with `${...}`; a negative file whose name does
// not pass that filter is skipped before the query runs, so the negative check
// would pass on an empty file set. The probe writes the positive content into
// every negative file name: the rule must then alert, which is only possible
// when the name reaches the query.
func requireSourceNegativeIsReachable(t *testing.T, frame *sfvm.SFFrame) {
	t.Helper()
	rule := frame.GetRule()
	posFs, err := frame.ExtractVerifyFilesystemAndLanguage()
	require.NoError(t, err)
	negFs, err := frame.ExtractNegativeFilesystemAndLanguage()
	require.NoError(t, err)
	if len(posFs) == 0 || len(negFs) == 0 {
		return
	}
	posBody := firstFixtureBody(t, posFs[0])
	probe := filesys.NewVirtualFs()
	for _, name := range fixtureFileNames(t, negFs[0]) {
		probe.AddFile(name, posBody)
	}
	target, err := ssaapi.NewSourceQueryTargetFromFS("negative-probe", probe)
	if err != nil {
		t.Fatalf("source negative probe target: %v", err)
	}
	result, err := target.SyntaxFlowRule(rule)
	if err != nil {
		// A filter that rejects every file is exactly the vacuous case.
		t.Fatalf("source negative fixture %v never reaches the rule: %v", fixtureFileNames(t, negFs[0]), err)
	}
	alerts := 0
	for _, name := range result.GetAlertVariables() {
		alerts += len(result.GetValues(name))
	}
	require.NotZero(t, alerts,
		"source negative fixture %v never reaches the rule: the positive content written into it produces no alert",
		fixtureFileNames(t, negFs[0]))
}

func fixtureFileNames(t *testing.T, vf *sfvm.VerifyFileSystem) []string {
	t.Helper()
	var names []string
	require.NoError(t, filesys.Recursive(".", filesys.WithFileSystem(vf.GetVirtualFs()),
		filesys.WithFileStat(func(p string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			names = append(names, p)
			return nil
		})))
	require.NotEmpty(t, names, "fixture filesystem is empty")
	return names
}

func firstFixtureBody(t *testing.T, vf *sfvm.VerifyFileSystem) string {
	t.Helper()
	name := fixtureFileNames(t, vf)[0]
	raw, err := vf.GetVirtualFs().ReadFile(name)
	require.NoError(t, err)
	return string(raw)
}
