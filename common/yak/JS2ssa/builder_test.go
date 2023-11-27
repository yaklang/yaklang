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

//go:embed b.js
var test string

func TestJsReals(t *testing.T) {
	prog := ParseSSA(test, none)
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

func TestPos(t *testing.T) {
	prog := ParseSSA(`
	 function b(e) {
        a = null,
        return "string" === typeof e && (e = function(e) {
            if (0 === (e = e.trim().toLowerCase()).length) return ! 1;
            var t = !1;
            if (A[e]) e = A[e],
            t = !0;
            else if ("transparent" === e) return {
                r: 0,
                g: 0,
                b: 0,
                a: 0,
                format: "name"
            };
            var n = S.rgb.exec(e);
            if (n) return {
                r: n[1],
                g: n[2],
                b: n[3]
            };
            if (n = S.rgba.exec(e)) return {
                r: n[1],
                g: n[2],
                b: n[3],
                a: n[4]
            };
            if (n = S.hsl.exec(e)) return {
                h: n[1],
                s: n[2],
                l: n[3]
            };
            if (n = S.hsla.exec(e)) return {
                h: n[1],
                s: n[2],
                l: n[3],
                a: n[4]
            };
            if (n = S.hsv.exec(e)) return {
                h: n[1],
                s: n[2],
                v: n[3]
            };
            if (n = S.hsva.exec(e)) return {
                h: n[1],
                s: n[2],
                v: n[3],
                a: n[4]
            };
            if (n = S.hex8.exec(e)) return {
                r: y(n[1]),
                g: y(n[2]),
                b: y(n[3]),
                a: m(n[4]),
                format: t ? "name": "hex8"
            };
            if (n = S.hex6.exec(e)) return {
                r: y(n[1]),
                g: y(n[2]),
                b: y(n[3]),
                format: t ? "name": "hex"
            };
            if (n = S.hex4.exec(e)) return {
                r: y(n[1] + n[1]),
                g: y(n[2] + n[2]),
                b: y(n[3] + n[3]),
                a: m(n[4] + n[4]),
                format: t ? "name": "hex8"
            };
            if (n = S.hex3.exec(e)) return {
                r: y(n[1] + n[1]),
                g: y(n[2] + n[2]),
                b: y(n[3] + n[3]),
                format: t ? "name": "hex"
            };
            return ! 1
        } (e)),
        "object" === typeof e && (E(e.r) && E(e.g) && E(e.b) ? (t = e.r, n = e.g, r = e.b, i = {
            r: 255 * d(t, 255),
            g: 255 * d(n, 255),
            b: 255 * d(r, 255)
        },
        l = !0, c = "%" === String(e.r).substr( - 1) ? "prgb": "rgb") : E(e.h) && E(e.s) && E(e.v) ? (a = p(e.s), s = p(e.v), i = function(e, t, n) {
            e = 6 * d(e, 360),
            t = d(t, 100),
            n = d(n, 100);
            var r = Math.floor(e),
            i = e - r,
            o = n * (1 - t),
            a = n * (1 - i * t),
            s = n * (1 - (1 - i) * t),
            u = r % 6;
            return {
                r: 255 * [n, a, o, o, s, n][u],
                g: 255 * [s, n, n, a, o, o][u],
                b: 255 * [o, o, s, n, n, a][u]
            }
        } (e.h, a, s), l = !0, c = "hsv") : E(e.h) && E(e.s) && E(e.l) && (a = p(e.s), u = p(e.l), i = function(e, t, n) {
            var r, i, o;
            if (e = d(e, 360), t = d(t, 100), n = d(n, 100), 0 === t) i = n,
            o = n,
            r = n;
            else {
                var a = n < .5 ? n * (1 + t) : n + t - n * t,
                s = 2 * n - a;
                r = v(s, a, e + 1 / 3),
                i = v(s, a, e),
                o = v(s, a, e - 1 / 3)
            }
            return {
                r: 255 * r,
                g: 255 * i,
                b: 255 * o
            }
        } (e.h, a, u), l = !0, c = "hsl"), Object.prototype.hasOwnProperty.call(e, "a") && (o = e.a)),
        o = function(e) {
            return e = parseFloat(e),
            (isNaN(e) || e < 0 || e > 1) && (e = 1),
            e
        } (o),
        {
            ok: l,
            format: e.format || c,
            r: Math.min(255, Math.max(i.r, 0)),
            g: Math.min(255, Math.max(i.g, 0)),
            b: Math.min(255, Math.max(i.b, 0)),
            a: o
        }
    }
	`, none)
	prog.Show()
}
