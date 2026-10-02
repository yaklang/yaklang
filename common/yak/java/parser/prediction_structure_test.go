package javaparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// Build grammar-shaped input before corrupting tokens. Unlike byte edits alone,
// this reaches nested alternatives with matching delimiters and complete bodies.
// Budgets bound the generator itself; malformed outputs still reach the oracle.
type predictionStructure struct {
	data         []byte
	index, nodes int
}

func (g *predictionStructure) pick(n int) int {
	if g.index == len(g.data) {
		return 0
	}
	b := g.data[g.index]
	g.index++
	return int(b) % n
}

func (g *predictionStructure) typeName(depth int) string {
	g.nodes++
	if depth == 0 || g.nodes >= 48 {
		return []string{"T", "U", "record", "零"}[g.pick(4)]
	}
	switch g.pick(7) {
	case 1:
		return "T<" + g.typeName(depth-1) + ">"
	case 2:
		return "T<" + g.typeName(depth-1) + "," + g.typeName(depth-1) + ">"
	case 3:
		return "T<? extends " + g.typeName(depth-1) + "[]>"
	case 4:
		return "T<? super " + g.typeName(depth-1) + ">"
	case 5:
		return "a.T<" + g.typeName(depth-1) + ">.U"
	case 6:
		return "T<@A " + g.typeName(depth-1) + ">"
	default:
		return "T"
	}
}

func (g *predictionStructure) expression(depth int) string {
	g.nodes++
	if depth == 0 || g.nodes >= 48 {
		return []string{"x", "record", "yield", "零", "1", "null", "this", "\"s\""}[g.pick(8)]
	}
	nested := func() string { return g.expression(depth - 1) }
	switch g.pick(16) {
	case 0:
		return "x"
	case 1:
		return "(" + nested() + ")"
	case 2:
		op := []string{"<", ">", ">>", ">>>", "<<", "+", "*", "&&", "|", "==", "<="}[g.pick(11)]
		return "(" + nested() + ")" + op + "(" + nested() + ")"
	case 3:
		return "f(" + nested() + "," + nested() + ")"
	case 4:
		return "f(" + []string{"x", "(x,y)", "(T x)", "(var x)", "(@A T x)"}[g.pick(5)] + " -> " + nested() + ")"
	case 5:
		return "(" + g.typeName(3) + " & U)(" + nested() + ")"
	case 6:
		return g.typeName(3) + []string{"::new", "::m", "::<U>m", "[]::new", ".class"}[g.pick(5)]
	case 7:
		return "(" + nested() + ") ? (" + nested() + ") : (" + nested() + ")"
	case 8:
		return "x" + []string{"=", "+=", "<<=", ">>>="}[g.pick(4)] + "(" + nested() + ")"
	case 9:
		return "new " + g.typeName(3) + "(" + nested() + ")"
	case 10:
		return "new T[]{" + nested() + "," + nested() + ",}"
	case 11:
		return []string{"!", "~", "+", "-"}[g.pick(4)] + "(" + nested() + ")"
	case 12:
		return "f(" + nested() + ").<T>m(" + nested() + ")"
	case 13:
		return "switch(x){case 1 -> " + nested() + ";default -> " + nested() + ";}"
	case 14:
		return "(" + nested() + ") instanceof " + g.typeName(3) + " t"
	default:
		return "f(" + nested() + ").x++"
	}
}

func predictionStructureSource(data []byte) string {
	g := predictionStructure{data: data}
	context, trivia := g.pick(8), g.pick(4)
	expr := g.expression(4)
	var source string
	switch context {
	case 0:
		source = methodSource("return " + expr + ";")
	case 1:
		source = methodSource("T<X> before; Object x=" + expr + "; return x;")
	case 2:
		source = "class C { Object x=" + expr + "; }"
	case 3:
		source = methodSource("f(" + expr + ",x -> " + expr + ");")
	case 4:
		source = methodSource("if(" + expr + ") f(); else { T<X> x; }")
	case 5:
		source = methodSource("for(T<X> x:" + expr + "){ f(x); }")
	case 6:
		source = methodSource("try(T<X> x=" + expr + "){ f(x); }catch(E|F e){}")
	default:
		source = methodSource("switch(x){case 1: f(" + expr + ");break;default: return " + expr + ";}")
	}
	// Whitespace positions cannot merge operators or split string literals.
	return strings.ReplaceAll(source, " ", []string{" ", "/*g*/", "\n", "//g\r\n"}[trivia])
}

func TestPredictionGeneratedStructures(t *testing.T) {
	reference := newPredictionAutomata()
	// Each expression production has a fully accepted minimal derivation.
	// Combinations and corruption below additionally exercise rejected inputs.
	for production := 0; production < 16; production++ {
		source := predictionStructureSource([]byte{0, 0, byte(production)})
		checkPrediction(t, source, reference, true)
	}
	for context := 0; context < 8; context++ {
		for production := 0; production < 16; production++ {
			data := []byte{byte(context), byte(production % 4), byte(production)}
			for i := 0; i < 48; i++ {
				data = append(data, byte(production*17+i*31+context))
			}
			t.Run(fmt.Sprintf("%d_%d", context, production), func(t *testing.T) {
				source := predictionStructureSource(data)
				checkPrediction(t, source, reference, false)
				for action := 0; action < 8; action++ {
					checkPrediction(t, predictionMutation(source, production+context, action), reference, false)
				}
			})
		}
	}
}

