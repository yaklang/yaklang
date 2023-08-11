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

					if node, ok := inst.(Node); ok {
						// value-user check
						if value, ok := inst.(Value); ok {
							if slices.Contains(node.GetValues(), value) {
								t.Fatalf("fatal inst %s has this self", inst)
							}
							for _, user := range node.GetUsers() {
								if !slices.Contains(user.GetValues(), value) {
									t.Fatalf("fatal user %s not't have it %s in value", user, inst)
								}
							}
						}
						if user, ok := inst.(User); ok {
							for _, value := range node.GetValues() {
								if !slices.Contains(value.GetUsers(), user) {
									t.Fatalf("fatal value %s not't have it %s in user", value, inst)
								}
							}
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
a = c * 23 
d = a + 11
d = a >> 11
		`
		ir := `
yak-main
entry0:
	<int64> t0 = <int64> 42 add <int64> 42
	<int64> t1 = <int64> t0 add <int64> 33
	<int64> t2 = <int64> t1 mul <int64> 23
	<int64> t3 = <int64> t2 add <int64> 11
	<int64> t4 = <int64> t2 shr <int64> 11
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
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
	<[]int64> t0 = Interface []int64 [<int64> 1, <int64> 1]
	<[]int64> t1 = Interface []int64 [<int64> 0, <int64> 0]
	<int64> t2 = <[]int64> t0 field[<int64> 1]
	update [<int64> t2] = <int64> 1
	<int64> t4 = <int64> t2 add <int64> 2
	<int64> t5 = <int64> t2 add <int64> 2
	<int64> t6 = <int64> t2 add <int64> 1
	update [<int64> t2] = <int64> t6
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
	<bool> t0 = <int64> 5 lt <int64> 2
	If [<bool> t0] true -> if.true2, false -> if.done1
if.done1: <- if.true2 entry0
	jump -> b3
if.true2: <- entry0
	<int64> t3 = <int64> 5 add <int64> 6
	jump -> if.done1
b3: <- if.done1
	<int64> t5 = <int64> 1 add <int64> 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elseif2", func(t *testing.T) {
		src := `
a = 5
d = 1
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
	<bool> t0 = <int64> 5 lt <int64> 2
	If [<bool> t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	<int64> t3 = phi [<int64> 1, if.true2] [<int64> 1, if.true4] [<int64> t12, if.true6] [<int64> t16, if.true8] [<int64> t20, if.true10] [<int64> t22, if.false11]
	jump -> b12
if.true2: <- entry0
	<int64> t4 = <int64> 5 add <int64> 6
	jump -> if.done1
if.elif3: <- entry0
	<bool> t6 = <int64> 5 lt <int64> 4
	If [<bool> t6] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	<int64> t8 = <int64> 5 add <int64> 9
	jump -> if.done1
if.elif5: <- if.elif3
	<bool> t10 = <int64> 5 lt <int64> 6
	If [<bool> t10] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	<int64> t12 = <int64> 5 add <int64> 5
	jump -> if.done1
if.elif7: <- if.elif5
	<bool> t14 = <int64> 5 lt <int64> 10
	If [<bool> t14] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	<int64> t16 = <int64> 5 add <int64> 20
	jump -> if.done1
if.elif9: <- if.elif7
	<bool> t18 = <int64> 5 lt <int64> 20
	If [<bool> t18] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	<int64> t20 = <int64> 5 add <int64> 30
	jump -> if.done1
if.false11: <- if.elif9
	<int64> t22 = <int64> 5 add <int64> 40
	jump -> if.done1
b12: <- if.done1
	<int64> t24 = <int64> 1 add <int64> 2
		`
		prog := parseSSA(src)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakMain(t, prog, ir)
	})

	t.Run("Ifelse_elif2", func(t *testing.T) {
		src := `
a = 5
e = 1
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
e = e + 2
		`
		ir := `
yak-main
entry0:
	<bool> t0 = <int64> 5 lt <int64> 2
	If [<bool> t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	<int64> t3 = phi [<int64> 1, if.true2] [<int64> t8, if.true4] [<int64> t12, if.true6] [<int64> t16, if.true8] [<int64> t20, if.true10] [<int64> 1, if.false11]
	jump -> b12
if.true2: <- entry0
	<int64> t4 = <int64> 5 add <int64> 6
	jump -> if.done1
if.elif3: <- entry0
	<bool> t6 = <int64> 5 lt <int64> 4
	If [<bool> t6] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	<int64> t8 = <int64> 5 add <int64> 9
	jump -> if.done1
if.elif5: <- if.elif3
	<bool> t10 = <int64> 5 lt <int64> 6
	If [<bool> t10] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	<int64> t12 = <int64> 5 add <int64> 10
	jump -> if.done1
if.elif7: <- if.elif5
	<bool> t14 = <int64> 5 lt <int64> 10
	If [<bool> t14] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	<int64> t16 = <int64> 5 add <int64> 20
	jump -> if.done1
if.elif9: <- if.elif7
	<bool> t18 = <int64> 5 lt <int64> 20
	If [<bool> t18] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	<int64> t20 = <int64> 5 add <int64> 30
	jump -> if.done1
if.false11: <- if.elif9
	<int64> t22 = <int64> 5 add <int64> 7
	jump -> if.done1
b12: <- if.done1
	<int64> t24 = <int64> t3 add <int64> 2
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
	a *= 2
}
b = a + 1
		`
		ir := `
yak-main
entry0:
	jump -> loop.header1
loop.header1: <- entry0 loop.latch4
	<int64> t3 = phi [<int64> 0, entry0] [<int64> t10, loop.latch4]
	<int64> t4 = phi [<int64> 0, entry0] [<int64> t8, loop.latch4]
	<bool> t1 = <int64> t4 lt <int64> 10
	If [<bool> t1] true -> loop.body2, false -> loop.exit3
loop.body2: <- loop.header1
	<bool> t5 = <int64> t3 gt-eq <int64> 3
	If [<bool> t5] true -> if.true6, false -> if.done5
loop.exit3: <- loop.header1 if.true8
	jump -> b11
loop.latch4: <- b9 b10
	<int64> t10 = phi [<int64> t3, b9] [<int64> t17, b10]
	<int64> t8 = <int64> t4 add <int64> 1
	jump -> loop.header1
if.done5: <- loop.body2
	jump -> b10
if.true6: <- loop.body2
	<bool> t12 = <int64> t3 eq <int64> 3
	If [<bool> t12] true -> if.true8, false -> if.done7
if.done7: <- if.true6
	jump -> b9
if.true8: <- if.true6
	jump -> loop.exit3
b9: <- if.done7
	jump -> loop.latch4
b10: <- if.done5
	<int64> t17 = <int64> t3 mul <int64> 2
	jump -> loop.latch4
b11: <- loop.exit3
	<int64> t19 = <int64> t3 add <int64> 1
		`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		CompareYakMain(t, prog, ir)
	})

}

