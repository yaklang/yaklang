package js2ssa

import (
	"fmt"
	"os"
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

func TestReturn(t *testing.T) {
	prog := ParseSSA(`
		f = () => {a = 1; b = 2; return a, b;}
		console.log(f())
	`, none)
	prog.Show()
}

func TestLong(t *testing.T) {
	prog := ParseSSA(`
	(this["webpackJsonppalm-kit-desktop"]=this["webpackJsonppalm-kit-desktop"]||[]).push([[2],[function(e,t,n){"use strict";e.exports=n(785)},function(e,t,n){"use strict";n.d(t,"p",(function(){return g})),n.d(t,"G",(function(){return v})),n.d(t,"d",(function(){return m})),n.d(t,"I",(function(){return y})),n.d(t,"J",(function(){return A})),n.d(t,"m",(function(){return b})),n.d(t,"i",(function(){return _})),n.d(t,"r",(function(){return x})),n.d(t,"s",(function(){return w})),n.d(t,"K",(function(){return S})),n.d(t,"u",(function(){return E})),n.d(t,"k",(function(){return O})),n.d(t,"H",(function(){return C})),n.d(t,"N",(function(){return k})),n.d(t,"n",(function(){return M})),n.d(t,"o",(function(){return T})),n.d(t,"F",(function(){return j})),n.d(t,"c",(function(){return P})),n.d(t,"h",(function(){return I})),n.d(t,"t",(function(){return B})),n.d(t,"w",(function(){return N})),n.d(t,"C",(function(){return L})),n.d(t,"D",(function(){return D})),n.d(t,"z",(function(){return R})),n.d(t,"A",(function(){return F})),n.d(t,"E",(function(){return z})),n.d(t,"v",(function(){return H})),n.d(t,"x",(function(){return V})),n.d(t,"y",(function(){return G})),n.d(t,"B",(function(){return W})),n.d(t,"l",(function(){return q})),n.d(t,"O",(function(){return Q})),n.d(t,"P",(function(){return Y})),n.d(t,"Q",(function(){return K})),n.d(t,"S",(function(){return X})),n.d(t,"M",(function(){return Z})),n.d(t,"b",(function(){return $})),n.d(t,"T",(function(){return J})),n.d(t,"R",(function(){return ee})),n.d(t,"f",(function(){return oe})),n.d(t,"e",(function(){return ae})),n.d(t,"g",(function(){return se})),n.d(t,"j",(function(){return ue})),n.d(t,"q",(function(){return le})),n.d(t,"L",(function(){return ce})),n.d(t,"a",(function(){return fe}));var r=n(99),i=k(["Function","RegExp","Date","Error","CanvasGradient","CanvasPattern","Image","Canvas"])}]])	`, none)
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

func TestJsReal(t *testing.T) {
	// data, err := os.ReadFile("C:\\codefile\\a1.js")
	// if err != nil {
	// 	fmt.Println("读取文件时发生错误:", err)
	// 	return
	// }
	data := `a = 1`
	// 将文件内容转换为字符串
	content := string(data)
	prog := ParseSSA(content, none)
	prog.Show()
}

func TestSome(t *testing.T) {
	prog := ParseSSA(`
	for(var a of {2,3,4}){
		b = a
		print(b)
	}
	`, none)
	prog.Show()
}
