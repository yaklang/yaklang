package antlr4yak

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func adversarialSelectCases(count int, body bool) string {
	var source strings.Builder
	source.WriteString("chosen=-1; outer=999; select {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&source, "case outer,ok:=<-channels[%d]:", i)
		if body {
			fmt.Fprintf(&source, " assert ok && outer==%d; chosen=%d; break", i, i)
		}
		source.WriteByte('\n')
	}
	source.WriteString("}; assert outer==999")
	return source.String()
}

func TestSelectAdversarialDispatchBoundaries(t *testing.T) {
	for _, count := range []int{1, 2, 7, 8, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129, 1024} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			engine := New()
			channels := make([]chan int, count)
			engine.ImportLibs(map[string]any{"channels": channels})
			source := adversarialSelectCases(count, true)
			codes, err := engine.Compile(source)
			if err != nil {
				t.Fatal(err)
			}
			for _, chosen := range []int{0, count / 2, count - 1} {
				ch := make(chan int, 1)
				ch <- chosen
				channels[chosen] = ch
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := engine.GetVM().ExecYakCode(ctx, source, codes, yakvm.None)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				got, _ := engine.GetVar("chosen")
				if got != chosen || len(ch) != 0 {
					t.Fatalf("chosen=%v, want=%d, buffered=%d", got, chosen, len(ch))
				}
				channels[chosen] = nil
			}
		})
	}
}

func TestSelectAdversarialEveryDispatchLeaf(t *testing.T) {
	const count = 33
	engine := New()
	channels := make([]chan int, count)
	engine.ImportLibs(map[string]any{"channels": channels})
	source := adversarialSelectCases(count, true)
	codes, err := engine.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	for chosen := 0; chosen < count; chosen++ {
		ch := make(chan int, 1)
		ch <- chosen
		channels[chosen] = ch
		if err := engine.GetVM().ExecYakCode(context.Background(), source, codes, yakvm.None); err != nil {
			t.Fatal(err)
		}
		got, _ := engine.GetVar("chosen")
		if got != chosen {
			t.Fatalf("chosen=%v, want=%d", got, chosen)
		}
		channels[chosen] = nil
	}
}

func TestSelectAdversarialOperandOrderAndLosingLeft(t *testing.T) {
	for _, chosen := range []int{17, 18} {
		var order []string
		var expected []string
		ch := make(chan int, 1)
		if chosen%2 == 0 {
			ch <- chosen
		}
		engine := New()
		engine.ImportLibs(map[string]any{
			"channel": func(i int) chan int {
				order = append(order, fmt.Sprintf("ch%d", i))
				if i == chosen {
					return ch
				}
				return nil
			},
			"value": func(i int) int { order = append(order, fmt.Sprintf("value%d", i)); return i },
			"index": func(i int) int { order = append(order, fmt.Sprintf("left%d", i)); return 0 },
		})
		var source strings.Builder
		source.WriteString("slot=[-1]; chosen=-1; select {")
		for i := 0; i < 32; i++ {
			expected = append(expected, fmt.Sprintf("ch%d", i))
			if i%2 == 0 {
				fmt.Fprintf(&source, "case slot[index(%d)]=<-channel(%d): chosen=%d; break\n", i, i, i)
			} else {
				fmt.Fprintf(&source, "case channel(%d)<-value(%d): chosen=%d; break\n", i, i, i)
				expected = append(expected, fmt.Sprintf("value%d", i))
			}
		}
		source.WriteString("}")
		if err := engine.SafeEvalWithoutCache(context.Background(), source.String()); err != nil {
			t.Fatal(err)
		}
		if chosen%2 == 0 {
			expected = append(expected, fmt.Sprintf("left%d", chosen))
		} else {
			if got := <-ch; got != chosen {
				t.Fatalf("sent=%d want=%d", got, chosen)
			}
		}
		if !reflect.DeepEqual(order, expected) {
			t.Fatalf("operand order: %v, want %v", order, expected)
		}
		if got, _ := engine.GetVar("chosen"); got != chosen {
			t.Fatalf("chosen=%v want=%d", got, chosen)
		}
	}
}

