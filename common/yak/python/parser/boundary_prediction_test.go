package pythonparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

type wrappedPredictionStream struct{ antlr.TokenStream }

func boundaryTokens(source string) *antlr.CommonTokenStream {
	lexer := NewPythonLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.LA(1)
	return stream
}

func TestPredictionScannerBounds(t *testing.T) {
	reference := newPredictionAutomata()
	for _, source := range []string{
		"x=(]", "x=(1", "x=" + strings.Repeat("(", 513) + "1" + strings.Repeat(")", 513) + "\n",
		"x=f(\n" + strings.Repeat("# trivia\n", 4096) + "1)\n",
	} {
		input := boundaryTokens(source)
		before := input.Index()
		if alt := rootPrefix(input); alt != 0 {
			t.Fatalf("root exceeded bound or guessed invalid brackets: %d", alt)
		}
		if input.Index() != before {
			t.Fatal("root consumed tokens")
		}
		// Move to RHS to exercise the separate tuple scan with identical bounds.
		input.Consume()
		input.Consume()
		before = input.Index()
		if alt := tuplePrefix(input); alt != 0 {
			t.Fatalf("tuple exceeded bound or guessed invalid brackets: %d", alt)
		}
		if input.Index() != before {
			t.Fatal("tuple consumed tokens")
		}
		checkPrediction(t, source, reference, false)
	}
	for _, predict := range []func(antlr.TokenStream) int{rootPrefix, tuplePrefix} {
		input := &wrappedPredictionStream{boundaryTokens("x+1\n")}
		if alt := predict(input); alt != 0 {
			t.Fatal("scanned unsupported stream")
		}
	}
	for _, test := range []struct {
		source      string
		root, tuple int
	}{
		{"", 4, 2}, {"x", 1, 2}, {"x,y\n", 1, 1}, {"*x,\n", 1, 1}, {"x; y\n", 1, 2},
		{"x\ny\n", 2, 2}, {"x@(y)\n", 1, 2}, {"lambda x,y:x\n", 1, 0}, {"`x,y`\n", 1, 0},
		{"match=1\n", 1, 2}, {"if x:\n    pass\n", 0, 2},
	} {
		t.Run(test.source, func(t *testing.T) {
			input := boundaryTokens(test.source)
			before := input.Index()
			if got := rootPrefix(input); got != test.root {
				t.Fatalf("root: %d want %d", got, test.root)
			}
			if got := tuplePrefix(input); got != test.tuple {
				t.Fatalf("tuple: %d want %d", got, test.tuple)
			}
			if before != input.Index() {
				t.Fatal("prediction consumed input")
			}
		})
	}
	if smallPrefix(antlr.TokenInvalidType) != 0 || atomPrefix(PythonParserMINUS) != 0 {
		t.Fatal("invalid or ambiguous prefix guessed")
	}
}

func TestPredictionDecisionShapes(t *testing.T) {
	atn := newPredictionAutomata().atn
	for _, shape := range []struct{ decision, rule, kind, alts int }{
		{0, PythonParserRULE_root, antlr.ATNStateBlockStart, 4},
		{5, PythonParserRULE_stmt, antlr.ATNStateBlockStart, 2},
		{23, PythonParserRULE_compound_stmt, antlr.ATNStateBlockStart, 7},
		{75, PythonParserRULE_small_stmt, antlr.ATNStateBlockStart, 16},
		{79, PythonParserRULE_testlist_star_expr, antlr.ATNStateBlockStart, 2},
		{95, PythonParserRULE_test, antlr.ATNStateBlockStart, 2},
		{96, PythonParserRULE_test, antlr.ATNStateBlockStart, 2},
		{117, PythonParserRULE_expr, antlr.ATNStateStarLoopEntry, 2},
		{118, PythonParserRULE_expr, antlr.ATNStateBlockStart, 2},
		{127, PythonParserRULE_atom, antlr.ATNStateBlockStart, 11},
	} {
		s := atn.DecisionToState[shape.decision]
		if s.GetRuleIndex() != shape.rule || s.GetStateType() != shape.kind || len(s.GetTransitions()) != shape.alts {
			t.Fatalf("grammar changed decision %d; audit shortcut", shape.decision)
		}
	}
}

