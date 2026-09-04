package tests

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const savedCSharpFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var codeFs embed.FS

func mustReadCodeFixture(t *testing.T, codePath string) string {
	t.Helper()
	raw, err := codeFs.ReadFile(codePath)
	require.NoError(t, err)
	return string(raw)
}

func validateSource(t *testing.T, filename string, src string, caches ...*ssa.AntlrCache) {
	t.Helper()
	name := strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")
	if name == "" {
		name = "inline.cs"
	}
	t.Run(fmt.Sprintf("syntax file: %v", name), func(t *testing.T) {
		var cache *ssa.AntlrCache
		if len(caches) > 0 {
			cache = caches[0]
		}
		antlr4util.ResetSLLFirstCounters()
		start := time.Now()
		ast, err := csharp2ssa.Frontend(src, cache)
		parseDur := time.Since(start)
		require.NoError(t, err, "parse AST FrontEnd error")
		require.NotNil(t, ast)
		require.LessOrEqual(t, parseDur, savedCSharpFixtureMaxParseDuration, "parse took too long for %s", name)
		stats := antlr4util.SLLFirstCountersSnapshot()
		t.Logf("csharp fixture=%s parse=%s sll_attempts=%d fallbacks=%d cancelled=%d errors=%d",
			name, parseDur, stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError)
		require.Zero(t, stats.FallbackError, "SLL fallback error for %s", name)
	})
}

func TestAllSyntaxForCSharp_G4(t *testing.T) {
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()
	found := false
	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(filePath, ".cs") {
			return nil
		}
		raw, err := codeFs.ReadFile(filePath)
		require.NoError(t, err)
		validateSource(t, filePath, string(raw), cache)
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no embed syntax files found")
	require.Equal(t, csharpGAFixtureCount, len(listCSharpASTFixtures(t)), "syntax walk must see exactly 100 C# fixtures")
}

func TestCSharpIfPreprocessor(t *testing.T) {
	src := `#define DEBUG
class C {
#if DEBUG
    int x = 1;
#else
    int x = 2;
#endif
}
`
	validateSource(t, "preprocessor.cs", src)
}

func TestCSharpNamespaceUsingClass(t *testing.T) {
	src := `using System;
namespace Demo.App {
    public class Box {
        public int W;
        public int H;
        public int Area() { return W * H; }
    }
}
`
	validateSource(t, "namespace.cs", src)
}

func TestCSharpFileFiltersAreCaseInsensitive(t *testing.T) {
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	for _, path := range []string{"Demo.cs", "Demo.CS", "Demo.Cs"} {
		require.True(t, builder.FilterParseAST(path), path)
		require.True(t, builder.FilterFile(path), path)
	}
	for _, path := range []string{
		"Demo.CS", "Page.ASPX", "Control.ASCX", "Handler.ASHX",
		"Service.ASMX", "WEB.CONFIG", "View.CSHTML",
	} {
		require.True(t, builder.FilterPreHandlerFile(path), path)
	}
	require.False(t, builder.FilterParseAST("Page.ASPX"))
	require.False(t, builder.FilterFile("Page.ASPX"))
	require.False(t, builder.FilterPreHandlerFile("Demo.java"))
}
