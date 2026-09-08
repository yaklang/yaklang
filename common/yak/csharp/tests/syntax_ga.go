package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func init() {
	// Enable SLL counter recording for CI-on GA metrics. Must run before any
	// antlr4util.SLLFirstStatsEnabled() call (sync.Once).
	if strings.TrimSpace(os.Getenv("YAK_ANTLR_SLL_FIRST_STATS")) == "" {
		_ = os.Setenv("YAK_ANTLR_SLL_FIRST_STATS", "1")
	}
}

const csharpGAFixtureCount = 100

var csharpRequiredFamilies = []string{
	"types",
	"control",
	"async_linq",
	"preproc",
	"interpolated",
	"patterns",
	"operators",
	"generics",
	"attributes",
	"using",
}

var csharpExistingFamily = map[string]string{
	"basic.cs":                           "types",
	"control.cs":                         "control",
	"corpus_close_alts.cs":               "types",
	"corpus_contextual.cs":               "types",
	"corpus_keywords.cs":                 "attributes",
	"corpus_modifiers.cs":                "types",
	"corpus_operators.cs":                "operators",
	"corpus_predefined.cs":               "operators",
	"corpus_remaining.cs":                "types",
	"edge_async_linq.cs":                 "async_linq",
	"edge_members.cs":                    "types",
	"edge_preproc.cs":                    "preproc",
	"edge_types.cs":                      "types",
	"large_allinone.cs":                  "types",
	"preprocessor.cs":                    "preproc",
	"allinonenopreprocessor.cs":          "types",
	"c2430-ok.cs":                        "preproc",
	"c2430.cs":                           "preproc",
	"csharp7classmemberdeclarations.cs":  "types",
	"csharp7exprbodied.cs":               "types",
	"csharp7patterns.cs":                 "patterns",
	"csharp7privateprotected.cs":         "types",
	"csharp7refforeach.cs":               "control",
	"csharp7stackalloc.cs":               "operators",
	"csharp8asyncstreams.cs":             "async_linq",
	"csharp8defaultinterfacemembers.cs":  "types",
	"csharp8disposablerefstructs.cs":     "types",
	"csharp8indicesandranges.cs":         "operators",
	"csharp8miscfeatures.cs":             "interpolated",
	"csharp8nullcoalescingassignment.cs": "operators",
	"csharp8nullablereferencetypes.cs":   "types",
	"csharp8readonlymembers.cs":          "types",
	"csharp8staticlocalfunctions.cs":     "types",
	"csharp8switchexpressions.cs":        "patterns",
	"csharp8tuplepositionalpatterns.cs":  "patterns",
	"csharp8usingdeclarations.cs":        "using",
	"multiplicativeexprsinarglist.cs":    "operators",
	"using_var.cs":                       "using",
}

type csharpParseMetrics struct {
	Path     string
	Family   string
	Bytes    int
	Duration time.Duration
	SLL      antlr4util.SLLFirstCounters
}

func listCSharpASTFixtures(t testing.TB) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(filePath), ".cs") {
			return nil
		}
		out = append(out, strings.ReplaceAll(filePath, "\\", "/"))
		return nil
	})
	require.NoError(t, err)
	sort.Strings(out)
	return out
}

func csharpFixtureFamily(rel string) string {
	rel = strings.ToLower(strings.ReplaceAll(rel, "\\", "/"))
	if i := strings.Index(rel, "/ga/"); i >= 0 {
		rest := rel[i+len("/ga/"):]
		fam, _, ok := strings.Cut(rest, "/")
		if ok && fam != "" {
			return fam
		}
	}
	base := strings.ToLower(path.Base(rel))
	if fam, ok := csharpExistingFamily[base]; ok {
		return fam
	}
	return "other"
}

func parseCSharpFrontend(t testing.TB, src string, cache *ssa.AntlrCache) (csharpparser.ICompilation_unitContext, time.Duration, antlr4util.SLLFirstCounters) {
	t.Helper()
	antlr4util.ResetSLLFirstCounters()
	start := time.Now()
	ast, err := csharp2ssa.Frontend(src, cache)
	dur := time.Since(start)
	stats := antlr4util.SLLFirstCountersSnapshot()
	require.NoError(t, err, "parse AST FrontEnd error")
	require.NotNil(t, ast)
	require.LessOrEqual(t, dur, savedCSharpFixtureMaxParseDuration)
	require.Zero(t, stats.FallbackError, "SLL fallback error on shipped Frontend")
	return ast, dur, stats
}

func compactCSharpSource(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	prevSpace := false
	for _, r := range src {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