func TestPredictionVersionAndModeIsolation(t *testing.T) {
	for _, version := range []PythonVersion{PythonVersionAutodetect, PythonVersion2, PythonVersion3} {
		for _, mode := range []int{antlr.PredictionModeSLL, antlr.PredictionModeLL, antlr.PredictionModeLLExactAmbigDetection} {
			for _, source := range []string{"print x\n", "exec x in g,l\n", "nonlocal x\n", "x:T=1\n", "try:\n    pass\nexcept E,e:\n    pass\n", "type=1\nmatch=2\n", "x=-2**-3\n"} {
				t.Run(fmt.Sprintf("%v_%v_%q", version, mode, source), func(t *testing.T) {
					parse := func(fast bool) (result predictionResult) {
						errors := antlr4util.NewErrorListener()
						lexer := NewPythonLexer(antlr.NewInputStream(source))
						lexer.RemoveErrorListeners()
						lexer.AddErrorListener(errors)
						p := NewPythonParser(antlr.NewCommonTokenStream(lexer, 0))
						p.SetFastPrediction(fast)
						p.Version = version
						newPredictionAutomata().apply(p)
						p.GetInterpreter().SetPredictionMode(mode)
						p.RemoveErrorListeners()
						p.AddErrorListener(errors)
						result.tree = p.Root()
						result.err = errors.Error()
						result.next = p.GetTokenStream().LA(1)
						result.version = p.Version
						return
					}
					original, fast := parse(false), parse(true)
					if fmt.Sprint(original.err) != fmt.Sprint(fast.err) || original.next != fast.next || original.version != fast.version || predictionSnapshot(original.tree) != predictionSnapshot(fast.tree) {
						t.Fatal("version/mode changed results")
					}
				})
			}
		}
	}
}

// Alternate token sources are allowed by CommonTokenStream. Unexpected
// synthetic newlines and large amounts of hidden trivia must retain ATN.
type alteredPredictionToken struct {
	antlr.Token
	kind, channel int
}

func (t alteredPredictionToken) GetTokenType() int { return t.kind }
func (t alteredPredictionToken) GetChannel() int   { return t.channel }

type alteredPredictionLexer struct {
	*PythonLexer
	trivia          int
	last            antlr.Token
	visibleNewlines bool
}

func (l *alteredPredictionLexer) NextToken() antlr.Token {
	if l.last != nil && l.trivia > 0 {
		l.trivia--
		return alteredPredictionToken{l.last, PythonParserWS, antlr.TokenHiddenChannel}
	}
	token := l.PythonLexer.NextToken()
	l.last = token
	if l.visibleNewlines && token.GetTokenType() == PythonParserNEWLINE {
		return alteredPredictionToken{token, PythonParserLINE_BREAK, antlr.TokenDefaultChannel}
	}
	return token
}
func TestPredictionUnexpectedTokenStreams(t *testing.T) {
	for _, fixture := range []struct {
		source   string
		trivia   int
		newlines bool
	}{
		{"x=(1\n2)", 0, true},
		{"x\n", boundaryPredictionTokens - 3, false},
	} {
		lexer := &alteredPredictionLexer{PythonLexer: NewPythonLexer(antlr.NewInputStream(fixture.source)), trivia: fixture.trivia, visibleNewlines: fixture.newlines}
		lexer.RemoveErrorListeners()
		input := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		input.LA(1)
		before := input.Index()
		if rootPrefix(input) != 0 {
			t.Fatal("unexpected stream did not retain ATN")
		}
		if input.Index() != before {
			t.Fatal("boundary scan consumed source")
		}
	}
}
