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
			str := f.String()
			fmt.Printf("%s\n", str)
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

func TestAssignInBasicBlock(t *testing.T) {
	t.Run("Assign_InTac_OnBlock", func(t *testing.T) {

		src := `
a = 42 
b = a 
c = a + b 
a = c + 23 
d = a + 1
		`
		ir := `
yak-main
entry0:
	%0 = 42 add 42
	%1 = %0 add 23
	%2 = %1 add 1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		CompareYakMain(t, prog, ir)
	})
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
	%1 = If [%0] true -> true2, false -> done1
done1: <- true2 entry0
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_simple", func(t *testing.T) {
		src := `
a = 5
if a > 2 {
	b = 6 + 4
} else {
	c = 7 + 3
}
d = 1 + 2
		`
		ir := `
yak-main
entry0:
	%0 = 5 gt 2
	%1 = If [%0] true -> true2, false -> false3
done1: <- true2 false3
	%2 = 1 add 2
true2: <- entry0
	%3 = 6 add 4
	%4 = jump -> done1
false3: <- entry0
	%5 = 7 add 3
	%6 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_RecursiveReadValue", func(t *testing.T) {
		src := `
a = 5
if a > 2 {
	b = 6
	a = a + b 
} else {
	c = 7 
	a = a + c
}
d = 1 + 2
		`
		ir := `
yak-main
entry0:
	%0 = 5 gt 2
	%1 = If [%0] true -> true2, false -> false3
done1: <- true2 false3
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
false3: <- entry0
	%5 = 5 add 7
	%6 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elseif1", func(t *testing.T) {
		src := `
a = 5
if a < 2 {
	b = 6
	a = a + b 
} else if a < 4 {
	e = a + 9
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
	%1 = If [%0] true -> true2, false -> elif3
done1: <- true2 true4 false5
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> true4, false -> false5
true4: <- elif3
	%7 = 5 add 9
	%8 = jump -> done1
false5: <- elif3
	%9 = 5 add 7
	%10 = jump -> done1
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
	%1 = If [%0] true -> true2, false -> elif3
done1: <- true2 true4 true6 true8 true10 false11
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> true4, false -> elif5
true4: <- elif3
	%7 = 5 add 9
	%8 = jump -> done1
elif5: <- elif3
	%9 = 5 lt 6
	%10 = If [%9] true -> true6, false -> elif7
true6: <- elif5
	%11 = 5 add 5
	%12 = jump -> done1
elif7: <- elif5
	%13 = 5 lt 10
	%14 = If [%13] true -> true8, false -> elif9
true8: <- elif7
	%15 = 5 add 20
	%16 = jump -> done1
elif9: <- elif7
	%17 = 5 lt 20
	%18 = If [%17] true -> true10, false -> false11
true10: <- elif9
	%19 = 5 add 30
	%20 = jump -> done1
false11: <- elif9
	%21 = 5 add 40
	%22 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elif1", func(t *testing.T) {
		src := `
a = 5
if a < 2 {
	b = 6
	a = a + b 
} elif a < 4 {
	e = a + 9
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
	%1 = If [%0] true -> true2, false -> elif3
done1: <- true2 true4 false5
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> true4, false -> false5
true4: <- elif3
	%7 = 5 add 9
	%8 = jump -> done1
false5: <- elif3
	%9 = 5 add 7
	%10 = jump -> done1
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
	%1 = If [%0] true -> true2, false -> elif3
done1: <- true2 true4 true6 true8 true10 false11
	%2 = 1 add 2
true2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
elif3: <- entry0
	%5 = 5 lt 4
	%6 = If [%5] true -> true4, false -> elif5
true4: <- elif3
	%7 = 5 add 9
	%8 = jump -> done1
elif5: <- elif3
	%9 = 5 lt 6
	%10 = If [%9] true -> true6, false -> elif7
true6: <- elif5
	%11 = 5 add 10
	%12 = jump -> done1
elif7: <- elif5
	%13 = 5 lt 10
	%14 = If [%13] true -> true8, false -> elif9
true8: <- elif7
	%15 = 5 add 20
	%16 = jump -> done1
elif9: <- elif7
	%17 = 5 lt 20
	%18 = If [%17] true -> true10, false -> false11
true10: <- elif9
	%19 = 5 add 30
	%20 = jump -> done1
false11: <- elif9
	%21 = 5 add 7
	%22 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})
}

func TestPhi(t *testing.T) {

	t.Run("Phi_IFelse_merge", func(t *testing.T) {
		src := `
a = 5
if a > 2 {
	b = 6
	a = a + b 
} else {
	c = 7 
	a = a + c
}
d = a + 3
		`
		ir := `
yak-main
entry0:
	%0 = 5 gt 2
	%1 = If [%0] true -> true2, false -> false3
done1: <- true2 false3
	%3 = phi [%4, true2] [%6, false3]
	%2 = %3 add 3
true2: <- entry0
	%4 = 5 add 6
	%5 = jump -> done1
false3: <- entry0
	%6 = 5 add 7
	%7 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	// test trivial phi

	// shoud no phi instruction
	t.Run("Phi_IFelse_notmerge", func(t *testing.T) {
		src := `
a = 5 + 3
if a > 2 {
	b = a + 7 
} else {
	c = a + 6
}
d = a + 3
		`
		ir := `
yak-main
entry0:
	%0 = 5 add 3
	%1 = %0 gt 2
	%2 = If [%1] true -> true2, false -> false3
done1: <- true2 false3
	%3 = %0 add 3
true2: <- entry0
	%4 = %0 add 7
	%5 = jump -> done1
false3: <- entry0
	%6 = %0 add 6
	%7 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})
	t.Run("Phi_IFelse_notmerge2", func(t *testing.T) {
		src := `
a = 5
if a > 2 {
	b = a + 7 
} else {
	c = a + 6
}
d = a + 3
		`
		ir := `
yak-main
entry0:
	%0 = 5 gt 2
	%1 = If [%0] true -> true2, false -> false3
done1: <- true2 false3
	%2 = 5 add 3
true2: <- entry0
	%3 = 5 add 7
	%4 = jump -> done1
false3: <- entry0
	%5 = 5 add 6
	%6 = jump -> done1
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	//TODO: add test for function: `func (phi *Phi) triRemoveTrivialPhi() Value `
}

// TODO: add loop test for function: `readVariableRecursive`
func TestLoop(t *testing.T) {

	t.Run("looptest", func(t *testing.T) {
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
	%8 = %5 add 3
loop.latch4: <- loop.body2
	%9 = %4 add 1
	%10 = jump -> loop.header1
		`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})
}
