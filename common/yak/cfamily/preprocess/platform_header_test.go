package preprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// A platform variant header present in the tree must not contribute macros to
// translation units that never include it. Php-src keeps Zend/zend_config.w32.h
// and ext/mysqlnd/config-win.h next to the portable headers; merging them made
// every Linux unit see _WIN32, `interface` as `struct` and `not` as `!`.
func TestPlatformVariantHeaderDoesNotLeakMacros(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/main.h", []byte("#define PORTABLE 1\n"), 0o644))
	require.NoError(t, fs.WriteFile("include/config.w32.h", []byte(`
#define _WIN32 1
#define interface struct
#define not !
`), 0o644))
	require.NoError(t, fs.WriteFile("main.c", []byte("int f(void) { return PORTABLE; }\n"), 0o644))

	project := BuildProject(fs, DefaultConfig())
	out, err := project.PreprocessTU("main.c", "int f(void) { return PORTABLE; }\n")
	require.NoError(t, err)
	require.Contains(t, out, "return 1;", "portable header macros must still apply")

	tables := project.collectMacroEnvironment("main.c", "int f(void) { return PORTABLE; }\n")
	require.Equal(t, "1", tables.Object["PORTABLE"])
	require.NotContains(t, tables.Object, "_WIN32")
	require.NotContains(t, tables.Object, "interface")
	require.NotContains(t, tables.Object, "not")
}

// An explicit include of a platform header still applies its macros.
func TestPlatformVariantHeaderAppliesWhenIncluded(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/config.w32.h", []byte("#define W32_ONLY 7\n"), 0o644))
	require.NoError(t, fs.WriteFile("main.c", []byte("#include \"config.w32.h\"\nint f(void) { return W32_ONLY; }\n"), 0o644))

	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include"}
	project := BuildProject(fs, cfg)
	tables := project.collectMacroEnvironment("main.c", "#include \"config.w32.h\"\nint f(void) { return W32_ONLY; }\n")
	require.Equal(t, "7", tables.Object["W32_ONLY"])
}

func TestIsPlatformVariantHeader(t *testing.T) {
	cases := map[string]bool{
		"Zend/zend_config.w32.h":          true,
		"main/config.w32.h":               true,
		"ext/mysqlnd/config-win.h":        true,
		"win32/console.h":                 true,
		"include/windows.h":               true,
		"ext:sys/queue.h":                 false,
		"main/php.h":                      false,
		"Zend/zend_string.h":              false,
		"ext/standard/php_math.h":         false,
	}
	for p, want := range cases {
		require.Equal(t, want, isPlatformVariantHeader(p), p)
	}
}
// The shared table must not absorb other-platform macros through a project
// header: php-src com_dotnet headers include <ocidl.h>, which defines
// `interface` as `struct`. A translation unit that includes such a header still
// gets its macros.
func TestExternalPlatformHeaderDoesNotLeakFromProjectHeader(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("ext/com.h", []byte("#include <windows.h>\n#define COM_LOCAL 1\n"), 0o644))
	src := "int f(void) { return COM_LOCAL; }\n"
	require.NoError(t, fs.WriteFile("main.c", []byte(src), 0o644))

	ext := filesys.NewVirtualFs()
	require.NoError(t, ext.WriteFile("windows.h", []byte("#define interface struct\n#define WIN_ONLY 1\n"), 0o644))

	cfg := DefaultConfig()
	cfg.ExternalIncludeFS = []fi.FileSystem{ext}
	project := BuildProject(fs, cfg)

	tables := project.collectMacroEnvironment("main.c", src)
	require.Equal(t, "1", tables.Object["COM_LOCAL"], "project header macros still apply")
	require.NotContains(t, tables.Object, "interface")
	require.NotContains(t, tables.Object, "WIN_ONLY")

	tu := project.collectMacroEnvironment("main.c", "#include \"ext/com.h\"\n"+src)
	require.Equal(t, "1", tu.Object["WIN_ONLY"], "explicit include chain still expands")
}
