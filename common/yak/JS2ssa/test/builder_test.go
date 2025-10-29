package test

import (
	_ "embed"
	"fmt"
	_ "net/http/pprof"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func ParseSSA(code string) (*ssaapi.Program, error) {
	return ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JS))
}

func TestDemo1(t *testing.T) {
	prog, err := ParseSSA(`
	function test(a, b){
		return a + b;
	}
	sum = test(1,2);
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	fmt.Println(prog.GetErrors())
}

func TestDemo2(t *testing.T) {
	prog, err := ParseSSA(`
	try{
		a = 1;
		b = 2;
	}catch{

	}finally{
		c = 3;
	}`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestSwitch(t *testing.T) {
	prog, err := ParseSSA(`
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
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestBreak(t *testing.T) {
	prog, err := ParseSSA(`
	a = 2;
	label1: 
	{
		print(a);
		a = 1;
		break label1;
	}
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func Test_Main(t *testing.T) {
	prog, err := ParseSSA(`
	var b = (()=>{return window.location.hostname + "/app/"})()
	window.location.href = b + "/login.html?ts=";
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestNew(t *testing.T) {
	prog, err := ParseSSA(`
	// 创建一个XMLHttpRequest对象
	let xhr = new XMLHttpRequest()
	// 调用open函数填写请求方式和url地址
	xhr.open('GET', 'http://*****')
	// 调用send函数发送请求
	xhr.send()
	// 监听load事件，响应请求后的结果
	xhr.addEventListener('load', function a() {
		console.log(this.response)
	})
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestFunc(t *testing.T) {
	prog, err := ParseSSA(`
	(function() {})

	function myFunction(x, y=10) {
		// y is 10 if not passed or undefined
		return x + y;
	}
	 
	a = myFunction(0, 2) // 输出 2
	b = myFunction(5); // 输出 15, y 参数的默认值

	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestElseIf(t *testing.T) {
	prog, err := ParseSSA(`
	a = 2
	if(a < 1){
		a++;
	} else if ((a > 1) && (a < 3)){
		print(a)
	} else{
		b = a
	}
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestTrueOrFalse(t *testing.T) {
	prog, err := ParseSSA(`
	function tof(a, b){
	}

	b = tof(true, false);
	print(b)
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestThis(t *testing.T) {
	prog, err := ParseSSA(`
	xhr.addEventListener("load", function() {
        console.log(this.add);
    })
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestIdentifier(t *testing.T) {
	prog, err := ParseSSA(`
	$(document).ready(function(){
		$("button").click(function(){
		  $.get("/example/jquery/demo_test.asp",function(data,status){
			alert("数据：" + data + "\n状态：" + status);
		  });
		});
	  });
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestTry(t *testing.T) {
	prog, err := ParseSSA(`
  let url = "https://api.github.com/users/ruanyf";
   try {
    let response = await fetch(url);
    return await response.json();
  } catch (error) {
    console.log('Request Failed', error);
  }
 
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestLet(t *testing.T) {
	prog2, err := ParseSSA(`
  	let response = await fetch(url);
  
`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog2.Show()
}

func TestReturn(t *testing.T) {
	prog, err := ParseSSA(`
		f = () => {a = 1; b = 2; return a, b;}
		console.log(f())
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}
func TestBitNot(t *testing.T) {
	prog, err := ParseSSA(`
		a = ~0b1
		b = -(-(1))
		print(a)
		print(b)
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestObject(t *testing.T) {
	prog, err := ParseSSA(`
	c = {2:_}
	d = {1,2,3,4,5}
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestUse(t *testing.T) {
	// var 多个值
	code := `
		o = function() {o = 1}
		c = a && o()
		target = c
	`
	// check(t, code, "print", "phi")
	check(t, TestCase{
		code:   code,
		ref:    "target",
		target: []string{"phi"},
		fuzz:   true,
	})
}

func TestNumber(t *testing.T) {
	prog, err := ParseSSA(`
		a < 1e-6 ? 1 : 2
	`)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}

func TestTemplateString(t *testing.T) {
	prog, err := ParseSSA("a = 12; print(`aaa${a}bbb`)")
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}
