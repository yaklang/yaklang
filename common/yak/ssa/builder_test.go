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

					if slices.Contains(inst.GetValues(), inst.(Value)) {
						t.Fatalf("fatal inst %s has this self", inst)
					}
					// value-user check
					for _, value := range inst.GetValues() {
						if !slices.Contains(value.GetUsers(), inst.(User)) {
							t.Fatalf("fatal value %s not't have it %s in user", value, inst)
						}
					}
					for _, user := range inst.GetUsers() {
						if !slices.Contains(user.GetValues(), inst.(Value)) {
							t.Fatalf("fatal user %s not't have it %s in value", user, inst)
						}
					}
				}

			}
		}

	}

}

func showProg(prog *Program) string {
	ret := ""
	for _, pkg := range prog.Packages {
		for _, f := range pkg.funcs {
			ret += f.DisAsm(DisAsmWithoutSource)
		}
	}
	fmt.Println(ret)
	return ret
}

type TestProgram struct {
	pkg []TestPackage
}

type TestPackage struct {
	funs map[string]string
}

func CompareIR(t *testing.T, got, want, fun string) {
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
		t.Fatalf("IR comparison func [%s] error: got %d lines, want %d lines", fun, len(cleanGot), len(cleanWant))
	}
	for i := range cleanGot {
		if cleanGot[i] != cleanWant[i] {
			t.Fatalf("IR comparison func [%s] error: line %d\ngot:\n%s\nwant:\n%s", fun, i+1, cleanGot[i], cleanWant[i])
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
			CompareIR(t, got, want, f.name)
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

func TestBasicBlock(t *testing.T) {
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
	t0 = 42 add 42
	t1 = t0 add 33
	t2 = t1 add 23
	t3 = t2 add 11
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Assign_Make_Slice", func(t *testing.T) {
		src := `
b1 = make([]int, 1)
b2 = make([]int, 0)

b1[1] = 1 
b2 = b1[1]
b  = b1[1] + 2
c  = b2 + 2
b1[1] += 1
		`
		ir := `
yak-main
entry0:
	t0 = Interface []int [1, 1]
	t1 = Interface []int [0, 0]
	t2 = t0 field[1]
	update [t2] = 1
	t4 = t2 add 2
	t5 = t2 add 2
	t6 = t2 add 1
	update [t2] = t6
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
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
	t0 = 5 lt 2
	If [t0] true -> if.true2, false -> if.done1
if.done1: <- if.true2 entry0
	jump -> b3
if.true2: <- entry0
	t3 = 5 add 6
	jump -> if.done1
b3: <- if.done1
	t5 = 1 add 2
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
	t0 = 5 lt 2
	If [t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	t3 = phi [t12, if.true6] [t16, if.true8] [t20, if.true10] [t22, if.false11]
	jump -> b12
if.true2: <- entry0
	t4 = 5 add 6
	jump -> if.done1
if.elif3: <- entry0
	t6 = 5 lt 4
	If [t6] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	t8 = 5 add 9
	jump -> if.done1
if.elif5: <- if.elif3
	t10 = 5 lt 6
	If [t10] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	t12 = 5 add 5
	jump -> if.done1
if.elif7: <- if.elif5
	t14 = 5 lt 10
	If [t14] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	t16 = 5 add 20
	jump -> if.done1
if.elif9: <- if.elif7
	t18 = 5 lt 20
	If [t18] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	t20 = 5 add 30
	jump -> if.done1
if.false11: <- if.elif9
	t22 = 5 add 40
	jump -> if.done1
b12: <- if.done1
	t24 = 1 add 2
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
	t0 = 5 lt 2
	If [t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	jump -> b12
if.true2: <- entry0
	t3 = 5 add 6
	jump -> if.done1
if.elif3: <- entry0
	t5 = 5 lt 4
	If [t5] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	t7 = 5 add 9
	jump -> if.done1
if.elif5: <- if.elif3
	t9 = 5 lt 6
	If [t9] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	t11 = 5 add 10
	jump -> if.done1
if.elif7: <- if.elif5
	t13 = 5 lt 10
	If [t13] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	t15 = 5 add 20
	jump -> if.done1
if.elif9: <- if.elif7
	t17 = 5 lt 20
	If [t17] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	t19 = 5 add 30
	jump -> if.done1
if.false11: <- if.elif9
	t21 = 5 add 7
	jump -> if.done1
b12: <- if.done1
	t23 = 1 add 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})
}

func TestLoop(t *testing.T) {
	// 	t.Run("looptest_range", func(*testing.T) {
	// 		code := `
	// a = 0
	// for  range 10 {
	// 	a += 1
	// }
	// b = a + 2
	// 		`
	// 		prog := parseSSA(code)
	// 		CheckProgram(t, prog)
	// 		showProg(prog)
	// 	})

	t.Run("looptest_breakcontinue", func(t *testing.T) {
		code := `
a = 0 
for i=0; i<10; i++{
	if a >= 3{
		if a == 3{
			break
		}
		continue
	}
	a += 2
}
b = a + 1
		`
		ir := `
yak-main
entry0:
	jump -> loop.header1
loop.header1: <- entry0 loop.latch4
	t3 = phi [0, entry0] [t10, loop.latch4]
	t4 = phi [0, entry0] [t8, loop.latch4]
	t1 = t4 lt 10
	If [t1] true -> loop.body2, false -> loop.exit3
loop.body2: <- loop.header1
	t5 = t3 gt-eq 3
	If [t5] true -> if.true6, false -> if.done5
loop.exit3: <- loop.header1 if.true8
	jump -> b11
loop.latch4: <- b9 b10
	t10 = phi [t3, b9] [t17, b10]
	t8 = t4 add 1
	jump -> loop.header1
if.done5: <- loop.body2
	jump -> b10
if.true6: <- loop.body2
	t12 = t3 eq 3
	If [t12] true -> if.true8, false -> if.done7
if.done7: <- if.true6
	jump -> b9
if.true8: <- if.true6
	jump -> loop.exit3
b9: <- if.done7
	jump -> loop.latch4
b10: <- if.done5
	t17 = t3 add 2
	jump -> loop.latch4
b11: <- loop.exit3
	t19 = t3 add 1
		`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		CompareYakMain(t, prog, ir)
	})

}

func TestClosure(t *testing.T) {
	t.Run("closure_simple", func(t *testing.T) {
		code := `
ca = 11
// freevalue ca
a = (arg1) =>{
	// test cfg in closure function
	b = 1
	if 1 > 2{
		b = arg1 + 2
	}else {
		b = ca + 2
	}
	c = b + 1
}
// call a (1) [11]
b = a(1)
// call a (2) [11]
b = a(2)
ca = 22
// call a (2) [22]
b = a(3)

// "ca" is Const-Value,because closure(a) not modify "ca"
d = ca + b 
if b {
	ca = 12
}else {
	ca = 13 
}
// this is phi-instruction 
// ca = phi(12, 13)
c = ca + b

// "cadd" is field-value, because closure(add) modify "cadd"
cadd = 0
// field[cadd]
add = () => {cadd ++}
// call add() [field_cadd]
add()
e = cadd + 1

va = 11
c = fn(pc1, pc2, pc3) {
	// modify global.ca, this ca is field
	// update [field ca]
	ca = 55
	// call a (va) []
	// xx add field_ca
	return 13 + a(va) + pc2 * pc3 + ca 
}
vc = c(1, 2, 3) + 13

d = fn(pc1, pc2, pc3) {
	// not modify global.ca
	ca := 55
	// call a (va) []
	return 13 + a(va) + pc2 * pc3 + ca
}
vd = d(1, 2, 3) + 13
		`
		ir := []string{
			`
yak-main 
entry0:
        t0 = call yak-main$1 (1) [11]
        t1 = call yak-main$1 (2) [11]
        t2 = call yak-main$1 (3) [22]
        t3 = 22 add t2
        If [t2] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3 
        t6 = phi [12, if.true2] [13, if.false3] 
        jump -> b4
if.true2: <- entry0 
        jump -> if.done1
if.false3: <- entry0 
        jump -> if.done1
b4: <- if.done1
        t9 = t6 add t2
        t10 = yak-main-symbol field[cadd]
        t11 = call yak-main$2 () [t10]
        t12 = t10 add 1
        t13 = yak-main-symbol field[ca]
        t14 = call yak-main$3 (1, 2, 3) [t13, yak-main$1, 11]
        t15 = t14 add 13
        t16 = call yak-main$4 (1, 2, 3) [yak-main$1, 11]
        t17 = t16 add 13
			`,
			`
yak-main$1 arg1
parent: yak-main
pos:   4:4   -  13:0  : (arg1)=>{
freeValue: ca
entry0:
        t0 = 1 gt 2
        If [t0] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3
        t3 = phi [t4, if.true2] [t6, if.false3]
        jump -> b4
if.true2: <- entry0
        t4 = arg1 add 2
        jump -> if.done1
if.false3: <- entry0
        t6 = ca add 2
        jump -> if.done1
b4: <- if.done1
        t8 = t3 add 1
			`,
			`
yak-main$2
parent: yak-main
pos:  36:6   -  36:20 : ()=>{cadd++}
freeValue: t0
entry0:
        t1 = t0 add 1
        update [t0] = t1
			`,
			`
yak-main$3 pc1, pc2, pc3
parent: yak-main
pos:  42:4   -  49:0  : fn(pc1,pc2,pc3){
freeValue: t0, a, va
entry0:
        update [t0] = 55
        t2 = call a (va) []
        t3 = 13 add t2
        t4 = pc2 mul pc3
        t5 = t3 add t4
        t6 = t5 add t0
        ret t6
			`,
			`
yak-main$4 pc1, pc2, pc3
parent: yak-main
pos:  52:4   -  57:0  : fn(pc1,pc2,pc3){
freeValue: a, va
entry0:
        t0 = call a (va) []
        t1 = 13 add t0
        t2 = pc2 mul pc3
        t3 = t1 add t2
        t4 = t3 add 55
        ret t4
			`,
		}

		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakFunc(t, prog, ir)
	})

	t.Run("closure_factor", func(t *testing.T) {
		code := `
set = (a)=>{
	// freevalue a
	return () => {
		return a
	}
}
c = 12

// freevalue c
set2  = (a) =>{
	// freevalue a,c
	return () => {
		return a + c
	}
}
// call set (1) []
f0 = set(1)
// call set2 (2) [12(c)]
f1 = set2(2)
// call f0 () []
// call f1 () [12(c)]
fret = f0() + f1()

c = 13
// call f1 () [13(c)]
f1()

// freevalue: f1
call = (b) => {
	// call f1 () [] // 这里是捕获的,可能会改变，没办法分析
	return b + f1()
}
// call call() [f1(f1)]
call()
		`
		ir := []string{
			`
yak-main 
entry0:
        t0 = call yak-main$1 (1) []
        t1 = call yak-main$3 (2) [12]
        t2 = call t0 () []
        t3 = call t1 () [12]
        t4 = t2 add t3
        t5 = call t1 () [13]
        t6 = call yak-main$5 () [t1]
`,
			`
yak-main$1 a
parent: yak-main
pos:   2:6   -   7:0  : (a)=>{
entry0:
        ret yak-main$1$2
`,
			`
yak-main$1$2
parent: yak-main$1
pos:   4:8   -   6:1  : ()=>{
freeValue: a
entry0:
        ret a
`,
			`
yak-main$3 a
parent: yak-main
pos:  11:8   -  16:0  : (a)=>{
freeValue: c
entry0:
        ret yak-main$3$4
`,
			`
yak-main$3$4
parent: yak-main$3
pos:  13:8   -  15:1  : ()=>{
freeValue: a, c
entry0:
        t0 = a add c
        ret t0
`,
			`
yak-main$5 b
parent: yak-main
pos:  30:7   -  33:0  : (b)=>{
freeValue: f1
entry0:
        t0 = call f1 () [] 
        t1 = b add t0
        ret t1
`,
		}
		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakFunc(t, prog, ir)
	})

}
