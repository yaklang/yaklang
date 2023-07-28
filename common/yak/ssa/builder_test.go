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
					t.Fatalf("fatal basic block %s[%d] error pointer to function", b.Comment, i)
				}

				// CFG check
				for _, succ := range b.Succs {
					if !slices.Contains(succ.Preds, b) {
						t.Fatalf("fatal block success %s not't have it %s in pred", succ.Comment, b.Comment)
					}
				}

				for _, pred := range b.Preds {
					if !slices.Contains(pred.Succs, b) {
						t.Fatalf("fatal block pred %s not't have it %s in succs", pred.Comment, b.Comment)
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
		`
		ir := `
yak-main
entry0:
	%0 = 42 add 42
	%1 = %0 add 23
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
		`
		ir := `
yak-main
entry0:
	%0 = 42 add 42
	%1 = %0 add 33
	%2 = %1 add 23
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		CompareYakMain(t, prog, ir)
	})
}

func TestIfStmt(t *testing.T) {
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
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		ir := `
yak-main
entry0:
	%0 = 5 gt 2
	%1 = If [%0] true -> b2, false -> b3
done1: <- b2 b3
	%2 = 1 add 2
b2: <- entry0
	%3 = 6 add 4
	%4 = jump -> done1
b3: <- entry0
	%5 = 7 add 3
	%6 = jump -> done1
		`
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
	%1 = If [%0] true -> b2, false -> b3
done1: <- b2 b3
	%2 = 1 add 2
b2: <- entry0
	%3 = 5 add 6
	%4 = jump -> done1
b3: <- entry0
	%5 = 5 add 7
	%6 = jump -> done1
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
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		ir := `
yak-main
entry0:
        %0 = 5 gt 2
        %1 = If [%0] true -> b2, false -> b3
done1: <- b2 b3
        %3 = phi [%4, b2] [%6, b3]
        %2 = %3 add 3
b2: <- entry0
        %4 = 5 add 6
        %5 = jump -> done1
b3: <- entry0
        %6 = 5 add 7
        %7 = jump -> done1
		`
		CompareYakMain(t, prog, ir)
	})

}
