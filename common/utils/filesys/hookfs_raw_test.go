package filesys

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Dependency scanners only need the textual source, so honouring the
// preprocessing hook would run a full macro expansion per file. ReadFileRaw
// must return the original bytes while the normal read still sees the hook.
func TestHookFSReadFileRawBypassesReadHooks(t *testing.T) {
	inner := NewVirtualFs()
	require.NoError(t, inner.WriteFile("pkg/main.c", []byte("#include \"a.h\"\n"), 0o644))

	hooked := NewHookFS(inner)
	calls := 0
	hooked.AddReadHook(&ReadHook{
		Matcher: SuffixMatcher(".c"),
		AfterRead: func(ctx *ReadHookContext, data []byte) ([]byte, error) {
			calls++
			return []byte("expanded"), nil
		},
	})

	raw, err := hooked.ReadFileRaw("pkg/main.c")
	require.NoError(t, err)
	require.Equal(t, "#include \"a.h\"\n", string(raw))
	require.Zero(t, calls, "raw read must not run the read hook")

	hookedData, err := hooked.ReadFile("pkg/main.c")
	require.NoError(t, err)
	require.Equal(t, "expanded", string(hookedData))
	require.Equal(t, 1, calls)
}

func TestHookFSReadFileRawKeepsPathMappingAndSkip(t *testing.T) {
	inner := NewVirtualFs()
	require.NoError(t, inner.WriteFile("real/main.c", []byte("int x;\n"), 0o644))

	hooked := NewHookFS(inner)
	hooked.SetPathHook(func(name string) (string, error) {
		if name == "alias/main.c" {
			return "real/main.c", nil
		}
		return name, nil
	})
	hooked.SetSkipMatcher(HookMatcherFunc(func(name string) bool {
		return strings.HasSuffix(name, "hidden.c")
	}))

	data, err := hooked.ReadFileRaw("alias/main.c")
	require.NoError(t, err)
	require.Equal(t, "int x;\n", string(data), "path mapping still applies")

	_, err = hooked.ReadFileRaw("hidden.c")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// ReadFileRaw must also fall back to ReadFile for filesystems without raw
// support, so callers can use it unconditionally.
func TestReadFileRawFallsBackToReadFile(t *testing.T) {
	inner := NewVirtualFs()
	require.NoError(t, inner.WriteFile("a.c", []byte("int a;\n"), 0o644))

	data, err := ReadFileRaw(inner, "a.c")
	require.NoError(t, err)
	require.Equal(t, "int a;\n", string(data))
}
