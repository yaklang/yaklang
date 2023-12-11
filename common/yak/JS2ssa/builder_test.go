package js2ssa

import (
	_ "embed"
	"fmt"
	"regexp"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func none(*ssa.FunctionBuilder) {}

func check(t *testing.T, code string, funcs string, regex string) {
	re, err := regexp.Compile(".*" + regex + ".*")
	if err != nil {
		t.Fatal(err)
	}

	prog := ParseSSA(code, none)
	prog.ShowWithSource()

	showFunc := prog.Packages[0].Funcs[0].GetValuesByName(funcs)[0]
	for _, v := range showFunc.GetUsers() {
		line := v.LineDisasm()
		fmt.Println(line)
		if !re.Match(utils.UnsafeStringToBytes(line)) {
			t.Fatal(line)
		}
	}
}

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

func Test_Main(t *testing.T) {
	prog := ParseSSA(`
	var b = (()=>{return window.location.hostname + "/app/"})()
	window.location.href = b + "/login.html?ts=";
	`, none)
	prog.Show()
}

func TestNew(t *testing.T) {
	prog := ParseSSA(`
	// 创建一个XMLHttpRequest对象
	let xhr = new XMLHttpRequest()
	// 调用open函数填写请求方式和url地址
	xhr.open('GET', 'http://*****')
	// 调用send函数发送请求
	xhr.send()
	// 监听load事件，响应请求后的结果
	xhr.addEventListener('load', function (
		console.log(this.response)
	})
	`, none)
	prog.Show()
}

func TestFunc(t *testing.T) {
	prog := ParseSSA(`
	(function() {})

	function myFunction(x, y=10) {
		// y is 10 if not passed or undefined
		return x + y;
	}
	 
	a = myFunction(0, 2) // 输出 2
	b = myFunction(5); // 输出 15, y 参数的默认值

	`, none)
	prog.Show()
}

func TestElseIf(t *testing.T) {
	prog := ParseSSA(`
	a = 2
	if(a < 1){
		a++;
	} else if ((a > 1) && (a < 3)){
		print(a)
	} else{
		b = a
	}
	`, none)
	prog.Show()
}

func TestTrueOrFalse(t *testing.T) {
	prog := ParseSSA(`
	function tof(a, b){
	}

	b = tof(true, false);
	print(b)
	`, none)
	prog.Show()
}

func TestThis(t *testing.T) {
	prog := ParseSSA(`
	xhr.addEventListener("load", function() {
        console.log(this.add);
    })
	`, none)
	prog.Show()
}

func TestIdentifier(t *testing.T) {
	prog := ParseSSA(`
	$(document).ready(function(){
		$("button").click(function(){
		  $.get("/example/jquery/demo_test.asp",function(data,status){
			alert("数据：" + data + "\n状态：" + status);
		  });
		});
	  });
	`, none)
	prog.Show()
}

func TestTry(t *testing.T) {
	prog := ParseSSA(`
  let url = "https://api.github.com/users/ruanyf";
   try {
    let response = await fetch(url);
    return await response.json();
  } catch (error) {
    console.log('Request Failed', error);
  }
 
`, none)
	prog.Show()
}

func TestLet(t *testing.T) {
	prog2 := ParseSSA(`
  	let response = await fetch(url);
  
`, none)

	prog2.Show()
}

func TestExpr(t *testing.T) {
	prog := ParseSSA(`
	for(a=1,s=1;a<11&&s<20;a++,s++){
		a+1,s+a;
	}
	`, none)
	prog.Show()
}

func TestReturn(t *testing.T) {
	prog := ParseSSA(`
		f = () => {a = 1; b = 2; return a, b;}
		console.log(f())
	`, none)
	prog.Show()
}
func TestBitNot(t *testing.T) {
	prog := ParseSSA(`
		a = ~0b1
		b = -(-(1))
		print(a)
		print(b)
	`, none)
	prog.Show()
}

func TestObject(t *testing.T) {
	prog := ParseSSA(`
	c = {2:_}
	d = {1,2,3,4,5}
	`, none)
	prog.Show()
}

func TestUse(t *testing.T) {
	// var 多个值
	code := `
		o = function() {o = 1}
		c = a && o()
		print(c)
	`
	check(t, code, "print", "phi")
}

func TestNumber(t *testing.T) {
	prog := ParseSSA(`
		a < 1e-6 ? 1 : 2
	`, none)
	prog.Show()
}
