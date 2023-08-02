package ssa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"golang.org/x/exp/slices"
)

func parseSSA(src string) *Program {
	inputStream := antlr.NewInputStream(src)
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)
	prog := NewProgram(p)
	prog.Build()
	return prog
}

// check block-graph and value-user chain
func CheckProgram(t *testing.T, prog *Program) {
	for i, pkg := range prog.Packages {
		if pkg.Prog != prog {
			t.Fatalf("fatal pkg %s[%d] error pointer to programe", pkg.name, i)
		}
		for i, f := range pkg.funcs {
			if f.Package != pkg {
				t.Fatalf("fatal function %s[%d] error pointer to package", f.name, i)
			}

			parent := f.parent
			if parent != nil {
				if !slices.Contains(parent.AnonFuncs, f) {
					t.Fatalf("fatal function parent %s not't have it %s", parent.name, f.name)
				}
			}

			for i, b := range f.Blocks {
				if b.Parent != f {
					t.Fatalf("fatal basic block %s[%d] error pointer to function", b.Name, i)
				}

				// CFG check
				for _, succ := range b.Succs {
					if !slices.Contains(succ.Preds, b) {
						t.Fatalf("fatal block success %s not't have it %s in pred", succ.Name, b.Name)
					}
				}

				for _, pred := range b.Preds {
					if !slices.Contains(pred.Succs, b) {
						t.Fatalf("fatal block pred %s not't have it %s in succs", pred.Name, b.Name)
					}
				}

				for i, inst := range b.Instrs {
					if inst.GetBlock() != b {
						t.Fatalf("fatal instruction %s[%d] error pointer to block", inst, i)
					}
					if inst.GetParent() != f {
						t.Fatalf("fatal instruction %s[%d] error pointer to function", inst, i)
					}

					// value-user check
					for _, value := range inst.GetValue() {
						if !slices.Contains(value.GetUser(), inst.(User)) {
							t.Fatalf("fatal value %s not't have it %s in user", value, inst)
						}
					}
					for _, user := range inst.GetUser() {
						if !slices.Contains(user.GetValue(), inst.(Value)) {
							t.Fatalf("fatal user %s not't have it %s in value", user, inst)
						}
					}
				}

			}
		}

	}

}

func showProg(prog *Program) {
	for _, pkg := range prog.Packages {
		for _, f := range pkg.funcs {
			fmt.Printf("%s\n", f)
		}
	}
}

type TestProgram struct {
	pkg []TestPackage
}

type TestPackage struct {
	funs map[string]string
}

func CompareIR(t *testing.T, got, want string) {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	var cleanGot, cleanWant []string
	for _, line := range gotLines {
		line = strings.TrimLeft(line, " \t\r")
		line = strings.TrimRight(line, " \t\r")
		line = strings.ReplaceAll(line, " ", "")
		line = strings.ReplaceAll(line, "\t", "")
		if line != "" {
			cleanGot = append(cleanGot, line)
		}
	}
	for _, line := range wantLines {
		line = strings.TrimLeft(line, " \t\r")
		line = strings.TrimRight(line, " \t\r")
		line = strings.ReplaceAll(line, " ", "")
		line = strings.ReplaceAll(line, "\t", "")
		if line != "" {
			cleanWant = append(cleanWant, line)
		}
	}
	if len(cleanGot) != len(cleanWant) {
		t.Fatalf("IR comparison error: got %d lines, want %d lines", len(cleanGot), len(cleanWant))
	}
	for i := range cleanGot {
		if cleanGot[i] != cleanWant[i] {
			t.Fatalf("IR comparison error: line %d\ngot:\n%s\nwant:\n%s", i+1, cleanGot[i], cleanWant[i])
		}
	}
}

