package antlr4yak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakfmt"
)

func TestFormatterIndependentOfCompiler(t *testing.T) {
	e := New()
	e.SetStrictMode(true)
	source := `include "missing-formatter-file.yak";f=func(a){return external(a)}`
	first, err := e.FormattedAndSyntaxChecking(source)
	if err != nil {
		t.Fatal(err)
	}
	if e.HaveEvaluatedCode() {
		t.Fatal("format changed engine symbol table")
	}
	second, err := e.FormattedAndSyntaxChecking(first)
	if err != nil || second != first {
		t.Fatal("not stable", err)
	}
	if _, err := e.Compile(source); err == nil {
		t.Fatal("semantic compiler check was bypassed")
	}
	if got, err := e.YakBuiltinfmtWithError("@"); err == nil || got != "" {
		t.Fatal(got, err)
	}
	if got := e.YakBuiltinfmt("@"); got != "@" {
		t.Fatal("builtin must keep invalid source")
	}
	e2 := New()
	if err := e2.SafeEval(context.Background(), `a=1;assert yakfmt("x=1") == "x = 1";assert a==1`); err != nil {
		t.Fatal(err)
	}
}
func TestFormatterExecutionEquivalence(t *testing.T) {
	sources := []string{
		`a,b=1,2;f=func(x){if(x>0){return x+1};return 0};assert f(a)+b==4`,
		`total=0;for i=0;i<5;i++{if i==2{continue};total+=i};assert total==8;for k,v:=range [1,2]{total+=v};assert total==11`,
		`x=1;switch x{case 1:x=2;fallthrough;case 2:x+=1;default:x=0};assert x==3`,
		`ch=make(chan int,1);ch<-7;select{case v,ok:= <-ch:assert ok && v==7;default:assert false};select{case <-ch:assert false;default:assert true}`,
		`f=func(){defer recover();panic("boom")};f();a=1;try{panic(2)}catch e{a=3}finally{a+=1};assert a==4`,
		`f=func(x map[string]interface{}) (int,error){return x["n"],nil};a,b=f({"n":2});assert a==2 && b==nil`,
		"f=func(a){return f\"值 ${a+1}\"};assert f(1)==\"值 2\";x=<<<TAG\r\n  原文 \r\nTAG\nassert x==\"  原文 \"",
	}
	for i, source := range sources {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			formatted, err := yakfmt.Format(source)
			if err != nil {
				t.Fatal(err)
			}
			for _, code := range []string{source, formatted} {
				if err := New().SafeEval(context.Background(), code); err != nil {
					t.Fatalf("%v\n%s", err, code)
				}
			}
		})
	}
}
func TestFormatterConcurrentSameEngine(t *testing.T) {
	e := New()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				got, err := e.FormattedAndSyntaxChecking(`select{case v:= <-ch:println(v);default:println(0)}`)
				if err != nil || got == "" {
					t.Error(got, err)
				}
			}
		}()
	}
	wg.Wait()
	if e.HaveEvaluatedCode() {
		t.Fatal("formatter changed scope")
	}
}

// Large literal extraction uses rune offsets, and must preserve exact payloads
// while avoiding ANTLR's quadratic subtree text concatenation.
func TestCompilerLargeLiteralPayloads(t *testing.T) {
	cases := map[string]struct{ source, want string }{}
	for _, sep := range []string{"\n", "\r\n"} {
		body := strings.Repeat("  原文 {} // ;\t"+sep, 2048) + "末尾"
		name := "heredoc_LF"
		if sep == "\r\n" {
			name = "heredoc_CRLF"
		}
		cases[name] = struct{ source, want string }{"value=<<<TAG" + sep + body + sep + "TAG\nassert value == want", body}
	}
	for _, quote := range []string{"'", "\"", "`"} {
		body := strings.Repeat("原文 ; {} // ", 2048)
		cases["template_"+quote] = struct{ source, want string }{"value=f" + quote + body + "${number} " + body + quote + ";assert value == want", body + "7 " + body}
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			formatted, err := yakfmt.Format(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			for i, source := range []string{tc.source, formatted} {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					e := New()
					e.ImportLibs(map[string]interface{}{"want": tc.want, "number": 7})
					if err := e.SafeEval(context.Background(), source); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}
