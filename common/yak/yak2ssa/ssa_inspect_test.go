package yak2ssa

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

func TestSSA_SMOKING(t *testing.T) {
	prog := ParseSSA(`
print(11)
try {
	print(a)
}catch {
	print(b)
} finally {
	print(c)
}
print(d)
	`)
	prog.Show()
	fmt.Println(prog.GetErrors().String())
}

func TestSSA_SMOKING_2(t *testing.T) {
	prog := ParseSSA(`
a = make([]int, 0)
b = len(a) || len(a)
	`,
		WithAnalyzeOpt(
			ssa4analyze.WithAnalyzer(nil),
			ssa4analyze.WithPass(false),
		),
	)
	prog.Show()
	fmt.Println(prog.GetErrors().String())
}

func TestSSA_SMOKING_3(t *testing.T) {
	prog := ParseSSA(`var a = {"c": 1}; a + 1; ; var a = 1;c= a + 1; a = c + 2;`)
	prog.Show()
}

func TestSSA_SMOKING_4(t *testing.T) {
	prog := ParseSSA(`
f = (a, b, c) => {
	return {
		"a": a,
		"b": b,
		"c": c,
	}
}
// parameter 
// func = (arg) => {
// 	b = arg[1]  + 1
// 	return b
// }
// // return 
// func2 = () => {
// 	return make([]int, 0)
// }
// a = func2() 
// a[1] = 112

// func3 = () => {
// 	return 1, 2, 3, 4
// }
// a, b, c, d = func3()

// func4 = (arg) => {
// 	arg[1] += 1
// 	return arg
// }
	`,
		WithAnalyzeOpt(
			ssa4analyze.WithPass(false),
			ssa4analyze.WithAnalyzer(nil),
		),
	)
	if prog == nil {
		t.Fatal("parse ssa error")
	}
	prog.Show()
	for _, v := range prog.GetErrors() {
		fmt.Printf("%#v\n", v)
	}
}

func TestSSA_SMOKING_5(t *testing.T) {
	prog := ParseSSA(`
c = 1
d = 1
a = () => {
	print(c)
	print(c)
	c = 3
	b()
}

d = 2
b = () => {
	print(d)
	print("b")
}

d = 3
c = 2
a()
	`, WithAnalyzeOpt())
	prog.Show()
}

func TestSSA_SMOKING_MAP(t *testing.T) {
	prog := ParseSSA(`
a = 1
{
	a := 2
	print(a)
}
print(a)
	`)
	prog.Show()
	fmt.Println(prog.GetErrors().String())
}