func Compare(t *testing.T, prog *Program, want *TestProgram) {
	if len(prog.Packages) != len(want.pkg) {
		t.Fatalf("program package size erro: %d(want) vs %d(got)", len(prog.Packages), len(want.pkg))
	}
	for i := range prog.Packages {
		pkg := prog.Packages[i]
		want := want.pkg[i]
		if len(pkg.funcs) != len(want.funs) {
			t.Fatalf("package's [%s] function size erro: %d(want) vs %d(got)", pkg.name, len(pkg.funcs), len(want.funs))
		}
		for i := range pkg.funcs {
			f := pkg.funcs[i]
			want, ok := want.funs[f.name]
			if !ok {
				t.Fatalf("con't get this function in want: %s", f.name)
			}
			got := f.String()
			CompareIR(t, got, want)

		}
	}

}

func CompareYakMain(t *testing.T, prog *Program, ir string) {
	want := &TestProgram{
		[]TestPackage{
			{
				funs: map[string]string{
					"yak-main": ir,
				},
			},
		},
	}
	Compare(t, prog, want)
}

func CompareYakFunc(t *testing.T, prog *Program, ir []string) {
	funs := make(map[string]string)
	for _, ir := range ir {
		irs := strings.Split(ir, "\n")
		// set
		for _, line := range irs {
			if strings.TrimSpace(line) != "" {
				words := strings.Split(line, " ")
				funs[words[0]] = ir
				break
			}
		}
	}

	want := &TestProgram{
		[]TestPackage{
			{
				funs: funs,
			},
		},
	}
	Compare(t, prog, want)

}

