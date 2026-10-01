package pythonparser

import (
	"fmt"
	"testing"

	"github.com/yaklang/antlr/v4"
)

// Restrict the random oracle's resource use, rather than excluding invalid
// syntax. Larger generic, parameter and recursion bounds have fixed tests.
func predictionFuzzInput(source string) bool {
	if len(source) > 4096 {
		return false
	}
	depth := 0
	for _, r := range source {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
		if depth > 48 {
			return false
		}
	}
	return true
}

func predictionMutation(source string, position, action int) string {
	lexer := NewPythonLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokens.Fill()
	var visible []antlr.Token
	for _, token := range tokens.GetAllTokens() {
		if token.GetTokenType() != antlr.TokenEOF && token.GetChannel() == antlr.TokenDefaultChannel {
			visible = append(visible, token)
		}
	}
	if len(visible) == 0 {
		return source
	}
	token := visible[position%len(visible)]
	runes := []rune(source)
	start, end := token.GetStart(), token.GetStop()+1
	if start < 0 || end > len(runes) {
		return source
	}
	replacement := []string{"", ")", ":", "lambda", "match", "type", "**", "\n", "$", "# comment\n"}[action%10]
	return string(runes[:start]) + replacement + string(runes[end:])
}

func TestPredictionTokenMutations(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		source := "before=f(lambda x:x)\nx=" + expr + "\n"
		for action := 0; action < 10; action++ {
			for _, position := range []int{i + 1, i + 7} {
				mutated := predictionMutation(source, position, action)
				t.Run(fmt.Sprintf("%03d_%d_%d", i, action, position), func(t *testing.T) { checkPrediction(t, mutated, reference, false) })
			}
		}
	}
}

func FuzzPrediction(f *testing.F) {
	for _, expr := range predictionExpressions {
		f.Add("x="+expr+"\n", uint16(0), uint8(0))
	}
	for _, stmt := range predictionStatements {
		f.Add(stmt, uint16(0), uint8(0))
	}
	for _, source := range []string{"", "x", "x=$1\n", "x='", "match=1\n", "match x:\n    case 1:\n        pass\n", "type T=list[int]\n", "print x\nnonlocal y\n", "if x:\n\tpass", "x=1\r\n", "x=\x00\n"} {
		f.Add(source, uint16(0), uint8(0))
	}

	reference := newPredictionAutomata()
	f.Fuzz(func(t *testing.T, source string, position uint16, action uint8) {
		if !predictionFuzzInput(source) {
			return
		}
		// Half of mutations preserve the source exactly. The other half use
		// token boundaries from the real lexer, so edits target grammar rules.
		if action&1 != 0 {
			source = predictionMutation(source, int(position), int(action>>1))
		}
		if !predictionFuzzInput(source) {
			return
		}
		checkPrediction(t, source, reference, false)
		// A previous failed parse must not change version or indentation state.
		got := parsePrediction("def f(x:T):\n    return x\n", true, nil)
		if got.err != nil || got.next != antlr.TokenEOF || got.version != PythonVersionAutodetect {
			t.Fatalf("state leaked after %q: %v (%v)", source, got.err, got.version)
		}
	})
}
