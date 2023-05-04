package main

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
)

func main() {
	inputStream := antlr.NewInputStream(`
if true {
    return}

ab+1*2
f"\"asdf" 
123
0xAfAA
0b10111
0o127
[1,2,3]
{1:23}
{1:23, "abc": 123} + 1
1+1

abc.hasPrefix(1+1)

if 1+1 {
    1+1 
}

if 1+1 {   } elif true {}
if 1+1 {} elif true {} else {a=1+1;}
ab?1+1:1 


/* switch */
switch true {
case "1+1":
    1+1
    a = 123
case "true":
case "123":
    1+1
    break;;;
    case "123":
default:
    1+1
}
switch true {
case 1:
}
switch true {
default:
123
}
switch true {
case 1,1,23,3: 
case 21,3,5:
}

// for 语句
for 1 { println(1)
a+1
asdfasd+1123}

// func def
a = func(id,a...) {}
a()
fc()
fn{a+1;a+11233;}
fn(p1,p2,p3){}
fn{a+1}
fn abc(p1,p2,p3){}
fn abc(p1,p2,p3...){}

// for range
for range [1,2,3] {
    println(1,2,3)
}

for i,b = range [1,2,3] {
    println(1,2,3)
}

for 1 = range [1,2,3] { println(1) }
`)
	lex := yak.NewYaklangLexer(inputStream)

	//for _, t := range lex.GetAllTokens() {
	//	println(t.GetText())
	//}

	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)
	vt := &yakast.YakCompiler{}
	p.Program().Accept(vt)
}
