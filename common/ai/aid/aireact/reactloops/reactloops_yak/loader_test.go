package reactloops_yak

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// 加载 embed FS 中所有内置 yak focus mode，验证 hello_yak 等出现在 reactloops 注册表中。
// 关键词: yak focus loader embed test, builtin focus modes registered
func TestLoadAllFromEmbed_BuiltinModes(t *testing.T) {
	require.NoError(t, LoadAllFromEmbed(), "load embed should succeed")

	for _, name := range []string{
		"hello_yak",
		"yak_scan_demo",
		"comprehensive_showcase",
	} {
		_, ok := reactloops.GetLoopFactory(name)
		require.True(t, ok, "expect %s to be registered", name)

		_, ok = reactloops.GetYakFocusModeBundle(name)
		require.True(t, ok, "expect %s bundle cached", name)
	}
}

// 多次调用 LoadAllFromEmbed 必须幂等（once 锁住）。
// 关键词: yak focus loader embed idempotent
func TestLoadAllFromEmbed_Idempotent(t *testing.T) {
	require.NoError(t, LoadAllFromEmbed())
	require.NoError(t, LoadAllFromEmbed())
}

// LoadSingleFile：单文件 + 同级 sidekick 的完整加载链路。
// 关键词: yak focus loader load single file, sidekick co-located
func TestLoadSingleFile_WithSidekick(t *testing.T) {
	tmp := t.TempDir()
	uniq := utils.RandStringBytes(6)
	mainName := "demo_" + uniq + ".ai-focus.yak"
	sidekickName := "demo_" + uniq + "_helper.yak"
	otherFocusName := "another_" + uniq + ".ai-focus.yak"

	mainPath := filepath.Join(tmp, mainName)
	sidekickPath := filepath.Join(tmp, sidekickName)
	otherFocusPath := filepath.Join(tmp, otherFocusName)

	require.NoError(t, os.WriteFile(mainPath, []byte(`
__VERBOSE_NAME__ = greetingFromSidekick()
__MAX_ITERATIONS__ = 3
`), 0o644))

	require.NoError(t, os.WriteFile(sidekickPath, []byte(`
greetingFromSidekick = func() {
    return "demo via sidekick"
}
`), 0o644))

	// 同级目录中的另一个 ai-focus.yak 不能被错误吞为 sidekick。
	require.NoError(t, os.WriteFile(otherFocusPath, []byte(`__VERBOSE_NAME__ = "other"`), 0o644))

	bundle, err := LoadSingleFile(mainPath)
	require.NoError(t, err)
	require.Equal(t, "demo_"+uniq, bundle.Name)
	require.Equal(t, mainPath, bundle.EntryFile)
	require.NotEmpty(t, bundle.EntryCode)
	require.Len(t, bundle.Sidekicks, 1, "only one *.yak sidekick should be picked")
	require.Equal(t, sidekickPath, bundle.Sidekicks[0].Path)
	require.Contains(t, bundle.Sidekicks[0].Content, "greetingFromSidekick")

	// 注册并验证 metadata 来自 sidekick 函数返回值。
	require.NoError(t, reactloops.RegisterYakFocusModeFromBundle(bundle))
	meta, ok := reactloops.GetLoopMetadata(bundle.Name)
	require.True(t, ok)
	require.Equal(t, "demo via sidekick", meta.VerboseName)
}

// LoadSingleFile：路径必须以 .ai-focus.yak 结尾。
// 关键词: yak focus loader bad suffix
func TestLoadSingleFile_BadSuffix(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "demo.yak")
	require.NoError(t, os.WriteFile(bad, []byte("__VERBOSE_NAME__ = \"x\""), 0o644))

	_, err := LoadSingleFile(bad)
	require.Error(t, err)
}

// LoadSingleFile：空路径直接报错。
// 关键词: yak focus loader empty path
func TestLoadSingleFile_EmptyPath(t *testing.T) {
	_, err := LoadSingleFile("")
	require.Error(t, err)
}

// LoadSingleFile：路径不存在时返回 read 错误。
// 关键词: yak focus loader not exist
func TestLoadSingleFile_NotExist(t *testing.T) {
	_, err := LoadSingleFile("/path/does/not/exist/anything.ai-focus.yak")
	require.Error(t, err)
}

// LoadAllFromDir：目录下两个子目录各有自己的 ai-focus.yak。
// 关键词: yak focus loader load all from dir, two modes
func TestLoadAllFromDir_TwoModes(t *testing.T) {
	tmp := t.TempDir()
	uniq := utils.RandStringBytes(6)

	a := filepath.Join(tmp, "alpha_"+uniq)
	b := filepath.Join(tmp, "beta_"+uniq)
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))

	aName := "alpha_" + uniq
	bName := "beta_" + uniq
	require.NoError(t, os.WriteFile(filepath.Join(a, aName+".ai-focus.yak"), []byte(`
__VERBOSE_NAME__ = "Alpha"
__MAX_ITERATIONS__ = 2
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(a, "alpha-helper.yak"), []byte(`
alphaHelper = func() { return "alpha" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(b, bName+".ai-focus.yak"), []byte(`
__VERBOSE_NAME__ = "Beta"
__MAX_ITERATIONS__ = 4
`), 0o644))

	require.NoError(t, LoadAllFromDir(tmp))

	for _, name := range []string{aName, bName} {
		_, ok := reactloops.GetLoopFactory(name)
		require.True(t, ok, "expect %s factory", name)
	}
}

// LoadAllFromDir 空目录返回 nil（无 walk 错误，无注册项）。
// 关键词: yak focus loader empty dir
func TestLoadAllFromDir_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, LoadAllFromDir(tmp))
}

// LoadAllFromDir 路径不存在返回 walk 错误。
// 关键词: yak focus loader bad dir
func TestLoadAllFromDir_BadDir(t *testing.T) {
	require.Error(t, LoadAllFromDir("/path/never/exists/__yakfm__"))
}

// LoadAllFromDir 空字符串报错。
// 关键词: yak focus loader empty arg
func TestLoadAllFromDir_EmptyArg(t *testing.T) {
	require.Error(t, LoadAllFromDir(""))
}

// shouldTreatAsSidekick：边界 case 单元测试。
// 关键词: yak focus loader sidekick filter
func TestShouldTreatAsSidekick(t *testing.T) {
	cases := []struct {
		filename  string
		entryFile string
		want      bool
	}{
		{"helper.yak", "main.ai-focus.yak", true},
		{"main.ai-focus.yak", "main.ai-focus.yak", false},
		{"other.ai-focus.yak", "main.ai-focus.yak", false},
		{"README.md", "main.ai-focus.yak", false},
		{"helper.YAK", "main.ai-focus.yak", false}, // 大小写敏感
	}
	for _, c := range cases {
		got := shouldTreatAsSidekick(c.filename, c.entryFile)
		require.Equalf(t, c.want, got, "filename=%s entry=%s", c.filename, c.entryFile)
	}
}

// deriveName：把 *.ai-focus.yak / *.yak 文件名转为 focus mode 名。
// 关键词: yak focus loader derive name
func TestDeriveName(t *testing.T) {
	require.Equal(t, "hello", deriveName("hello.ai-focus.yak"))
	require.Equal(t, "hello", deriveName("hello.yak"))
	require.Equal(t, "hello", deriveName("hello"))
}
