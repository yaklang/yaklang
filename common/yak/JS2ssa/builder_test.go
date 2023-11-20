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
	}else if((a > 1) && (a < 3)){
		print(a)
	}else{
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

func TestFunction(t *testing.T) {
	prog := ParseSSA(`
	function ajax(url, type, data, success) {
		// 创建一个XMLHttpRequest对象
		const xhr = new XMLHttpRequest()
		// 判断type请求方式
		if (type == 'get') {
			// 判断data的数据类型转换成字符串
			if (Typeof(data) == "object") {
				// data = (new URLSearchParams(data)).toString()
			}
			// 设置请求方式和请求地址
			xhr.open(type, url + '?' + data)
			// 发送请求
			xhr.send()
		} else if (type == 'post') {
			// 设置请求方式和请求地址
			xhr.open(type, url)
			// 判断数据是不是字符串
			if (Typeof(data) == "string") {
				// 设置对应的content-type
				xhr.setRequestHeader('Content-type', 'application/x-www-form-urlencoded')
				xhr.send(data)
			} else if (Typeof(data) == "object") {
			} else {
				xhr.setRequestHeader('Content-type', 'application/json')
				const str = JSON.stringify(data);
				console.log(Typeof(str))
				xhr.send(str)
			}
		}
	}
	`, none)
	prog.Show()
}

func TestExpr(t *testing.T) {
	prog := ParseSSA(`
	for(a=1,s=1;a<11&&s<20;a++,s++){
		a+1,s+a;
	}
	`, none)
	prog.Show()
}