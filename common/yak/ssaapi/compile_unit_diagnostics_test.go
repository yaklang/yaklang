package ssaapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestJavaPHPProjectDiagnosticsAreNotReplicated(t *testing.T) {
	for _, language := range []ssaconfig.Language{ssaconfig.JAVA, ssaconfig.PHP} {
		for _, workers := range []int{1, 4} {
			for _, files := range []int{4, 16} {
				t.Run(fmt.Sprintf("%s/workers=%d/files=%d", language, workers, files), func(t *testing.T) {
					fs := filesys.NewVirtualFs()
					for i := 0; i < files; i++ {
						code := fmt.Sprintf("<?php // file %d\nbreak;", i)
						if language == ssaconfig.JAVA {
							code = fmt.Sprintf("package p%d; class Bad%d { void run() { int x = 1; x.missing(); } }", i, i)
						}
						fs.AddFile(fmt.Sprintf("p%d/Bad%d.%s", i, i, language), code)
					}
					programs, err := ParseProjectWithFS(fs, WithLanguage(language), WithMemory(), ssaconfig.WithCompileConcurrency(workers))
					require.NoError(t, err)
					require.Len(t, programs, 1)
					seen := make(map[*ssa.SSAError]bool)
					counts := make(map[string]int)
					for _, diagnostic := range programs[0].GetErrors() {
						require.NotNil(t, diagnostic)
						require.False(t, seen[diagnostic], "diagnostic must reach the application once")
						seen[diagnostic] = true
						if diagnostic.Tag == "ObjectError" || diagnostic.Tag == "break statement not in loop or switch: raw %v" {
							require.NotNil(t, diagnostic.Pos)
							counts[diagnostic.Pos.GetEditor().GetFilename()]++
						}
					}
					require.Len(t, counts, files, "retain diagnostics from every source file")
					for i := 0; i < files; i++ {
						require.Equal(t, 1, counts[fmt.Sprintf("Bad%d.%s", i, language)])
					}
				})
			}
		}
	}
}
