package js2ssa

import (
	"fmt"
	"testing"
)

func TestDemo1(m *testing.T) {
	prog := ParseSSA(`
	function test(a, b){
		return a + b;
	}
	sum = test(1,2);
	`)
	prog.Show()
	fmt.Println(prog.GetErrors())
}

func TestDemo2(t *testing.T) {
	prog := ParseSSA(`
	try{
		a = 1;
		b = 2;
	}catch{

	}finally{
		c = 3;
	}`)
	prog.Show()
}

func TestBreak(t *testing.T) {
	prog := ParseSSA(`
	a = 0 
	print(a)

	label : {
		print(a)
		a = 1 
		print(a)
	}

	// print(a)
	// a = 3
	// break label

	if 1 {
		a = 3 
		print(a)
		break label
	}
	print(a)

	for (i=1;i<10;i++){
		a = 2 
		print(a)
		if (i == 2) {
			break label
		}else {
			if (i == 4){
				a = 4 
				break label
			}
		}
	}
	`)
	prog.Show()
}
