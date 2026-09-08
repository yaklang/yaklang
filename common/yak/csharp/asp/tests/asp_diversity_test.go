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
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	aspparser "github.com/yaklang/yaklang/common/yak/csharp/asp/parser"
)

func init() {
	if strings.TrimSpace(os.Getenv("YAK_ANTLR_SLL_FIRST_STATS")) == "" {
		_ = os.Setenv("YAK_ANTLR_SLL_FIRST_STATS", "1")
	}
}

const aspMinFixtureCount = 50

var aspRequiredFamilies = []string{
	"directive",
	"declaration",
	"scriptlet",
	"echo",
	"databind",
	"nested",
	"tags",
	"attributes",
	"script",
	"style",
	"comments",
}

func listASPFixtures(t testing.TB) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(aspFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		low := strings.ToLower(filePath)
		if strings.HasSuffix(low, ".aspx") || strings.HasSuffix(low, ".asp") {
			out = append(out, strings.ReplaceAll(filePath, "\\", "/"))
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(out)
	return out
}

func aspFixtureFamily(rel string) string {
	rel = strings.ToLower(strings.ReplaceAll(rel, "\\", "/"))
	if i := strings.Index(rel, "/ga/"); i >= 0 {
		rest := rel[i+len("/ga/"):]
		fam, _, ok := strings.Cut(rest, "/")
		if ok && fam != "" {
			return fam
		}
	}
	base := path.Base(rel)
	switch base {
	case "hello.aspx", "form.aspx":
		return "scriptlet"
	case "databind.aspx":
		return "databind"
	case "nested.aspx":
		return "nested"
	case "corpus_alts.aspx":
		return "tags"
	case "corpus_toplevel.aspx":
		return "tags"
	case "attribute_echo.aspx":
		return "attributes"
	case "classic_loop.aspx":
		return "scriptlet"
	case "webforms_page.aspx":
		return "directive"
	case "island.aspx":
		return "script"
	default:
		return "other"
	}
}

func TestASPFrontCorpusAtLeast50Diverse(t *testing.T) {
	files := listASPFixtures(t)
	require.GreaterOrEqual(t, len(files), aspMinFixtureCount, "need at least 50 ASP/ASPX fixtures")

	rawSeen := map[string]string{}
	normSeen := map[string]string{}
	byFamily := map[string][]string{}

	for _, rel := range files {
		raw, err := aspFs.ReadFile(rel)
		require.NoError(t, err)
		src := string(raw)
		require.NotEmpty(t, src, rel)
		sum := sha256.Sum256(raw)
		key := hex.EncodeToString(sum[:])
		if prev, ok := rawSeen[key]; ok {
			t.Fatalf("byte-identical clone: %s and %s", prev, rel)
		}
		rawSeen[key] = rel
		norm := compactWS(src)
		nsum := sha256.Sum256([]byte(norm))
		nkey := hex.EncodeToString(nsum[:])
		if prev, ok := normSeen[nkey]; ok {
			t.Fatalf("whitespace-normalized clone: %s and %s", prev, rel)
		}
		normSeen[nkey] = rel
		fam := aspFixtureFamily(rel)
		byFamily[fam] = append(byFamily[fam], rel)

		t.Run("parse/"+rel, func(t *testing.T) {
			antlr4util.ResetSLLFirstCounters()
			start := time.Now()
			ast, err := asp.Front(src)
			dur := time.Since(start)
			require.NoError(t, err)
			require.NotNil(t, ast)
			require.LessOrEqual(t, dur, savedASPFrontFixtureMaxParseDuration)
			stats := antlr4util.SLLFirstCountersSnapshot()
			require.Zero(t, stats.FallbackError)
			t.Logf("asp fixture=%s family=%s parse=%s bytes=%d sll_attempts=%d fallbacks=%d errors=%d",
				rel, fam, dur, len(src), stats.SLLAttempts, stats.Fallbacks, stats.FallbackError)
		})
	}
	for _, fam := range aspRequiredFamilies {
		require.NotEmpty(t, byFamily[fam], "required ASP family %s has no fixture", fam)
		t.Logf("family %s count=%d", fam, len(byFamily[fam]))
	}
}

func TestASPFrontNamedConstructsPerFamily(t *testing.T) {
	cases := []struct {
		file string
		kind string
		want string
	}{
		{"code/ga/directive/page.aspx", "directive", "Page"},
		{"code/ga/declaration/field.aspx", "declaration", "DeclField"},
		{"code/ga/scriptlet/assign.aspx", "scriptlet", "keptA"},
		{"code/ga/echo/ident.aspx", "echo", "e"},
		{"code/ga/databind/eval.aspx", "databind", "Eval"},
		{"code/ga/script/server.aspx", "script", "GaAspServer"},
		{"code/mixed/island.aspx", "script", "IslandValue"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			raw, err := aspFs.ReadFile(tc.file)
			require.NoError(t, err)
			ast, err := asp.Front(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
			require.Contains(t, ast.GetText(), tc.want)
			found := false
			var walk func(antlr.Tree)
			walk = func(n antlr.Tree) {
				if n == nil || found {
					return
				}
				text := ""
				if pr, ok := n.(antlr.ParserRuleContext); ok && pr != nil {
					text = pr.GetText()
				}
				if strings.Contains(text, tc.want) {
					switch tc.kind {
					case "directive":
						if _, ok := n.(aspparser.IAspDirectiveContext); ok {
							found = true
						}
					case "declaration":
						if _, ok := n.(aspparser.IAspDeclarationContext); ok {
							found = true
						}
					case "scriptlet":
						if _, ok := n.(aspparser.IAspScriptletContext); ok {
							found = true
						}
					case "echo":
						if _, ok := n.(aspparser.IAspExpressionContext); ok {
							found = true
						}
					case "databind":
						if _, ok := n.(aspparser.IAspDatabindContext); ok {
							found = true
						}
					case "script":
						if _, ok := n.(aspparser.IScriptContext); ok {
							found = true
						}
					}
				}
				for i := 0; i < n.GetChildCount(); i++ {
					walk(n.GetChild(i))
				}
			}
			walk(ast)
			require.True(t, found, "shipped asp.Front must contain %s node with %s", tc.kind, tc.want)
		})
	}
}

func compactWS(src string) string {
	var b strings.Builder
	prev := false
	for _, r := range src {
		if unicode.IsSpace(r) {
			if !prev {
				b.WriteByte(' ')
				prev = true
			}
			continue
		}
		prev = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
