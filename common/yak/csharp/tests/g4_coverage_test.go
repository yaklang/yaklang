package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	aspparser "github.com/yaklang/yaklang/common/yak/csharp/asp/parser"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
)

var csharpSpecOnlyUnreachable = []string{
	"input",
	"input_element",
	"input_section",
	"input_section_part",
	"operator_or_punctuator",
	"token",
}

// csharpATNUnenteredAlts are parser alternatives that are reference-reachable
// from `prog` but that this recognizer never constructs:
// predicates (IsDelegateTypeName etc. — DeclareType is never called from PreScan),
// inlining into primary_expression, or prediction preferring an overlapping alt
// (invocation vs nameof, class_type vs 'dynamic'/'notnull', constant_pattern vs var_pattern).
// The coverage test fails if one of these starts being visited (remove it) or if a
// new reachable alt is neither visited nor listed here.
var csharpATNUnenteredAlts = []string{
	"delegate_creation_expression#1",
	"delegate_type#1",
	"designation#1",
	"designation#2",
	"designations#1",
	"discard_pattern#1",
	"enum_type#1",
	"member_access#3",
	"named_entity_target#1",
	"named_entity_target#2",
	"named_entity_target#5",
	"non_array_type#3",
	"non_array_type#4",
	"non_array_type#5",
	"non_array_type#6",
	"non_nullable_reference_type#1",
	"non_nullable_reference_type#2",
	"non_nullable_reference_type#5",
	"non_nullable_value_type#2",
	"pattern#3",
	"pattern#6",
	"post_decrement_expression#1",
	"post_increment_expression#1",
	"primary_constraint#4",
	"primary_constraint#5",
	"primary_expression#20",
	"secondary_constraint#2",
	"statement_expression#3",
	"statement_expression#5",
	"statement_expression#6",
	"tuple_designation#1",
	"tuple_expression#2",
	"var_pattern#1",
}

func loadCSharpParserG4(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "CSharpParser.g4"))
	require.NoError(t, err)
	return string(raw)
}

func loadASPParserG4(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "asp", "ASPParser.g4"))
	require.NoError(t, err)
	return string(raw)
}

func matchAltsFromTree(root antlr.Tree, inv *g4Inventory, ruleNames []string, symbolic []string) map[string]bool {
	hit := map[string]bool{}
	var walk func(antlr.Tree)
	walk = func(n antlr.Tree) {
		if n == nil {
			return
		}
		if pr, ok := n.(antlr.ParserRuleContext); ok && pr != nil {
			idx := pr.GetRuleIndex()
			if idx >= 0 && idx < len(ruleNames) {
				name := ruleNames[idx]
				rule := inv.Rules[name]
				if rule != nil && rule.Reachable {
					tokTexts, tokSyms, childRules := directChildren(pr, ruleNames, symbolic)
					for _, alt := range rule.Alts {
						if altSatisfied(alt, tokTexts, tokSyms, childRules) {
							hit[altKey(name, alt.Index)] = true
						}
					}
					if len(rule.Alts) == 1 {
						hit[altKey(name, 1)] = true
					}
				}
			}
		}
		for i := 0; i < n.GetChildCount(); i++ {
			walk(n.GetChild(i))
		}
	}
	walk(root)
	return hit
}

func directChildren(pr antlr.ParserRuleContext, ruleNames, symbolic []string) (tokTexts, tokSyms, childRules []string) {
	for i := 0; i < pr.GetChildCount(); i++ {
		ch := pr.GetChild(i)
		if tok, ok := ch.(antlr.Token); ok && tok != nil {
			tokTexts = append(tokTexts, tok.GetText())
			ti := tok.GetTokenType()
			if ti >= 0 && ti < len(symbolic) && symbolic[ti] != "" {
				tokSyms = append(tokSyms, symbolic[ti])
			}
			continue
		}
		if tn, ok := ch.(antlr.TerminalNode); ok && tn != nil && tn.GetSymbol() != nil {
			sym := tn.GetSymbol()
			tokTexts = append(tokTexts, sym.GetText())
			ti := sym.GetTokenType()
			if ti >= 0 && ti < len(symbolic) && symbolic[ti] != "" {
				tokSyms = append(tokSyms, symbolic[ti])
			}
			continue
		}
		if cr, ok := ch.(antlr.ParserRuleContext); ok && cr != nil {
			ri := cr.GetRuleIndex()
			if ri >= 0 && ri < len(ruleNames) {
				childRules = append(childRules, ruleNames[ri])
			}
		}
	}
	return
}

