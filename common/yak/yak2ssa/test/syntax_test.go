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
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const savedYakFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		builder, ok := yak2ssa.CreateBuilder().(*yak2ssa.SSABuilder)
		require.True(t, ok)
		defer builder.Clearup()

		start := time.Now()
		_, err := builder.ParseAST(src, builder.GetAntlrCache())
		parseDur := time.Since(start)
		require.NoError(t, err, "parse AST FrontEnd error")
		require.LessOrEqual(t, parseDur, savedYakFixtureMaxParseDuration, "parse took too long for %s", filename)

		vf := filesys.NewVirtualFs()
		vf.AddFile(filename, src)
		progs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.Yak),
			ssaapi.WithMemory(),
		)
		require.NoError(t, err, "build fixture %s", filename)
		require.NotEmpty(t, progs, "no program compiled for %s", filename)
	})
}

func TestAllSyntaxForYak_G4(t *testing.T) {
	found := false
	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path.Base(filePath), ".yak") {
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