func TestSelectAdversarialLargeControlFlow(t *testing.T) {
	prefix := "select {" + strings.Repeat("case <-disabled: panic(\"disabled\");\n", 32) + "default:"
	for _, source := range []string{
		"f=()=>{" + prefix + "return 7}}; assert f()==7",
		"count=0; for i in 3 {" + prefix + "count++; continue}; panic(\"continue\")}; assert count==3",
		"count=0; for i in 3 {" + prefix + "break}; count++}; assert count==3",
		prefix + "try {break} catch e {panic(e)}}; assert true",
	} {
		engine := New()
		engine.ImportLibs(map[string]any{"disabled": (chan int)(nil)})
		if err := engine.SafeEvalWithoutCache(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSelectAdversarialLargeBytecodeRoundTrip(t *testing.T) {
	source := adversarialSelectCases(33, true)
	engine := New()
	formatted, err := engine.FormattedAndSyntaxChecking(source)
	if err != nil {
		t.Fatal(err)
	}
	again, err := engine.FormattedAndSyntaxChecking(formatted)
	if err != nil || again != formatted {
		t.Fatalf("large formatting unstable: %v", err)
	}
	data, err := engine.Marshal(formatted, nil)
	if err != nil {
		t.Fatal(err)
	}
	other := New()
	channels := make([]chan int, 33)
	channels[31] = make(chan int, 1)
	channels[31] <- 31
	other.ImportLibs(map[string]any{"channels": channels})
	if err := other.SafeExecYakc(context.Background(), data, nil, formatted); err != nil {
		t.Fatal(err)
	}
	if chosen, _ := other.GetVar("chosen"); chosen != 31 {
		t.Fatalf("decoded chosen=%v", chosen)
	}
}

func TestSelectAdversarialInvalidDispatchIndexes(t *testing.T) {
	for _, index := range []any{-1, 16, 17, 1.5} {
		engine := New()
		engine.ImportLibs(map[string]any{"channels": make([]chan int, 16), yakvm.SelectBuiltinName: func(cases []*yakvm.Value) []any { return []any{index, 0, false} }})
		source := adversarialSelectCases(16, true) + "; assert chosen == -1"
		if err := engine.SafeEvalWithoutCache(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSelectAdversarialSyntax(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"parentheses128", "select {case value,ok:= " + strings.Repeat("(", 128) + "<-ch" + strings.Repeat(")", 128) + ": assert value==7 && ok}"},
		{"nested96", strings.Repeat("select {default:", 96) + "assert true" + strings.Repeat("}", 96)},
		{"ternary channel", `select {case v:=<-(true ? ch : disabled): assert v==7}`},
		{"keywords in comments", `select { /* select { case <-bad: } */ case v:=<-ch: assert v==7 // default: select {}
}`},
		{"contextual prefix", strings.Repeat("select=1; f=(select)=>select; assert f(select)==1\n", 128) + "select {case v:=<-ch: assert v==7}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, bailed, err := sllBailParse(tc.source)
			if bailed || err != nil {
				t.Fatalf("SLL bailed=%v error=%v", bailed, err)
			}
			engine := New()
			ch := make(chan int, 1)
			ch <- 7
			engine.ImportLibs(map[string]any{"ch": ch, "disabled": (chan int)(nil)})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := engine.SafeEvalWithoutCache(ctx, tc.source); err != nil {
				t.Fatal(err)
			}
			formatted, err := engine.FormattedAndSyntaxChecking(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			again, err := engine.FormattedAndSyntaxChecking(formatted)
			if err != nil || again != formatted {
				t.Fatalf("unstable formatting: %v", err)
			}
		})
	}
}

func TestSelectAdversarialRejectsLateErrors(t *testing.T) {
	for _, suffix := range []string{"case v:=1:}", "default: default:}", "case <-ch: if true {fallthrough}}", "case <-ch:"} {
		source := "select {" + strings.Repeat("case <-ch:\n", 1024) + suffix
		if _, err := New().Compile(source); err == nil {
			t.Fatalf("accepted invalid suffix %q", suffix)
		}
	}
}

func BenchmarkSelectAdversarialFrontend(b *testing.B) {
	for _, count := range []int{16, 256, 1024, 4096} {
		source := adversarialSelectCases(count, false)
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.Run("parse", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for i := 0; i < b.N; i++ {
					_, err := antlr4util.ParseASTWithSLLFirst(source, yak.NewYaklangLexer, yak.NewYaklangParser, nil, nil, func(p *yak.YaklangParser) yak.IProgramContext { return p.Program() })
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("compile", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for i := 0; i < b.N; i++ {
					c := yakast.NewYakCompiler()
					if !c.Compiler(source) {
						b.Fatal(c.GetErrors())
					}
				}
			})
		})
	}
}

func BenchmarkSelectAdversarialExecution(b *testing.B) {
	for _, count := range []int{2, 16, 64, 256, 1024} {
		for _, last := range []bool{false, true} {
			position := 0
			label := "first"
			if last {
				position = count - 1
				label = "last"
			}
			b.Run(fmt.Sprintf("%d/%s", count, label), func(b *testing.B) {
				engine := New()
				channels := make([]chan int, count)
				ch := make(chan int, 1)
				channels[position] = ch
				engine.ImportLibs(map[string]any{"channels": channels})
				source := adversarialSelectCases(count, false)
				codes, err := engine.Compile(source)
				if err != nil {
					b.Fatal(err)
				}
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					ch <- position
					if err := engine.GetVM().ExecYakCode(ctx, source, codes, yakvm.None); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
