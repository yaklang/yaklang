package test

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const savedJSFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		start := time.Now()
		_, err := js2ssa.Frontend(src)
		parseDur := time.Since(start)
		require.NoError(t, err, "parse AST FrontEnd error")
		require.LessOrEqual(t, parseDur, savedJSFixtureMaxParseDuration, "parse took too long for %s", filename)

		vf := filesys.NewVirtualFs()
		vf.AddFile(filename, src)
		progs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.JS),
			ssaapi.WithMemory(),
		)
		require.NoError(t, err, "build fixture %s", filename)
		require.NotEmpty(t, progs, "no program compiled for %s", filename)
	})
}

func TestAllSyntaxForJS_G4(t *testing.T) {
	found := false
	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path.Base(filePath), ".js") {
			return nil
		}
		raw, err := codeFs.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("cannot read syntax fs %s: %w", filePath, err)
		}
		validateSource(t, filePath, string(raw))
		found = true
		return nil
	})
	require.NoError(t, err, "walk code fixtures")
	require.True(t, found, "no embed syntax files found")
}