func TestPredictionNonTypePrimaryContract(t *testing.T) {
	for _, fixture := range []struct {
		source string
		want   int
	}{
		{"a<b;", 5}, {"a<b<c;", 5}, {"a<b+c", 5}, {"a<b", 5},
		{"a<<b", 5}, {"a</*g*/<b>>c", 5}, {"T<<X>>.class", 5},
		{"T<X>.class", 0}, {"T<X>[][].class", 0}, {"T<X>.U<Y>.class", 0},
		{"T<@A X>.class", 0}, {"T<X> @A [].class", 0}, {"T<X> a.@A [].class", 0},
		{"T<X>::new", 0}, {"T<X[>::new", 0}, {"a.b", 0}, {"int<x", 0},
	} {
		stream := predictionTokens(fixture.source)
		index := stream.Index()
		if got := nonTypePrimaryPrefix(stream); got != fixture.want {
			t.Fatalf("prefix %q: %d != %d", fixture.source, got, fixture.want)
		}
		if stream.Index() != index {
			t.Fatal("primary prefix scan consumed input")
		}
	}
	for _, depth := range []int{511, 512, 513} {
		stream := predictionTokens("a" + strings.Repeat("<a", depth) + ";")
		want := 5
		if depth > declarationPrefixDepth {
			want = 0
		}
		if nonTypePrimaryPrefix(stream) != want {
			t.Fatalf("primary depth budget %d", depth)
		}
	}
	for _, count := range []int{4092, 4093, 4096} {
		stream := predictionTokens("a<" + strings.Repeat("/*c*/", count) + "x;")
		want := 0
		if count == 4092 {
			want = 5
		}
		if got := nonTypePrimaryPrefix(stream); got != want || len(stream.GetAllTokens()) > declarationPrefixTokens+1 {
			t.Fatalf("primary raw-token budget %d: %d", count, got)
		}
	}
	stream := predictionTokens("a<b;")
	if nonTypePrimaryPrefix(otherPredictionStream{stream}) != 0 {
		t.Fatal("non-common stream must use ATN")
	}
	uninitialized := antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream("a<b;")), 0)
	if nonTypePrimaryPrefix(uninitialized) != 0 {
		t.Fatal("uninitialized stream must use ATN")
	}
	hidden := antlr.NewCommonTokenStream(NewJavaLexer(antlr.NewInputStream(" /*c*/ a<b;")), antlr.TokenHiddenChannel)
	hidden.LA(1)
	if nonTypePrimaryPrefix(hidden) != 0 {
		t.Fatal("hidden-channel stream must use ATN")
	}
}

func TestPredictionPrimaryRelationalBoundaries(t *testing.T) {
	reference := newPredictionAutomata()
	for _, n := range []int{32, 128, 511, 512, 513, 1024} {
		for _, operator := range []string{"<", "<<", "<a>"} {
			source := methodSource("return a" + strings.Repeat(operator+"a", n) + ";")
			checkPrediction(t, source, reference, true)
		}
	}
}

func TestPredictionTypeClassLiteralFallback(t *testing.T) {
	reference := newPredictionAutomata()
	for _, typeName := range []string{"T<X>", "T<X>[]", "T<X>.U<Y>", "T<@A X>", "T<X> @A []", "T<X> a.@A []", "T<X>[] a.b.@A []", "int[]"} {
		for _, context := range []string{"return %s.class;", "Object x=%s.class;", "f(%s.class);"} {
			checkPrediction(t, methodSource(fmt.Sprintf(context, typeName)), reference, true)
		}
	}
}

func TestPredictionClassLiteralSLL(t *testing.T) {
	sources := []string{
		"@A(value={a.b.T.class,T.class}) class C {}",
		"class C { @A(T.class) T m(){return T.class;} }",
		"package p; import q.T; class C { Class<?> k=T.class; }",
	}
	for _, expr := range []string{"T.class", "a.b.T.class", "T<X>.U<Y>.class", "T<X>[][].class", strings.Repeat("T<", 32) + "X" + strings.Repeat(">", 32) + ".class"} {
		sources = append(sources, methodSource("return "+expr+";"))
	}
	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			original := parsePrediction(source, false, nil)
			p := NewJavaParser(predictionTokens(source))
			newPredictionAutomata().apply(p)
			p.RemoveErrorListeners()
			p.SetErrorHandler(antlr4util.NewBailErrorStrategy())
			p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("class literal incorrectly cancelled the SLL pass: %T", r)
				}
			}()
			tree := p.CompilationUnit()
			if original.err != nil || p.GetTokenStream().LA(1) != antlr.TokenEOF || predictionSnapshot(tree) != predictionSnapshot(original.tree) {
				t.Fatal("class literal changed original AST or left input")
			}
		})
	}
}

func FuzzPredictionStructure(f *testing.F) {
	for production := 0; production < 16; production++ {
		f.Add([]byte{byte(production % 8), 0, byte(production), 2, 0, 6, 1, 4, 5, 3}, uint16(0), uint8(0))
	}
	reference := newPredictionAutomata()
	f.Fuzz(func(t *testing.T, data []byte, position uint16, action uint8) {
		if len(data) > 128 {
			return
		}
		source := predictionStructureSource(data)
		if action&1 != 0 {
			source = predictionMutation(source, int(position), int(action>>1))
		}
		checkPrediction(t, source, reference, false)
	})
}

func BenchmarkJavaStructureGeneration(b *testing.B) {
	inputs := make([][]byte, 16)
	bytes := 0
	for production := range inputs {
		data := []byte{byte(production % 8), byte(production % 4), byte(production)}
		for i := 0; i < 48; i++ {
			data = append(data, byte(production*17+i*31))
		}
		inputs[production] = data
		bytes += len(predictionStructureSource(data))
	}
	b.SetBytes(int64(bytes / len(inputs)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if predictionStructureSource(inputs[i%len(inputs)]) == "" {
			b.Fatal("empty generated source")
		}
	}
}