func altSatisfied(alt g4Alt, tokTexts, tokSyms, childRules []string) bool {
	if len(alt.RequiredTok) == 0 && len(alt.RequiredRules) == 0 {
		return true
	}
	for _, tok := range alt.RequiredTok {
		if !hasToken(tok, tokTexts, tokSyms) {
			return false
		}
	}
	for _, r := range alt.RequiredRules {
		if !containsStr(childRules, r) {
			return false
		}
	}
	return true
}

func hasToken(want string, texts, syms []string) bool {
	if containsStr(texts, want) {
		return true
	}
	if containsStr(syms, want) {
		return true
	}
	// lexer token names DEFAULT/TRUE/FALSE/NULL match lowercase literals
	low := strings.ToLower(want)
	return containsStr(texts, low)
}

func containsStr(xs []string, w string) bool {
	for _, x := range xs {
		if x == w {
			return true
		}
	}
	return false
}

func csharpLexerModesFromSource(t *testing.T, src string) map[string]bool {
	t.Helper()
	input := antlr.NewInputStream(src)
	lex := csharpparser.NewCSharpLexer(input)
	lex.InitCSharpLexer()
	hit := map[string]bool{"DEFAULT_MODE": true}
	for _, tok := range lex.GetAllTokens() {
		if tok == nil || tok.GetTokenType() == antlr.TokenEOF {
			continue
		}
		switch tok.GetTokenType() {
		case csharpparser.CSharpLexerInterpolated_Regular_String_Start,
			csharpparser.CSharpLexerInterpolated_Regular_String_Mid,
			csharpparser.CSharpLexerInterpolated_Regular_String_End,
			csharpparser.CSharpLexerRegular_Interpolation_Format:
			hit["IRS_CONT"] = true
		case csharpparser.CSharpLexerInterpolated_Verbatim_String_Start,
			csharpparser.CSharpLexerInterpolated_Verbatim_String_Mid,
			csharpparser.CSharpLexerInterpolated_Verbatim_String_End,
			csharpparser.CSharpLexerVerbatim_Interpolation_Format:
			hit["IVS_CONT"] = true
		case csharpparser.CSharpLexerDEFINE, csharpparser.CSharpLexerUNDEF,
			csharpparser.CSharpLexerELIF, csharpparser.CSharpLexerENDIF,
			csharpparser.CSharpLexerLINE, csharpparser.CSharpLexerSHARP:
			hit["DIRECTIVE_MODE"] = true
		case csharpparser.CSharpLexerERROR, csharpparser.CSharpLexerWARNING,
			csharpparser.CSharpLexerREGION, csharpparser.CSharpLexerENDREGION,
			csharpparser.CSharpLexerPRAGMA, csharpparser.CSharpLexerNULLABLE,
			csharpparser.CSharpLexerTEXT:
			hit["DIRECTIVE_MODE"] = true
			hit["DIRECTIVE_TEXT"] = true
		}
	}
	return hit
}

func aspLexerModesFromSource(t *testing.T, src string) map[string]bool {
	t.Helper()
	input := antlr.NewInputStream(src)
	lex := aspparser.NewASPLexer(input)
	hit := map[string]bool{"DEFAULT_MODE": true}
	for _, tok := range lex.GetAllTokens() {
		if tok == nil || tok.GetTokenType() == antlr.TokenEOF {
			continue
		}
		switch tok.GetTokenType() {
		case aspparser.ASPLexerBLOB_CONTENT, aspparser.ASPLexerBLOB_CLOSE,
			aspparser.ASPLexerSCRIPTLET_OPEN, aspparser.ASPLexerECHO_EXPRESSION_OPEN,
			aspparser.ASPLexerDATABIND_OPEN, aspparser.ASPLexerDIRECTIVE_BEGIN,
			aspparser.ASPLexerDECLARATION_BEGIN:
			hit["ASP_BLOB"] = true
		case aspparser.ASPLexerTAG_IDENTIFIER, aspparser.ASPLexerTAG_BEGIN,
			aspparser.ASPLexerCLOSE_TAG_BEGIN, aspparser.ASPLexerTAG_CLOSE,
			aspparser.ASPLexerTAG_SLASH_END:
			hit["TAG"] = true
		case aspparser.ASPLexerATTVAL_VALUE:
			hit["ATTVALUE"] = true
		case aspparser.ASPLexerSCRIPT_OPEN, aspparser.ASPLexerSCRIPT_BODY:
			hit["SCRIPT"] = true
		case aspparser.ASPLexerSTYLE_OPEN, aspparser.ASPLexerSTYLE_BODY:
			hit["STYLE"] = true
		}
	}
	return hit
}

