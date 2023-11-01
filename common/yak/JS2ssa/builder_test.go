package js2ssa

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func none(*ssa.FunctionBuilder) {}

func TestDemo1(m *testing.T) {
	prog := ParseSSA(`
	function test(a, b){
		return a + b;
	}
	sum = test(1,2);
	`, none)
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
	}`, none)
	prog.Show()
}

func TestSwitch(t *testing.T) {
	prog := ParseSSA(`
	const fruit = "apple";

switch (fruit) {
  case "apple":
  case "banana":
    print("这是一个香蕉");
  case "orange":
    print("这是一个橙子");
  default:
    print("未知水果");
}
	`, none)

	prog.Show()
}

func TestPoint(t *testing.T) {
	prog := ParseSSA(`
		// setTimeout(()=>{window.location = "www"});
		// setTimeout(()=>1);
		// function b() {
		// 	return 1;
		// }

		a = setTimeout

if (true) {
    setTimeout(a('window.location.href= "http://www.baidu.com"'))
}

if (window){// 弱类型 这个为true 定义了就true
    setTimeout(a('window.location.href= "http://www.baidu.com"'))
}

if (NaN){// JS的特色 这里视为false
    setTimeout(a('window.location.href= "http://www.baidu.com"'))
}

for (var i=0; i<5; i++) {
      if (i = 5){
        setTimeout(a('window.location.href= "http://www.baidu.com"'))
      }
}

a =  window
a.location.href = "www.baidu.com"

function b(c) {
    c.location.href = "www.baidu.com"
}
b(a)
	`, none)
	prog.Show()
}

func TestBreak(t *testing.T) {
	prog := ParseSSA(`
	a = 2;
	label1: 
	{
		print(a);
		a = 1;
		break label1;
	}
	`, none)

	prog.ShowWithSource()
}

func TestMain(t *testing.T) {
	prog := ParseSSA(`
	var b = (()=>{return window.location.hostname + "/app/"})()
	window.location.href = b + "/login.html?ts=";
	`, none)
	prog.Show()
}