func TestSwitch(t *testing.T) {
	t.Run("switch_simple", func(t *testing.T) {
		code := `
a = 2
switch a {
case 1, 2:
	fallthrough
case 3:
case 4:
	fallthrough
default:
}
	`
		ir := `
yak-main 
entry0:
	switch <int64> 2 default:[switch.default2] {<int64> 1:switch.handler3, <int64> 2:switch.handler3, <int64> 3:switch.handler4, <int64> 4:switch.handler5}
switch.done1: <- switch.handler4 switch.default2
                jump -> b6
switch.default2: <- entry0 switch.handler5
                jump -> switch.done1
switch.handler3: <- entry0
                jump -> switch.handler4
switch.handler4: <- entry0 switch.handler3
                jump -> switch.done1
switch.handler5: <- entry0
                jump -> switch.default2
b6: <- switch.done1
	`
		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
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
	return c
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
		<int64> t0 = call <> yak-main$1 (<int64> 1) [<int64> 11]
		<int64> t1 = call <> yak-main$1 (<int64> 2) [<int64> 11]
		<int64> t2 = call <> yak-main$1 (<int64> 3) [<int64> 22]
		<int64> t3 = <int64> 22 add <int64> t2
		If [<int64> t2] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3 
		<int64> t6 = phi [<int64> 12, if.true2] [<int64> 13, if.false3]
        jump -> b4
if.true2: <- entry0 
        jump -> if.done1
if.false3: <- entry0 
        jump -> if.done1
b4: <- if.done1
		<int64> t9 = <int64> t6 add <int64> t2
		<int64> t10 = yak-main-symbol field[<string> cadd]
		<untyped nil> t11 = call <> yak-main$2 () [<int64> t10]
		<int64> t12 = <int64> t10 add <int64> 1
		<int64> t13 = yak-main-symbol field[<string> ca]
		<int64> t14 = call <> yak-main$3 (<int64> 1, <int64> 2, <int64> 3) [<int64> t13, <> yak-main$1, <int64> 11]
		<int64> t15 = <int64> t14 add <int64> 13
		<int64> t16 = call <> yak-main$4 (<int64> 1, <int64> 2, <int64> 3) [<> yak-main$1, <int64> 11]
		<int64> t17 = <int64> t16 add <int64> 13
			`,
			`
yak-main$1 <int64> arg1
parent: yak-main
pos:   4:4   -  14:0  : (arg1)=>{
freeValue: <int64> ca
return: <int64> t9
entry0:
        <bool> t0 = <int64> 1 gt <int64> 2
        If [<bool> t0] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3
        <int64> t3 = phi [<int64> t4, if.true2] [<int64> t6, if.false3]
        jump -> b4
if.true2: <- entry0
        <int64> t4 = <int64> arg1 add <int64> 2
        jump -> if.done1
if.false3: <- entry0
        <int64> t6 = <int64> ca add <int64> 2
        jump -> if.done1
b4: <- if.done1
        <int64> t8 = <int64> t3 add <int64> 1
        ret <int64> t8
			`,
			`
yak-main$2
parent: yak-main
pos:  37:6   -  37:20 : ()=>{cadd++}
freeValue: <int64> t0
entry0:
        <int64> t1 = <int64> t0 add <int64> 1
        update [<int64> t0] = <int64> t1
			`,
			`
yak-main$3 <> pc1, <int64> pc2, <int64> pc3
parent: yak-main
pos:  43:4   -  50:0  : fn(pc1,pc2,pc3){
freeValue: <int64> t0, <> a, <> va
return: <int64> t7
entry0:
        update [<int64> t0] = <int64> 55
        <int64> t2 = call <> a (<> va) []
        <int64> t3 = <int64> 13 add <int64> t2
        <int64> t4 = <int64> pc2 mul <int64> pc3
        <int64> t5 = <int64> t3 add <int64> t4
        <int64> t6 = <int64> t5 add <int64> t0
        ret <int64> t6
			`,
			`
yak-main$4 <> pc1, <int64> pc2, <int64> pc3
parent: yak-main
pos:  53:4   -  58:0  : fn(pc1,pc2,pc3){
freeValue: <> a, <> va
return: <int64> t5
entry0:
        <int64> t0 = call <> a (<> va) []
        <int64> t1 = <int64> 13 add <int64> t0
        <int64> t2 = <int64> pc2 mul <int64> pc3
        <int64> t3 = <int64> t1 add <int64> t2
        <int64> t4 = <int64> t3 add <int64> 55
        ret <int64> t4
			`,
		}

		prog := parseSSA(code)
		CheckProgram(t, prog)
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
	<> t0 = call <> yak-main$1 (<int64> 1) []
	<> t1 = call <> yak-main$3 (<int64> 2) [<int64> 12]
	<> t2 = call <> t0 () []
	<> t3 = call <> t1 () [<int64> 12]
	<> t4 = <> t2 add <> t3
	<> t5 = call <> t1 () [<int64> 13]
	<> t6 = call <> yak-main$5 () [<> t1]
`,
			`
yak-main$1 <> a
parent: yak-main
pos:   2:6   -   7:0  : (a)=>{
return: <> t0
entry0:
	ret <> yak-main$1$2
`,
			`
yak-main$1$2
parent: yak-main$1
pos:   4:8   -   6:1  : ()=>{
freeValue: <> a
return: <> t0
entry0:
	ret <> a
`,
			`
yak-main$3 <> a
parent: yak-main
pos:  11:8   -  16:0  : (a)=>{
freeValue: <> c
return: <> t0
entry0:
	ret <> yak-main$3$4
`,
			`
yak-main$3$4
parent: yak-main$3
pos:  13:8   -  15:1  : ()=>{
freeValue: <> a, <> c
return: <> t1
entry0:
	<> t0 = <> a add <> c
	ret <> t0
`,
			`
yak-main$5 <> b
parent: yak-main
pos:  30:7   -  33:0  : (b)=>{
freeValue: <> f1
return: <> t2
entry0:
	<> t0 = call <> f1 () []
	<> t1 = <> b add <> t0
	ret <> t1
`,
		}
		prog := parseSSA(code)
		CheckProgram(t, prog)
		// showProg(prog)
		CompareYakFunc(t, prog, ir)
	})

}