func TestCSharpG4InventoryUnreachableIsDocumented(t *testing.T) {
	inv := parseParserG4(loadCSharpParserG4(t), "prog")
	require.Equal(t, csharpSpecOnlyUnreachable, inv.Unreachable,
		"spec-only unreachable rules changed; update the committed inventory")
	require.Contains(t, inv.Reachable, "compilation_unit")
	require.Contains(t, inv.Reachable, "keyword")
	require.GreaterOrEqual(t, inv.reachableAltCount(), 900)
	for _, key := range csharpATNUnenteredAlts {
		parts := strings.Split(key, "#")
		require.Len(t, parts, 2, key)
		rule := inv.Rules[parts[0]]
		require.NotNil(t, rule, "unentered alt %s is not a parser rule", key)
		require.True(t, rule.Reachable, "unentered alt %s is not start-rule reachable", key)
		found := false
		for _, a := range rule.Alts {
			if altKey(rule.Name, a.Index) == key {
				found = true
				break
			}
		}
		require.True(t, found, "unentered alt %s does not exist on the rule", key)
	}
}

func TestCSharpAndASPG4BranchCoverage_Frontend(t *testing.T) {
	csInv := parseParserG4(loadCSharpParserG4(t), "prog")
	aspInv := parseParserG4(loadASPParserG4(t), "aspDocuments")
	require.Equal(t, csharpSpecOnlyUnreachable, csInv.Unreachable)
	require.Empty(t, aspInv.Unreachable)

	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()

	csHits := map[string]bool{}
	lexerModes := map[string]bool{}
	parsed := 0
	csRuleNames := csharpparser.NewCSharpParser(nil).GetRuleNames()
	csSymNames := csharpparser.NewCSharpLexer(nil).GetSymbolicNames()
	aspRuleNames := aspparser.NewASPParser(nil).GetRuleNames()
	aspSymNames := aspparser.NewASPLexer(nil).GetSymbolicNames()

	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(filePath, ".cs") {
			return nil
		}
		raw, err := codeFs.ReadFile(filePath)
		require.NoError(t, err)
		src := string(raw)
		t.Run("cs/"+filePath, func(t *testing.T) {
			start := time.Now()
			ast, err := csharp2ssa.Frontend(src, cache)
			require.NoError(t, err, "parse AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration, "parse took too long for %s", filePath)
			// prog is the start rule; Frontend returns compilation_unit
			csHits[altKey("prog", 1)] = true
			for k, v := range matchAltsFromTree(ast, csInv, csRuleNames, csSymNames) {
				if v {
					csHits[k] = true
				}
			}
			for m, v := range csharpLexerModesFromSource(t, src) {
				if v {
					lexerModes[m] = true
				}
			}
		})
		parsed++
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, parsed, 20, "expected expanded public+generated C# corpus")

	unentered := map[string]bool{}
	for _, k := range csharpATNUnenteredAlts {
		unentered[k] = true
	}
	var missing []string
	var unexpectedlyHit []string
	for _, name := range csInv.Reachable {
		rule := csInv.Rules[name]
		for _, alt := range rule.Alts {
			key := altKey(name, alt.Index)
			hit := csHits[key]
			if unentered[key] {
				if hit {
					unexpectedlyHit = append(unexpectedlyHit, key+"  "+alt.Text)
				}
				continue
			}
			if !hit {
				missing = append(missing, key+"  "+alt.Text)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpectedlyHit)
	t.Logf("C# reachable alts=%d visited=%d documented-unentered=%d", csInv.reachableAltCount(), csInv.reachableAltCount()-len(csharpATNUnenteredAlts)-len(missing), len(csharpATNUnenteredAlts))
	require.Empty(t, unexpectedlyHit, "documented ATN-unentered alt was visited; remove it from csharpATNUnenteredAlts")
	if len(missing) > 0 {
		t.Logf("uncovered C# reachable alts (%d/%d):", len(missing), csInv.reachableAltCount())
		limit := len(missing)
		if limit > 80 {
			limit = 80
		}
		for _, m := range missing[:limit] {
			t.Log("  ", m)
		}
	}
	require.Empty(t, missing, "reachable C# parser alternatives not visited by shipped Frontend (and not in the committed unentered inventory)")

	for _, mode := range []string{"DEFAULT_MODE", "IRS_CONT", "IVS_CONT", "DIRECTIVE_MODE", "DIRECTIVE_TEXT"} {
		require.True(t, lexerModes[mode], "C# lexer mode %s was not entered", mode)
	}

	// ASP corpus (fixtures live next to asp.Front)
	aspHits := map[string]bool{}
	aspModes := map[string]bool{}
	aspRoot := filepath.Join("..", "asp", "tests", "code")
	aspCount := 0
	err = filepath.WalkDir(aspRoot, func(path string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		low := strings.ToLower(path)
		if !strings.HasSuffix(low, ".aspx") && !strings.HasSuffix(low, ".asp") {
			return nil
		}
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		rel, _ := filepath.Rel(aspRoot, path)
		src := string(raw)
		t.Run("asp/"+rel, func(t *testing.T) {
			start := time.Now()
			ast, err := asp.Front(src)
			require.NoError(t, err, "parse ASP AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration)
			for k, v := range matchAltsFromTree(ast, aspInv, aspRuleNames, aspSymNames) {
				if v {
					aspHits[k] = true
				}
			}
			for m, v := range aspLexerModesFromSource(t, src) {
				if v {
					aspModes[m] = true
				}
			}
		})
		aspCount++
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, aspCount, 5)

	var aspMissing []string
	for _, name := range aspInv.Reachable {
		rule := aspInv.Rules[name]
		for _, alt := range rule.Alts {
			if !aspHits[altKey(name, alt.Index)] {
				aspMissing = append(aspMissing, altKey(name, alt.Index)+"  "+alt.Text)
			}
		}
	}
	sort.Strings(aspMissing)
	require.Empty(t, aspMissing, "reachable ASP parser alternatives not visited by shipped Front")
	for _, mode := range []string{"DEFAULT_MODE", "ASP_BLOB", "TAG", "ATTVALUE", "SCRIPT", "STYLE"} {
		require.True(t, aspModes[mode], "ASP lexer mode %s was not entered", mode)
	}
}

func TestCSharpPublicSampleNamedConstruct_Frontend(t *testing.T) {
	src := mustReadCodeFixture(t, "code/public/CSharp8SwitchExpressions.cs")
	start := time.Now()
	ast, err := csharp2ssa.Frontend(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration)
	names := collectTypeNames(ast)
	require.Contains(t, names, "CSharp8SwitchExpressions", "collected public sample must produce a named class in the shipped AST")
	require.Contains(t, ast.GetText(), "DescribeSeason")
}

func TestASPPublicSampleNamedScriptlet_Front(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "asp", "tests", "code", "public", "classic_loop.aspx"))
	require.NoError(t, err)
	start := time.Now()
	ast, err := asp.Front(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration)
	var found bool
	var walk func(antlr.Tree)
	walk = func(n antlr.Tree) {
		if n == nil {
			return
		}
		if sl, ok := n.(aspparser.IAspScriptletContext); ok && strings.Contains(sl.GetText(), "for") {
			found = true
		}
		for i := 0; i < n.GetChildCount(); i++ {
			walk(n.GetChild(i))
		}
	}
	walk(ast)
	require.True(t, found, "collected ASP sample must contain a for-loop scriptlet node")
}
