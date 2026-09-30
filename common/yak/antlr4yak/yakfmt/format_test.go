package yakfmt

import (
	"strconv"
	"strings"
	"testing"
)

func TestFormatGolden(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"function", `func(a){if (xx){return xxx}}`, "func(a) {\n    if (xx) {\n        return xxx\n    }\n}\n"},
		{"assignment", `var a,b=1,2;a+= -1;b++;a[0]=b.$x`, "var a, b = 1, 2\na += -1\nb++\na[0] = b.$x\n"},
		{"types", `f=func(a map[string]interface{},b []int) (string,error){return nil,nil};x=make(chan int,2);y=[]int{1,2}`, "f = func(a map[string]interface{}, b []int) (string, error) {\n    return nil, nil\n}\nx = make(chan int, 2)\ny = []int{1, 2}\n"},
		{"if init", `if x:=f();x>0{return x}elif x<0{return -x}else if (y){return 0}else{return 1}`, "if x := f(); x > 0 {\n    return x\n} elif x < 0 {\n    return -x\n} else if (y) {\n    return 0\n} else {\n    return 1\n}\n"},
		{"for", `for i=0;i<5;i++{if i==2{continue};break};for k,v:=range [1,2]{println(k,v)};for v in [3]{println(v)}`, "for i = 0; i < 5; i++ {\n    if i == 2 {\n        continue\n    }\n    break\n}\nfor k, v := range [1, 2] {\n    println(k, v)\n}\nfor v in [3] {\n    println(v)\n}\n"},
		{"switch", `switch x{case 1,2:a++;fallthrough;case 3:break;default:a--}`, "switch x {\ncase 1, 2:\n    a++\n    fallthrough\ncase 3:\n    break\ndefault:\n    a--\n}\n"},
		{"select", `select{case ch<-1:println(1);default:break;case v,ok:= <-ch:println(v,ok)}`, "select {\ncase ch <- 1:\n    println(1)\ndefault:\n    break\ncase v, ok := <-ch:\n    println(v, ok)\n}\n"},
		{"try", `try{panic("a")}catch e{println(e)}finally{defer recover()}`, "try {\n    panic(\"a\")\n} catch e {\n    println(e)\n} finally {\n    defer recover()\n}\n"},
		{"closures", `go fn{f()};defer func(){f()}();x=(a,b)=>a+b;y=v=>{return v}`, "go fn {\n    f()\n}\ndefer func() {\n    f()\n}()\nx = (a, b) => a + b\ny = v => {\n    return v\n}\n"},
		{"operators", `a=- -b+ + +c;assert a not in b, true?1:2;x=a[1:2:3];y=a[:];z=f(a...)~`, "a = - -b + + +c\nassert a not in b, true ? 1 : 2\nx = a[1:2:3]\ny = a[:]\nz = f(a...)~\n"},
		{"comments", "// hello\na=1// tail\n\n\n# before\nb=2 /* end */", "// hello\na = 1 // tail\n\n# before\nb = 2 /* end */\n"},
		{"collections", "x={\"a\":1,\n\"b\":2};y=[\n1,2\n]", "x = {\n    \"a\": 1,\n    \"b\": 2,\n}\ny = [\n    1,\n    2,\n]\n"},
		{"templates", "x=f\"hi ${a+1}, ${f(2)}\"; y=f`line\n${true?1:2}`", "x = f\"hi ${a + 1}, ${f(2)}\"\ny = f`line\n${true ? 1 : 2}`\n"},
		{"heredoc", "x=<<<TXT\r\n  原文 \r\nTXT\ny=1", "x = <<<TXT\r\n  原文 \r\nTXT\ny = 1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			again, err := Format(got)
			if err != nil {
				t.Fatalf("output invalid: %v\n%s", err, got)
			}
			if got != again {
				t.Fatalf("not idempotent:\n%s\nsecond:\n%s", got, again)
			}
		})
	}
}
func TestFormatInvalid(t *testing.T) {
	for _, s := range []string{"a =", "dump(123)))", "if {", "@", `x="unterminated`, "select { case : }"} {
		t.Run(s, func(t *testing.T) {
			got, err := Format(s)
			if err == nil || got != "" {
				t.Fatalf("got %q, %v", got, err)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Fatalf("missing source location: %v", err)
			}
		})
	}
	for _, s := range []string{"", " \t\r\n"} {
		got, err := Format(s)
		if got != "" || err != nil {
			t.Fatal(got, err)
		}
	}
}
func BenchmarkFormat(b *testing.B) {
	for _, size := range []int{1, 100, 1000} {
		source := strings.Repeat("f = func(a) { if (a > 0) { return a + 1 }; return 0 }\n", size)
		b.Run(fmtSize(size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for i := 0; i < b.N; i++ {
				if _, err := Format(source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func fmtSize(n int) string {
	if n == 1 {
		return "small"
	}
	if n == 100 {
		return "5KB"
	}
	return "50KB"
}

func TestFormatWidthAndComments(t *testing.T) {
	cases := []string{
		"a=[" + strings.Repeat("1,", 40) + "2]",
		"a=f(" + strings.Repeat("x,", 40) + "y)",
		"a={" + strings.Repeat("1:2,", 20) + "3:4}",
		"a=make(\nchan int,\n1,2)",
		"func() (\n// types\nint,\nerror\n) {}",
		"func(a /* first */,\n// next\nb) {\n// body  \nreturn a+b // last  \n}",
		"f(\"" + strings.Repeat("x", 120) + "\")",
		"a = [ /* empty */ ]\nb = { /* empty */ }",
		"a=[\n]\nfunc() {}",
		"a=[\n[1,2],\n{3:4}\n]",
		"a=1// trailing spaces  ",
		"func(){// hello\nx=1}",
		"a=1 /* newline\ncomment */\nb=2",
		"for(i=0;i<1;i++){}\nselect {}",
		"select\n{}", // contextual identifier followed by a separate block
	}
	for i, s := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) { assertRoundTrip(t, s, nil) })
	}
}

// Lexer modes reuse punctuation as template character tokens. Statement and
// width handling must never mistake their text for a default-mode separator.
func TestFormatTemplateTokenIsolation(t *testing.T) {
	for _, quote := range []string{"'", "\"", "`"} {
		for _, payload := range []string{";", "; ${x} ;", "{}[](),:?+-*/<>=!&|%^~", strings.Repeat(";", 120)} {
			source := "f(f" + quote + payload + quote + ",2)"
			assertRoundTrip(t, source, nil)
			got, err := Format(source)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) > lineWidth && !strings.HasPrefix(got, "f(\n    f") {
				t.Fatalf("long template payload did not expand argument group: %q", got)
			}
		}
	}
}
