package ssaapi

import (
	"fmt"
	"path"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestYakEntryAndIncludeSources(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, workers := range []int{1, 3} {
			t.Run(fmt.Sprintf("legacy-%t/workers-%d", legacy, workers), func(t *testing.T) {
				if legacy {
					t.Setenv("YAK_SSA_COMPILE_UNIT_LEGACY", "1")
				} else {
					t.Setenv("YAK_SSA_COMPILE_UNIT_LEGACY", "0")
				}
				for _, test := range []struct {
					name, main, dep, entry, base string
					auto, noEntry                bool
				}{
					{name: "standalone-path-first", main: "z/main.yak", dep: "a/b.yak", entry: "z/main.yak"},
					{name: "include-path-first", main: "a/main.yak", dep: "z/b.yak", entry: ".\\a\\main.yak"},
					{name: "automatic-language", main: "z/main.yak", dep: "a/b.yak", entry: "z/main.yak", auto: true},
					{name: "relative-program-path", main: "src/main.yak", dep: "src/deps/b.yak", entry: "main.yak", base: "src"},
					{name: "no-entry-independent-first", main: "z/main.yak", dep: "a/b.yak", noEntry: true},
					{name: "no-entry-include-first", main: "a/main.yak", dep: "z/b.yak", noEntry: true},
				} {
					t.Run(test.name, func(t *testing.T) {
						fs := filesys.NewVirtualFs()
						include := test.dep
						if test.base != "" {
							include = "deps/b.yak"
						}
						fs.AddFile(test.main, fmt.Sprintf("include %q; dump(b)", include))
						fs.AddFile(test.dep, `b = 3`)
						options := []ssaconfig.Option{ssaapi.WithMemory(), ssaconfig.WithCompileConcurrency(workers)}
						if !test.auto {
							options = append(options, ssaapi.WithLanguage(ssaconfig.Yak))
						}
						if test.base != "" {
							options = append(options, ssaapi.WithProgramPath(test.base))
						}
						if !test.noEntry {
							options = append(options, ssaapi.WithFileSystemEntry(test.entry))
							// This source remains in the FS, but is not an executable root.
							fs.AddFile(path.Join(test.base, "unused/other.yak"), `unreachable = 99; dump("unexpected root")`)
						}
						programs, err := ssaapi.ParseProjectWithFS(fs, options...)
						require.NoError(t, err)
						require.Len(t, programs, 1)
						result, err := programs[0].SyntaxFlowWithError(`dump(* as $sink)`)
						require.NoError(t, err)
						values := result.GetValues("sink")
						require.Len(t, values, 1, "one call site must have its actual included value")
						require.Equal(t, "3", values[0].String())
						if !test.noEntry {
							require.Len(t, programs[0].Ref("b"), 1)
							require.Empty(t, programs[0].Ref("unreachable"))
							unused := path.Join(test.base, "unused/other.yak")
							// Source APIs use /programName/path URLs, rather than FS keys.
							unusedURL := path.Join("/", programs[0].Program.GetProgramName(), unused)
							editor, found := programs[0].Program.GetEditor(unusedURL)
							require.True(t, found, "the source registry retains unexecuted files")
							require.Contains(t, editor.GetSourceCode(), "unexpected root")
							require.False(t, programs[0].Program.ShouldVisit(unusedURL), "registered source is not executed source")
							require.NotContains(t, programs[0].Program.FileList, unusedURL)
						}
					})
				}
				t.Run("two-distinct-includes", func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					fs.AddFile("m/main.yak", `include "x/b.yak"; include "y/c.yak"; dump(b); dump(c)`)
					fs.AddFile("x/b.yak", `b = 3`)
					fs.AddFile("y/c.yak", `c = 4`)
					programs, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithMemory(), ssaapi.WithLanguage(ssaconfig.Yak), ssaconfig.WithCompileConcurrency(workers), ssaapi.WithFileSystemEntry("m/main.yak,m/main.yak"))
					require.NoError(t, err)
					require.Len(t, programs, 1)
					result, err := programs[0].SyntaxFlowWithError(`dump(* as $sink)`)
					require.NoError(t, err)
					var values []string
					for _, value := range result.GetValues("sink") {
						values = append(values, value.String())
					}
					sort.Strings(values)
					require.Equal(t, []string{"3", "4"}, values, "neither include can borrow the caller's visited marker")
				})
				t.Run("invalid-entry", func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					fs.AddFile("main.yak", `b = 3`)
					_, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithMemory(), ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithFileSystemEntry("absent.yak"))
					require.ErrorIs(t, err, ssaapi.ErrNoFoundCompiledFile)
				})
				t.Run("filesystem-relative-before-program-relative", func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					fs.AddFile("src/main.yak", `dump(3)`)
					fs.AddFile("src/src/main.yak", `dump(4)`)
					programs, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithMemory(), ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramPath("src"), ssaconfig.WithCompileConcurrency(workers), ssaapi.WithFileSystemEntry("src/main.yak"))
					require.NoError(t, err)
					require.Len(t, programs, 1)
					result, err := programs[0].SyntaxFlowWithError(`dump(* as $sink)`)
					require.NoError(t, err)
					values := result.GetValues("sink")
					require.Len(t, values, 1)
					require.Equal(t, "3", values[0].String())
				})
				t.Run("entry-matches-scanned-case", func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					fs.AddFile("Main.yak", `dump(3)`)
					_, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithMemory(), ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithFileSystemEntry("main.yak"))
					require.ErrorIs(t, err, ssaapi.ErrNoFoundCompiledFile)
				})
			})
		}
	}
}
