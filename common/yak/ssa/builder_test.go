package ssa

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/exp/slices"
)

// check block-graph and value-user chain
func CheckProgram(t *testing.T, prog *Program) {
	// showProg(prog)

	checkInst := func(v Node) {
		if phi, ok := v.(*Phi); ok {
			if !slices.Contains(phi.GetBlock().Phis, phi) {
				t.Fatalf("fatal phi inst %s not't in block %s", phi, phi.GetBlock().Name)
			}
			// phi is ok return
			return
		}
		if inst, ok := v.(Instruction); ok {
			if block := inst.GetBlock(); block != nil {
				// inst must in inst.block
				if !slices.Contains(block.Instrs, inst) {
					t.Fatalf("fatal inst %s not't in block %s", inst, inst.GetBlock().Name)
				}
			} else if inst != inst.GetParent().symbol {
				t.Fatalf("fatal inst %s not't have block ", inst)
			}
		}
	}

	checkValue := func(value Value) {
		if user, ok := value.(User); ok {
			if slices.Contains(user.GetValues(), value) {
				t.Fatalf("fatal inst %s has this self", value)
			}
		}
		for _, user := range value.GetUsers() {
			if !slices.Contains(user.GetValues(), value) {
				t.Fatalf("fatal user %s not't have it %s in value", user, value)
			}
			checkInst(user)
		}
	}
	checkUser := func(user User) {
		if value, ok := user.(Value); ok {
			if slices.Contains(value.GetUsers(), user) {
				t.Fatalf("fatal inst %s has this self", user)
			}
		}

		for _, value := range user.GetValues() {
			if !slices.Contains(value.GetUsers(), user) {
				t.Fatalf("fatal value %s not't have it %s in user", value, user)
			}
			checkInst(value)
		}

	}
	checkNode := func(node Node) {
		// value-user check
		if value, ok := node.(Value); ok {
			checkValue(value)
		}
		if user, ok := node.(User); ok {
			checkUser(user)
		}
	}

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
						checkNode(node)
					}
				}

				for _, phi := range b.Phis {
					if len(phi.Edge) != len(b.Preds) {
						t.Fatalf("fatal Phi-instruction %s edge error", phi)
					}

					for i, e := range phi.Edge {
						if e != nil {
							checkValue(e)
						} else {
							t.Fatalf("fatal phi-instruction[%s] edge[%d] for block[%s] is nil!\n", phi.variable, i, b.Preds[i].Name)
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
			ret += f.DisAsm(DisAsmDefault)
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
// (1) = (n)
a = 1, 2, 3 
// (n) = (1)
b, c, d = a
d = b + c + d
a = 1, "2", true
// (n) = (1)
b, c, d = a
d = "arst" + "a"
var e, f, g 
var e = 1 
var f = e + 2
		`
		ir := `
yak-main
entry0:
	<number> t0 = <number> 42 add <number> 42
	<number> t1 = <number> t0 add <number> 33
	<number> t2 = <number> t1 mul <number> 23
	<number> t3 = <number> t2 add <number> 11
	<number> t4 = <number> t2 shr <number> 11
	<[]number> t5 = Interface []number [<number> 3, <number> 3]
	<number> t6 = <[]number> t5 field[<number> 0]
	update [<number> t6] = <number> 1
	<number> t8 = <[]number> t5 field[<number> 1]
	update [<number> t8] = <number> 2
	<number> t10 = <[]number> t5 field[<number> 2]
	update [<number> t10] = <number> 3
	<number> t12 = <number> t6 add <number> t8
	<number> t13 = <number> t12 add <number> t10
	<struct {number,string,boolean}> t14 = Interface struct {number,string,boolean} [<number> 3, <number> 3]
	<number> t15 = <struct {number,string,boolean}> t14 field[<number> 0]
	update [<number> t15] = <number> 1
	<string> t17 = <struct {number,string,boolean}> t14 field[<number> 1]
	update [<string> t17] = <string> 2
	<boolean> t19 = <struct {number,string,boolean}> t14 field[<number> 2]
	update [<boolean> t19] = <boolean> true
	<string> t21 = <string> arst add <string> a
	<number> t22 = <number> 1 add <number> 2
		`
		prog := ParseSSA(src)
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
	<[]number> t0 = Interface []number [<number> 1, <number> 1]
	<[]number> t1 = Interface []number [<number> 0, <number> 0]
	<number> t2 = <[]number> t0 field[<number> 1]
	update [<number> t2] = <number> 1
	<number> t4 = <number> t2 add <number> 2
	<number> t5 = <number> t2 add <number> 2
	<number> t6 = <number> t2 add <number> 1
	update [<number> t2] = <number> t6
		`
		prog := ParseSSA(src)
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
	<boolean> t0 = <number> 5 lt <number> 2
	If [<boolean> t0] true -> if.true2, false -> if.done1
if.done1: <- if.true2 entry0
	jump -> b3
if.true2: <- entry0
	<number> t2 = <number> 5 add <number> 6
	jump -> if.done1
b3: <- if.done1
	<number> t5 = <number> 1 add <number> 2
		`
		prog := ParseSSA(src)
		CheckProgram(t, prog)
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
	<boolean> t0 = <number> 5 lt <number> 2
	If [<boolean> t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	<number> t24 = phi [<number> 1, if.true2] [<number> 1, if.true4] [<number> t10, if.true6] [<number> t14, if.true8] [<number> t18, if.true10] [<number> t20, if.false11]
	jump -> b12
if.true2: <- entry0
	<number> t2 = <number> 5 add <number> 6
	jump -> if.done1
if.elif3: <- entry0
	<boolean> t4 = <number> 5 lt <number> 4
	If [<boolean> t4] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	<number> t6 = <number> 5 add <number> 9
	jump -> if.done1
if.elif5: <- if.elif3
	<boolean> t8 = <number> 5 lt <number> 6
	If [<boolean> t8] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	<number> t10 = <number> 5 add <number> 5
	jump -> if.done1
if.elif7: <- if.elif5
	<boolean> t12 = <number> 5 lt <number> 10
	If [<boolean> t12] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	<number> t14 = <number> 5 add <number> 20
	jump -> if.done1
if.elif9: <- if.elif7
	<boolean> t16 = <number> 5 lt <number> 20
	If [<boolean> t16] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	<number> t18 = <number> 5 add <number> 30
	jump -> if.done1
if.false11: <- if.elif9
	<number> t20 = <number> 5 add <number> 40
	jump -> if.done1
b12: <- if.done1
	<number> t23 = <number> 1 add <number> 2
		`
		prog := ParseSSA(src)
		CheckProgram(t, prog)
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
	<boolean> t0 = <number> 5 lt <number> 2
	If [<boolean> t0] true -> if.true2, false -> if.elif3
if.done1: <- if.true2 if.true4 if.true6 if.true8 if.true10 if.false11
	<number> t23 = phi [<number> 1, if.true2] [<number> t6, if.true4] [<number> t10, if.true6] [<number> t14, if.true8] [<number> t18, if.true10] [<number> 1, if.false11]
	jump -> b12
if.true2: <- entry0
	<number> t2 = <number> 5 add <number> 6
	jump -> if.done1
if.elif3: <- entry0
	<boolean> t4 = <number> 5 lt <number> 4
	If [<boolean> t4] true -> if.true4, false -> if.elif5
if.true4: <- if.elif3
	<number> t6 = <number> 5 add <number> 9
	jump -> if.done1
if.elif5: <- if.elif3
	<boolean> t8 = <number> 5 lt <number> 6
	If [<boolean> t8] true -> if.true6, false -> if.elif7
if.true6: <- if.elif5
	<number> t10 = <number> 5 add <number> 10
	jump -> if.done1
if.elif7: <- if.elif5
	<boolean> t12 = <number> 5 lt <number> 10
	If [<boolean> t12] true -> if.true8, false -> if.elif9
if.true8: <- if.elif7
	<number> t14 = <number> 5 add <number> 20
	jump -> if.done1
if.elif9: <- if.elif7
	<boolean> t16 = <number> 5 lt <number> 20
	If [<boolean> t16] true -> if.true10, false -> if.false11
if.true10: <- if.elif9
	<number> t18 = <number> 5 add <number> 30
	jump -> if.done1
if.false11: <- if.elif9
	<number> t20 = <number> 5 add <number> 7
	jump -> if.done1
b12: <- if.done1
	<number> t24 = <number> t23 add <number> 2
		`
		prog := ParseSSA(src)
		CheckProgram(t, prog)
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
        <number> t15 = phi [<number> 0, entry0] [<number> t13, loop.latch4]
        <number> t17 = phi [<number> 0, entry0] [<number> t16, loop.latch4]
        <boolean> t0 = <number> t15 lt <number> 10
        If [<boolean> t0] true -> loop.body2, false -> loop.exit3
loop.body2: <- loop.header1
        <boolean> t3 = <number> t17 gt-eq <number> 3
        If [<boolean> t3] true -> if.true6, false -> if.done5
loop.exit3: <- loop.header1 if.true8
        jump -> b11
loop.latch4: <- b9 b10
        <number> t16 = phi [<number> t17, b9] [<number> t11, b10]
        <number> t13 = <number> t15 add <number> 1
        jump -> loop.header1
if.done5: <- loop.body2
        jump -> b10
if.true6: <- loop.body2
        <boolean> t5 = <number> t17 eq <number> 3
        If [<boolean> t5] true -> if.true8, false -> if.done7
if.done7: <- if.true6
        jump -> b9
if.true8: <- if.true6
        jump -> loop.exit3
b9: <- if.done7
        jump -> loop.latch4
b10: <- if.done5
        <number> t11 = <number> t17 mul <number> 2
        jump -> loop.latch4
b11: <- loop.exit3
        <number> t19 = <number> t17 add <number> 1
		`
		prog := ParseSSA(code)
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
	switch <number> 2 default:[switch.default2] {<number> 1:switch.handler3, <number> 2:switch.handler3, <number> 3:switch.handler4, <number> 4:switch.handler5}
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
		prog := ParseSSA(code)
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
        <number> t0 = call <> yak-main$1 (<number> 1) [<number> 11]
        <number> t1 = call <> yak-main$1 (<number> 2) [<number> 11]
        <number> t2 = call <> yak-main$1 (<number> 3) [<number> 22]
        <number> t3 = <number> 22 add <number> t2
        If [<number> t2] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3
        <number> t8 = phi [<number> 12, if.true2] [<number> 13, if.false3]
        jump -> b4
if.true2: <- entry0
        jump -> if.done1
if.false3: <- entry0
        jump -> if.done1
b4: <- if.done1
        <number> t9 = <number> t8 add <number> t2
        cadd-capture = yak-main-symbol field[<string> cadd]
        <> t11 = call <> yak-main$2 () [cadd-capture]
        <number> t12 = cadd-capture add <number> 1
        ca-capture = yak-main-symbol field[<string> ca]
        <number> t14 = call <> yak-main$3 (<number> 1, <number> 2, <number> 3) [ca-capture, <> yak-main$1, <number> 11]
        <number> t15 = <number> t14 add <number> 13
        <number> t16 = call <> yak-main$4 (<number> 1, <number> 2, <number> 3) [<> yak-main$1, <number> 11]
        <number> t17 = <number> t16 add <number> 13
			`,
			`
yak-main$1 <number> arg1
parent: yak-main
pos:   4:4   -  14:0  : (arg1)=>{
freeValue: <number> ca
return: <number> t9
entry0:
        <boolean> t0 = <number> 1 gt <number> 2
        If [<boolean> t0] true -> if.true2, false -> if.false3
if.done1: <- if.true2 if.false3
        <number> t7 = phi [<number> t2, if.true2] [<number> t4, if.false3]
        jump -> b4
if.true2: <- entry0
        <number> t2 = <number> arg1 add <number> 2
        jump -> if.done1
if.false3: <- entry0
        <number> t4 = <number> ca add <number> 2
        jump -> if.done1
b4: <- if.done1
        <number> t8 = <number> t7 add <number> 1
        ret <number> t8
			`,
			`
yak-main$2
parent: yak-main
pos:  37:6   -  37:20 : ()=>{cadd++}
freeValue: cadd-capture
entry0:
        <number> t1 = cadd-capture add <number> 1
        update [cadd-capture] = <number> t1
			`,
			`
yak-main$3 <> pc1, <number> pc2, <number> pc3
parent: yak-main
pos:  43:4   -  50:0  : fn(pc1,pc2,pc3){
freeValue: ca-capture, <> a, <> va
return: <number> t7
entry0:
        update [ca-capture] = <number> 55
        <number> t2 = call <> a (<> va) []
        <number> t3 = <number> 13 add <number> t2
        <number> t4 = <number> pc2 mul <number> pc3
        <number> t5 = <number> t3 add <number> t4
        <number> t6 = <number> t5 add ca-capture
        ret <number> t6
			`,
			`
yak-main$4 <> pc1, <number> pc2, <number> pc3
parent: yak-main
pos:  53:4   -  58:0  : fn(pc1,pc2,pc3){
freeValue: <> a, <> va
return: <number> t5
entry0:
        <number> t0 = call <> a (<> va) []
        <number> t1 = <number> 13 add <number> t0
        <number> t2 = <number> pc2 mul <number> pc3
        <number> t3 = <number> t1 add <number> t2
        <number> t4 = <number> t3 add <number> 55
        ret <number> t4
			`,
		}

		prog := ParseSSA(code)
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
	<> t0 = call <> yak-main$1 (<number> 1) []
	<> t1 = call <> yak-main$3 (<number> 2) [<number> 12]
	<> t2 = call <> t0 () []
	<> t3 = call <> t1 () [<number> 12]
	<> t4 = <> t2 add <> t3
	<> t5 = call <> t1 () [<number> 13]
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
		prog := ParseSSA(code)
		CheckProgram(t, prog)
		CompareYakFunc(t, prog, ir)
	})

	t.Run("closure_mutiple", func(t *testing.T) {
		code := `
a = (a, b, c...) => {
	return a, b, c
}

e = (a1, b, c, d...) => {
	a(a1, b, c...)
}

// multiple return 
print(a(1, 2, 3, 4, "3"))

// extra return list
b, c, d = a(1, 2, 3, 4, "3")
b = c + d 
print(b, c, d)
`
		ir := []string{
			`
yak-main
entry0:
        <struct {,,struct {}}> t0 = call <> yak-main$1 (<number> 1, <number> 2, <number> 3, <number> 4, <string> 3) []
        <> t1 = call <> print (<struct {,,struct {}}> t0) []
        <struct {,,struct {}}> t2 = call <> yak-main$1 (<number> 1, <number> 2, <number> 3, <number> 4, <string> 3) []
        <> t3 = <struct {,,struct {}}> t2 field[<number> 0]
        <> t4 = <struct {,,struct {}}> t2 field[<number> 1]
        <> t5 = <struct {,,struct {}}> t2 field[<number> 2]
        <> t6 = <> t4 add <> t5
        <> t7 = call <> print (<> t6, <> t4, <> t5) []
	`,
			`
yak-main$1 <> a, <> b, <struct {}> c
parent: yak-main
pos:   2:4   -   4:0  : (a,b,c...)=>{
return: <struct {,,struct {}}> t0
entry0:
        ret <> a, <> b, <struct {}> c
`,
			`
yak-main$2 <> a1, <> b, <struct {}> c, <struct {}> d
parent: yak-main
pos:   6:4   -   8:0  : (a1,b,c,d...)=>{
freeValue: <> a
entry0:
        <> t0 = call <> a (<> a1, <> b, <struct {}> c) []
`,
		}
		prog := ParseSSA(code)
		CheckProgram(t, prog)
		CompareYakFunc(t, prog, ir)
	})

	t.Run("closure_instancecode", func(t *testing.T) {
		code := `
	
// normal func
a = func(){
	return 11
}() + 12
a = func{
	return 11
} + 12

// capture
d = func{
	return a + 1
}
`
		ir := []string{
			`
yak-main
entry0:
	<number> t0 = call <> yak-main$1 () []
	<number> t1 = <number> t0 add <number> 12
	<number> t2 = call <> yak-main$2 () []
	<number> t3 = <number> t2 add <number> 12
	<number> t4 = call <> yak-main$3 () [<number> t3]
`,
			`
yak-main$1
parent: yak-main
pos:   4:4   -   6:0  : func(){
return: <number> t0
entry0:
	ret <number> 11
`,
			`
yak-main$2
parent: yak-main
pos:   7:4   -   9:0  : func{
return: <number> t0
entry0:
	ret <number> 11
`,
			`
yak-main$3
parent: yak-main
pos:  12:4   -  14:0  : func{
freeValue: <number> a
return: <number> t1
entry0:
	<number> t0 = <number> a add <number> 1
	ret <number> t0
`,
		}
		prog := ParseSSA(code)
		CheckProgram(t, prog)
		CompareYakFunc(t, prog, ir)
	})

	t.Run("closure_defer", func(t *testing.T) {
		code := `
// instance function
defer func{
    print("defer func 1")
}

// function call
defer func(){
    print("defer func 2")
}()
defer () => {
    print("defer func 3")
}()

// anonymouse function
defer func(){
    print("defer func 4")
}

defer () => {
    print("defer func 5")
}

print("main")
			`
		ir := []string{
			`
yak-main
entry0:
        <> t0 = call <> print (<string> main) []
        <> t1 = call <> yak-main$3 () []
        <> t2 = call <> yak-main$2 () []
        <> t3 = call <> yak-main$1 () []
`,
			`
yak-main$1
parent: yak-main
pos:   3:6   -   5:0  : func{
entry0:
        <> t0 = call <> print (<string> defer func 1) []
`,
			`
yak-main$2
parent: yak-main
pos:   8:6   -  10:0  : func(){
entry0:
        <> t0 = call <> print (<string> defer func 2) []
`,
			`
yak-main$3
parent: yak-main
pos:  11:6   -  13:0  : ()=>{
entry0:
        <> t0 = call <> print (<string> defer func 3) []
`,
		}
		prog := ParseSSA(code)
		CheckProgram(t, prog)
		CompareYakFunc(t, prog, ir)
	})

}