func TestAssignInBasicBlock(t *testing.T) {
	t.Run("Assign_InChained_OnBlock", func(t *testing.T) {
		src := `
a = 42 
b = a 
c = a + b + 33
a = c + 23 
d = a + 11
		`
		ir := `
yak-main
entry0:
	%0 = 42 add 42
	%1 = %0 add 33
	%2 = %1 add 23
	%3 = %2 add 11
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		CompareYakMain(t, prog, ir)
	})
}

func TestIfStmt(t *testing.T) {
	t.Run("Ifstmt", func(t *testing.T) {
		src := `
a = 5
if a < 2 {
	b = 6
	a = a + b 
}
d = 1 + 2
		`
		ir := `
yak-main
entry0:
	%0 = 5 lt 2
	%1 = If [%0] true -> if.true2, false -> if.done1
if.done1: <- if.true2 entry0
	%2 = jump -> b3
if.true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> if.done1
b3: <- if.done1
	%5 = 1 add 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elseif2", func(t *testing.T) {
		src := `
a = 5
if a < 2 {
	b = 6
	a = a + b 
} else if a < 4 {
	e = a + 9
} else if a < 6{
	d = a + 5
} else if a < 10{
	d = a + 20
} else if a < 20 {
	d = a + 30
} else {
	d = a + 40
}
d = 1 + 2
		`
		ir := `
yak-main
entry0:
	%0 = 5 lt 2
	%1 = If [%0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	%2 = jump -> b12
if.true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> if.done1
if.elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	%7 = 5 add 9
	%8 = jump -> if.done1
if.elif5: <- if.elif3
	%9 = 5 lt 6
	%10 = If [%9] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	%11 = 5 add 5
	%12 = jump -> if.done1
if.elif7: <- if.elif5
	%13 = 5 lt 10
	%14 = If [%13] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	%15 = 5 add 20
	%16 = jump -> if.done1
if.elif9: <- if.elif7
	%17 = 5 lt 20
	%18 = If [%17] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	%19 = 5 add 30
	%20 = jump -> if.done1
if.false11: <- if.elif9
	%21 = 5 add 40
	%22 = jump -> if.done1
b12: <- if.done1
	%23 = 1 add 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elif2", func(t *testing.T) {
		src := `
a = 5
if a < 2 {
	b = 6
	a = a + b 
} elif a < 4 {
	e = a + 9
} elif a < 6 {
	e = a + 10
} elif a < 10 {
	e = a + 20
} elif a < 20 {
	e = a + 30
} else {
	c = 7 
	a = a + c
}
d = 1 + 2
		`
		ir := `
yak-main
entry0:
	%0 = 5 lt 2
	%1 = If [%0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	%2 = jump -> b12
if.true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> if.done1
if.elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	%7 = 5 add 9
	%8 = jump -> if.done1
if.elif5: <- if.elif3
	%9 = 5 lt 6
	%10 = If [%9] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	%11 = 5 add 10
	%12 = jump -> if.done1
if.elif7: <- if.elif5
	%13 = 5 lt 10
	%14 = If [%13] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	%15 = 5 add 20
	%16 = jump -> if.done1
if.elif9: <- if.elif7
	%17 = 5 lt 20
	%18 = If [%17] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	%19 = 5 add 30
	%20 = jump -> if.done1
if.false11: <- if.elif9
	%21 = 5 add 7
	%22 = jump -> if.done1
b12: <- if.done1
	%23 = 1 add 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})
}

// TODO: add loop test for function: `readVariableRecursive`
func TestLoop(t *testing.T) {

	t.Run("looptest_normal", func(t *testing.T) {
		code := `
	a = 10
	b = a + 1
	for i=0;i<b;i++ {
		b = b + i
	}
	c = b + 3
			`
		ir := `
yak-main
entry0:
	%0 = 10 add 1
	%1 = jump -> loop.header1
loop.header1: <- entry0 loop.latch4
	%4 = phi [0, entry0] [%9, loop.latch4]
	%5 = phi [%0, entry0] [%6, loop.latch4]
	%2 = %4 lt %5
	%3 = If [%2] true -> loop.body2, false -> loop.exit3
loop.body2: <- loop.header1
	%6 = %5 add %4
	%7 = jump -> loop.latch4
loop.exit3: <- loop.header1
	%8 = jump -> b5
loop.latch4: <- loop.body2
	%9 = %4 add 1
	%10 = jump -> loop.header1
b5: <- loop.exit3
	%11 = %5 add 3
		`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("looptest_noexpression", func(t *testing.T) {
		code := `
	a = 10
	b = a + 1
	for i=0;;i++ {
		b = b + i
	}
	c = b + 3
			`
		ir := `
yak-main
entry0:
	%0 = 10 add 1
	%1 = jump -> loop.header1
loop.header1: <- entry0 loop.latch4
	%3 = phi [0, entry0] [%8, loop.latch4]
	%4 = phi [%0, entry0] [%5, loop.latch4]
	%2 = If [true] true -> loop.body2, false -> loop.exit3
loop.body2: <- loop.header1
	%5 = %4 add %3
	%6 = jump -> loop.latch4
loop.exit3: <- loop.header1
	%7 = jump -> b5
loop.latch4: <- loop.body2
	%8 = %3 add 1
	%9 = jump -> loop.header1
b5: <- loop.exit3
	%10 = %4 add 3
		`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		showProg(prog)
		CompareYakMain(t, prog, ir)
	})

}

func TestClosure(t *testing.T) {
	t.Run("closure_simple", func(t *testing.T) {
		code := `
a = () => {return 11}
va = a() + 11

func b() {
	return 12
}
vb = b() + 12

c = fn() {
	return 13
}
vc = c() + 13
		`
		ir := []string{
			`
yak-main
entry0:
	%0 = makeClosure Anonymousfunc1
	%1 = call %0
	%2 = %1 add 11
	%3 = makeClosure b
	%4 = call %3
	%5 = %4 add 12
	%6 = makeClosure Anonymousfunc3
	%7 = call %6
	%8 = %7 add 13
			`,

			`
Anonymousfunc1
parent: yak-main
entry0:
	%0 = ret 11,
			`,

			`
b
parent: yak-main
entry0:
	%0 = ret 12,
			`,

			`
Anonymousfunc3
parent: yak-main
entry0:
	%0 = ret 13,
			`,
		}

		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakFunc(t, prog, ir)
	})
}
