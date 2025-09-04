package antlr4yak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod/lib/utils"
)

type testStructed struct {
	A []int
}
type Number uint16

func (n Number) String() string {
	return fmt.Sprintf("number: %d", n)
}

func init() {
	Import("libx", map[string]interface{}{
		"abc": func() string {
			return "called-abc"
		},
	})
	Import("testMap", map[string]interface{}{
		"Remove": func() bool {
			return true
		},
	})
	// Import("NewOrderedMap", orderedmap.New)
	Import("testOrderedMap", map[string]any{
		"StringMap": func(v map[string]any) map[string]any {
			return v
		},
		"AnyMap": func(v map[any]any) map[any]any {
			return v
		},
		"ToOrderedMap": func(v *orderedmap.OrderedMap) *orderedmap.OrderedMap {
			return v
		},
	})
	Import("jsonMarshal", func(v any) ([]byte, error) {
		return json.Marshal(v)
	})
	Import("timeNow", time.Now) // timeNow.Unix()
	Import("testIns", &testStructed{A: []int{1, 2, 3}})
	Import("wantInsSlice", []*testStructed{{A: []int{1, 2, 3}}})
	Import("dump", func(v ...interface{}) {
		spew.Dump(v...)
	})
	Import("print", func(v interface{}) {
		fmt.Println(v)
	})
	Import("println", func(v interface{}) {
		fmt.Println(v)
	})
	Import("printf", func(f string, items ...interface{}) {
		fmt.Printf(f, items...)
	})
	Import("handlerTest", func(h func(b bool) string) {
		fmt.Println(h(true))
	})
	Import("assert", func(v bool) {
		if !v {
			panic("assert failed")
		}
	})
	Import("panic", func(v interface{}) {
		panic(v)
	})
	Import("sleep", func(i float64) {
		utils.Sleep(i)
	})
	Import("sprint", func(i interface{}) string {
		return fmt.Sprint(i)
	})
	Import("len", func(i interface{}) int {
		// Reference: common\yak\yaklang\lib\builtin\builtin.go Len
		if m, ok := i.(*orderedmap.OrderedMap); ok {
			return m.Len()
		}
		return reflect.ValueOf(i).Len()
	})
	Import("typeof", func(i interface{}) reflect.Type {
		return reflect.TypeOf(i)
	})
	Import("close", func(i interface{}) {
		rv := reflect.ValueOf(i)
		if rv.Kind() == reflect.Chan {
			rv.Close()
		}
	})
	Import("testTypeCast", func(i int) {
		rv := reflect.ValueOf(i)
		if rv.Kind() == reflect.Chan {
			rv.Close()
		}
	})
	Import("package", map[string]interface{}{
		"test": func(v ...interface{}) (string, error) {
			return "test", errors.New("test error")
		},
	})
	Import("getUint16", func() uint16 {
		return 1
	})
	Import("getUint16Wrapper", func() (*struct{ A uint16 }, map[string]uint16, []uint16) {
		return &struct{ A uint16 }{A: 1}, map[string]uint16{"a": 1}, []uint16{1, 2, 3}
	})
	Import("NewSizedWaitGroup", utils2.NewSizedWaitGroup)
	Import("dur", func(i string) time.Duration {
		dur, _ := time.ParseDuration(i)
		return dur
	})
	Import("getNumber", func(i int) Number {
		return Number(i)
	})
}

func TestBoolEqualityComparison(t *testing.T) {
	code := `
assert !("true" == true)
`
	if err := NewExecutor(code).VM.SafeExec(); err != nil {
		panic(err)
	}
}

func TestRangeString(t *testing.T) {
	code := `
for index, i := range "abc" {
	if index == 0 { assert i == "a" }
if index == 1 { assert i == "b" }
if index == 2 { assert i == "c" }
}

b = []
for i in "abc" {
	dump(i)
	b.Push(i)
}
assert b[0] == "a"
assert b[1] == "b"
assert b[2] == "c"


b = []
for i in "你好ww" {
	dump(i)
	b.Push(i)
}
assert b[0] == "你"
assert b[1] == "好"
assert b[2] == "w"
assert b[3] == "w"
`
	if err := NewExecutor(code).VM.SafeExec(); err != nil {
		panic(err)
	}
}

func TestBuildinMethod(t *testing.T) {
	code := `
assert "abc".StartsWith("a"),"StartsWith error"
assert "abc".HasPrefix("a"),"StartsWith error"
assert b"abc".EndsWith("c"),"EndsWith error"
`
	if err := NewExecutor(code).VM.SafeExec(); err != nil {
		panic(err)
	}
}

func TestChanAndINOP(t *testing.T) {
	code := `
assert true && "b" in "abc"  // in 的优先级要高于 &&
c = make(chan var, 1)
c <- 1 > 0 &&  true
d = <-c
assert d
`
	if err := NewExecutor(code).VM.SafeExec(); err != nil {
		panic(err)
	}
}

// 本测试只针对try-catch与流程控制、return共用的情况,try-catch的其他情况在其他测试中已经覆盖
func TestTryCatchWithProcessControl(t *testing.T) {
	code := `
//return test
a = fn(mode){
	s = 0
	for i = range 2{
		try{
			if mode == "break"{
				s += 2
				break		
			}
			if mode == "return"{
				s += 4
				return s
			}
			if mode == "continue"{
				s += 8
				continue
			}
		}catch e{
			s += 1
		} finally{
			return s
		}
	}
	dump(s)
	return s
}
assert a("break")==2,"break error"
assert a("return")==4,"return error"
assert a("continue")==16,"continue error"

`
	NewExecutor(code).VM.Exec()
}

func TestCheckBreakAndContinue(t *testing.T) {
	code := `
for{
	break
}
fn{
break
}
continue
`
	if err := NewExecutor(code).VM.SafeExec(); !(err != nil && strings.Contains(fmt.Sprint(err), "break statement can only be used in for or switch") && strings.Contains(fmt.Sprint(err), "continue statement can only be used in for")) {
		panic(" failed to check break and continue at compiler time")
	}
}

func TestNewExecutor_MemberCallInTemplateString(t *testing.T) {
	NewExecutor(`
a = {"a":1}
assert f"bac\$" == "bac$"
assert f"${a.a}" == "1"
d = {1:23, "bbb": a}
e = 1
assert d.$e == 23
assert f"${d.$e}" == "23"
`).VM.Exec()
}

func TestNewExecutor_MultiQuote(t *testing.T) {
	code := `dump(123)))`
	if err := NewExecutor(code).VM.SafeExec(); !strings.Contains(err.Error(), "compile error") {
		panic(err)
	}
}

func TestNewExecutor_LoopAssign(t *testing.T) {
	code := `
a = [[1, 2]]
a[0][0] = 3
assert a[0][0] == 3

b = [testIns]
b[0].A[0] = 4
assert b[0].A[0] == 4
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_OverrideMethod(t *testing.T) {
	NewExecutor(`assert testMap.Remove();
globalVarFailed = false
try {
assert testMap.Has("Remove")
} catch e {
globalVarFailed = true
}

assert globalVarFailed

a = {}
dump(a)
a["CCC"] = 1
dump(a)
assert a.Has("CCC")
assert !a.Has("BBB")
a.Remove("CCC")
assert !a.Has("CCC")


`).VM.DebugExec()
}

func TestNewExecutor_Unpack(t *testing.T) {
	code := `
fn {
	defer fn {
		assert recover() != nil
	}
	a, b = fn{return 1, 2, 3}
}

fn {
	defer fn {
		assert recover() != nil
	}
	a, b = 1,2,3
}
a, b = 1, 2
assert a == 1
assert b == 2
`
	NewExecutor(code).VM.DebugExec()
}

func TestNewExecutor_EscapeChar(t *testing.T) {
	_marshallerTest(`
assert '\x20' == " "[0]
assert '\'' == "'"[0]
`)
}

func TestNewExecutor_HijackVMFrame(t *testing.T) {
	_marshallerTest(`
`)
}

func TestMapBuildInMethod(t *testing.T) {
	code := `
a = {"a":1,"b":2}
assert len({}.Entries()) == 0
assert len(a.Entries()) == 2
assert len(a.Items()) == 2
assert len(a.Keys()) == 2
assert a.Keys()[0] in ["a", "b"]
assert a.Values()[0] in [1,2]
a.ForEach(func(k,v){	
	assert k in ["a","b"]
	assert v in [1,2]
})
a.Set("c",3)
assert len(a) == 3
a.Remove("a")
a.Delete("a")
assert len(a) == 2
assert a.Has("a") == false
assert a.Has("c") == true

`
	_marshallerTest(code)
	// _formattest(code)
}

func TestNewExecutor_FixFunctionVariableParam(t *testing.T) {
	code := `
defer fn{
	err := recover()
	if !err.HasPrefix("function a params number not match"){
		panic("参数匹配出错")
	}
}
a = fn(a){
	dump(a, b, c)
}
a(1, 2, 3)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_YakWrappedFunctionUnpack(t *testing.T) {
	code := `
m = {}
m.v = (a, b) => {}
m.v("aaa", {})
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_YakWrappedFunctionAssign(t *testing.T) {
	code := `
a = {"risk":()=>{}}
assert typeof(a) == map[string]var
a.risk = () => {
	
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor(t *testing.T) {
	code := `
a = func{
    defer dump("After AAA")
    dump("AAA")
    return 1
}
assert a == 1
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ExitCode(t *testing.T) {
	code := `
a = fn(){
	for a = range 3{
		return
	}
}
a()
a = 1
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_DeferFix(t *testing.T) {
	code := `
b = 1
go fn{
	testFun_a = 1
	defer fn {
		assert testFun_a == 1
		assert b == 1
		a = 1
	}
	assert testFun_a == 1
	assert a == nil
}
sleep(1)
assert b == 1
assert a == nil
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Goroutine(t *testing.T) {
	code := `
mapV = {}

a = fn(result){
	fn{
		mapV[string(result)] = 1
		dump(result)
		defer fn{
			
		}
	}
}

for i = range 100{
    a(i)
}

sleep(5)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Priority(t *testing.T) {
	code := `
a =fn(){
	return false
}
assert !a(), "非运算优先级测试失败"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Continue(t *testing.T) {
	code := `

//测试c风格for的continue
i0Times = 0
for i = 0; i < 2; i++{
	if i == 0{
		i0Times++
		assert i0Times < 2, "continue未更新迭代对象索引错误"
		continue
	}
	dump(i)
}
//测试for range的continue
i0Times = 0
for i = range 2{
	if i == 0{
		i0Times++
		assert i0Times < 2, "continue未更新迭代对象索引错误"
		continue
	}
	dump(i)
}

`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ElseIf(t *testing.T) {
	code := `
// test else 
a = 0
if false {
	a = 1
} else if false {
	a = 2
} else if 1 == 2 {
	a = 3
} else {
	a = 4
}
assert a == 4
  
// test else if
a = 0
if false {
	a = 1
} else if false {
	a = 2
} else if 1 != 2 {
	a = 3
}
assert a == 3
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_FixMemberCall(t *testing.T) {
	code := `
dump(
	timeNow().String(), 1111111
)

`
	_marshallerTest(code)
	_formattest(code)
}

func abc(a string) {
}

func TestNew2(t *testing.T) {
	code := `
switch {
case 1,2,3:
    if true {
break
    }
    
}
`
	_marshallerTest(code)
	_formattest(code)
}

func Test_ForRangeChannel(t *testing.T) {
	code := `
ch1 = fn{
	ch = make(chan string, 10);
	go fn{
		for range 10 { ch<-"hello" }; 
		close(ch)
	}
	return ch;
}
dump(ch1)

count = 0
for result = range ch1 {
	count++
	dump(result)
	assert typeof(result) == string
}
assert count == 10

ch1 = fn{
	ch = make(chan string, 10);
	go fn{
		for range 10 { ch<-"hello" }; 
		close(ch)
	}
	return ch;
}
dump(ch1)

count = 0
for result in ch1 {
	count++
	dump(result)
	assert typeof(result) == string
}
assert count == 10
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ForRange_Break(t *testing.T) {
	code := `
for range 10{
	break
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_EmptyForRange(t *testing.T) {
	code := `
for range nil {
	panic(1)
}

for in nil {
	panic(2)
}
for range [] {
	panic(3)
}
for range {} {
	panic(4)
}

count = 0
for range 4 {count++}
assert count == 4

for in [] {count++}
assert count == 4

a = make(chan int, 1)
a <- 123
close(a)
for in a {count++}
assert count == 5
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ForIn_UnPack(t *testing.T) {
	code := `
a = [[1,2], [3, 4], ["aaa", "bbb"]]
for i in a {
	assert i in a
}

// unpack
i = 0
for v1, v2 in a {
	assert v1 in a[i]
	assert v2 in a[i]
	i++
}

// unexpected unpack length
fn {
	i = 0
	defer fn{
		assert recover() != nil
		assert i == 2
	}
	a = [[1,2], [3, 4], ["aaa", "bbb","ccc"]]
	for v1, v2 in a {
		assert v1 in a[i]
		assert v2 in a[i]
		i++
	}
}



`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_CommentAnywhere(t *testing.T) {
	code := `
// abc
d = func(/*abc*/a,b   /*asdfasdf*/
,/*EEE*/// Hello
c/*123*/){}
// abasd
e = func(
	/*abc*/a, // Hello 
b   /*asdfasdf*/,c/*123*/){
}
/*HHH*/
f = 1111 + 1111
`
	NewExecutor(code).VM.DebugExec()
}

func TestNewExecutor_TryCatchFinally(t *testing.T) {
	code := `
// test panic
c = 0
try {
	panic(1)
} catch e {
	c++
	dump("catch 1")
	assert e == 1
} finally {
	dump("finally 1")
	c++
}
assert c == 2

// test normal finally
c = 0
try {
	c++
} catch e {
	assert e == 1
} finally {
	dump("finally 2")
	c++
}
assert c == 2

// test without finally
c = 0
try {
	panic(1)
} catch e {
	dump("catch 3")
	c++
	assert e == 1
}
assert c == 1

// test catch without name
c = 0
try {
	panic(1)
} catch {
	dump("catch 4")
	c++
}
assert c == 1
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_SelfAssign(t *testing.T) {
	code := `
a = [1, 2, 3]
a[0]++
a[1] -= 1
assert a[0] == 2
assert a[1] == 1
b = {1: 2, 3: 4}
b[1]++
assert b[1] == 3
b[3] -= 1
assert b[3] == 3
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor3(t *testing.T) {
	code := `
handlerTest(b => sprint(b))
/*
Origin: /a
----
Location: index.php -> /a/index.php # DVWA   /login -> index.php /index.phps /login/index.php pma 
Location: /index.php -> /index.php  # 
Location: http://... -> http://...  # 
Location: https://... -> https://   # forceHttps
*/
    `
	_marshallerTest(code)
	_formattest(code)
}

func _printWithLine(i string) {
	lines := strings.Split(i, "\n")
	for index, l := range lines {
		fmt.Printf("%10d | %s\n", index+1, l)
	}
}

func _viewLexerTokens(i string) {
	inputStream := antlr.NewInputStream(i)
	lex := yak.NewYaklangLexer(inputStream)
	for _, t := range lex.GetAllTokens() {
		fmt.Printf("chan: %v idx: %v: raw: %v\n", t.GetChannel(), t.GetTokenIndex(), t)
	}
}

func _marshallerTestWithCtx(i string, ctx context.Context, debug bool) {
	m := yakvm.NewCodesMarshaller()
	cl := compiler(i)
	if err := cl.GetErrors(); len(err) > 0 {
		panic(err)
	}
	oldSymbolTable, oldCodes := cl.GetRootSymbolTable(), cl.GetOpcodes()
	_ = oldSymbolTable
	_ = oldCodes
	bytes, err := m.Marshal(oldSymbolTable, oldCodes)
	if err != nil {
		spew.Dump(err)
		panic("marshal to bytes failed")
	}
	symbolTable, codes, err := m.Unmarshal(bytes)
	if err != nil {
		panic(err)
	}

	// check
	var (
		checkValue func(index int, name string, c1, c2 *yakvm.Code, v1, v2 *yakvm.Value)
		checkCode  func(index int, c1, c2 *yakvm.Code)
		checkCodes func(codes1, codes2 []*yakvm.Code)
	)

	checkValue = func(index int, name string, c1, c2 *yakvm.Code, v1, v2 *yakvm.Value) {
		if v1 == nil && v2 == nil {
			return
		}

		if v1 != nil {
			if v1.TypeVerbose != v2.TypeVerbose {
				panic(fmt.Sprintf("no.%d opcode %s type not equal: %s[%s] != %s[%s]", index, name, c1, v1.TypeVerbose, c2, v2.TypeVerbose))
			}

			if v1.GetLiteral() != v2.GetLiteral() {
				panic(fmt.Sprintf("no.%d opcode %s literal not equal: %s[%s] != %s[%s]", index, name, c1, v1.GetLiteral(), c2, v2.GetLiteral()))
			}

			typ1, typ2 := reflect.TypeOf(v1.Value), reflect.TypeOf(v2.Value)
			if typ1 != typ2 {
				panic(fmt.Sprintf("no.%d opcode %s value type not equal: %s[%s] != %s[%s]", index, name, c1, typ1, c2, typ2))
			}

			if v1.IsYakFunction() && v2.IsYakFunction() {
				// 暂时只检查function code
				f1, f2 := v1.Value.(*yakvm.Function), v2.Value.(*yakvm.Function)
				checkCodes(f1.GetCodes(), f2.GetCodes())

			} else if v1.IsCodes() && v2.IsCodes() {
				checkCodes(v1.Codes(), v2.Codes())
			} else if !reflect.DeepEqual(v1.Value, v2.Value) {
				panic(fmt.Sprintf("no.%d opcode op1 not equal: %s[%#v] != %s[%#v]", index, c1, v1.Value, c2, v2.Value))
			}
		}
	}
	checkCode = func(index int, c1, c2 *yakvm.Code) {
		if c1 == nil && c2 == nil {
			return
		}

		if c1.Opcode != c2.Opcode {
			panic(fmt.Sprintf("no.%d opcode flag not equal: %s != %s", index, c1, c2))
		}

		if c1.Unary != c2.Unary {
			panic(fmt.Sprintf("no.%d opcode unary not equal: %s[%d] != %s[%d]", index, c1, c1.Unary, c2, c2.Unary))
		}
	}

	checkCodes = func(codes1, codes2 []*yakvm.Code) {
		if len(codes1) == 0 && len(codes2) == 0 {
			return
		}
		if len(codes1) != len(codes2) {
			panic(fmt.Sprintf("codes length not equal: %d != %d", len(codes1), len(codes2)))
		}

		for index := 0; index < len(codes1); index++ {
			c1, c2 := codes1[index], codes2[index]
			checkCode(index, c1, c2)
			checkValue(index, "op1", c1, c2, c1.Op1, c2.Op1)
			checkValue(index, "op2", c1, c2, c1.Op2, c2.Op2)
		}
	}

	if len(bytes) <= 0 {
		panic("Consume Bytes To SymbolTable n Codes failed: Empty Bytes")
	}

	if symbolTable == nil {
		fmt.Printf("SymbolTable: %v\n", symbolTable)
		fmt.Printf("Codes Length: %v\n", len(codes))
		fmt.Printf("Error: %v", err)
		panic("cannot marshal symboltable")
	}

	checkCodes(oldCodes, codes)

	// _ = symbolTable
	// _ = codes
	// cl.SetRootSymTable(symbolTable)
	// cl.SetOpcodes(codes)

	vm := yakvm.NewWithSymbolTable(symbolTable)
	vm.ImportLibs(buildinLib)
	var checkErr error
	err = vm.Exec(ctx, func(frame *yakvm.Frame) {
		frame.SetOriginCode(i)
		if debug {
			frame.DebugExec(codes)
		} else {
			frame.NormalExec(codes)
		}
		checkErr = frame.CheckExit()
	})
	if err != nil {
		panic(err)
	}
	if checkErr != nil {
		panic(checkErr)
	}
}

func _marshallerTest(i string, debugs ...bool) {
	debug := false
	if len(debugs) > 0 {
		debug = debugs[0]
	}
	_marshallerTestWithCtx(i, context.Background(), debug)
}

func showAst(i string) {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(i))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	ast := parser.Program()
	tree := ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser())
	println(tree)
}

func _formattest(i string, debugs ...bool) []*yakvm.Code {
	debug := false
	if len(debugs) > 0 {
		debug = debugs[0]
	}

	e := NewExecutor(i)
	if errs := e.Compiler.GetErrors(); len(errs) > 0 {
		spew.Dump(i)
		panic(errs)
	}
	codes1 := e.VM.GetCodes()
	code2 := e.Compiler.GetFormattedCode()
	e2 := NewExecutor(code2)
	if errs := e2.Compiler.GetErrors(); len(errs) > 0 {
		panic(errs)
	}
	codes2 := e2.VM.GetCodes()

	if len(codes1) != len(codes2) || debug {
		println("------------------------------------")
		_printWithLine(i)
		println("------------------------------------")
		_printWithLine(code2)
		println("---------------CODE1------------------")
		yakvm.ShowOpcodes(codes1)
		println("---------------CODE2------------------")
		yakvm.ShowOpcodes(codes2)
	}
	if len(codes1) != len(codes2) {
		panic("code format error, code1-length: " + fmt.Sprint(len(codes1)) + " formatted length: " + fmt.Sprint(len(codes2)))
	}

	if debug {
		println("-------Runtime Opcode-------------")
		e2.VM.DebugExec()
	} else {
		e2.VM.NormalExec()
	}
	return codes1
}

func _formatCodeTest(i string) (string, string) {
	e := NewExecutor(i)
	codes1 := e.VM.GetCodes()
	code2 := e.Compiler.GetFormattedCode()
	e2 := NewExecutor(code2)
	codes2 := e2.VM.GetCodes()
	if len(codes1) != len(codes2) {
		println("------------------------------------")
		_printWithLine(i)
		println("------------------------------------")
		_printWithLine(code2)
		println("---------------CODE1------------------")
		yakvm.ShowOpcodes(codes1)
		println("---------------CODE2------------------")
		yakvm.ShowOpcodes(codes2)
		panic("code format error, code1-length: " + fmt.Sprint(len(codes1)) + " formatted length: " + fmt.Sprint(len(codes2)))
	}
	return i, code2
}

func TestNewExecutor_YakFuncCallPanic(t *testing.T) {
	code := `
	func t(a, b) {
		dump(1)
	}
	t()
`
	test1 := func() {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic")
			}
		}()
		_marshallerTest(code)
	}

	test2 := func() {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic")
			}
		}()
		_formattest(code)
	}
	test1()
	test2()
}

func TestNewExecutor6(t *testing.T) {
	code := "{" +
		"\n \n" +
		"" +
		"     \n" +
		"" +
		"" +
		"\r\r\r\r\n" +
		"}"
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor66(t *testing.T) {
	code := "dump(\"\", \"\",\r\nsprint(\"\"))"
	e := NewExecutor(code)
	e.VM.NormalExec()
}

func TestTypedLiteral(t *testing.T) {
	code := `
a = map[string]int{"1":2, "3": 4}
assert typeof(a) == map[string]int
assert len(a) == 2
assert a["1"] == 2
assert a["3"] == 4
b = map[string]int{}
assert typeof(b) == map[string]int
assert len(b) == 0

c = []int{}
assert typeof(c) == []int
assert len(c) == 0
d = []int{1,2,3}
assert typeof(d) == []int
assert len(d) == 3
assert d[0] == 1
assert d[1] == 2
assert d[2] == 3
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4(t *testing.T) {
	codes := `
assert libx.abc() == "called-abc"
dump("基础语句测试")
// hello
// Comment
/*123123
qweqwe*/
# 123
var a,b,c,d
assert a == undefined
var a = 1
assert a == 1
fn{
    var a =3
assert a == 3
}
assert a == 1, sprint(a)
for range 10{
    var a =3
assert a == 3
}
assert a == 1, sprint(a)
if true{
    var a =3
assert a == 3
}
assert a == 1, sprint(a)
switch {
case true:
a := 4
assert a == 4
}
assert a == 1, sprint(a)
// define expression
assert []byte("abc") == "abc"
assert typeof([]byte("abc")) == []byte 
assert !true == false
assert !nil == true
assert !undefined == true
assert -19 == 1-20
assert []byte == [  ]byte
assert []byte == [] byte
assert var == var
assert string != []byte
assert ()=>{return 1}() == 1
assert ()=>{return}() == undefined
assert ()=>{return}() == nil
assert func(){return 1}() == 1
assert func(){return 1+1}() == 2
assert func(){return}() == undefined
assert func(){return}() == nil
assert fn(){return 1}() == 1
assert fn(){return 1+1}() == 2
assert fn(){return}() == undefined
assert fn(){return}() == nil
assert def(){return 1}() == 1
assert def(){return 1+1}() == 2
assert def(){return}() == undefined
assert def(){return}() == nil
b := def(){
    var a = 3
    defer func{
        a = recover()
    }
    panic(7)
    return a
}
c = b()
assert undefined == undefined
dump(c)
dump(undefined)
assert c == undefined, c
ida
ab
asdf
asd
assert 0b10 == 2
assert 0xa == 10
assert 0xff == 255
assert true != false
assert true == true
assert false == false
assert 'a'=="abc"[0]
assert 'c' == 'a' + 2
assert 0xa == 0o12
assert 10 == 0o12
assert .1123 == 0.1123
abc = 123
assert abc == 123
f =  f"abc: ${123}"
assert f == "abc: 123"
e = "abc$"
dump(e)
assert "abc$" == e
assert undefined == nil
assert nil == undefined
c = {}
assert c["masdfasjmkasdjklf"] == nil
assert c["masdfasjmkasdjklf"] == undefined
d = {1:23, "bbb": a}
e = 1
assert d[1] == 23
assert d.bbb == a
a = [1,2,3,4,5]
assert a[1] == 2
assert a[-1] == 5
a.Append(4)
assert a[-1] == 4
e = c => c +1
assert e(5) == 6
e = () => 5 +1
assert e() == 6
e = (a...) => a[0]+1
assert e(5) == 6
e = (c, a...) => c+1
assert e(5) == 6
e = fn(c, a...){return c+1+a[0]+a[1]+a[2]}
assert e(5, 1,2,3) == 6 + 3 + 1 + 2
e = (c, a...) => c+1+a[0]+a[1]+a[2]
assert e(5, 1,2,3) == 6 + 3 + 1 + 2
assert (1+3)*2 == 8
l = make([]var, 30,30)
l.Append(123)
assert l[-1] == 123
assert (1+3)*2 == 8
l = make([]var, 30)
l.Append(123)
assert l[-1] == 123
c = make(chan var)
go fn{c<-123}
result := <-c
dump(result)
assert result == 123
a = 1
b = 1
a += true ? 1:0
b += false ? 1:0
assert a == 2
assert b == 1
a = 1
b = 1
a += true ? false? 0: 1:0
b += false ? true ? 1:123 : false?1:0
assert a == 2
assert b == 1
a = []var{1,2,3,"asdfasdf"}
dump(a)
a = []byte{1,2,3,4}
assert a[-1] == 4
b = []string{b"123123", "12312"}
assert b[-2] == "123123"
assert []int{1,2,3} == [1,2,3]
abc = {1:123, "aa":"bb"}
key = 1
abc.$key = 1234
assert abc[1] == 1234
for i = 0; i < 10; dump(1) {
    i++
    assert 017 == 15
}
    assert 017 == 15
assert "\"" == ` + "`" + `"` + "`" + `
a = 1
assert a == 1
a := 2
assert a == 2
a++
assert a == 3
a,b = 123,333
assert a == 123 && b == 333
a = [1,2,3]
a[0] = 2
// a[0]++
assert a[0] == 2
a[0], a[1] = 222,333
assert a[0] == 222 && a[1] == 333
a[0]+=1
assert a[0] == 223
a = {"abc":2}
a.abc++
assert a.abc == 3
getMember = func() {
    var a = {"abc": 2}
    defer fn{
        err := recover()
dump(err)
        assert err != nil
    }
    a.bbb
    assert false, "getMember panic test failed"
}
getMember()
a = 312
b
{
    var a = 331
    assert a == 331
}
assert a == 312
;;;;;;;;;;;;
;;;;;;;;;;;;
;;;;;;;;;;;;
;;;;;;;;;;;;
a = 123
b = 123
if a == 123 {
    a ++
} elif b == 123 {
    a++
}else{
    a++
}
assert a == 123+1
a = 125
b = 123
if a == 123 {
    a ++
} elif b == 123 {
    a++
}else{
    a++
}
assert a == 125+1
a = 125
b = 126
if a == 123 {
    a ++
} elif b == 123 {
    a++
}else{
    a++
}
dump(a)
assert a == 125+1
a = 123
if a == 123 {
    a+=1
}else{
    a+=2
}
assert a == 124
a = 123
if a == 1 {
    a+=1
    var a = 12333
}
assert a == 123
a = 3
c = 44
switch {
case true:
a++
    var c = 11111
    fallthrough
case false:
a+=2
//assert c == 11111
    break
    a++
default:
a++
}
assert a == 6
assert c == 44
c = 1
switch 1 {
case 2-1:
    c = 333
assert c == 333
    var c = 2
assert c == 2
}
assert c == 333
c = 1
switch 1 {
case 2-1:
    c = 333
assert c == 333
    var c = 2
assert c == 2
default:
    c++
}
assert c == 333
/*
    for-range
*/
var a = 1
for range 5 {
    if _ == 0 {
        a = 4
        var a = 333
    }
}
assert a == 4
i := 1111
for i = 1; i < 20; i++{}
assert i == 20
i := 1111
for i := 1; i < 20; i++{}
assert i == 1111
a = 1
for{
    a++
    if a > 10 {
break
}
}
assert a == 11
a = 1
for ;a>1;a++ {
    
    if a > 10 {
break
}
}
assert a == 1
i := 1111
for ; i < 20; i++{dump(i)}
assert i == 1111
assert fn{return 1} == 1
assert fn{if false {return 1} else {return 2}} == 2
c := make(chan int)
go func{c<-1}
assert <-c == 1
a = make(map[var]var)
for range 10 {
    go fn{a[_]=_}
}
sleep(0.3)
assert len(a) < 10
a = 0
for i:=0;i<10;i++{
    if i > 4 {  break }
a++ ;
}
assert a == 5,a 
a,b = [1, "abc"]
assert a == 1
assert b == "abc"
`
	_marshallerTest(codes)
	_formattest(codes)
}

func TestNewExecutor4_ARROWFuncAndDefinition(t *testing.T) {
	code := `
	a = 1
	dump(a)
	{
		a = 3
		assert a == 3
		var a = 56
		assert a != 3
	}
	dump(a)
	assert a == 3
	a = 1
	{
		a = 3
		assert a == 3
		a := 56
		assert a != 3
	}
	dump(a)
	assert a == 3
	a = 1
	{
		a = 3
		assert a == 3
		var a = 56
		assert a != 3
	}
	dump(a)
	assert a == 3
	a = 1
	{
		a = 3
		assert a == 3
		a = 56
		assert a != 3
	}
	dump(a)
	assert a != 3
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4_FuncAndDefinition(t *testing.T) {
	code := `a = (b)=>{return b+1}
    assert a(1) == 2
a := 3
    assert a == 3
    dump("START DEF B as arrow func")
c = 1
    b = ()=>{
        assert a == 3, "in func"
        var a = 5
dump(a)
        assert a == 5
c++
    }
assert c == 1
    b()
assert c == 2
dump(a)
assert a == 3`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_DeclareVariables(t *testing.T) {
	code := `
var a,b,c,d
assert a == undefined
assert b == undefined
assert c == undefined
assert !d == true
var a = 1
assert a == 1
a = 2
c = 3`
	_marshallerTest(code)
	_formattest(code)
}

var ftest = func(code string) {
	e := NewExecutor(code)
	formattedcode := e.Compiler.GetFormattedCode()
	println(formattedcode)
}

func TestNewExecutor_Formatter3(t *testing.T) {
	code := `func A(a /* type A */,
		// abc
		b /* 123 */, 
		) {
		b = 1 + 1
		return b
	}`
	ftest(code)
}

func TestNewExecutor_Formatter2(t *testing.T) {
	Import("p", func(v ...interface{}) {
		fmt.Println(v...)
	})

	// 超出长度换行的情况
	code := `p("aaaaaaaaaaaaaa", "bbbbbbbbbbbbbb", "cccccccccccccc", "ddddddddddddddd", "eeeeeeeeeeeeeee", "fffffffffffffffff", "ggggggggggggggggg", "hhhhhhhhhhhhhhhhhh")`
	ftest(code)
	// 每个参数一行的情况
	code = `p("longlonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglong", "bbbbbbbbbbbbbb", "cccccccccccccc", "ddddddddddddddd", "eeeeeeeeeeeeeee", "fffffffffffffffff", "ggggggggggggggggg", "hhhhhhhhhhhhhhhhhh")`
	ftest(code)
	// 超出长度换行的情况带...的情况
	code = `p("aaaaaaaaaaaaaa", "bbbbbbbbbbbbbb", "cccccccccccccc", "ddddddddddddddd", "eeeeeeeeeeeeeee", "fffffffffffffffff", "ggggggggggggggggg", vvv...)`
	ftest(code)
	code = `p("longlonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglong", "bbbbbbbbbbbbbb", "cccccccccccccc", "ddddddddddddddd", "eeeeeeeeeeeeeee", "fffffffffffffffff", "ggggggggggggggggg", vvv...)`
	ftest(code)
}

func TestNewExecutor_Formatter(t *testing.T) {
	// include
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	file.WriteString(`
abc = func(){
    return "test"
}
`)
	defer os.Remove(file.Name())
	includeTestCase := fmt.Sprintf(`include"%s"`, file.Name())
	includeExpected := fmt.Sprintf(`include "%s"`, file.Name())

	// func with name
	funcTestCase := `func abc(){a=1
a=2}`
	funcExpected := `func abc() {
    a = 1
    a = 2
}`

	// if
	ifTestCase := `if a==1{a=1
b=2} elif a==2{a=2
b=3} else{a=3}`
	ifExpected := `if a == 1 {
    a = 1
    b = 2
} elif a == 2 {
    a = 2
    b = 3
} else {
    a = 3
}`
	switchTestCase := `switch abc{
case 1,2,3:
f++
fallthrough
case false:
f++
break
default:
f++
}`
	switchExpected := `switch abc {
case 1, 2, 3:
    f++
    fallthrough
case false:
    f++
    break
default:
    f++
}`

	switchTestCase2 := `i => {
    switch i {
        case "abc":
        println(i)
        default:
        println(i)
    }
}`
	switchExpected2 := `i => {
    switch i {
    case "abc":
        println(i)
    default:
        println(i)
    }
}`
	forTestCase := `for i=0;i<5;i++ {print(i)}`
	forExpected := `for i = 0; i < 5; i++ {
    print(i)
}`
	forRangeTestCase := `for i=range [1,2,3] {print(i)}`
	forRangeExpected := `for i = range [1, 2, 3] {
    print(i)
}`
	forRangeTestCase2 := `for i in[1,2,3] {print(i)
continue
}`
	forRangeExpected2 := `for i in [1, 2, 3] {
    print(i)
    continue
}`
	goTestCase := `go fn{print("test")}`
	goExpected := `go fn { print("test") }`
	goTestCase2 := "go func(){print(\"test\")}()\n"
	goExpected2 := "go func() { print(\"test\") }()"
	deferTestCase := `defer func(){print("test")}()`
	deferExpected := `defer func() { print("test") }()`
	deferTestCase2 := `defer println(1)
println(2)`
	deferTestExpected2 := deferTestCase2
	inlineTestCase := `abc = func() {return "short"}`
	inlineExpected := `abc = func() { return "short" }`
	inlineTestCase2 := `abc = func() {return "longlonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglong"}`
	inlineExpected2 := `abc = func() {
    return "longlonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglonglong"
}`
	assertTestCase := `assert a==1`
	assertExpected := `assert a == 1`
	assertTestCase2 := `assert a==1,"test"`
	assertExpected2 := `assert a == 1, "test"`
	multiBlockTestCase := `fn {{a = 1;b = 2}}`
	multiBlockExpected := `fn {
    {
        a = 1
        b = 2
    }
}`

	testcases := map[string]string{
		`// qwe
		a+1`: `// qwe
a + 1`,
		`//   asd
		a+1`: `//   asd
a + 1`,
		`/* qwe */
		b=1+1`: `/* qwe */
b = 1 + 1`,
		`b=1+1// test`:          `b = 1 + 1// test`,
		`"123"`:                 `"123"`,
		`x"123"`:                `x"123"`,
		`1.5`:                   `1.5`,
		`true`:                  `true`,
		`undefined`:             `undefined`,
		`nil`:                   `nil`,
		`'a'`:                   `'a'`,
		`[1,2,3]`:               `[1, 2, 3]`,
		`{"a":"b","c":"d",5:6}`: `{"a": "b", "c": "d", 5: 6}`,
		`print(a)`:              `print(a)`,
		`int( 1.5)`:             `int(1.5)`,
		`panic("qwe")`:          `panic("qwe")`,
		`recover()`:             `recover()`,
		`i=>i+1`:                `i => i + 1`,
		`(a, b, c)=>i+1`:        `(a, b, c) => i + 1`,
		`fn (i) {1 +  1
2 - 1}`: `fn(i) {
    1 + 1
    2 - 1
}`,
		`a<-1`:                     `a <- 1`,
		`a in b`:                   `a in b`,
		`make([]int,1)`:            `make([]int, 1)`,
		`make(map[string]int,0,0)`: `make(map[string]int, 0, 0)`,
		`make(chan int,0,0)`:       `make(chan int, 0, 0)`,
		`a[1]`:                     `a[1]`,
		`a[1:]`:                    `a[1:]`,
		`a[1:2]`:                   `a[1:2]`,
		`a[1:2:3]`:                 `a[1:2:3]`,
		`a[1::1]`:                  `a[1::1]`,
		`true && false`:            `true && false`,
		`true && (false || false)`: `true && (false || false)`,
		`true?1:0`:                 `true ? 1 : 0`,
		`fn{1 +  1}`:               `fn { 1 + 1 }`,
		`func{1 +  1}`:             `func { 1 + 1 }`,
		`a=1`:                      `a = 1`,
		`a:=1`:                     `a := 1`,
		`a,b,c=1,2,3`:              `a, b, c = 1, 2, 3`,
		`a ++`:                     `a++`,
		`a --`:                     `a--`,
		`a+=1`:                     `a += 1`,
		`a[1]=1`:                   `a[1] = 1`,
		`a.b=1`:                    `a.b = 1`,
		includeTestCase:            includeExpected,
		funcTestCase:               funcExpected,
		ifTestCase:                 ifExpected,
		switchTestCase:             switchExpected,
		switchTestCase2:            switchExpected2,
		forTestCase:                forExpected,
		forRangeTestCase:           forRangeExpected,
		forRangeTestCase2:          forRangeExpected2,
		goTestCase:                 goExpected,
		goTestCase2:                goExpected2,
		deferTestCase:              deferExpected,
		deferTestCase2:             deferTestExpected2,
		inlineTestCase:             inlineExpected,
		inlineTestCase2:            inlineExpected2,
		assertTestCase:             assertExpected,
		assertTestCase2:            assertExpected2,
		multiBlockTestCase:         multiBlockExpected,
	}
	for testcase, expected := range testcases {
		inputStream := antlr.NewInputStream(testcase)
		lex := yak.NewYaklangLexer(inputStream)
		tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
		p := yak.NewYaklangParser(tokenStream)
		vt := yakast.NewYakCompiler()
		vt.AntlrTokenStream = tokenStream
		p.AddErrorListener(vt.GetParserErrorListener())
		vt.VisitProgram(p.Program().(*yak.ProgramContext))
		if len(vt.GetErrors()) <= 0 {
			for _, v := range vt.GetErrors() {
				t.Errorf("source: %#v, error: %s", testcase, v.Message)
			}
		}
		if vt.GetFormattedCode() != expected {
			t.Logf("got:\n%#v\n-------------\nexpected:\n%#v", vt.GetFormattedCode(), expected)
			t.Errorf("input:\n%s\n-------------\ngot:\n%s\n-------------\nexpected:\n%s", testcase, vt.GetFormattedCode(), expected)
		}
	}
}

//func TestNewExecutor_SyntaxError(t *testing.T) {
//	executor := NewExecutor(`
//a = 1 if )
//eval(` + "`" + `eval("assert a == 1")` + "`" + `)
//`)
//	if len(executor.Compiler.GetErrors()) <= 0 {
//		panic("syntax error parse failed")
//	}
//}

func TestNewExecutor_Include(t *testing.T) {
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	includeCode := `
abc = func(){
	return "test"
}
`

	file.WriteString(includeCode)
	defer os.Remove(file.Name())

	code := fmt.Sprintf(`
	include "%s"
	assert abc() == "test"
	`, file.Name())

	_marshallerTest(code)
	codes := _formattest(code)

	checkFilePath, checkCode := *codes[0].SourceCodeFilePath, *codes[0].SourceCodePointer
	if checkFilePath != file.Name() {
		t.Fatalf("include file path error, expected: %s, got: %s", file.Name(), checkFilePath)
	}
	if checkCode != includeCode {
		t.Fatalf("include file code error, expected: %#v, got: %#v", includeCode, checkCode)
	}
}

func TestNewExecutor_Eval(t *testing.T) {
	code := `
	a = 1
	eval(` + "`" + `eval("assert a == 1")` + "`" + `)
`
	_formattest(code)
	// todo: frame没有注入eval函数，所以不进行测试
	// _marshallerTest(code)
}

func TestNewExecutor_StructPtr(t *testing.T) {
	code := `
	testIns.A += 0
	testIns.A.Append(5)
	assert testIns.A[-1] == 5
	`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_SlicePush(t *testing.T) {
	code := `
a = [1,2,3]
a.Append(4)
dump(a)
assert a[3] == 4
b = {"abc": [1,2,3]}
b.abc.Append(5)
assert b.abc[3] == 5
c = [[1,2,3], 1, 2, 3]
c.Append(5)
c[0].Append(7777)
assert c[0][-1] == 7777
assert c[-1] == 5
dump(c)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Recover_PanicStack(t *testing.T) {
	code := `
a = 1
defer fn(){
assert a == 1
	dump("Start RECOVER")
	dump("END RECOVER")
}()
dump("Start PANIC")
dump("END PANIC")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Recover1(t *testing.T) {
	code := `
a = 1
defer fn(){
assert a == 1
	dump("Start RECOVER")
	assert recover()+1 == 2
	dump("END RECOVER")
}()
dump("Start PANIC")
panic(1)
dump("END PANIC")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Recover2(t *testing.T) {
	code := `
a = 1
defer fn(){
	dump("Start RECOVER")
assert a == 1
	err = recover()
	if err {dump(err)}else{dump("----------")}
	dump("END RECOVER")
}()
dump("Start PANIC")
b = a+1
dump("END PANIC")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Recover3(t *testing.T) {
	code := `
a = 1
defer fn(){
    defer fn{ 
		dump("defer in defer") 
		assert recover() != undefined  
	}
    dump("Start RECOVER")
	assert a == 2
    err = recover()
    if err {dump(err)}else{dump("----------")}
    dump("END RECOVER")
}()
dump("Start PANIC")
b = a+1
dump("END PANIC")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Recover4(t *testing.T) {
	code := `
a = 1
defer fn{
    dump("defer2", a)
    assert a == 2
    assert recover() == nil
}
defer fn{
    defer fn{assert recover() == 2}
    dump("defer1", a)
    a = a+1
    panic(2)
    dump(a)
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_BlockTest(t *testing.T) {
	code := `
a=1
dump("We are out block(PRE)")
{
    assert a == 1
    b = 23
    dump("We are in block")
}
dump("We are out block")
assert b != 23
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Array_MemberCall(t *testing.T) {
	Import("ptr", func(i interface{}) {
		fmt.Printf("%p", i)
	})
	code := `
	a = [1, "abc", "ccc"]
	assert a.Shift() == 1
	assert a.Len() == 2
	assert a.Shift() == "abc"
	assert a.Len() == 1
	a.Unshift("dddd")
	assert a.Len() == 2, a.Len()
	assert a[0] == "dddd"
	dump(a)
	a.Push("eee")
	assert a.Len() == 3
	assert a.Pop() == "eee"
	assert a.Len() == 2
	dump(a)
	assert a.Filter(c => c.HasPrefix("ddd")).Len() == 1
	assert a.Filter(c => c.HasPrefix("ccc")).Len() == 1
	bbb := a.Map(c => c.HasPrefix("ccc"))
	assert bbb.Pop() == true 
	assert bbb.Shift() == false 
	assert a.Map(b => b + "abc").Filter(c => c.HasSuffix("abc")).Len() == 2
	b := make([]var, 10, 10)
	assert b.Cap() == 10
	assert b.Capability() == 10
	b = [1,3,3]
	b.Push("abc")
	assert typeof(b) == []var
	assert typeof(b.StringSlice()) == []string
	assert typeof(b.GeneralSlice()) == []var
	assert typeof(b.GeneralSlice()) != []string
	assert typeof(b.StringSlice().GeneralSlice()) == []var
	a,b,c,d := b.StringSlice()
	assert a == "1"
	assert b == "3"
	assert c == "3"
	assert d == "abc"
	
	a = [1, 2, 3]
	a.Append(4)
	assert a == [1, 2, 3, 4], sprint(a)
	a.Extend([5, 6])
	assert a == [1, 2, 3, 4, 5, 6], sprint(a)
	// 
	a = [1, 2, 3, 4]
	v = a.Pop()
	assert a == [1, 2, 3], sprint(a)
	assert v == 4, v
	v = a.Pop(1)
	assert a == [1, 3], sprint(a)
	assert v == 2, v
	v = a.Pop(99999)
	assert a == [1], sprint(a)
	assert v == 3, v
	a = [1, 2, 3, 4, 5]
	v = a.Pop(-2)
	assert a == [1, 3, 4 ,5], sprint(a)
	assert v == 2, v
	v = a.Pop(-999999)
	assert a == [1, 3, 4], sprint(a)
	assert v == 5, v
	a.Insert(1, 2)
	assert a == [1, 2, 3, 4], sprint(a)
	a.Insert(999, 5)
	assert a == [1, 2, 3, 4, 5], sprint(a)
	a.Insert(-1, 999)
	assert a == [1, 2, 3, 4, 999, 5], sprint(a)
	a.Insert(-9999, 0)
	assert a == [0, 1, 2, 3, 4, 999, 5], sprint(a)
	//
	a = [1, 2, 1]
	a.Remove(1)
	assert a == [2, 1], sprint(a)
	a.Remove(1)
	assert a == [2], sprint(a)
	//
	a = [1, 2, 3, 4]
	a.Reverse()
	assert a == [4, 3, 2, 1], sprint(a)
	a = [1, 2, 3, 4, 5]
	a.Reverse()
	assert a == [5, 4, 3, 2, 1], sprint(a)
	//
	a = [4, 1, 3, 2]
	a.Sort()
	assert a == [1, 2, 3, 4], sprint(a)
	a = [4, 1, 3, 2]
	a.Sort(true)
	assert a == [4, 3, 2, 1], sprint(a)
	//
	a = [1, 2, 3]
	a.Clear()
	assert a == [], sprint(a)
	//
	a = [1, 2, 3, 1]
	assert a.Count(1) == 2, a.Count(1)
	assert a.Count(5) == 0, a.Count(5)
	//
	a = [1, 2, 3, 4]
	assert a.Index(0) == 1, a.Index(0)
	assert a.Index(2) == 3, a.Index(3)
	assert a.Index(9999) == 4, a.Index(9999)
	assert a.Index(-1) == 4, a.Index(-1)
	assert a.Index(-9999) == 1, a.Index(-9999)
	`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_String_MemberCall(t *testing.T) {
	code := `
a = "a123{{rs(10,20,10)}}";
assert a.First() == 'a'
dump(a.Fuzz())
assert len(a.Fuzz()[0]) >= 14 && len(a.Fuzz()[0]) <= 24 
assert len(a.Shuffle()) == 20
assert a.First() == 'a'
assert "abcdefg".Reverse() == "gfedcba"
assert "abcabc".Contains("abc") == true
assert "abcabc".Contains("qwe") == false
assert "abcabc".ReplaceN("abc", "123", 1) == "123abc"
assert "abcabc".Replace("abc", "123") == "123123"
assert "abcabc".ReplaceAll("abc", "123") == "123123"
assert "abc1abc".Split("1") == ["abc", "abc"]
assert "abc1abc1abc".SplitN("1", 2) == ["abc", "abc1abc"]
assert "1".Join(["abc", "abc"]) == "abc1abc"
assert "pabcp".Trim("p") == "abc"
assert "pabc".TrimLeft("p") == "abc"
assert "abcp".TrimRight("p") == "abc"
assert "abcdefg".HasPrefix("abc") == true
assert "abcdefg".HasSuffix("efg") == true
assert "abc".Zfill(5) == "00abc"
assert "abc".Zfill(2) == "abc"
assert "abc".Rzfill(5) == "abc00"
assert "abc".Rzfill(2) == "abc"
assert "abc".Ljust(5) == "abc  "
assert "abc".Ljust(2) == "abc"
assert "abc".Rjust(5) == "  abc"
assert "abc".Rjust(2) == "abc"
assert "abcabc".Count("abc") == 2
assert "abcabc".Count("qwe") == 0
assert "abcabc".Find("abc") == 0
assert "abcabc".Find("qwe") == -1
assert "abcabc".Rfind("abc") == 3
assert "abcabc".Rfind("qwe") == -1
assert "ABC".Lower() == "abc"
assert "abc".Upper() == "ABC"
assert "abc".Title() == "Abc"
assert "ABC".IsLower() == false
assert "abc".IsLower() == true
assert "ABC".IsUpper() == true
assert "abc".IsUpper() == false
assert "abc".IsTitle() == false
assert "Abc".IsTitle() == true
assert "abc".IsAlpha() == true
assert "abc1".IsAlpha() == false
assert "abc".IsDigit() == false
assert "123".IsDigit() == true
assert "abc".IsAlnum() == true
assert "abc1".IsAlnum() == true
assert "abc1 ".IsAlnum() == false
assert "abc".IsPrintable() == true
assert "abc1 ".IsPrintable() == true
assert "abc1 \xff".IsPrintable() == false`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Bytes_Bin_Op(t *testing.T) {
	code := `
assert b"qwe" + b"asd" == b"qweasd"
fn {
	defer fn {
		if recover() == nil {
			panic("should be error")
		}
	}
	b"qwe" + "asd"
}
assert b"qwe" * 3 == b"qweqweqwe"
assert b"qwe%s" % "asd" == b"qweasd"
assert b"qwe%s" % ["asd"] == b"qweasd"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Bytes_MemberCall(t *testing.T) {
	code := `
a = b"a123{{rs(10,20,10)}}";
assert a.First() == 'a'
dump(a.Fuzz())
assert len(a.Fuzz()[0]) >= 14 && len(a.Fuzz()[0]) <= 24 
assert len(a.Shuffle()) == 20
assert a.First() == 'a'
assert b"abcdefg".Reverse() == b"gfedcba"
assert b"abcabc".Contains(b"abc") == true
assert b"abcabc".Contains(b"qwe") == false
assert b"abcabc".ReplaceN(b"abc", b"123", 1) == b"123abc"
assert b"abcabc".Replace(b"abc", b"123") == b"123123"
assert b"abcabc".ReplaceAll(b"abc", b"123") == b"123123"
assert b"abc1abc".Split(b"1") == [b"abc", b"abc"]
assert b"abc1abc1abc".SplitN(b"1", 2) == [b"abc", b"abc1abc"]
assert b"1".Join([b"abc", b"abc"]) == b"abc1abc"
assert b"pabcp".Trim(b"p") == b"abc"
assert b"pabc".TrimLeft(b"p") == b"abc"
assert b"abcp".TrimRight(b"p") == b"abc"
assert b"abcdefg".HasPrefix(b"abc") == true
assert b"abcdefg".HasSuffix(b"efg") == true
assert b"abc".Zfill(5) == b"00abc"
assert b"abc".Zfill(2) == b"abc"
assert b"abc".Rzfill(5) == b"abc00"
assert b"abc".Rzfill(2) == b"abc"
assert b"abc".Ljust(5) == b"abc  "
assert b"abc".Ljust(2) == b"abc"
assert b"abc".Rjust(5) == b"  abc"
assert b"abc".Rjust(2) == b"abc"
assert b"abcabc".Count(b"abc") == 2
assert b"abcabc".Count(b"qwe") == 0
assert b"abcabc".Find(b"abc") == 0
assert b"abcabc".Find(b"qwe") == -1
assert b"abcabc".Rfind(b"abc") == 3
assert b"abcabc".Rfind(b"qwe") == -1
assert b"ABC".Lower() == "abc"
assert b"abc".Upper() == "ABC"
assert b"abc".Title() == "Abc"
assert b"ABC".IsLower() == false
assert b"abc".IsLower() == true
assert b"ABC".IsUpper() == true
assert b"abc".IsUpper() == false
assert b"abc".IsTitle() == false
assert b"Abc".IsTitle() == true
assert b"abc".IsAlpha() == true
assert b"abc1".IsAlpha() == false
assert b"abc".IsDigit() == false
assert b"123".IsDigit() == true
assert b"abc".IsAlnum() == true
assert b"abc1".IsAlnum() == true
assert b"abc1 ".IsAlnum() == false
assert b"abc".IsPrintable() == true
assert b"abc1 ".IsPrintable() == true
assert b"abc1 \xff".IsPrintable() == false`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Switch_MultiExpress_And_Or(t *testing.T) {
	code := `
a = 1
if a > 3 {
assert false
} elif a == 3 {
    assert false, "IF ELIF ERROR"
} elif a == 1 {
    a++
}else{
    assert false
}
assert a == 2
a = 1
b = 1
switch a {
case 1:
    b++
gg = 3
    fallthrough
case 2:
gg = 6
    b++
}
assert b == 3
assert gg == undefined
// 测试 fallthrough
f = 0
switch {
case 1:
    f++
    fallthrough
case false:
    f++
    break
default:
    f++
}
assert f == 2
c, d = 0, 0
dump(c,d)
switch {
case 0:
case 2,3,4,5,6,true,fn{d++}:
    c++
}
assert c == 1, sprint(c)
assert d == 0
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Typeof(t *testing.T) {
	code := `
a = 1
assert typeof(a) == int
b = make([]int, 0)
assert typeof(b) == []int
c = make(map[string]string, 0)
assert typeof(c) == map[string]string
d = make(chan int, 0)
assert typeof(d) == chan int
e = make(map[string][]int, 0)
assert typeof(e) == map[string][]int
f = make(map[string][]string, 0)
assert typeof(f) == map[string][]string
g = make(chan map[string][]string, 0)
assert typeof(g) == chan map[string][]string
h = make(chan var, 0)
assert typeof(h) == chan var
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Switch(t *testing.T) {
	code := `
g = 0
switch 1 {
case 1:
    g += 1
case 2:
    g += 2
default:
    g = -1
}
assert g == 1
// or case
g = 0
switch 1 {
case 1, 2:
    g += 2
case 2:
    g += 3
default:
    g = -1
}
assert g == 2
// empty switch expr
g = 0
switch{
case true:
    g += 1
}
assert g == 1
// short circult
g = 0
switch 1 {
case 1, fn{g++}:
    g += 1
case 2:
    g += 2
default:
    g = -1
}
assert g == 1
// default
g = 0
switch 3 {
case 1, 2:
    g += 1
case 2:
    g += 2
default:
    g += 3
}
assert g == 3
// without default
g = 0
switch {
case false, fn{g++}:
    g += 2
    break
}
assert g == 1
// fallthrough
g = 0
switch 1 {
case 1:
    g += 1
    fallthrough
case 2:
    g += 1
default:
    g = -1
}
assert g == 2
// last fallthrough
g = 0
switch 1 {
case 2:
    g += 1
case 2:
    g += 1
    fallthrough
default:
    g = -1
}
assert g == -1
// break
g = 0
switch 1 {
case 1:
    g += 1
    break
case 2:
    g += 1
default:
    g = -1
}
assert g == 1
`
	_marshallerTest(code)
	_formattest(code)
}

// func TestNewExecutor4_Goroutine(t *testing.T) {
// 	//var a = 1
// 	//go func() {
// 	//  time.Sleep(time.Second)
// 	//  println(a)
// 	//}()
// 	//time.Sleep(200 * time.Millisecond)
// 	//a = 2
// 	//time.Sleep(time.Second)

// 	for i := 0; i < 3; i++ {
// 		i := i
// 		go func() {
// 			time.Sleep(time.Millisecond * 300)
// 			println(i)
// 		}()
// 	}
// 	//for i := range make([]int, 3) {
// 	//  i := i
// 	//  go func() {
// 	//      time.Sleep(time.Millisecond * 300)
// 	//      println(i)
// 	//  }()
// 	//  i++
// 	//}
// 	time.Sleep(500 * time.Millisecond)

// 	//factory := func() func() {
// 	//  time.Sleep(time.Second)
// 	//  println("in factory")
// 	//  return func() {
// 	//      time.Sleep(time.Second)
// 	//      println("in function")
// 	//  }
// 	//}
// 	//
// 	//println("before go")
// 	//go factory()()
// 	//println("after go")
// 	//time.Sleep(2 * time.Second)
// }

func TestNewExecutor4_Scope_Go_Assign(t *testing.T) {
	code := `
a = 1
for a < 10000 {
    a++
    if a % 2000 == 0{ dump(a)}
    continue
}
dump(a)
// assert a == 10000
dump("finished")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4_Scope_Go(t *testing.T) {
	code := `
for a, b = range 4 {
    dump(a,b)
}
for a, b = range [1,2,3,4,5] {
    dump(a,b)
}
for a, b = range {1:1, 2:2, 3:3} {
    dump(a,b)
}
a = 0
for range 3 {
for range 10 {
a++
}
}
assert a == 30
a = 0
a = 1
go def{
    a = 3
}
sleep(0.3)
assert a == 3
a = {1:2}
assert a[1] == 2
a[1] = 3
assert a[1] == 3
a = 1
a++
a++
go func{a++;a++}
sleep(0.1)
assert a == 5
a = 1;
a++
go func{
    dump(a)
    assert a == 2
}
sleep(0.3)
ss = make(map[int]int)
for i = range 3 {
    go func(i){
        ss[i]=i
    }(i)
}
sleep(0.2)
dump(ss)
assert len(ss) == 3, "got %v" % len(ss)
ss = make(map[int]int)
for i = range 3 {
    go func{ss[i]=i}
}
sleep(0.2)
dump(ss)
assert len(ss) != 3, "got %v" % len(ss)
for i = range 3 {
}
go print("finished\n")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4_Scope(t *testing.T) {
	code := `a = 1
b = func(c){
    assert a == 1
    assert c == 1
    d = 3
    c = 4
}
b(a)
assert d == undefined
assert a == 1
assert c == undefined
dump("函数定义域一切正常")
s = make(map[int]int)
for index in 4 {
    s[index] = index
}
assert len(s) == 4
assert s[0] == 0
dump("For 单独使用定义域一切正常")
e = i => i+1
for i in 4 {
    assert e(i) == i+1
}
assert i == undefined
f = 1
f++
assert f == 2
go func{f++}
sleep(0.2)
assert f == 3
s = make(map[int]int)
for index = range 4 {
    func{
        s[index]=index
    }
}
sleep(0.3)
dump(s)
assert len(s) == 4
dddd = 123
c = func(ccc, dddd){
    dump(ccc)
    assert ccc == 1
    assert dddd == 23
eeee = 123
    return ccc
}
assert c(1, 23) == 1
assert ccc == undefined
assert dddd == 123
assert eeee != 123
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4_OpGo_N(t *testing.T) {
	code := `
a = 1
b = a
assert b == 1
a++
assert b == 1
assert a == 2
index2 = 1
for index3 = range 4 {}
assert index2 == 1, "index2 被 for 覆盖"
assert index3 == undefined, "index3 泄漏"
a = {}
for index = range 4 {
    c = index;
    go func(d){ 
        a[sprint(d)]=d
    }(c)
}
sleep(0.5)
dump(a)
assert len(a) == 4, "for + goroutine 无法使用新符号"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor4_OpGo(t *testing.T) {
	code := `
a = 1
assert a == 1
go func{
    assert a == 1
    dump("enter goroutine")
    sleep(0.5)
    a = 2
    assert a == 2, "goroutine 内部执行错误"
    dump("exit goroutine")
}
assert a == 1
sleep(1)
dump(a)
assert a == 2, "go定义域有问题", dump(a)
go func(){
    sleep(0.5)
    a++
}()
assert a == 2
sleep(0.6)
assert a == 3
go fn{a++}
sleep(0.2)
assert a == 4
`
	_marshallerTest(code)
	_formattest(code)
}

//func TestNewExecutor_DollarIdentifier(t *testing.T) {
//  NewExecutor(`
//
//
//a = {"abc": "def"}
//dump(a)
//dump(a.abc)
//assert a.abc == "def"
//
//b = [1,2,3,4,5]
//b[3] = 33333
//assert b[3] == 33333
//
//c = "abc"
//assert a.$c == "def"
//assert a.$c == a.abc
//
//a.cdef = "asdf"
//dump(a)
//
//
//`).VM.DebugExec()
//}

func TestNewExecutor_Elif(t *testing.T) {
	code := `
if true {
	print("1")
} elif true {
	panic("should not run this block")
}
`
	NewExecutor(code).VM.DebugExec()
}

func TestNewExecutor_ForRange2(t *testing.T) {
	code := `
a = 1
for range 1 {
    for range 23{
        a++
    }
}
assert a == 24
for range 1 {}
c = 12
for range 12 {c++}
assert c == 24
for range 12 {d = _; dump(d)}
assert d == undefined, "d is out!!!!! 定义域失败！"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ForRange(t *testing.T) {
	code := `
for range 1 {}
for range 1 {}
c = 12
for range 12 {c++}
assert c == 24
for range 12 {d = _; dump(d)}
assert d == undefined
for range 1 {}
for range 1 {}
c = 12
for range 12 {c++}
assert c == 24
for range 12 {d = _; dump(d)}
c = 2
for range 10 {
    c++
}
assert c == 12
x = ["a", "b"]
count = 0
for i = range x {
    dump(i, x[i])
    count++
}
assert(count == 2)
count = 0
for i, v := range x {
    dump(i, v)
    count++
}
assert(count == 2)
count = 0
for i = range 5 {
    dump(i)
    count++
}
assert(count == 5)
count = 0
for n, i = range 5 {
    dump(n, i)
    count++
}
assert(count == 5)
count = 0
y = {"a":"b", "c":"d"}
for k = range y {
    dump(k, y[k])
    count++
}
assert(count == 2)
count = 0
for k, v = range y {
    dump(k, v)
    count++
}
assert(count == 2)
count = 0
for i in [1, 2, 3] {
    dump(i)
    count++
}
assert(count == 3)
count = 0
for k in y {
    dump(k, y[k])
    count++
}
assert(count == 2)
count = 0
for k, v in y {
    dump(k, v)
    count++
}
assert(count == 2)
count = 0
z = [[1, 2], [3, 4]]
for k, v in z {
    dump(k, v)
    count++
}
assert(count == 2)
count = 0
z = [[1, 2, 3], [4, 5, 6]]
for k, v, vv in z {
    dump(k, v, vv)
    count++
}
assert(count == 2)
`
	_marshallerTest(code)
	_formattest(code)
}

// 这个有问题，复杂度不对
func TestNewExecutor2(t *testing.T) {
	_formattest(`
count=3
for {
    count++
    if count < 10 {
        dump(count)
    } else {
        break
    }
}
assert(count == 10)
c = 10000
for {
    count++
    if count >= c { break }
    continue
}
assert(count == c)
count=3
for false {
    count++
}
assert(count==3)
for ;; {
    count++
    if count > 3 {break
    }
}
assert(count==4)
// 定义域
for count=1;;count++ {
    if count > 10 {
        break
    }
}
assert(count==11)
for count=1;;count++ {
    if count > 10 {
        for i=0;i< 3;i++ {
            count++
        }
        break
    }
}
assert(count==14)
a = func{
    defer dump("After AAA")
    dump("AAA")
    return 1
}
assert(a == 1)
defer func{
    dump("Hello World!")
}
dump(1+1)
`)
}

func TestNewExecutor_Ternary(t *testing.T) {
	code := `
assert((2>1?(2<1?true:false):false)==false)
assert((2>1?true:false)==true)
assert((1?true:false)==true)
assert(({}?true:false)==false)
assert(([]?true:false)==false)
assert((nil?true:false)==false)
assert((a?true:false)==false)
assert((true?false?true:false:false)==false)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_comparisonBinary(t *testing.T) {
	code := `
assert((2>1)==true)
assert((2.0>1)==true)
assert((2>1.0)==true)
assert((2.0>1.0)==true)
assert((2<1)==false)
assert((2.0<1)==false)
assert((2<1.0)==false)
assert((2.0<1.0)==false)
assert((1>=1)==true)
assert((1.0>=1)==true)
assert((1>=1.0)==true)
assert((1.0>=1.0)==true)
assert((1<=1)==true)
assert((1.0<=1)==true)
assert((1<=1.0)==true)
assert((1.0<=1.0)==true)
assert((2==1)==false)
assert((2.0==1)==false)
assert((2==1.0)==false)
assert((2.0==1.0)==false)
assert((2!=1)==true)
assert((2.0!=1)==true)
assert((2!=1.0)==true)
assert((2.0!=1.0)==true)
assert(("2">"1")==true)
assert(("1">="1")==true)
assert(("1">="2")==fasle)
assert(("2"<"1")==fasle)
assert(("2"<="1")==fasle)
assert(("2"<="2")==true)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Logic(t *testing.T) {
	code := `
assert((true && false) == false)
assert((true && (true && false)) == false)
if (true && false) {
    panic("logic result error")
}
assert((true || false) == true)
assert((true || (true && false)) == true)
if (true || false) {
    dump("logic test finished")
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_BitBinary(t *testing.T) {
	code := `
assert(1 & 2 == 0)
assert(1 &^ 2 == 1)
assert(1 | 2 == 3)
assert(1 ^ 2 == 3)
assert(1 << 2 == 4)
assert(4 >> 2 == 1)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_in(t *testing.T) {
	code := `
assert("qwe" in "qweasd")
assert("qwe" in b"qweasd")
assert("qwe" in ["qwe", "asd"])
assert("qwe" in {"qwe":"asd"})

assert("A" in testIns)

assert(b"qwe" in "qweasd")
assert(b"qwe" in b"qweasd")
assert(["qwe"] in [["qwe"], "asd"])

// not in
assert("zxc" not in "qweasd")
assert("zxc" not in b"qweasd")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_InplaceAssign(t *testing.T) {
	code := `   
a = 1
a ++
assert(a == 2)
a --
assert(a == 1)
b = 1.5 
b ++
assert(b == 2.5)
b --
assert(b == 1.5)
c = 1
c += 1
assert(c == 2)
c -= 1
assert(c == 1)
d = 2 
d *= 2
assert(d == 4)
d /= 2
assert(d == 2)
e = 5
e %= 2
assert(e == 1)
f = 1
f &= 2
assert(f == 0)
g = 4
g &^= 5
assert(g == 0)
h = 1
h |= 2
assert(h == 3)
i = 4
i ^= 3
assert(i == 7)
j = 1
j <<= 2
assert(j == 4)
j = 4
j >>= 2
assert(j == 1)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_AssignShouldPanic(t *testing.T) {
	code := `
a, b = 1
`

	test1 := func() {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic")
			}
		}()
		_marshallerTest(code)
	}

	test2 := func() {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic")
			}
		}()
		_formattest(code)
	}
	test1()
	test2()
}

func TestNewExecutor_Assign(t *testing.T) {
	code := `
a = 1, 2
dump(a)
b, c = a
dump(b, c)
assert b == 1
assert c == 2
a = getUint16()
vStruct,vMap,vList = getUint16Wrapper()
vStruct.A = 1
vMap.a = 1
vList[0] = 1
vList.Append(1)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ForChannel(t *testing.T) {
	code := `
ch = make(chan int, 1)
go func {
    for range 4{
        ch <- 1
        sleep(0.5)
    }
    close(ch)
}
for v in ch {
    assert v == 1, sprint(v)
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Channel(t *testing.T) {
	code := `
ch = make(chan int, 1)
// dump(ch)
ch <- 1
v, ok = <- ch
assert v == 1
assert ok == true
ch <- 1
v2 = <- ch
assert v2 == 1
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ChannelWithOp(t *testing.T) {
	code := `
a = make(chan int)
go fn{a<-1}
assert <-a >= 1
go fn{a<-1}
assert !<-a == false 
go fn{a<-1}
//<-a += 1
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Make(t *testing.T) {
	code := `
a = make([][]int, 0, 0)
dump(a)
b = make(map[string]string)
dump(b)
c = make(map[string]int)
dump(c)
d = make(chan int, 0)
dump(d)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_OrderedMap(t *testing.T) {
	code := `
a = {1:"1", 2:"2", "3":"3"}
for range 100 {
	c = 0
	for _, v = range a {
		c++
		assert v == sprint(c)
	}
}
	`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_BasicOp(t *testing.T) {
	code := `
// not
assert(!true == false)
assert(!false == true)
// neg
assert(-1 == 0 - 1)
assert(-1.5 == 0 - 1.5)
// plus
assert(+1 == 1) 
assert(+1.5 == 1.5)
// add
assert(1 + 2 == 3)
assert(1 + 2.0 == 3.0)
assert(1.2 + 2 == 3.2)
assert(1.2 + 2.3 == 3.5)
assert([1, 2, 3] + [4] == [1, 2, 3, 4])
assert([1, 2, 3] + 4 == [1, 2, 3, 4])
assert([4] + [1, 2, 3] == [4, 1, 2, 3])
assert(4 + [1, 2, 3] == [4, 1, 2, 3])
assert("qwe" + "asd" == "qweasd")
assert(b"qwe" + b"asd" == b"qweasd")
// sub
assert(1 - 2 == -1)
// mul
assert(2 * 3 == 6)
assert(2 * 3.0 == 6.0)
assert(2.5 * 5 == 12.5)
assert(2.7 * 6.5 == 17.55)
assert("abc" * 2 == "abcabc")
assert(2 * "abc" == "abcabc")
// div
assert(2 / 3 == 0)
assert(6 / 3.0 == 2.0)
assert(2.5 / 5 == 0.5)
assert(2.5 / 5.0 == 0.5)
// mod
assert(5 % 2 == 1)
assert("v:%s %d" % ["abc", 1] == "v:abc 1")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor2_Assign(t *testing.T) {
	code := `
a,b,l = 1,2,[1,2,3]
dump(l)
dump(a,b)
assert(a == 1)
assert(b == 2)
c,d,e,f = [1,2,3,4]
dump(c,d,e,f)
assert(c == 1)
assert(d == 1+1)
assert(e == 1+2)
assert(f == 1+3)
a = 1
c= 3
dump(a)
assert(c == 3)
assert(a == 1)
b = 2
dump(a, b)
assert(a == 1)
assert(b == 2)
a = 3
assert(b == 2)
assert(a == 3)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor2_FunctionTableParams(t *testing.T) {
	code := `
myFunc = a => a + 1
assert (myFunc(123) == 124)
def testFunc(a,b,c) {
    dump(a)
    if a != 0x1 {
        panic("a is not right")
    }
    return b,c
}
vals := testFunc(1,2,333)
dump(vals)
b, c = vals
if b != 2 {
    panic("b is not right")
}
if c != 333 {
    dump(c)
    panic("c is not right")
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutorFunctionTable(t *testing.T) {
	code := `
a = [1.1,2.2,3.3]
dump(a[-1])
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_SliceCall(t *testing.T) {
	code := `
mapV = {1:2, 3:4}
sliceV = [1,2,3]
stringV = "123"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_GuessList(t *testing.T) {
	code := `
z = [1, 1.5]
y = ["1", 1.5]
x = [1, true]
a = [true] + 1
b = ["qwe"] + 1
c = [1.5] + 2
d = [1.5] + "q"
e = [1] + 1.5
f = [1] + true
//func abc() {
//  dump(1,2,3,4,1,2.333)
//}
//abc()
//dumpint(1.2);
//dumpint(1);
//dumpint(.123123);
//dumpint(123123);
// efg := [1,2,3,4,5,1]
// efg := [1,2,3,4,5,1.2]
// dump(efg)
//slice测试
slice = [1,2,3,4,5]
assert slice[::-3] == [5, 2]
assert slice[::-1] == [5, 4, 3, 2, 1]
//map测试
//a = {1:1,"1":"123",3:4}
//dump(a[1])
//string测试
//a = "123"
//dump(a[:1]+"aaa")
//dump(a[::-1])
//dump(a[1:-1])
//dump(a[0])
//加法测试
//a = 'a'
//dump(a)
//a = {1.1:23, 4: "abc"}
//b = a[1.1]
//a = 'b'; b = '\n'
//c = '\x43'
a = 13
assert a == 13, "asdfasdf"
//assert(a==13)
//assert(a-2==13-2)
//assert(c==undefined)
//b = 2
//assert(a==b-1+12) 
//
//a = 1
//b = 2
//  if true {
//println(123)
//c = 4;
//
//}
//assert(c == undefined)
//println(c)
`
	_marshallerTest(code)
	_formattest(code)
}

type user struct {
	Name string
	Sex  string
}

func (u *user) GetName() string {
	return u.Name
}

type testString string

func (t testString) String() string {
	return "test"
}

func TestNewExecutor_MemberCallFix(t *testing.T) {
	code := `
d = {1:23, "bbb": a}
e = 1
assert d.$e == 23
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ClosureAssignFix(t *testing.T) {
	code := `
f = () => 1
{
	a = 2
	f = () => a
}

{
	f2 = f
	assert f() == 2
	assert f2() == 2
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_ClosureScopeCopy(t *testing.T) {
	code := `
f = () => 0
set = (a)=>{
	return () => {
		return a
	}
}

f0 = set(1)
assert f0() == 1

f1 = set(2)
assert f1() == 2

assert f0() == 1 // !!
	`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Closure_Instance(t *testing.T) {
	code := `
	for i in 16 {
		go func {
			a = 0
			go func {
				sleep(0.5)
				a ++
				assert a == 1
				// println(a)
				// if a > 1{
				// 	println("nononono")
				// }
			}
		}
	}
	sleep(1)
	`
	for i := 0; i < 3; i++ {
		_marshallerTest(code)
		_formatCodeTest(code)
	}
}

// 测试 assert/字符串索引、拼接/结构体、和map调用成员
func TestNewExecutor_MemberCall(t *testing.T) {
	u := &user{
		Name: "派大星",
		Sex:  "男",
	}

	Import("getGoStruct", func() interface{} {
		return u
	})
	Import("getRequest", func() interface{} {
		req, _ := http.NewRequest("GET", "https://pie.dev", nil)
		return req
	})
	Import("getTestString", func() interface{} {
		return testString("qwe")
	})
	code := `
//v = 1
//v+=2
//assert v == 2, "PlusEq运算失败"

// v = getGoStruct()
// assert v.GetName() == "派大星", "获取go结构体成员（通过FunctionCall方式）失败"
//assert v.Name == "派大星", "获取go结构体成员（通过Identity方式）失败"
// memberName = "Name"
// assert v.$memberName == "派大星", "获取go结构体成员（通过Ref方式）失败"
// assert v["Name"] == "派大星", "获取go结构体成员（通过SliceCall方式）失败"
// v = {"name":"派大星"}
// assert v.name == "派大星", "获取map成员失败"

v = getRequest()
assert v.Header.Add != nil, "获取typed-map的go内置方法失败"

v = getTestString()
assert v.String() == "test", "获取typed-string的go内置方法失败"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_PrefixString(t *testing.T) {
	code := `
a = x` + "`" + `username: {{randstr(5) }}
password: {{randstr(4)}}` + "`" + `[0]
print(a)
dump(a)
assert len(a) == 30
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_TemplateStringFix2(t *testing.T) {
	code := `
assert f"\'" == "'"
assert f` + "`" + `"a` + "`" + ` == '"a'
assert f` + "`" + `\n` + "`" + ` == "\\n"
assert f"\$" == "$"
assert '\'a' == "'a"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_TemplateStringFix(t *testing.T) {
	code := `
name = "张三"

//测试double quote template
assert f"姓名1: \$${name}\\1\n姓名2: \$${name}\\2" == ` + "`" + `姓名1: $张三\1
姓名2: $张三\2` + "`" + `

//测试double quote
assert "姓名1: ${name}\\1\n姓名2: ${name}\\2" == ` + "`" + `姓名1: ${name}\1
姓名2: ${name}\2` + "`" + `

//测试back tick template
assert f` + "`" + `姓名1: \$${name}\\1
姓名2: \$` + "\\`" + `${name}` + "\\`" + `\\2` + "`" + ` == "姓名1: $张三\\1\n姓名2: $` + "`" + `张三` + "`" + `\\2"

//测试back tick template with "\n"
assert f` + "`" + `姓名1: \$${name}\\1\n姓名2: \$` + "\\`" + `${name}` + "\\`" + `\\2` + "`" + ` == "姓名1: $张三\\1\\n姓名2: $` + "`" + `张三` + "`" + `\\2"

//测试back tick
assert ` + "`" + `姓名1: \$${name}\\1
姓名2: \$${name}\\2` + "`" + ` == ` + "`" + `姓名1: \$${name}\\1
姓名2: \$${name}\\2` + "`" + `
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_TemplateString(t *testing.T) {
	code := "abc = f`abc${1+1}`; assert abc == `abc2`"
	_marshallerTest(code)
	_formattest(code)

	code = `
	assert f'\"a' == '"a'
	assert f'\'a' == "'a"
	
	name = f"小明"
	age = 18
	assert f"${name} + 1" == "小明 + 1"
	a = f"username: ${name} password: ${age+1}"
	assert a == "username: 小明 password: 19"
	`

	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_TemplateStringSingleQuote(t *testing.T) {
	code := `
	abc = f'abc${1+1}'; 
	assert abc == 'abc2'

	name = f'小明'
	age = 18
	assert f'${name} + 1' == '小明 + 1'
	a = f'username: ${name} password: ${age+1}'
	assert a == 'username: 小明 password: 19'
	`

	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_HexString(t *testing.T) {
	code := `
a = 0h1234
// dump(a, x"{{rs(10)}}".first())
assert a == b"\x12\x34"
assert a == "\x12\x34"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Func(t *testing.T) {
	code := `
out = 1
func dumpUser(name,age,a...) {
    assert out == 1
    dump(name)
    dump(age)
    dump(a)
}
dumpUser(2,3,4,5,6)
assert name == undefined
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_VariableFunctionParamFix(t *testing.T) {
	code := `
e = (c, a...) => c+1+a[0]+a[1]+a[2]
assert e(5, [1,2,3]...) == 6 + 3 + 1 + 2, e(5, [1,2,3]...)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_VariableFunctionParam(t *testing.T) {
	code := `
names = ["张三","李四","王五","赵六","田七","周八","吴九","郑十"]
p = 0
func test(name){
    assert name == names[p],"expect " + names[p] + ", but get " + name 
    p++
}
func dumpNames(name,others...) {
    test(name)
    dump(name)
    if len(others) > 0{
        dumpNames(others...)    
    }
}
dumpNames(names...)
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_StringSlice(t *testing.T) {
	code := `assert ("派大星"[0:1]+"小星" == "派小星"), "expect"+"派小星"`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_FackTypeCase(t *testing.T) {
	code := `
// 空值类型转换判断
assert int() == 0
assert string() == ""
assert []byte() == b""
assert float() == 0.0
//伪类型转换
//Yak内置类型：int（char）、float、string、bool
assert int(123) == 123, "convert int to int failed"
assert int(123.0) == 123, "convert float to int failed"
assert int("123") == 123, "convert string to int failed"
assert int(true) == 1, "convert boolean to int failed"
assert int(false) == 0, "convert boolean to int failed"
assert float(123.3) == 123.3, "convert floatint to float failed"
assert float(123) == 123, "convert int to float failed"
assert float("123.3") == 123.3, "convert string to float failed"
assert float(true) == 1, "convert boolean to float failed"
assert float(false) == 0, "convert boolean to float failed"
assert string(123.3) == "123.3", "convert float to string failed"
assert string(123) == "123", "convert int to string failed"
assert string("123.3") == "123.3", "convert string to string failed"
assert string(true) == "true", "convert boolean to string failed"
assert string(false) == "false", "convert boolean to string failed"
assert bool(123.3) == true, "convert float to bool failed"
assert bool(123) == true, "convert int to bool failed"
assert bool("123.3") == true, "convert string to bool failed"
assert bool(true) == true, "convert boolean to bool failed"
assert bool(false) == false, "convert boolean to bool failed"
assert bool(0) == false, "convert int to bool failed"
assert []byte("qwe") == 0h717765, "convert string to []byte failed"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestNewExecutor_Wavy(t *testing.T) {
	code := `
e = nil
try{
	a = package.test(1, 2, 3)~
} catch err{
	e = err
}
assert e != nil
`
	_marshallerTest(code)
	_formattest(code)
}

var longCodeForParserLexer, _ = codec.DecodeBase64(`Ly9XQUZDaGVjayA9IE1JVE1fUEFSQU1TWyJXQUZDaGVjayJdIC8v6buY6K6k5Li6dHJ1ZQpXQUZDaGVjayA9IHRydWUKClNJTUlMQVJJVFlfUklUSU8gPSAwLjk5OQpMT1dFUl9SQVRJT19CT1VORCA9IDAuMDIKVVBQRVJfUkFUSU9fQk9VTkQgPSAwLjk4CkRJRkZfVE9MRVJBTkNFID0gMC4wNQoKQ0xPU0VfVFlQRSA9IHswIDogYCdgLCAxIDogYCJgLCAyOiBgYCwgMzogYCcpYCwgNDpgIilgfQoKLy9jb25zdApGT1JNQVRfRVhDRVBUSU9OX1NUUklOR1MgPSBbIlR5cGUgbWlzbWF0Y2giLCAiRXJyb3IgY29udmVydGluZyIsICJQbGVhc2UgZW50ZXIgYSIsICJDb252ZXJzaW9uIGZhaWxlZCIsICJTdHJpbmcgb3IgYmluYXJ5IGRhdGEgd291bGQgYmUgdHJ1bmNhdGVkIiwgIkZhaWxlZCB0byBjb252ZXJ0IiwgInVuYWJsZSB0byBpbnRlcnByZXQgdGV4dCB2YWx1ZSIsICJJbnB1dCBzdHJpbmcgd2FzIG5vdCBpbiBhIGNvcnJlY3QgZm9ybWF0IiwgIlN5c3RlbS5Gb3JtYXRFeGNlcHRpb24iLCAiamF2YS5sYW5nLk51bWJlckZvcm1hdEV4Y2VwdGlvbiIsICJWYWx1ZUVycm9yOiBpbnZhbGlkIGxpdGVyYWwiLCAiVHlwZU1pc21hdGNoRXhjZXB0aW9uIiwgIkNGX1NRTF9JTlRFR0VSIiwgIkNGX1NRTF9OVU1FUklDIiwgIiBmb3IgQ0ZTUUxUWVBFICIsICJjZnF1ZXJ5cGFyYW0gY2ZzcWx0eXBlIiwgIkludmFsaWRQYXJhbVR5cGVFeGNlcHRpb24iLCAiSW52YWxpZCBwYXJhbWV0ZXIgdHlwZSIsICJBdHRyaWJ1dGUgdmFsaWRhdGlvbiBlcnJvciBmb3IgdGFnIiwgImlzIG5vdCBvZiB0eXBlIG51bWVyaWMiLCAiPGNmaWYgTm90IElzTnVtZXJpYygiLCAiaW52YWxpZCBpbnB1dCBzeW50YXggZm9yIGludGVnZXIiLCAiaW52YWxpZCBpbnB1dCBzeW50YXggZm9yIHR5cGUiLCAiaW52YWxpZCBudW1iZXIiLCAiY2hhcmFjdGVyIHRvIG51bWJlciBjb252ZXJzaW9uIGVycm9yIiwgInVuYWJsZSB0byBpbnRlcnByZXQgdGV4dCB2YWx1ZSIsICJTdHJpbmcgd2FzIG5vdCByZWNvZ25pemVkIGFzIGEgdmFsaWQiLCAiQ29udmVydC5Ub0ludCIsICJjYW5ub3QgYmUgY29udmVydGVkIHRvIGEgIiwgIkludmFsaWREYXRhRXhjZXB0aW9uIiwgIkFyZ3VtZW50cyBhcmUgb2YgdGhlIHdyb25nIHR5cGUiXQojLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS1XT1JLU1BBQ0UtLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLQojIEFscGhhYmV0IHVzZWQgZm9yIGhldXJpc3RpYyBjaGVja3MKSEVVUklTVElDX0NIRUNLX0FMUEhBQkVUID0gW2AiYCwgYCdgLCBgKWAsIGAoYCwgYCxgLCBgLmBdCgpEQk1TX0VSUk9SUyA9IHsKICAgICJNeVNRTCI6IFtgU1FMIHN5bnRheC4qTXlTUUxgLCBgV2FybmluZy4qbXlzcWxfLipgLCBgdmFsaWQgTXlTUUwgcmVzdWx0YCwgYE15U3FsQ2xpZW50XC5gXSwKICAgICJQb3N0Z3JlU1FMIjogW2BQb3N0Z3JlU1FMLipFUlJPUmAsIGBXYXJuaW5nLipcV3BnXy4qYCwgYHZhbGlkIFBvc3RncmVTUUwgcmVzdWx0YCwgYE5wZ3NxbFwuYF0sCiAgICAiTWljcm9zb2Z0IFNRTCBTZXJ2ZXIiOiBbYERyaXZlci4qIFNRTFtcLVxfXCBdKlNlcnZlcmAsIGBPTEUgREIuKiBTUUwgU2VydmVyYCwgYChcV3xcQSlTUUwgU2VydmVyLipEcml2ZXJgLCBgV2FybmluZy4qbXNzcWxfLipgLCBgKFxXfFxBKVNRTCBTZXJ2ZXIuKlswLTlhLWZBLUZdezh9YCwgYCg/cylFeGNlcHRpb24uKlxXU3lzdGVtXC5EYXRhXC5TcWxDbGllbnRcLmAsIGAoP3MpRXhjZXB0aW9uLipcV1JvYWRob3VzZVwuQ21zXC5gXSwKICAgICJNaWNyb3NvZnQgQWNjZXNzIjogW2BNaWNyb3NvZnQgQWNjZXNzIERyaXZlcmAsIGBKRVQgRGF0YWJhc2UgRW5naW5lYCwgYEFjY2VzcyBEYXRhYmFzZSBFbmdpbmVgXSwKICAgICJPcmFjbGUiOiBbYFxiT1JBLVswLTldWzAtOV1bMC05XVswLTldYCwgYE9yYWNsZSBlcnJvcmAsIGBPcmFjbGUuKkRyaXZlcmAsIGBXYXJuaW5nLipcV29jaV8uKmAsIGBXYXJuaW5nLipcV29yYV8uKmBdLAogICAgIklCTSBEQjIiOiBbYENMSSBEcml2ZXIuKkRCMmAsIGBEQjIgU1FMIGVycm9yYCwgYFxiZGIyX1x3K1woYF0sCiAgICAiU1FMaXRlIjogW2BTUUxpdGUvSkRCQ0RyaXZlcmAsIGBTUUxpdGUuRXhjZXB0aW9uYCwgYFN5c3RlbS5EYXRhLlNRTGl0ZS5TUUxpdGVFeGNlcHRpb25gLCBgV2FybmluZy4qc3FsaXRlXy4qYCwgYFdhcm5pbmcuKlNRTGl0ZTM6OmAsIGBcW1NRTElURV9FUlJPUlxdYF0sCiAgICAiU3liYXNlIjogW2AoP2kpV2FybmluZy4qc3liYXNlLipgLCBgU3liYXNlIG1lc3NhZ2VgLCBgU3liYXNlLipTZXJ2ZXIgbWVzc2FnZS4qYF0sCn0KCgoKIyBtaXJyb3JGaWx0ZXJlZEhUVFBGbG93IOWKq+aMgeWIsOeahOa1gemHj+S4uiBNSVRNIOiHquWKqOi/h+a7pOWHuueahOWPr+iDveWSjCAi5Lia5YqhIiDmnInlhbPnmoTmtYHph4/vvIzkvJroh6rliqjov4fmu6TmjokganMgLyBjc3Mg562J5rWB6YePCm1pcnJvckZpbHRlcmVkSFRUUEZsb3cgPSBmdW5jKGlzSHR0cHMgLyogYm9vbCAqLywgdXJsIC8qIHN0cmluZyAqLywgcmVxIC8qIFtdYnl0ZSAqLywgcnNwIC8qIFtdYnl0ZSAqLywgYm9keSAvKiBbXWJ5dGUgKi8pIHsKICAgIC8veWFraXRfb3V0cHV0KCJDaGVja2luZy4uLiIpCiAgICBnbyBwaXBlTGluZShpc0h0dHBzLCB1cmwsIHJlcSwgcnNwLCBib2R5KSAvL+WvueS6juavj+S4gOS4quivt+axguW8gOWQr+S4gOS4queLrOeri+WNj+eoiwogICAgCn0KCmZ1bmMgcGlwZUxpbmUoaXNIdHRwcyAvKmJvb2wqLywgdXJsIC8qc3RyaW5nKi8sIHJlcSAvKltdYnl0ZSovLCByc3AgLypbXWJ5dGUqLywgYm9keSAvKltdYnl0ZSovKXsKICAgIFRFTVBMQVRFX1BBR0VfUlNQIDo9IHJzcCAgLy/lkI7nu63orqHnrpdwYWdlcmF0aW/lgZrlr7nmr5TnmoTmraPluLjor7fmsYLpobXpnaIKICAgIHByZUNoZWNrKGlzSHR0cHMsIHJlcSkgLy/lgZrkuIDkupvliY3nva7mo4Dmn6Ug6YG/5YWN5peg5oSP5LmJ55qE5ZCO57ut5qOA5rWLCiAgICB3YWZBbmFseXNlKHJlcSwgcnNwLCBURU1QTEFURV9QQUdFX1JTUCwgaXNIdHRwcykgLy/liKTmlq3lkI7nq6/mmK/lkKblrZjlnKh3YWYg5Y+q5Yik5pat5L2c5Li65o+Q56S65L+h5oGvIOS4jeWBmui/m+S4gOatpeaTjeS9nCDlpoLmnpzmo4Dlh7rlrZjlnKjms6jlhaUg5YiZ5Y+v5Lul6ICD6JmR6ZmE5Yqg5L+h5oGvCiAgICBoZXVyaXN0aWNDaGVja0lmSW5qZWN0YWJsZSh1cmwsIHJlcSwgaXNIdHRwcywgVEVNUExBVEVfUEFHRV9SU1ApIC8v5byA5aeL5ZCv5Y+R5byPc3Fs5rOo5YWl5qOA5rWLCn0KCmZ1bmMgcHJlQ2hlY2soaXNIdHRwcyAvKiBib29sICovLCByZXEgLyogW11ieXRlICovKXsKICAgIGhlYWRlciwgXyA6PSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHJzcCkKICAgIGlmIHJlLk1hdGNoKGBIVFRQXC8uXC4uIDQwNGAsIGhlYWRlcil7CiAgICAgICAgZGllKCLljp/lp4vor7fmsYLotYTmupDkuI3lrZjlnKgiKQogICAgfSAvL+WPguiAg3NxbG1hcOeahOihjOS4uu+8jOWOn+Wni+ato+W4uOivt+axguS4jeW6lOivpTQwNAoKICAgIGZyZXEsIGVyciA6PSBmdXp6LkhUVFBSZXF1ZXN0KHJlcSwgZnV6ei5odHRwcyhpc0h0dHBzKSkKICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgIHlha2l0X291dHB1dCgicHJlY2hlY2vmnoTlu7pmdXp66Kej5p6Q5aSx6LSlIitwYXJzZVN0cmluZyhlcnIpKQogICAgICAgIGRpZShlcnIpCiAgICB9Ly/pgb/lhY3lkI7nu63mnoTpgKBmdXp66K+35rGC5Ye66ZSZCgogICAgaWYgbGVuKGZyZXEuR2V0Q29tbW9uUGFyYW1zKCkpID09IDB7CiAgICAgICAgeWFraXRfb3V0cHV0KCLml6Dlj6/kvpvms6jlhaXlj4LmlbAiKQogICAgICAgIGRpZSgi5rKh5pyJ5Y+v5L6b5rOo5YWl5Y+C5pWwIikKICAgIH0KICAgIAp9CgpmdW5jIHdhZkFuYWx5c2UocmVxIC8qIFtdYnl0ZSAqLywgcnNwIC8qIFtdYnl0ZSAqLywgb3JpZ2luYWwgLyogW11ieXRlICovLCBpc0h0dHBzIC8qIGJvb2wgKi8pewogICAgZnJlcSwgXyA6PSBmdXp6LkhUVFBSZXF1ZXN0KHJlcSwgZnV6ei5odHRwcyhpc0h0dHBzKSkKICAgIAogICAgZmxhZyA9IGZhbHNlCiAgICBpZiBXQUZDaGVja3sgLy8gV0FGQ2hlY2vpu5jorqTkuLp0cnVlCiAgICAgICAgSVBTX1dBRl9DSEVDS19SQVRJTyA6PSAwLjUKICAgICAgICBmb3IgXywgcGFyYW0gOj0gcmFuZ2UgZnJlcS5HZXRDb21tb25QYXJhbXMoKXsKICAgICAgICAgICAgSVBTX1dBRl9DSEVDS19QQVlMT0FEIDo9IHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgIiBBTkQgMT0xIFVOSU9OIEFMTCBTRUxFQ1QgMSxOVUxMLCc8c2NyaXB0PmFsZXJ0KFwiWFNTXCIpPC9zY3JpcHQ+Jyx0YWJsZV9uYW1lIEZST00gaW5mb3JtYXRpb25fc2NoZW1hLnRhYmxlcyBXSEVSRSAyPjEtLS8qKi87IEVYRUMgeHBfY21kc2hlbGwoJ2NhdCAuLi8uLi8uLi9ldGMvcGFzc3dkJykjIgogICAgICAgICAgICB0ZXN0UmVzcCwgZXJyIDo9IHBhcmFtLkZ1enooSVBTX1dBRl9DSEVDS19QQVlMT0FEKS5FeGVjRmlyc3QoKQogICAgICAgICAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICAgICAgICAgIGRpZSgid2Fm5qOA5rWL6K+35rGC5Ye66ZSZOiIgKyBwYXJzZVN0cmluZyhlcnIpKQogICAgICAgICAgICB9CiAgICAgICAgICAgIHJlc3VsdCA6PSBzdHIuQ2FsY1NpbWlsYXJpdHkob3JpZ2luYWwsIHRlc3RSZXNwLlJlc3BvbnNlUmF3KQogICAgICAgICAgICBpZiByZXN1bHQgPCBJUFNfV0FGX0NIRUNLX1JBVElPewogICAgICAgICAgICAgICAgZmxhZyA9IHRydWUKICAgICAgICAgICAgICAgIGJyZWFrCiAgICAgICAgICAgIH0KICAgICAgICB9CiAgICAgICAgaWYgZmxhZ3sKICAgICAgICAgICAgeWFraXRfb3V0cHV0KCLlkK/lj5HlvI9XQUbmo4DmtYs655uu5qCH5Li75py65Y+v6IO95a2Y5ZyoV0FGIikKICAgICAgICAgICAgcmV0dXJuCiAgICAgICAgfWVsc2V7CiAgICAgICAgICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLlkK/lj5HlvI9XQUbmo4DmtYs655uu5qCH5Li75py65Y+v6IO95LiN5a2Y5ZyoV0FGIikpCiAgICAgICAgICAgIHJldHVybgogICAgICAgIH0KICAgIH0KfQoKLyoKICsg5YWI6L+H5ruk5Ye65pyJ5pWI5Y+C5pWw77yM5Y2z5LiN5a2Y5Zyo6L2s5Z6L55qE5Y+C5pWwCiArIOS+neasoei/m+ihjFNRTOazqOWFpeWwneivleS4juWIpOWumiAKKi8KZnVuYyBoZXVyaXN0aWNDaGVja0lmSW5qZWN0YWJsZSh1cmwgLyogc3RyaW5nICovLCByZXEgLyogW11ieXRlICovLCBpc0h0dHBzLyogYm9vbCAqLywgVEVNUExBVEVfUEFHRV9SU1AgLypbXWJ5dGUqLyl7CiAgICBmcmVxLCBlcnIgOj0gZnV6ei5IVFRQUmVxdWVzdChyZXEsIGZ1enouaHR0cHMoaXNIdHRwcykpCiAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoImNoZWNrSWZJbmplY3RhYmxl5p6E5bu6ZnV6euino+aekOWksei0pSIrcGFyc2VTdHJpbmcoZXJyKSkKICAgICAgICBkaWUoZXJyKQogICAgfQogICAgCiAgICAvKuajgOa1i+aooeadv+mhtemdouiHqui6q+aYr+WQpuWtmOWcqOaehOmAoOmUmeivryovCiAgICB0ZW1wbGF0ZVJzcCwgZXJyIDo9IGZyZXEuRXhlY0ZpcnN0KCkKICAgIGlmIGVyciAhPSBuaWwgfHwgdGVtcGxhdGVSc3AuRXJyb3IgIT0gbmlsewogICAgICAgIHlha2l0X291dHB1dCgiY2hlY2tJZkluamVjdGFibGUgRnV6euivt+axguWHuumUmSIpCiAgICAgICAgZGllKGVycikKICAgIH0KICAgIGZvciBfLCB2YWx1ZSA6PSByYW5nZSBGT1JNQVRfRVhDRVBUSU9OX1NUUklOR1N7CiAgICAgICAgaWYgc3RyLkNvbnRhaW5zKGJvZHksIHZhbHVlKXsKICAgICAgICAgICAgeWFraXRfb3V0cHV0KCLmqKHmnb/or7fmsYLlj4LmlbDlrZjlnKjovazlnovplJnor6/vvIzor7fmj5DkvpvmraPluLjnmoTor7fmsYLlj4LmlbAiKSAvL+WSjHNxbG1hcOS4gOagtwogICAgICAgICAgICBkaWUoIuaooeadv+ivt+axguWPguaVsOWtmOWcqOi9rOWei+mUmeivr++8jOivt+aPkOS+m+ato+W4uOeahOivt+axguWPguaVsCIpCiAgICAgICAgfQogICAgICAgIAogICAgfQogICAgCiAgICAvKumBjeWOhuWQhOS4quWPguaVsOajgOafpeWFtuaYr+WQpuiiq+i9rOWeiyovIAogICAgaW5qZWN0YWJsZVBhcmFtc1BvcyA6PSBbXSAvL+mBv+WFjVBPU1Tor7fmsYLlh7rnjrDlj4LmlbDph43lkI3vvIzorrDlvZXlj4LmlbDkvY3nva4KICAgIHJlcU1ldGhvZCA9IGZyZXEuR2V0TWV0aG9kKCkKICAgIGlmIHJlcU1ldGhvZCAhPSAiR0VUIiAmJiByZXFNZXRob2QgIT0gIlBPU1QiewogICAgICAgIGRpZSgi6K+35rGC5pa55rOV5bCa5LiN5pSv5oyB5qOA5rWLIikKICAgIH0KICAgIHJhbmRvbVRlc3RTdHJpbmcgOj0gZ2V0RXJyb3JCYXNlZFByZUNoZWNrUGF5bG9hZCgpCiAgICByYW5kU3RyaW5nID0gcmFuZHN0cig0KQogICAgY2FzdCA6PSBmYWxzZQogICAgQ29tbW9uUGFyYW1zIDo9IGZyZXEuR2V0Q29tbW9uUGFyYW1zKCkKICAgIHlha2l0X291dHB1dChzdHIuZigi5oC75YWx5rWL6K+V5Y+C5pWw5YWxJXbkuKoiLCBsZW4oQ29tbW9uUGFyYW1zKSkpCgogICAgZm9yIHBvcywgcGFyYW0gOj0gcmFuZ2UgQ29tbW9uUGFyYW1zIHsKICAgICAgICAvL3ByaW50bG4oIuiOt+WPluWPguaVsO+8miIsIHN0ci5mKCJQb3NpdGlvbjogJXYgUGFyYW1OYW1lOiAldiBPcmlnaW5WYWx1ZTogJXYiLCBwYXJhbS5Qb3NpdGlvbigpLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKQogICAgICAgIGlmIHN0ci5NYXRjaEFsbE9mUmVnZXhwKHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pLCBgXlswLTldKyRgKXsKICAgICAgICAgICAgcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihyYW5kb21UZXN0U3RyaW5nICsgcmFuZFN0cmluZykuRXhlY0ZpcnN0KCkKICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihyYW5kb21UZXN0U3RyaW5nICsgcGFyc2VTdHIocmFuZG4oMSwgOTk5OSkpKS5FeGVjRmlyc3QoKQogICAgICAgIH0KICAgICAgICAKICAgICAgICBpZiBlcnIgIT0gbmlsIHx8IHJzcC5FcnJvciAhPSBuaWx7CiAgICAgICAgICAgIHlha2l0X291dHB1dCgiY2hlY2tJZkluamVjdGFibGUgRnV6euivt+axguWHuumUmSIpCiAgICAgICAgICAgIGRpZShlcnIpCiAgICAgICAgfQogICAgICAgIF8sIGJvZHkgOj0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChyc3AuUmVzcG9uc2VSYXcpCiAgICAgICAgYm9keSA9IHN0cmluZyhib2R5KQoKICAgICAgICBmb3IgXywgdmFsdWUgOj0gcmFuZ2UgRk9STUFUX0VYQ0VQVElPTl9TVFJJTkdTewogICAgICAgICAgICBpZiBzdHIuQ29udGFpbnMoYm9keSwgdmFsdWUpewogICAgICAgICAgICAgICAgY2FzdCA9IHRydWUKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dChyZXFNZXRob2QgKyAi5Y+C5pWwOiAiICsgcGFyYW0uTmFtZSgpICsgIiDlm6DmlbDlgLzovazlnovml6Dms5Xms6jlhaUiKQogICAgICAgICAgICAgICAgYnJlYWsKICAgICAgICAgICAgfQogICAgICAgIH0KICAgICAgICBpZiBjYXN0ewogICAgICAgICAgICBjYXN0ID0gZmFsc2UKICAgICAgICAgICAgY29udGludWUKICAgICAgICB9CiAgICAgICAgaW5qZWN0YWJsZVBhcmFtc1BvcyA9IGFwcGVuZChpbmplY3RhYmxlUGFyYW1zUG9zLCBwb3MpCiAgICAgICAgeWFraXRfb3V0cHV0KHJlcU1ldGhvZCArICLlj4LmlbA6ICIgKyBwYXJhbS5OYW1lKCkgKyAiIOacquajgOa1i+WIsOi9rOWeiyIpCiAgICB9CiAgICBpZiBsZW4oaW5qZWN0YWJsZVBhcmFtc1BvcykgPT0gMHsKICAgICAgICB5YWtpdF9vdXRwdXQoIuaXoOWPr+azqOWFpeWPguaVsCIpCiAgICAgICAgZGllKCLml6Dlj6/ms6jlhaXlj4LmlbAiKQogICAgfQogICAvKiDku47mraTlpITlvIDlp4vlrp7pmYXnmoRzcWzms6jlhaXmtYvor5Ug5a+55ZCE56eN5rOo5YWl57G75Z6L6L+b6KGM5rWL6K+VICovIAogICAgLy/lvIDlp4tzcWzms6jlhaXmo4DmtYsKICAgIGlmIGxlbihDb21tb25QYXJhbXMpICE9IGxlbihpbmplY3RhYmxlUGFyYW1zUG9zKXsKICAgICAgICBjYXN0ID0gdHJ1ZSAvL+ivtOaYjuaciemDqOWIhuWPguaVsOWtmOWcqOi9rOWeiwogICAgfQogICAgZm9yIF8sIHBvcyA6PSByYW5nZSBpbmplY3RhYmxlUGFyYW1zUG9zewogICAgICAgIGNoZWNrU3FsSW5qZWN0aW9uKHBvcywgcmVxLCBjYXN0LCBURU1QTEFURV9QQUdFX1JTUCwgQ29tbW9uUGFyYW1zKQogICAgfQp9CgoKZnVuYyBjaGVja1NxbEluamVjdGlvbihwb3MgLyogaW50ICovLCByZXEgLyogW11ieXRlICovLCBjYXN0RGV0ZWN0ZWQgLyogYm9vbCAqLywgVEVNUExBVEVfUEFHRV9SU1AgLyogW11ieXRlICovLCBDb21tb25QYXJhbXMvKiBbXSptdXRhdGUuRnV6ekhUVFBSZXF1ZXN0UGFyYW0gKi8pewogICAgY2hlY2tFcnJvckJhc2VkKHBvcywgcmVxLCBjYXN0RGV0ZWN0ZWQsIENvbW1vblBhcmFtcykKICAgIGNsb3NlVHlwZSwgbGluZUJyZWFrIDo9IGNoZWNrQ2xvc2VUeXBlKHBvcywgcmVxLCBDb21tb25QYXJhbXMpCiAgICBpZiBjbG9zZVR5cGUgPT0gLTF7CiAgICAgICAgZm9yIGluZGV4LCBwYXJhbSA6PSByYW5nZSBDb21tb25QYXJhbXN7CiAgICAgICAgICAgIGlmIGluZGV4ID09IHBvc3sKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi5Y+C5pWwOiV25pyq5qOA5rWL5Yiw6Zet5ZCI6L6555WMIiwgcGFyYW0uVmFsdWUoKVswXSkKICAgICAgICAgICAgICAgIHJldHVybgogICAgICAgICAgICB9CiAgICAgICAgfQogICAgfQogICAgY2hlY2tUaW1lQmFzZWRCbGluZChwb3MsIHJlcSwgQ29tbW9uUGFyYW1zLCBjbG9zZVR5cGUsIGxpbmVCcmVhaykKICAgIC8vY2hlY2tCb29sQmFzZWQocG9zLCBmcmVxKSAvL1RPRE86Ym9vbCBiYXNlZAogICAgY2hlY2tVbmlvbkJhc2VkKHBvcywgcmVxLCBURU1QTEFURV9QQUdFX1JTUCwgQ29tbW9uUGFyYW1zLCBjbG9zZVR5cGUsIGxpbmVCcmVhaykKICAgIC8vY2hlY2tTdGFja2VkSW5qZWN0aW9uCgp9CgpmdW5jIGNoZWNrRXJyb3JCYXNlZChwb3MgLyogaW50ICovLCByZXEgLyogW11ieXRlICovLCBjYXN0RGV0ZWN0ZWQgLyogYm9vbCAqLywgQ29tbW9uUGFyYW1zLyogW10qbXV0YXRlLkZ1enpIVFRQUmVxdWVzdFBhcmFtICovKSB7CiAgICBmcmVxLCBfIDo9IGZ1enouSFRUUFJlcXVlc3QocmVxKQoJaWYgY2FzdERldGVjdGVkIHsgLy/lkIzml7blrZjlnKjlj6/ms6jlhaXlkozkuI3lj6/ms6jlhaXlj4LmlbAg5LiU5ZCO56uv77yI5Y+v6IO95p2l6Ieq5pWw5o2u5bqTIOaYr+WQpuadpeiHquaVsOaNruW6k+mcgOimgei/m+S4gOatpeWIpOaWre+8ieWvuemUmeivr+eahOWPguaVsOexu+Wei+i/m+ihjOS6huaYjuehruWbnuaYvgoJCXBhcmFtTmFtZSA6PSAiIgoJCWZvciBpbmRleCwgcGFyYW0gOj0gcmFuZ2UgQ29tbW9uUGFyYW1zIHsKCQkJaWYgaW5kZXggPT0gcG9zIHsKCQkJCXBhcmFtTmFtZSA9IHBhcmFtLk5hbWUoKQoJCQkJcGF5bG9hZCA6PSBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSArIGdldEVycm9yQmFzZWRQcmVDaGVja1BheWxvYWQoKQoJCQkJcmVzdWx0LCBlcnIgPSBwYXJhbS5GdXp6KHBheWxvYWQpLkV4ZWNGaXJzdCgpCgkJCQlpZiBlcnIgIT0gbmlsIHsKCQkJCQl5YWtpdF9vdXRwdXQoIuWwneivleajgOa1iyBFcnJvci1CYXNlZCBTUUwgSW5qZWN0aW9uIFBheWxvYWQg5aSx6LSlIikKICAgICAgICAgICAgICAgICAgICByZXR1cm4KCQkJCX0KCQkJCWZvciBEQk1TLCByZWdleHBzID0gcmFuZ2UgREJNU19FUlJPUlMgewoJCQkJCWlmIHN0ci5NYXRjaEFueU9mUmVnZXhwKHJlc3VsdC5SZXNwb25zZVJhdywgcmVnZXhwcy4uLikgewoJCQkJCQl5YWtpdF9vdXRwdXQoIuehruiupOWQjuerr+aVsOaNruW6k+aKpemUmSIpCgkJCQkJCWNvZGVjUGF5bG9hZCA9IGNvZGVjLlN0cmNvbnZRdW90ZShzdHJpbmcocGF5bG9hZCkpCgkJCQkJCXJpc2suTmV3UmlzaygKCQkJCQkJCXJlc3VsdC5VcmwsCiAgICAgICAgICAgICAgICAgICAgICAgICAgICByaXNrLnNldmVyaXR5KCJjcml0aWNhbCIpLAoJCQkJCQkJcmlzay50aXRsZShzdHIuZigiRVJST1ItQmFzZWQgU1FMIEluamVjdGlvbjogWyV2OiV2XSBHdWVzcyBEQk1TOiAldiIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSwgREJNUykpLAoJCQkJCQkJcmlzay50aXRsZVZlcmJvc2Uoc3RyLmYoIuWPr+iDveWtmOWcqOWfuuS6jumUmeivr+eahCBTUUwg5rOo5YWlOiBb5Y+C5pWw5ZCNOiV2IOWOn+WAvDoldl0g54yc5rWL5pWw5o2u5bqT57G75Z6LOiAldiIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSwgREJNUykpLAoJCQkJCQkJcmlzay50eXBlICgic3FsaW5qZWN0aW9uIiksIAogICAgICAgICAgICAgICAgICAgICAgICAgICAgcmlzay5yZXF1ZXN0KHJlc3VsdC5SZXF1ZXN0UmF3KSwKICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucmVzcG9uc2UocmVzdWx0LlJlc3BvbnNlUmF3KSwKICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucGF5bG9hZChwYXlsb2FkKSwgCiAgICAgICAgICAgICAgICAgICAgICAgICAgICByaXNrLnBhcmFtZXRlcihwYXJhbS5OYW1lKCkpLAoJCQkJCSAgICApIC8v6ICD6JmR5aKe5Yqg5LiA5Lqb5o6i5rWLcGF5bG9hZCDmr5TlpoIgZXh0cmFjdHZhbHVl5oiW6ICFdXBkYXRleG1sIOi/meagt+WwseiDveehruiupO+8jOehruWunuWPr+S7peWIqeeUqOaKpemUmeeCuei/m+ihjOazqOWFpQogICAgICAgICAgICAgICAgICAgICAgICB5YWtpdF9vdXRwdXQoIuWPguaVsDogIiArIHBhcmFtTmFtZSArICLlrZjlnKjmiqXplJnms6jlhaUiKQoJCQkJCQlyZXR1cm4KCQkJCQl9CgkJCQl9CiAgICAgICAgICAgICAgICB5YWtpdF9vdXRwdXQoIuWPguaVsDogIiArIHBhcmFtTmFtZSArICIg5LiN5a2Y5Zyo5oql6ZSZ5rOo5YWlIikKCQkJfQoJCX0KCgl9ZWxzZXsgLy/kuYvliY3msqHmo4DmtYvliLDkuZ/lsLHmmK/or7Tlj6/og73msqHlr7nlj4LmlbDlgZpjYXN077yI57G75Z6L6L2s5o2i6ZSZ6K+v5LiN5a2Y5ZyoIOagueacrOayoeWBmuexu+Wei+ajgOa1i++8iSDov5nmrKHmjaLljLnphY3lhbPplK7lrZflkozmtYvor5XmiqXplJlwYXlsb2Fk55yL55yL6IO95ZCm5om+5Yiw5oql6ZSZCiAgICAgICAgcGFyYW1OYW1lIDo9ICIiCiAgICAgICAgZm9yIGluZGV4LCBwYXJhbSA6PSByYW5nZSBDb21tb25QYXJhbXMgewogICAgICAgICAgICBpZiBpbmRleCA9PSBwb3MgewogICAgICAgICAgICAgICAgcGFyYW1OYW1lID0gcGFyYW0uTmFtZSgpCiAgICAgICAgICAgICAgICBwYXlsb2FkIDo9IHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgZ2V0RXJyb3JCYXNlZFByZUNoZWNrUGF5bG9hZCgpCiAgICAgICAgICAgICAgICByZXN1bHQsIGVyciA9IHBhcmFtLkZ1enoocGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgICAgICAgICAgICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5qOA5rWLIEVycm9yLUJhc2VkIFNRTCBJbmplY3Rpb24gUGF5bG9hZCDlpLHotKUiKQogICAgICAgICAgICAgICAgICAgIHJldHVybgogICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgZm9yIERCTVMsIHJlZ2V4cHMgPSByYW5nZSBEQk1TX0VSUk9SUyB7CiAgICAgICAgICAgICAgICAgICAgaWYgc3RyLk1hdGNoQW55T2ZSZWdleHAocmVzdWx0LlJlc3BvbnNlUmF3LCByZWdleHBzLi4uKSB7CiAgICAgICAgICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi56Gu6K6k5ZCO56uv5pWw5o2u5bqT5oql6ZSZIikKICAgICAgICAgICAgICAgICAgICAgICAgY29kZWNQYXlsb2FkID0gY29kZWMuU3RyY29udlF1b3RlKHN0cmluZyhwYXlsb2FkKSkKICAgICAgICAgICAgICAgICAgICAgICAgLy9hZGRWdWwoKQogICAgICAgICAgICAgICAgICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgICAgICAgICAgICAgICAgICByZXN1bHQuVXJsLAogICAgICAgICAgICAgICAgICAgICAgICAgICAgcmlzay5zZXZlcml0eSgiY3JpdGljYWwiKSwKCQkJCQkJCXJpc2sudGl0bGUoc3RyLmYoIkVSUk9SLUJhc2VkIFNRTCBJbmplY3Rpb246IFsldjoldl0gR3Vlc3MgREJNUzogJXYiLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCksIERCTVMpKSwKCQkJCQkJCXJpc2sudGl0bGVWZXJib3NlKHN0ci5mKCLlj6/og73lrZjlnKjln7rkuo7plJnor6/nmoQgU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDljp/lgLw6JXZdIOeMnOa1i+aVsOaNruW6k+exu+WeizogJXYiLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCksIERCTVMpKSwKCQkJCQkJCXJpc2sudHlwZSAoInNxbGluamVjdGlvbiIpLCAKICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucmVxdWVzdChyZXN1bHQuUmVxdWVzdFJhdyksCiAgICAgICAgICAgICAgICAgICAgICAgICAgICByaXNrLnJlc3BvbnNlKHJlc3VsdC5SZXNwb25zZVJhdyksCiAgICAgICAgICAgICAgICAgICAgICAgICAgICByaXNrLnBheWxvYWQocGF5bG9hZCksIAogICAgICAgICAgICAgICAgICAgICAgICAgICAgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwKICAgICAgICAgICAgICAgICAgICAgICAgKSAvL+iAg+iZkeWinuWKoOS4gOS6m+aOoua1i3BheWxvYWQg5q+U5aaCIGV4dHJhY3R2YWx1ZeaIluiAhXVwZGF0ZXhtbCDov5nmoLflsLHog73noa7orqTvvIznoa7lrp7lj6/ku6XliKnnlKjmiqXplJnngrnov5vooYzms6jlhaUKICAgICAgICAgICAgICAgICAgICAgICAgcmV0dXJuCiAgICAgICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KCLlj4LmlbA6ICIgKyBwYXJhbU5hbWUgKyAi5LiN5a2Y5Zyo5oql6ZSZ5rOo5YWlIikKICAgICAgICAgICAgfQogICAgICAgIH0KICAgIH0KfQoKZnVuYyBjaGVja0Nsb3NlVHlwZShwb3MgLyogaW50ICovLCByZXEgLyogW11ieXRlICovLCBDb21tb25QYXJhbXMvKiBbXSptdXRhdGUuRnV6ekhUVFBSZXF1ZXN0UGFyYW0gKi8pewogICAgZnJlcSwgXyA6PSBmdXp6LkhUVFBSZXF1ZXN0KHJlcSwgZnV6ei5odHRwcyhpc0h0dHBzKSkKICAgIG9yaWdpblJlc3VsdCwgZXJyID0gZnJlcS5FeGVjRmlyc3QoKQogICAgaWYgZXJyICE9IG5pbHsKICAgICAgICB5YWtpdF9vdXRwdXQoZXJyKQogICAgICAgIHJldHVybiAtMSwgZmFsc2UKICAgIH0KICAgIGZvciBpbmRleCwgcGFyYW0gOj0gcmFuZ2UgQ29tbW9uUGFyYW1zewogICAgICAgIGlmIGluZGV4ID09IHBvc3sKICAgICAgICAgICAgbnVtZXJpYyA6PSBzdHIuTWF0Y2hBbGxPZlJlZ2V4cChwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSwgYF5bMC05XSskYCkKICAgICAgICAgICAgaWYgbnVtZXJpY3sKICAgICAgICAgICAgICAgIGNsb3NlVHlwZSA6PSBjaGVja1BhcmFtKHBhcmFtLCBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSwgb3JpZ2luUmVzdWx0LCBudW1lcmljKQogICAgICAgICAgICAgICAgaWYgY2xvc2VUeXBlID09IC0xeyAvL+ajgOa1i+aNouihjOaDheWGtQogICAgICAgICAgICAgICAgICAgIGNsb3NlVHlwZSA9IGNoZWNrUGFyYW0ocGFyYW0sIHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgIlxuIiwgb3JpZ2luUmVzdWx0LCBudW1lcmljKQogICAgICAgICAgICAgICAgICAgIGlmIGNsb3NlVHlwZSA9PSAtMXsKICAgICAgICAgICAgICAgICAgICAgICAgcmV0dXJuIC0xLCBmYWxzZSAvL+i/m+ihjOS4i+S4gOS4quWPguaVsOeahOajgOa1iwogICAgICAgICAgICAgICAgICAgIH1lbHNlewogICAgICAgICAgICAgICAgICAgICAgICByZXR1cm4gY2xvc2VUeXBlLCB0cnVlCiAgICAgICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgfWVsc2V7CiAgICAgICAgICAgICAgICAgICAgcmV0dXJuIGNsb3NlVHlwZSwgZmFsc2UKICAgICAgICAgICAgICAgIH0KICAgICAgICAgICAgfWVsc2V7IC8v5Y+C5pWw5Li65a2X56ym5Liy57G75Z6LCiAgICAgICAgICAgICAgICBjbG9zZVR5cGUgOj0gY2hlY2tQYXJhbShwYXJhbSwgcGFyc2VTdHIocGFyYW0uVmFsdWUoKVswXSksIG9yaWdpblJlc3VsdCwgbnVtZXJpYykKICAgICAgICAgICAgICAgIGlmIGNsb3NlVHlwZSA9PSAtMXsgLy/mo4DmtYvmjaLooYzmg4XlhrUKICAgICAgICAgICAgICAgICAgICBjbG9zZVR5cGUgPSBjaGVja1BhcmFtKHBhcmFtLCBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSArICJcbiIsIG9yaWdpblJlc3VsdCwgbnVtZXJpYykKICAgICAgICAgICAgICAgICAgICBpZiBjbG9zZVR5cGUgPT0gLTF7CiAgICAgICAgICAgICAgICAgICAgICAgIHJldHVybiAtMSwgZmFsc2UKICAgICAgICAgICAgICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgICAgICAgICAgICAgcmV0dXJuIGNsb3NlVHlwZSwgdHJ1ZQogICAgICAgICAgICAgICAgICAgIH0KICAgICAgICAgICAgICAgIH1lbHNlewogICAgICAgICAgICAgICAgICAgIHJldHVybiBjbG9zZVR5cGUsIGZhbHNlCiAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgIH0KICAgICAgICB9CiAgICB9Cgp9CgpmdW5jIGNoZWNrVGltZUJhc2VkQmxpbmQocG9zIC8qIGludCAqLywgcmVxIC8qIFtdYnl0ZSAqLywgQ29tbW9uUGFyYW1zLyogW10qbXV0YXRlLkZ1enpIVFRQUmVxdWVzdFBhcmFtICovLCBjbG9zZVR5cGUvKiBpbnQgKi8sIGxpbmVCcmVhay8qIGJvb2wgKi8pIHsKICAgIGZyZXEsIF8gOj0gZnV6ei5IVFRQUmVxdWVzdChyZXEpCiAgICAKICAgIGVyciwgc3RhbmRhcmRSZXNwVGltZSA6PSBnZXROb3JtYWxSZXNwb25kVGltZShyZXEpCiAgICBpZiBlcnIgIT0gbmlsewogICAgICAgIHlha2l0X291dHB1dChwYXJzZVN0cihlcnIpKSAvL+WboOiOt+WPluWTjeW6lOaXtumXtOWHuumUmSDkuI3lho3nu6fnu63mtYvor5Xml7bpl7Tnm7Lms6gKICAgICAgICByZXR1cm4KICAgIH0KICAgIHlha2l0X291dHB1dCgi572R56uZ55qE5q2j5bi45ZON5bqU5pe26Ze05bqU5bCP5LqOOiIgKyBwYXJzZVN0cihzdGFuZGFyZFJlc3BUaW1lKSArICJtcyIpCiAgICBpZiBsaW5lQnJlYWt7CiAgICAgICAgcGF5bG9hZCA9IHNwcmludGYoYCV2LyoqL0FuZC8qKi9TbGVlUCgldikjYCwgKCJcbiIgKyBDTE9TRV9UWVBFW2Nsb3NlVHlwZV0pLCBzdGFuZGFyZFJlc3BUaW1lICogMiAvMTAwMCArIDMpCiAgICB9ZWxzZXsKICAgICAgICBwYXlsb2FkID0gc3ByaW50ZihgJXYvKiovQW5kLyoqL1NsZWVQKCV2KSNgLCBDTE9TRV9UWVBFW2Nsb3NlVHlwZV0sIHN0YW5kYXJkUmVzcFRpbWUgKiAyIC8xMDAwICsgMykKICAgIH0KICAgIAogICAgZm9yIGluZGV4LCBwYXJhbSA6PSByYW5nZSBDb21tb25QYXJhbXMgewogICAgICAgIGlmIGluZGV4ID09IHBvcyB7CiAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5pe26Ze05rOo5YWlIikKICAgICAgICAgICAgcGF5bG9hZCA6PSAgcGFyc2VTdHIocGFyYW0uVmFsdWUoKVswXSkgKyBwYXlsb2FkCiAgICAgICAgICAgIHJlc3VsdCwgZXJyID0gcGFyYW0uRnV6eihwYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgICAgICAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5qOA5rWLIFRpbWUtQmFzZWQgQmxpbmQgU1FMIEluamVjdGlvbiBQYXlsb2FkIOWksei0pSIpCiAgICAgICAgICAgICAgICByZXR1cm4KICAgICAgICAgICAgfQogICAgICAgICAgICBpZiByZXN1bHQuRHVyYXRpb25NcyA+IHN0YW5kYXJkUmVzcFRpbWUgKyAxMDAwewogICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KHN0ci5mKCLlrZjlnKjln7rkuo7ml7bpl7TnmoQgU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDljp/lgLw6JXZdIiwgcGFyYW0uTmFtZSgpLCBwYXJhbS5WYWx1ZSgpKSkKICAgICAgICAgICAgICAgIGNvZGVjUGF5bG9hZCA9IGNvZGVjLlN0cmNvbnZRdW90ZShzdHJpbmcocGF5bG9hZCkpCiAgICAgICAgICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgICAgICAgICAgcmVzdWx0LlVybCwKICAgICAgICAgICAgICAgICAgICByaXNrLnNldmVyaXR5KCJjcml0aWNhbCIpLAogICAgICAgICAgICAgICAgICAgIHJpc2sudGl0bGUoc3RyLmYoIlRpbWUtQmFzZWQgQmxpbmQgU1FMIEluamVjdGlvbjogWyV2OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpLAogICAgICAgICAgICAgICAgICAgIHJpc2sudGl0bGVWZXJib3NlKHN0ci5mKCLlrZjlnKjln7rkuo7ml7bpl7TnmoQgU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDlgLw6JXZdIiwgcGFyYW0uTmFtZSgpLCBwYXJhbS5WYWx1ZSgpKSksCiAgICAgICAgICAgICAgICAgICAgcmlzay50eXBlKCJzcWxpbmplY3Rpb24iKSwgCiAgICAgICAgICAgICAgICAgICAgcmlzay5yZXF1ZXN0KHJlc3VsdC5SZXF1ZXN0UmF3KSwKICAgICAgICAgICAgICAgICAgICByaXNrLnJlc3BvbnNlKHJlc3VsdC5SZXNwb25zZVJhdyksCiAgICAgICAgICAgICAgICAgICAgcmlzay5wYXlsb2FkKHBheWxvYWQpLAogICAgICAgICAgICAgICAgICAgIHJpc2sucGFyYW1ldGVyKHBhcmFtLk5hbWUoKSksCiAgICAgICAgICAgICAgICApIAogICAgICAgICAgICAgICAgcmV0dXJuCiAgICAgICAgICAgIH1lbHNleyAvL+acquajgOWHuuW7tuaXtgogICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KCLmnKrmo4DmtYvliLBUaW1lQmFzZWTml7bpl7Tnm7Lms6giKQogICAgICAgICAgICAgICAgcmV0dXJuCiAgICAgICAgICAgIH0KICAgICAgICB9CiAgICB9Cn0KCmZ1bmMgY2hlY2tCb29sQmFzZWQocG9zIC8qIGludCAqLywgZnJlcSAvKiBmdXp6aHR0cCAqLykge30KCmZ1bmMgY2hlY2tVbmlvbkJhc2VkKHBvcyAvKiBpbnQgKi8sIHJlcSAvKiBbXWJ5dGUgKi8sIFRFTVBMQVRFX1BBR0VfUlNQIC8qIFtdYnl0ZSAqLywgQ29tbW9uUGFyYW1zLyogW10qbXV0YXRlLkZ1enpIVFRQUmVxdWVzdFBhcmFtICovLCBjbG9zZVR5cGUvKiBpbnQgKi8sIGxpbmVCcmVhayAvKiBib29sICovKSB7CiAgICAvL+WFiOS9v+eUqE9SREVSIEJZ54yc5YiX5pWwCiAgICBpZiBndWVzc0NvbHVtbk51bShwb3MsIHJlcSwgVEVNUExBVEVfUEFHRV9SU1AgLyogW11idHllICovLCBDb21tb25QYXJhbXMsIGNsb3NlVHlwZSwgbGluZUJyZWFrKSAhPSAtMXsKICAgICAgICByZXR1cm4KICAgIH0KICAgIGlmIGJydXRlQ29sdW1uTnVtKHBvcywgcmVxLCBURU1QTEFURV9QQUdFX1JTUCAvKiBbXWJ0eWUgKi8sIENvbW1vblBhcmFtcywgY2xvc2VUeXBlLCBsaW5lQnJlYWspICE9IC0xewogICAgICAgIHJldHVybgogICAgfSAKICAgIHlha2l0X291dHB1dCgi5pyq5qOA5rWL5YiwVU5JT07ogZTlkIjms6jlhaUiKQogICAgCiAgICAKfQoKZnVuYyBndWVzc0NvbHVtbk51bShwb3MgLyogaW50ICovLCByZXEgLyogW11ieXRlICovLCBURU1QTEFURV9QQUdFX1JTUCAvKiBbXWJ5dGUgKi8gLCBDb21tb25QYXJhbXMvKiBbXSptdXRhdGUuRnV6ekhUVFBSZXF1ZXN0UGFyYW0gKi8sIGNsb3NlVHlwZS8qIGludCAqLywgbGluZUJyZWFrLyogYm9vbCAqLyl7CiAgICBmcmVxLCBfIDo9IGZ1enouSFRUUFJlcXVlc3QocmVxKQogICAgT1JERVJfQllfU1RFUCA6PSAxMCAKICAgIE9SREVSX0JZX01BWCA6PSAxMDAwCiAgICBsb3dDb2xzLCBoaWdoQ29scyA9IDEsIE9SREVSX0JZX1NURVAKICAgIGZvdW5kID0gZmFsc2UKICAgIERFRkFVTFRfUkFUSU8gOj0gLTEKCiAgICBjb25kaXRpb25fMSwgREVGQVVMVF9SQVRJTywgXyA9IG9yZGVyQnlUZXN0KDEsIHBvcywgcmVxLCBURU1QTEFURV9QQUdFX1JTUCwgREVGQVVMVF9SQVRJTywgQ29tbW9uUGFyYW1zLCBjbG9zZVR5cGUsIGxpbmVCcmVhaykKICAgIGNvbmRpdGlvbl8yLCBERUZBVUxUX1JBVElPLCBfID0gb3JkZXJCeVRlc3QocmFuZG4oOTk5OSw5OTk5OTkpLHBvcywgcmVxLCBURU1QTEFURV9QQUdFX1JTUCwgREVGQVVMVF9SQVRJTywgQ29tbW9uUGFyYW1zLCBjbG9zZVR5cGUsIGxpbmVCcmVhaykKCiAgICBpZiBjb25kaXRpb25fMSAmJiAhY29uZGl0aW9uXzIgewogICAgICAgIGZvciAhZm91bmR7CiAgICAgICAgICAgIGNvbmRpdGlvbl92b2xhdGlsZSwgREVGQVVMVF9SQVRJTywgXyA9IG9yZGVyQnlUZXN0KGhpZ2hDb2xzLCBwb3MsIHJlcSwgVEVNUExBVEVfUEFHRV9SU1AsIERFRkFVTFRfUkFUSU8sIENvbW1vblBhcmFtcywgY2xvc2VUeXBlLCBsaW5lQnJlYWspCiAgICAgICAgICAgIGlmIGNvbmRpdGlvbl92b2xhdGlsZXsKICAgICAgICAgICAgICAgIGxvd0NvbHMgPSBoaWdoQ29scwogICAgICAgICAgICAgICAgaGlnaENvbHMgKz0gT1JERVJfQllfU1RFUAoKICAgICAgICAgICAgICAgIGlmIGhpZ2hDb2xzID4gT1JERVJfQllfTUFYewogICAgICAgICAgICAgICAgICAgIGJyZWFrCiAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgIH1lbHNlewogICAgICAgICAgICAgICAgZm9yICFmb3VuZHsKICAgICAgICAgICAgICAgICAgICBtaWQgPSBoaWdoQ29scyAtIG1hdGguUm91bmQoKGhpZ2hDb2xzIC0gbG93Q29scykgLyAyKQogICAgICAgICAgICAgICAgICAgIGNvbmRpdGlvbl92b2xhdGlsZV9zZWMsIERFRkFVTFRfUkFUSU8sIHJlc3VsdCA9IG9yZGVyQnlUZXN0KG1pZCwgcG9zLCByZXEsIFRFTVBMQVRFX1BBR0VfUlNQICxERUZBVUxUX1JBVElPLCBDb21tb25QYXJhbXMsIGNsb3NlVHlwZSwgbGluZUJyZWFrKQogICAgICAgICAgICAgICAgICAgIGlmIGNvbmRpdGlvbl92b2xhdGlsZV9zZWN7CiAgICAgICAgICAgICAgICAgICAgICAgIGxvd0NvbHMgPSBtaWQKICAgICAgICAgICAgICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgICAgICAgICAgICAgaGlnaENvbHMgPSBtaWQKICAgICAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgICAgICAgICAgaWYgKGhpZ2hDb2xzIC0gbG93Q29scykgPCAyewogICAgICAgICAgICAgICAgICAgICAgICBjb2x1bW5OdW0gOj0gbG93Q29scwogICAgICAgICAgICAgICAgICAgICAgICBmb3VuZCA9IHRydWUKICAgICAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgICAgICB9CgogICAgICAgICAgICAgICAgZm9yIGluZGV4LCBwYXJhbSA6PSByYW5nZSBDb21tb25QYXJhbXMgewogICAgICAgICAgICAgICAgICAgIGlmIGluZGV4ID09IHBvcyB7CiAgICAgICAgICAgICAgICAgICAgICAgIGlmIGxpbmVCcmVha3sKICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICBwYXlsb2FkIDo9IHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgIlxuIiArQ0xPU0VfVFlQRVtjbG9zZVR5cGVdICsgYC8qKi9PUkRlUi8qKi9iWS8qKi9gK3BhcnNlU3RyKGNvbHVtbk51bSkgKyAiIyIKICAgICAgICAgICAgICAgICAgICAgICAgfWVsc2V7CiAgICAgICAgICAgICAgICAgICAgICAgICAgICBwYXlsb2FkIDo9IHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgQ0xPU0VfVFlQRVtjbG9zZVR5cGVdICsgYC8qKi9PUkRlUi8qKi9iWS8qKi9gK3BhcnNlU3RyKGNvbHVtbk51bSkgKyAiIyIKICAgICAgICAgICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJlc3VsdC5VcmwsCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2suc2V2ZXJpdHkoImNyaXRpY2FsIiksCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sudGl0bGUoc3RyLmYoIlVuaW9uLUJhc2VkIFNRTCBJbmplY3Rpb246IFsldjoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKSwKICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgcmlzay50aXRsZVZlcmJvc2Uoc3RyLmYoIuWtmOWcqOWfuuS6jlVOSU9OIFNRTCDms6jlhaU6IFvlj4LmlbDlkI06JXYg5YC8OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpLAogICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICByaXNrLnR5cGUgKCJzcWxpbmplY3Rpb24iKSwgCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucGF5bG9hZChwYXlsb2FkKSwgCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucGFyYW1ldGVyKHBhcmFtLk5hbWUoKSksCiAgICAgICAgICAgICAgICAgICAgCiAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICkKICAgICAgICAgICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KHN0ci5mKCLlrZjlnKjln7rkuo5VTklPTiBTUUwg5rOo5YWlOiBb5Y+C5pWw5ZCNOiV2IOWOn+WAvDoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKClbMF0pKQogICAgICAgICAgICAgICAgICAgICAgICB5YWtpdF9vdXRwdXQoc3RyLmYoIlVOSU9OIOWIl+aVsOe7j+i/h09SREVSIEJZIOaOoua1i+S4uiIgKyBwYXJzZVN0cihjb2x1bW5OdW0pKSkKICAgICAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgICAgICAgfQogICAgICAgICAgICB9CiAgICAgICAgICAgICAgICByZXR1cm4gIGNvbHVtbk51bQogICAgICAgICAgICB9CiAgICAgICAgfQogICAgcmV0dXJuIC0xCn0KICAgIApmdW5jIG9yZGVyQnlUZXN0KG51bWJlciAvKmludCovLCBwb3MgLyogaW50ICovLCByZXEgLyogW11ieXRlICovLCBURU1QTEFURV9QQUdFX1JTUCAvKiBbXWJ5dGUgKi8sIERFRkFVTFRfUkFUSU8vKiBmbG9hdCAqLywgQ29tbW9uUGFyYW1zLyogW10qbXV0YXRlLkZ1enpIVFRQUmVxdWVzdFBhcmFtICovLCBjbG9zZVR5cGUvKiBpbnQgKi8sIGxpbmVCcmVhayAvKiBib29sICovKXsKICAgIGZyZXEsIF8gOj0gZnV6ei5IVFRQUmVxdWVzdChyZXEpCiAgICBmb3IgaW5kZXgsIHBhcmFtIDo9IHJhbmdlIENvbW1vblBhcmFtcyB7CiAgICAgICAgaWYgaW5kZXggPT0gcG9zIHsKICAgICAgICAgICAgaWYgbGluZUJyZWFrewogICAgICAgICAgICAgICAgcGF5bG9hZCA6PSBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSArICJcbiIgK0NMT1NFX1RZUEVbY2xvc2VUeXBlXSArIGAvKiovT1JEZVIvKiovYlkvKiovYCtwYXJzZVN0cihudW1iZXIpICsgIiMiCiAgICAgICAgICAgIH1lbHNlewogICAgICAgICAgICAgICAgcGF5bG9hZCA6PSBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSArIENMT1NFX1RZUEVbY2xvc2VUeXBlXSArIGAvKiovT1JEZVIvKiovYlkvKiovYCtwYXJzZVN0cihudW1iZXIpICsgIiMiCiAgICAgICAgICAgIH0KICAgICAgICAgICAgCiAgICAgICAgICAgIHJlc3VsdCwgZXJyID0gcGFyYW0uRnV6eihwYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgICAgICAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5qOA5rWLIE9yZGVyIGJ5IOWksei0pSIpCiAgICAgICAgICAgICAgICByZXR1cm4gZmFsc2UKICAgICAgICAgICAgfQogICAgICAgICAgICBjb25kaXRpb24sIERFRkFVTFRfUkFUSU8gPSBjb21wYXJpc29uKHJlc3VsdCwgVEVNUExBVEVfUEFHRV9SU1AsIERFRkFVTFRfUkFUSU8pCiAgICAgICAgICAgIHJldHVybiAhc3RyLk1hdGNoQW55T2ZSZWdleHAocmVzdWx0LlJlc3BvbnNlUmF3LCBbIih3YXJuaW5nfGVycm9yKToiLCAib3JkZXIgKGJ5fGNsYXVzZSkiLCAidW5rbm93biBjb2x1bW4iLCAiZmFpbGVkIl0uLi4pICYmIGNvbmRpdGlvbiB8fCBzdHIuTWF0Y2hBbnlPZlJlZ2V4cChyZXN1bHQuUmVzcG9uc2VSYXcsImRhdGEgdHlwZXMgY2Fubm90IGJlIGNvbXBhcmVkIG9yIHNvcnRlZCIpLERFRkFVTFRfUkFUSU8sIHJlc3VsdAogICAgICAgIAogICAgICAgIH0KICAgIH0KCgp9CgpmdW5jIGNvbXBhcmlzb24ocmVzdWx0IC8qIGZ1enpodHRwICovLCBURU1QTEFURV9QQUdFX1JTUCAvKiBbXWJ5dGUgKi8sIERFRkFVTFRfUkFUSU8pewogICAgY29kZVJlc3VsdCA6PSByZXN1bHQuUmVzcG9uc2UuU3RhdHVzQ29kZQogICAgVEVNUExBVEVfUlNQLCBlcnIgOj0gc3RyLlBhcnNlU3RyaW5nVG9IVFRQUmVzcG9uc2UoVEVNUExBVEVfUEFHRV9SU1ApCiAgICBfLCBURU1QTEFURV9CT0RZIDo9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQoVEVNUExBVEVfUEFHRV9SU1ApCiAgICBfLCByZXN1bHRCb2R5IDo9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQocmVzdWx0LlJlc3BvbnNlUmF3KQogICAgaWYgZXJyICE9IG5pbHsKICAgICAgICBwYW5pYyhlcnIpIC8vIOeVuOW9ouWTjeW6lOWMhQogICAgfQogICAgVEVNUExBVEVfQ09ERSA6PSBURU1QTEFURV9SU1AuU3RhdHVzQ29kZQogICAgaWYgY29kZVJlc3VsdCA9PSBURU1QTEFURV9DT0RFeyAvL+WTjeW6lOeggeebuOWQjAogICAgICAgIHJhdGlvIDo9IHN0ci5DYWxjU2ltaWxhcml0eShyZXN1bHRCb2R5LCBURU1QTEFURV9CT0RZKQogICAgICAgIGlmIERFRkFVTFRfUkFUSU8gPT0gLTF7CiAgICAgICAgICAgIGlmIHJhdGlvID49IExPV0VSX1JBVElPX0JPVU5EICYmIHJhdGlvIDw9IFVQUEVSX1JBVElPX0JPVU5EewogICAgICAgICAgICAgICAgREVGQVVMVF9SQVRJTyA9IHJhdGlvCiAgICAgICAgICAgIH0KICAgICAgICB9CiAgICAgICAgaWYgcmF0aW8gPiBVUFBFUl9SQVRJT19CT1VORHsKICAgICAgICAgICAgcmV0dXJuIHRydWUsIERFRkFVTFRfUkFUSU8KICAgICAgICB9ZWxpZiByYXRpbyA8IExPV0VSX1JBVElPX0JPVU5EewogICAgICAgICAgICByZXR1cm4gZmFsc2UsIERFRkFVTFRfUkFUSU8KICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgcmV0dXJuIChyYXRpbyAtIERFRkFVTFRfUkFUSU8pID4gRElGRl9UT0xFUkFOQ0UsIERFRkFVTFRfUkFUSU8KICAgICAgICB9CiAgICB9CiAgICByZXR1cm4gZmFsc2UsIERFRkFVTFRfUkFUSU8KCn0KCmZ1bmMgYnJ1dGVDb2x1bW5OdW0ocG9zIC8qIGludCAqLywgcmVxIC8qIFtdYnl0ZSAqLywgVEVNUExBVEVfUEFHRV9SU1AgLyogW11ieXRlICovICwgQ29tbW9uUGFyYW1zLyogW10qbXV0YXRlLkZ1enpIVFRQUmVxdWVzdFBhcmFtICovLCBjbG9zZVR5cGUvKiBpbnQgKi8sIGxpbmVCcmVhay8qIGJvb2wgKi8pewogICAgCiAgICAvKiBVUFBFUl9DT1VOVCAtIExPV0VSX0NPVU5UICpNVVNUKiA+PSA1ICovCiAgICBMT1dFUl9DT1VOVCA9IDEKICAgIFVQUEVSX0NPVU5UID0gMzAKICAgIAogICAgZXJyLCBzdGFuZGFyZFJlc3BUaW1lIDo9IGdldE5vcm1hbFJlc3BvbmRUaW1lKHJlcSkKICAgIGlmIGVyciAhPSBuaWx7CiAgICAgICAgeWFraXRfb3V0cHV0KHBhcnNlU3RyKGVycikpCiAgICAgICAgcmV0dXJuCiAgICB9CgogICAgZnJlcSwgXyA6PSBmdXp6LkhUVFBSZXF1ZXN0KHJlcSkKICAgIHJhbmRTdHIgOj0gYCJgICsgcmFuZHN0cig1KSArIGAiYCArICIsIgoKICAgIFRFTVBMQVRFX1JTUCwgZXJyIDo9IHN0ci5QYXJzZVN0cmluZ1RvSFRUUFJlc3BvbnNlKFRFTVBMQVRFX1BBR0VfUlNQKQogICAgXywgVEVNUExBVEVfQk9EWSA6PSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KFRFTVBMQVRFX1BBR0VfUlNQKQogICAgCiAgICBpZiBlcnIgIT0gbmlsewogICAgICAgIHBhbmljKGVycikgLy8g55W45b2i5ZON5bqU5YyFCiAgICB9CgogICAgcmF0aW9zID0gbWFrZShtYXBbaW50XWZsb2F0KQoKICAgIGZvciBpIDo9IExPV0VSX0NPVU5UOyBpPD0gVVBQRVJfQ09VTlQ7IGkrK3sKICAgICAgICBpZiBsaW5lQnJlYWt7CiAgICAgICAgICAgIHBheWxvYWQgOj0gcGFyc2VTdHIocGFyYW0uVmFsdWUoKVswXSkgKyAiXG4iICsgQ0xPU0VfVFlQRVtjbG9zZVR5cGVdICtgLyoqL1VuaU9uLyoqL0FsbC8qKi9TZWxlY3QvKiovYCArIHN0ci5SZXBlYXQocmFuZFN0ciwgaSkKICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgcGF5bG9hZCA6PSBwYXJzZVN0cihwYXJhbS5WYWx1ZSgpWzBdKSArIENMT1NFX1RZUEVbY2xvc2VUeXBlXSArYC8qKi9VbmlPbi8qKi9BbGwvKiovU2VsZWN0LyoqL2AgKyBzdHIuUmVwZWF0KHJhbmRTdHIsIGkpCiAgICAgICAgfQogICAgICAgIAogICAgICAgIHBheWxvYWQgPSBzdHIuVHJpbVJpZ2h0KHBheWxvYWQsICIsIikgKyAiIyIKICAgICAgICBmb3IgaW5kZXgsIHBhcmFtIDo9IHJhbmdlIENvbW1vblBhcmFtc3sKICAgICAgICAgICAgaWYgaW5kZXggPT0gcG9zewogICAgICAgICAgICAgICAgcmVzdWx0LCBlcnIgOj0gcGFyYW0uRnV6eihwYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgICAgICAgICAgICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KCLlsJ3or5Xmo4DmtYsgVW5pb24gU1FMIEluamVjdGlvbiBQYXlsb2FkIOWksei0pSIpCiAgICAgICAgICAgICAgICAgICAgcmV0dXJuCiAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgICAgIAogICAgICAgICAgICAgICAgXywgcmVzdWx0Qm9keSA6PSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHJlc3VsdC5SZXNwb25zZVJhdykKICAgICAgICAgICAgICAgIAogICAgICAgICAgICAgICAgcmF0aW8gOj0gc3RyLkNhbGNTaW1pbGFyaXR5KHJlc3VsdEJvZHksIFRFTVBMQVRFX0JPRFkpCiAgICAgICAgICAgICAgICByYXRpb3NbaV0gPSByYXRpbwogICAgICAgICAgICAgICAgdGltZS5TbGVlcCgwLjMpIC8vIOmBv+WFjei/h+S6jumikee5geivt+axguWvvOiHtOmAn+eOh+iiq+mZkOWItui/m+iAjOWvvOiHtOe7k+aenOWBj+W3rgogICAgICAgICAgICAgICAgY29udGludWUKICAgICAgICAgICAgfQoKICAgICAgICB9CiAgICB9CiAgICAKICAgIC8qIOWvuXJhdGlvc+aOkuW6j+WOu+mZpOacgOWkp+acgOWwj+WAvCAqLwogICAgc2lnbmlmaWNhbmNlID0gbWFrZShtYXBbaW50XWZsb2F0KQogICAgbG93ZXN0ID0gMQogICAgaGlnaGVzdCA9IDAKICAgIGxvd2VzdF9jb3VudCA9IDAKICAgIGhpZ2hlc3RfY291bnQgPSAwCiAgICBkaXN0aW5ndWlzaCA9IC0xCgogICAgZm9yIGluZGV4LCB2YWx1ZSA6PSByYW5nZSByYXRpb3N7CiAgICAgICAgaWYgdmFsdWUgPiBoaWdoZXN0ewogICAgICAgICAgICBoaWdoZXN0ID0gdmFsdWUKICAgICAgICB9CiAgICAgICAgaWYgdmFsdWUgPCBsb3dlc3R7CiAgICAgICAgICAgIGxvd2VzdCA9IHZhbHVlCiAgICAgICAgfQogICAgfQogICAgCiAgICBtaWRkbGUgPSBtYWtlKG1hcFtpbnRdZmxvYXQpCiAgICBmb3IgaW5kZXgsIHZhbHVlIDo9IHJhbmdlIHJhdGlvc3sKICAgICAgICBpZiB2YWx1ZSAhPSBoaWdoZXN0ICYmIHZhbHVlICE9IGxvd2VzdHsKICAgICAgICAgICAgbWlkZGxlW2luZGV4XSA9IHZhbHVlCiAgICAgICAgICAgIGNvbnRpbnVlCiAgICAgICAgfQogICAgICAgIGlmIHZhbHVlID09IGhpZ2hlc3R7CiAgICAgICAgICAgIHNpZ25pZmljYW5jZVtpbmRleF0gPSB2YWx1ZQogICAgICAgICAgICBoaWdoZXN0X2NvdW50ICs9IDEKICAgICAgICAgICAgY29udGludWUKICAgICAgICB9CiAgICAgICAgaWYgdmFsdWUgPT0gbG93ZXN0ewogICAgICAgICAgICBzaWduaWZpY2FuY2VbaW5kZXhdID0gdmFsdWUKICAgICAgICAgICAgbG93ZXN0X2NvdW50ICs9IDEKICAgICAgICAgICAgY29udGludWUKICAgICAgICB9CiAgICB9ICAgIAoKICAgIGlmIGxlbihtaWRkbGUpID09IDAgJiYgaGlnaGVzdCAhPSBsb3dlc3R7IC8v5a2Y5Zyo5ZSv5LiA5Yy65YiG5YC8IOS4lCBoaWdoZXN0ICE9IGxvd2VzdCDov5nkv6nnm7jnrYnor7TmmI7miYDmnIlyYXRpb+mDveebuOWQjAogICAgICAgIGlmIGhpZ2hlc3RfY291bnQgPT0gMXsKICAgICAgICAgICAgZGlzdGluZ3Vpc2ggPSBoaWdoZXN0CiAgICAgICAgfWVsaWYgbG93ZXN0X2NvdW50ID09IDF7CiAgICAgICAgICAgIGRpc3Rpbmd1aXNoID0gbG93ZXN0CiAgICAgICAgfQogICAgfQoKICAgIGlmIGRpc3Rpbmd1aXNoICE9IC0xIHsKICAgICAgICBjb2x1bW5OdW0gPSAiIgogICAgICAgIAogICAgICAgIGZvciBpbmRleCwgdmFsdWUgOj0gcmFuZ2UgcmF0aW9zewogICAgICAgICAgICBpZiB2YWx1ZSA9PSBkaXN0aW5ndWlzaHsKICAgICAgICAgICAgICAgIGNvbHVtbk51bSA9IHBhcnNlU3RyKGluZGV4KQogICAgICAgICAgICB9CiAgICAgICAgfQogICAgICAgCiAgICAgICAgZm9yIGluZGV4LCBwYXJhbSA6PSByYW5nZSBDb21tb25QYXJhbXN7CiAgICAgICAgICAgIGlmIGluZGV4ID09IHBvc3sKICAgICAgICAgICAgICAgIHBheWxvYWQgOj0gcGFyc2VTdHIocGFyYW0uVmFsdWUoKVswXSkgKyBDTE9TRV9UWVBFW2Nsb3NlVHlwZV0gK2AvKiovVW5pT24vKiovQWxsLyoqL1NlbGVjdC8qKi9gICsgc3RyLlJlcGVhdChyYW5kU3RyLCBwYXJzZUludChjb2x1bW5OdW0pKQogICAgICAgICAgICAgICAgcGF5bG9hZCA9IHN0ci5UcmltUmlnaHQocGF5bG9hZCwgIiwiKSArICIjIiAKICAgICAgICAgICAgICAgIHJlc3VsdCwgXyA6PSBwYXJhbS5GdXp6KHBheWxvYWQpLkV4ZWNGaXJzdCgpCiAgICAgICAgICAgICAgICB5YWtpdF9vdXRwdXQoc3RyLmYoIuWtmOWcqFVOSU9OIFNRTCDms6jlhaU6IFvlj4LmlbDlkI06JXYg5Y6f5YC8OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpCiAgICAgICAgICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgICAgICAgICAgcmVzdWx0LlVybCwKICAgICAgICAgICAgICAgICAgICByaXNrLnNldmVyaXR5KCJjcml0aWNhbCIpLAogICAgICAgICAgICAgICAgICAgIHJpc2sudGl0bGUoc3RyLmYoIlVOSU9OIFNRTCBJbmplY3Rpb246IFsldjoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKSwKICAgICAgICAgICAgICAgICAgICByaXNrLnRpdGxlVmVyYm9zZShzdHIuZigi5a2Y5ZyoVU5JT04gU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDlgLw6JXZdIiwgcGFyYW0uTmFtZSgpLCBwYXJhbS5WYWx1ZSgpKSksCiAgICAgICAgICAgICAgICAgICAgcmlzay50eXBlICgic3FsaW5qZWN0aW9uIiksIAogICAgICAgICAgICAgICAgICAgIHJpc2sucmVxdWVzdChyZXN1bHQuUmVxdWVzdFJhdyksCiAgICAgICAgICAgICAgICAgICAgcmlzay5yZXNwb25zZShyZXN1bHQuUmVzcG9uc2VSYXcpLAogICAgICAgICAgICAgICAgICAgIHJpc2sucGF5bG9hZChwYXlsb2FkKSwKICAgICAgICAgICAgICAgICAgICByaXNrLnBhcmFtZXRlcihwYXJhbS5OYW1lKCkpLAogICAgICAgICAgICAgICAgKSAKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dChzdHIuZigi5a2Y5Zyo5Z+65LqOVU5JT04gU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDljp/lgLw6JXZdIiwgcGFyYW0uTmFtZSgpLCBwYXJhbS5WYWx1ZSgpWzBdKSkKICAgICAgICAgICAgICAgIHlha2l0X291dHB1dChzdHIuZigiVU5JT04g5YiX5pWw57uP6L+HVU5JT04gQnJ1dGVGb3JjZSByYXRpb+aOoua1i+S4uiIgKyBjb2x1bW5OdW0pKQogICAgICAgICAgICAgICAgcmV0dXJuIHBhcnNlSW50KGNvbHVtbk51bSkKICAgICAgICAgICAgfSAgICAgICAgCiAgICAgICAgfQogICAgfQoKICAgIC8qIOWcqOaXoOazleS7jumhtemdouS4rWdyZXDlh7rnmoTml7blgJkg5omN6YCJ55So5pe26Ze05bCd6K+VICovCiAgICBmb3IgaSA6PSBMT1dFUl9DT1VOVDsgaTw9IFVQUEVSX0NPVU5UOyBpKyt7CiAgICAgICAgaWYgbGluZUJyZWFrewogICAgICAgICAgICBwYXlsb2FkIDo9IHBhcnNlU3RyKHBhcmFtLlZhbHVlKClbMF0pICsgIlxuIiArIENMT1NFX1RZUEVbY2xvc2VUeXBlXSArYC8qKi9VbmlPbi8qKi9TZWxlY3QvKiovYCArIHN0ci5SZXBlYXQocmFuZFN0ciwgKGktMSkpCiAgICAgICAgfWVsc2V7CiAgICAgICAgICAgIHBheWxvYWQgOj0gcGFyc2VTdHIocGFyYW0uVmFsdWUoKVswXSkgKyBDTE9TRV9UWVBFW2Nsb3NlVHlwZV0gK2AvKiovVW5pT24vKiovU2VsZWN0LyoqL2AgKyBzdHIuUmVwZWF0KHJhbmRTdHIsIChpLTEpKQogICAgICAgIH0KICAgICAgICBwYXlsb2FkICs9IHNwcmludGYoYFNMZWVwKCV2KSNgLCBzdGFuZGFyZFJlc3BUaW1lICogMiAvMTAwMCArIDMpCgogICAgICAgIGZvciBpbmRleCwgcGFyYW0gOj0gcmFuZ2UgQ29tbW9uUGFyYW1zewogICAgICAgICAgICBpZiBpbmRleCA9PSBwb3N7CiAgICAgICAgICAgICAgICAvL3ByaW50bG4ocGF5bG9hZCkKICAgICAgICAgICAgICAgIHJlc3VsdCwgZXJyIDo9IHBhcmFtLkZ1enoocGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgICAgICAgICAgICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgICAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5qOA5rWLIFVuaW9uIFNRTCBJbmplY3Rpb24gUGF5bG9hZCDlpLHotKUiKQogICAgICAgICAgICAgICAgICAgIHJldHVybgogICAgICAgICAgICAgICAgfQogICAgICAgICAgICAgICAgaWYgcmVzdWx0LkR1cmF0aW9uTXMgPiBzdGFuZGFyZFJlc3BUaW1lICsgMSB7CiAgICAgICAgICAgICAgICAgICAgeWFraXRfb3V0cHV0KHN0ci5mKCLlrZjlnKhVTklPTiBTUUwg5rOo5YWlOiBb5Y+C5pWw5ZCNOiV2IOWOn+WAvDoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKQogICAgICAgICAgICAgICAgICAgIC8vY29kZWNQYXlsb2FkID0gY29kZWMuU3RyY29udlF1b3RlKHN0cmluZyhwYXlsb2FkKSkKICAgICAgICAgICAgICAgICAgICAKICAgICAgICAgICAgICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgICAgICAgICAgICAgIHJlc3VsdC5VcmwsCiAgICAgICAgICAgICAgICAgICAgICAgIHJpc2suc2V2ZXJpdHkoImNyaXRpY2FsIiksCiAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sudGl0bGUoc3RyLmYoIlVOSU9OIFNRTCBJbmplY3Rpb246IFsldjoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKSwKICAgICAgICAgICAgICAgICAgICAgICAgcmlzay50aXRsZVZlcmJvc2Uoc3RyLmYoIuWtmOWcqFVOSU9OIFNRTCDms6jlhaU6IFvlj4LmlbDlkI06JXYg5YC8OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpLAogICAgICAgICAgICAgICAgICAgICAgICByaXNrLnR5cGUgKCJzcWxpbmplY3Rpb24iKSwgCiAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucmVxdWVzdChyZXN1bHQuUmVxdWVzdFJhdyksCiAgICAgICAgICAgICAgICAgICAgICAgIHJpc2sucmVzcG9uc2UocmVzdWx0LlJlc3BvbnNlUmF3KSwKICAgICAgICAgICAgICAgICAgICAgICAgcmlzay5wYXlsb2FkKHBheWxvYWQpLCAKICAgICAgICAgICAgICAgICAgICAgICAgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwKICAgICAgICAgICAgICAgICAgICApIAogICAgICAgICAgICAgICAgICAgIHlha2l0X291dHB1dChzdHIuZigi5Y+v6IO95a2Y5Zyo5Z+65LqOVU5JT04gU1FMIOazqOWFpTogW+WPguaVsOWQjToldiDljp/lgLw6JXZdIiwgcGFyYW0uTmFtZSgpLCBwYXJhbS5WYWx1ZSgpWzBdKSkKICAgICAgICAgICAgICAgICAgICB5YWtpdF9vdXRwdXQoc3RyLmYoIlVOSU9OIOWIl+aVsOe7j+i/h1VOSU9OIEJydXRlRm9yY2Ugc2xlZXDmjqLmtYvkuLoiICsgcGFyc2VTdHIoaSkpKQogICAgICAgICAgICAgICAgICAgIHJldHVybiBpCiAgICAgICAgICAgICAgICB9ZWxzZXsKICAgICAgICAgICAgICAgICAgICB0aW1lLlNsZWVwKDAuMykgLy8g6YG/5YWN6L+H5LqO6aKR57mB6K+35rGC5a+86Ie057uT5p6c5YGP5beuCiAgICAgICAgICAgICAgICAgICAgY29udGludWUKICAgICAgICAgICAgICAgIH0KICAgICAgICAgICAgfQogICAgICAgIH0KICAgIH0KICAgIHJldHVybiAtMQp9CgoKLy/ku6XkuIvkuLrlt6XlhbfnsbvovoXliqnlh73mlbAKZnVuYyBnZXRFcnJvckJhc2VkUHJlQ2hlY2tQYXlsb2FkKCl7CiAgICByYW5kb21UZXN0U3RyaW5nIDo9ICIiCiAgICBmb3IgaTo9MCA7aTwxMCA7aSsreyAvL+eUn+aIkOmVv+W6puS4ujEw55qE5rWL6K+V5a2X56ym5LiyCiAgICAgICAgcmFuZG9tVGVzdFN0cmluZyArPSBIRVVSSVNUSUNfQ0hFQ0tfQUxQSEFCRVRbcmFuZG4oMCxsZW4oSEVVUklTVElDX0NIRUNLX0FMUEhBQkVUKS0xKV0KICAgIH0KICAgIHJldHVybiByYW5kb21UZXN0U3RyaW5nCn0KCi8qIOWvueebruagh+WPkei1tzXmrKHor7fmsYLov5Tlm57mraPluLjlk43lupTml7bpl7QgKi8KZnVuYyBnZXROb3JtYWxSZXNwb25kVGltZShyZXEgLyogW11ieXRlICovKXsKICAgIGZyZXEsIF8gOj0gZnV6ei5IVFRQUmVxdWVzdChyZXEpCiAgICB0aW1lUmVjIDo9IFtdCiAgICAvL3lha2l0X291dHB1dChwYXJzZVN0cmluZyhsZW4oZnJlcS5HZXRDb21tb25QYXJhbXMoKSkpKQogICAgLy90aW1lLnNsZWVwKDUpCiAgICBmb3IgaTo9MDsgaTw1IDsgaSsrIHsKICAgICAgICByc3AsIGVyciA6PSBmcmVxLkV4ZWNGaXJzdCgpCiAgICAgICAgLy90aW1lLnNsZWVwKDEpCiAgICAgICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgICAgIHlha2l0X291dHB1dCgi5bCd6K+V5qOA5rWLIFRpbUJhc2VkLUJsaW5k5ZON5bqU5pe26Ze05aSx6LSlIikKICAgICAgICAgICAgcmV0dXJuIGVyciwgLTEKICAgICAgICB9CiAgICAgICAgdGltZVJlYyA9IGFwcGVuZCh0aW1lUmVjLCByc3AuRHVyYXRpb25NcykKICAgICAgICAKICAgIH0KCiAgICByZXR1cm4gbmlsLCAobWVhbih0aW1lUmVjKSArIDcgKiBzdGREZXZpYXRpb24odGltZVJlYykpIC8vOTkuOTk5OTk5OTk5NzQ0MCUg5q2j5bi455qE5ZON5bqU5pe26Ze05bqU6K+l5bCP5LqO562J5LqO6L+Z5Liq5YC8Cn0KCmZ1bmMgbWVhbih2KXsKICAgIHJlcyAgPSAwCiAgICBuID0gbGVuKHYpCiAgICBmb3IgaSA6PSAwOyBpIDwgbjsgaSsrIHsKICAgICAgICByZXMgKz0gdltpXQogICAgfQogICAgcmV0dXJuIHJlcyAvIGZsb2F0NjQobikKfQoKZnVuYyBzdGREZXZpYXRpb24odil7CiAgICB2YXJpYW5jZSA6PSBmdW5jKHYpewogICAgICAgIHJlcyAgPSAwCiAgICAgICAgbSA9IG1lYW4odikKICAgICAgICBuID0gbGVuKHYpCiAgICAgICAgZm9yIGkgOj0gMDsgaSA8IG47IGkrKyB7CiAgICAgICAgICAgIHJlcyArPSAodltpXSAtIG0pICogKHZbaV0gLSBtKQogICAgICAgIH0KICAgICAgICByZXR1cm4gcmVzIC8gZmxvYXQ2NChuLTEpCiAgICB9CiAgICByZXR1cm4gbWF0aC5TcXJ0KHZhcmlhbmNlKHYpKQp9CgpmdW5jIGNoZWNrUGFyYW0ocGFyYW0sIG9yaWdpblZhbHVlLCBvcmlnaW5SZXNwb25zZSwgaXNOdW1lcmljKSB7CiAgICBmb3IgdHlwZTo9MDsgdHlwZTxsZW4oQ0xPU0VfVFlQRSk7IHR5cGUrK3sKICAgICAgICBzd2l0Y2ggdHlwZXsKICAgICAgICAgICAgY2FzZSAwOgogICAgICAgICAgICAgICAgcmVzIDo9IGNoZWNrVHlwZTAocGFyYW0sIG9yaWdpblZhbHVlLCBvcmlnaW5SZXNwb25zZSwgaXNOdW1lcmljKQogICAgICAgICAgICAgICAgaWYgcmVzewogICAgICAgICAgICAgICAgICAgIHJldHVybiAwCiAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgIGNhc2UgMToKICAgICAgICAgICAgICAgIHJlcyA6PSBjaGVja1R5cGUxKHBhcmFtLCBvcmlnaW5WYWx1ZSwgb3JpZ2luUmVzcG9uc2UsIGlzTnVtZXJpYykKICAgICAgICAgICAgICAgIGlmIHJlc3sKICAgICAgICAgICAgICAgICAgICByZXR1cm4gMQogICAgICAgICAgICAgICAgfQogICAgICAgICAgICBjYXNlIDI6CiAgICAgICAgICAgICAgICByZXMgOj0gY2hlY2tUeXBlMihwYXJhbSwgb3JpZ2luVmFsdWUsIG9yaWdpblJlc3BvbnNlLCBpc051bWVyaWMpCiAgICAgICAgICAgICAgICBpZiByZXN7CiAgICAgICAgICAgICAgICAgICAgcmV0dXJuIDIKICAgICAgICAgICAgICAgIH0KICAgICAgICAgICAgY2FzZSAzOgogICAgICAgICAgICAgICAgcmVzIDo9IGNoZWNrVHlwZTMocGFyYW0sIG9yaWdpblZhbHVlLCBvcmlnaW5SZXNwb25zZSwgaXNOdW1lcmljKQogICAgICAgICAgICAgICAgaWYgcmVzewogICAgICAgICAgICAgICAgICAgIHJldHVybiAzCiAgICAgICAgICAgICAgICB9CiAgICAgICAgICAgIGNhc2UgNDoKICAgICAgICAgICAgICAgIHJlcyA6PSBjaGVja1R5cGU0KHBhcmFtLCBvcmlnaW5WYWx1ZSwgb3JpZ2luUmVzcG9uc2UsIGlzTnVtZXJpYykKICAgICAgICAgICAgICAgIGlmIHJlc3sKICAgICAgICAgICAgICAgICAgICByZXR1cm4gNAogICAgICAgICAgICAgICAgfQogICAgICAgIH0KICAgIH0KICAgIHJlcyA6PWNoZWNrVHlwZU9yZGVyQnkocGFyYW0sIG9yaWdpblZhbHVlLCBvcmlnaW5SZXNwb25zZSwgaXNOdW1lcmljKQogICAgaWYgcmVzewogICAgICAgIGRpZSgi5qOA5rWL5YiwT1JERVIgQlkiKSAvL25vdGhpbmcgZWxzZSB0byBkbwogICAgfQogICAgcmV0dXJuIC0xCn0KCi8qIOa1i+ivlSAnIOmXreWQiOexu+WeiyAqLwpmdW5jIGNoZWNrVHlwZTAocGFyYW0sIG9yaWdpblZhbHVlLCBvcmlnaW5SZXNwb25zZSwgSXNOdW1lcmljKXsKICAgIGRlZmVyIGZ1bmN7CiAgICAgICAgZXJyIDo9IHJlY292ZXIoKQogICAgICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgICAgICB5YWtpdF9vdXRwdXQoZXJyLkVycm9yKCkpCiAgICAgICAgfQogICAgfQogICAgaWYgSXNOdW1lcmljewogICAgICAgIHBhcmFtVHlwZSA6PSAi5pWw5a2XIgogICAgICAgIHJhbmQxID0gcmFuZG4oMSwgMjAwMDApCiAgICAgICAgcG9zaXRpdmVQYXlsb2FkID0gc3ByaW50ZigiJXYnLyoqL0FORC8qKi8nJXYnPScldiIsIG9yaWdpblZhbHVlLCByYW5kMSwgcmFuZDEpCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gc3ByaW50ZigiJXYnLyoqL0FORC8qKi8nJXYnPScldiIsIG9yaWdpblZhbHVlLCByYW5kMSwgcmFuZDErMSkKICAgIH1lbHNlewogICAgICAgIHBhcmFtVHlwZSA6PSAi5a2X56ym5LiyIgogICAgICAgIHJhbmRTdHJpbmcgOj0gcmFuZHN0cig0KQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9IHNwcmludGYoIiV2Jy8qKi9BTkQvKiovJyV2Jz0nJXYiLCBvcmlnaW5WYWx1ZSwgcmFuZFN0cmluZywgcmFuZFN0cmluZykKICAgICAgICBuZWdhdGl2ZVBheWxvYWQgPSBzcHJpbnRmKCIldicvKiovQU5ELyoqLycldic9JyV2Iiwgb3JpZ2luVmFsdWUsIHJhbmRTdHJpbmcsIHJhbmRzdHIoNSkpCiAgICB9CiAgICAKCiAgICByZXMgPSBvcmlnaW5SZXNwb25zZQogICAgXywgYm9keU9yaWdpbiA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQocmVzLlJlc3BvbnNlUmF3KQoKICAgIHAxcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihwb3NpdGl2ZVBheWxvYWQpLkV4ZWNGaXJzdCgpCiAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoc3ByaW50ZigoInJlcXVlc3QgcG9zaXRpdmUgcnNwIGVycm9yOiAlcyIpLCBlcnIpKQogICAgICAgIHJldHVybgogICAgfQogICAgXywgcEJvZHkgPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHAxcnNwLlJlc3BvbnNlUmF3KQoKICAgIG4xcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihuZWdhdGl2ZVBheWxvYWQpLkV4ZWNGaXJzdCgpCiAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoc3ByaW50ZigicmVzcG9uc2UgbmVnYXRpdmUgcnNwIGVycm9yOiAldiIsIGVycikpCiAgICAgICAgcmV0dXJuCiAgICB9CiAgICBfLCBuQm9keSA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQobjFyc3AuUmVzcG9uc2VSYXcpCgogICAgaWYgcmVzLlJlc3BvbnNlUmF3ID09IG5pbCB8fCBwMXJzcC5SZXNwb25zZVJhdyA9PSBuaWwgewogICAgICAgIHlha2l0X291dHB1dCgicmVzcG9uc2UgZW1wdHkiKQogICAgICAgIHJldHVybgogICAgfQoKICAgIG9wUmVzdWx0IDo9IHN0ci5DYWxjU2ltaWxhcml0eShib2R5T3JpZ2luLCBwQm9keSkKICAgIAogICAgaWYgb3BSZXN1bHQgPCBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICBSRUFTT04gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOWBh+WumuWNleW8leWPt+i+ueeVjO+8jFsldl3kuI7ljp/lj4LmlbDnu5PmnpzkuI3nm7jlkIwiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCkKICAgICAgICB5YWtpdF9vdXRwdXQoUkVBU09OKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHBuUmVzdWx0IDo9IHN0ci5DYWxjU2ltaWxhcml0eShwQm9keSwgbkJvZHkpCiAgIAogICAgaWYgcG5SZXN1bHQgPiBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICByZWFzb24gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOazqOWFpeajgOafpeWksei0pe+8muWOn+WboO+8mlsldl0g5LiOIFsldl0g57uT5p6c57G75Ly8L+ebuOWQjDog55u45Ly85bqm5Li677yaJXYiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCwgbmVnYXRpdmVQYXlsb2FkLCBwblJlc3VsdCkKICAgICAgICB5YWtpdF9vdXRwdXQocmVhc29uKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJolduWei1sldl0g5Y2V5byV5Y+36Zet5ZCI44CRIiwgcGFyYW1UeXBlLCBvcmlnaW5WYWx1ZSkpCgogICAgcmlzay5OZXdSaXNrKHJlcy5VcmwsIHJpc2sudGl0bGUoCiAgICAgICAgc3ByaW50ZigiTWF5YmUgU1FMIEluamVjdGlvbjogW3BhcmFtIC0gdHlwZTpzdHIgdmFsdWU6JXYgc2luZ2xlLXF1b3RlXSIsIG9yaWdpblZhbHVlKSwKICAgICksIHJpc2sudGl0bGVWZXJib3NlKHNwcmludGYoIueWkeS8vFNRTOazqOWFpe+8muOAkOWPguaVsO+8miV2WyV2XSDljZXlvJXlj7fpl63lkIjjgJEiLHBhcmFtVHlwZSwgb3JpZ2luVmFsdWUpKSwgcmlzay50eXBlKCJzcWxpbmplY3Rpb24iKSwgcmlzay5wYXlsb2FkKGNvZGVjLlN0cmNvbnZRdW90ZShuZWdhdGl2ZVBheWxvYWQpKSwgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwgcmlzay5yZXF1ZXN0KG4xcnNwLlJlcXVlc3RSYXcpLCByaXNrLnJlc3BvbnNlKG4xcnNwLlJlc3BvbnNlUmF3KSkKICAgIHJldHVybiB0cnVlCn0KCgovKiDmtYvor5UgIiDpl63lkIjnsbvlnosgKi8KZnVuYyBjaGVja1R5cGUxKHBhcmFtLCBvcmlnaW5WYWx1ZSwgb3JpZ2luUmVzcG9uc2UsIElzTnVtZXJpYyl7CiAgICBkZWZlciBmdW5jewogICAgICAgIGVyciA6PSByZWNvdmVyKCkKICAgICAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICAgICAgeWFraXRfb3V0cHV0KGVyci5FcnJvcigpKQogICAgICAgIH0KICAgIH0KICAgIGlmIElzTnVtZXJpY3sKICAgICAgICBwYXJhbVR5cGUgOj0gIuaVsOWtlyIKICAgICAgICByYW5kMSA9IHJhbmRuKDEsIDIwMDAwKQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9IHNwcmludGYoYCV2Ii8qKi9BTkQvKiovIiV2Ij0iJXZgLCBvcmlnaW5WYWx1ZSwgcmFuZDEsIHJhbmQxKQogICAgICAgIG5lZ2F0aXZlUGF5bG9hZCA9IHNwcmludGYoYCV2Ii8qKi9BTkQvKiovIiV2Ij0iJXZgLCBvcmlnaW5WYWx1ZSwgcmFuZDEsIHJhbmQxKzEpCiAgICB9ZWxzZXsKICAgICAgICBwYXJhbVR5cGUgOj0gIuWtl+espuS4siIKICAgICAgICByYW5kU3RyaW5nIDo9IHJhbmRzdHIoNCkKICAgICAgICBwb3NpdGl2ZVBheWxvYWQgPSBzcHJpbnRmKGAldiIvKiovQU5ELyoqLyIldiI9IiV2YCwgb3JpZ2luVmFsdWUsIHJhbmRTdHJpbmcsIHJhbmRTdHJpbmcpCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gc3ByaW50ZihgJXYiLyoqL0FORC8qKi8iJXYiPSIldmAsIG9yaWdpblZhbHVlLCByYW5kU3RyaW5nLCByYW5kc3RyKDUpKQogICAgfQogICAgCgogICAgcmVzID0gb3JpZ2luUmVzcG9uc2UKICAgIF8sIGJvZHlPcmlnaW4gPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHJlcy5SZXNwb25zZVJhdykKCiAgICBwMXJzcCwgZXJyIDo9IHBhcmFtLkZ1enoocG9zaXRpdmVQYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KHNwcmludGYoKCJyZXF1ZXN0IHBvc2l0aXZlIHJzcCBlcnJvcjogJXMiKSwgZXJyKSkKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KICAgIF8sIHBCb2R5ID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChwMXJzcC5SZXNwb25zZVJhdykKCiAgICBuMXJzcCwgZXJyIDo9IHBhcmFtLkZ1enoobmVnYXRpdmVQYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KHNwcmludGYoInJlc3BvbnNlIG5lZ2F0aXZlIHJzcCBlcnJvcjogJXYiLCBlcnIpKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQogICAgXywgbkJvZHkgPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KG4xcnNwLlJlc3BvbnNlUmF3KQoKICAgIGlmIHJlcy5SZXNwb25zZVJhdyA9PSBuaWwgfHwgcDFyc3AuUmVzcG9uc2VSYXcgPT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoInJlc3BvbnNlIGVtcHR5IikKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KCiAgICBvcFJlc3VsdCA6PSBzdHIuQ2FsY1NpbWlsYXJpdHkoYm9keU9yaWdpbiwgcEJvZHkpCiAgIAogICAgaWYgb3BSZXN1bHQgPCBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICBSRUFTT04gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOWBh+WumuWPjOW8leWPt+i+ueeVjO+8jFsldl3kuI7ljp/lj4LmlbDnu5PmnpzkuI3nm7jlkIwiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCkKICAgICAgICB5YWtpdF9vdXRwdXQoUkVBU09OKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHBuUmVzdWx0IDo9IHN0ci5DYWxjU2ltaWxhcml0eShwQm9keSwgbkJvZHkpCiAgICAKICAgIGlmIHBuUmVzdWx0ID4gU0lNSUxBUklUWV9SSVRJTyB7CiAgICAgICAgcmVhc29uID0gc3ByaW50Zigi5Y+C5pWw5Li6JXbvvIzms6jlhaXmo4Dmn6XlpLHotKXvvJrljp/lm6DvvJpbJXZdIOS4jiBbJXZdIOe7k+aenOexu+S8vC/nm7jlkIw6IOebuOS8vOW6puS4uu+8miV2IiwgcGFyYW1UeXBlLCBwb3NpdGl2ZVBheWxvYWQsIG5lZ2F0aXZlUGF5bG9hZCwgcG5SZXN1bHQpCiAgICAgICAgeWFraXRfb3V0cHV0KHJlYXNvbikKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KCiAgICB5YWtpdF9vdXRwdXQoc3ByaW50Zigi55aR5Ly8U1FM5rOo5YWl77ya44CQ5Y+C5pWw77yaJXblnotbJXZdIOWPjOW8leWPt+mXreWQiOOAkSIsIHBhcmFtVHlwZSwgb3JpZ2luVmFsdWUpKQoKICAgIHJpc2suTmV3UmlzayhyZXMuVXJsLCByaXNrLnRpdGxlKAogICAgICAgIHNwcmludGYoIk1heWJlIFNRTCBJbmplY3Rpb246IFtwYXJhbSAtIHR5cGU6c3RyIHZhbHVlOiV2IHNpbmdsZS1xdW90ZV0iLCBvcmlnaW5WYWx1ZSksCiAgICApLCByaXNrLnRpdGxlVmVyYm9zZShzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJoldlsldl0g5Y+M5byV5Y+36Zet5ZCI44CRIixwYXJhbVR5cGUsIG9yaWdpblZhbHVlKSksIHJpc2sudHlwZSgic3FsaW5qZWN0aW9uIiksIHJpc2sucGF5bG9hZChjb2RlYy5TdHJjb252UXVvdGUobmVnYXRpdmVQYXlsb2FkKSksIHJpc2sucGFyYW1ldGVyKHBhcmFtLk5hbWUoKSksIHJpc2sucmVxdWVzdChuMXJzcC5SZXF1ZXN0UmF3KSwgcmlzay5yZXNwb25zZShuMXJzcC5SZXNwb25zZVJhdykpCiAgICByZXR1cm4gdHJ1ZQp9CgoKLyog5rWL6K+V5peg6Zet5ZCI57G75Z6LICovCmZ1bmMgY2hlY2tUeXBlMihwYXJhbSwgb3JpZ2luVmFsdWUsIG9yaWdpblJlc3BvbnNlLCBJc051bWVyaWMpewogICAgIGRlZmVyIGZ1bmN7CiAgICAgICAgZXJyIDo9IHJlY292ZXIoKQogICAgICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgICAgICB5YWtpdF9vdXRwdXQoZXJyLkVycm9yKCkpCiAgICAgICAgfQogICAgfQoKICAgIGlmIElzTnVtZXJpY3sKICAgICAgICBwYXJhbVR5cGUgOj0gIuaVsOWtlyIKICAgICAgICByYW5kMSA9IHJhbmRuKDEsIDIwMDAwKQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9IHNwcmludGYoIiV2LyoqL0FORC8qKi8ldj0ldiIsIG9yaWdpblZhbHVlLCByYW5kMSwgcmFuZDEpCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gc3ByaW50ZigiJXYvKiovQU5ELyoqLyV2PSV2Iiwgb3JpZ2luVmFsdWUsIHJhbmQxLCByYW5kMSsxKQogICAgfWVsc2V7CiAgICAgICAgcGFyYW1UeXBlIDo9ICLlrZfnrKbkuLIiCiAgICAgICAgcmFuZFN0cmluZyA6PSByYW5kc3RyKDQpCiAgICAgICAgcG9zaXRpdmVQYXlsb2FkID0gc3ByaW50ZigiJXYvKiovQU5ELyoqLycldic9JyV2JyIsIG9yaWdpblZhbHVlLCByYW5kU3RyaW5nLCByYW5kU3RyaW5nKQogICAgICAgIG5lZ2F0aXZlUGF5bG9hZCA9IHNwcmludGYoIiV2LyoqL0FORC8qKi8nJXYnPScldiciLCBvcmlnaW5WYWx1ZSwgcmFuZFN0cmluZywgcmFuZHN0cig1KSkKICAgIH0KICAgIAoKICAgIHJlcyA9IG9yaWdpblJlc3BvbnNlCiAgICBfLCBib2R5T3JpZ2luID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChyZXMuUmVzcG9uc2VSYXcpCgogICAgcDFyc3AsIGVyciA6PSBwYXJhbS5GdXp6KHBvc2l0aXZlUGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgIHlha2l0X291dHB1dChzcHJpbnRmKCgicmVxdWVzdCBwb3NpdGl2ZSByc3AgZXJyb3I6ICVzIiksIGVycikpCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CiAgICBfLCBwQm9keSA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQocDFyc3AuUmVzcG9uc2VSYXcpCgogICAgbjFyc3AsIGVyciA6PSBwYXJhbS5GdXp6KG5lZ2F0aXZlUGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgIHlha2l0X291dHB1dChzcHJpbnRmKCJyZXNwb25zZSBuZWdhdGl2ZSByc3AgZXJyb3I6ICV2IiwgZXJyKSkKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KICAgIF8sIG5Cb2R5ID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChuMXJzcC5SZXNwb25zZVJhdykKCiAgICBpZiByZXMuUmVzcG9uc2VSYXcgPT0gbmlsIHx8IHAxcnNwLlJlc3BvbnNlUmF3ID09IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KCJyZXNwb25zZSBlbXB0eSIpCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CgogICAgb3BSZXN1bHQgOj0gc3RyLkNhbGNTaW1pbGFyaXR5KGJvZHlPcmlnaW4sIHBCb2R5KQogICAgCiAgICBpZiBvcFJlc3VsdCA8IFNJTUlMQVJJVFlfUklUSU8gewogICAgICAgIFJFQVNPTiA9IHNwcmludGYoIuWPguaVsOS4uiV277yM5YGH5a6a5peg6L6555WM77yMWyV2XeS4juWOn+WPguaVsOe7k+aenOS4jeebuOWQjCIsIHBhcmFtVHlwZSwgcG9zaXRpdmVQYXlsb2FkKQogICAgICAgIHlha2l0X291dHB1dChSRUFTT04pCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CgogICAgcG5SZXN1bHQgOj0gc3RyLkNhbGNTaW1pbGFyaXR5KHBCb2R5LCBuQm9keSkKICAgIAogICAgaWYgcG5SZXN1bHQgPiBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICByZWFzb24gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOazqOWFpeajgOafpeWksei0pe+8muWOn+WboO+8mlsldl0g5LiOIFsldl0g57uT5p6c57G75Ly8L+ebuOWQjDog55u45Ly85bqm5Li677yaJXYiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCwgbmVnYXRpdmVQYXlsb2FkLCBwblJlc3VsdCkKICAgICAgICB5YWtpdF9vdXRwdXQocmVhc29uKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJolduWei1sldl0g5peg6L6555WM6Zet5ZCI44CRIiwgcGFyYW1UeXBlLCBvcmlnaW5WYWx1ZSkpCgogICAgcmlzay5OZXdSaXNrKHJlcy5VcmwsIHJpc2sudGl0bGUoCiAgICAgICAgc3ByaW50ZigiTWF5YmUgU1FMIEluamVjdGlvbjogW3BhcmFtIC0gdHlwZTpzdHIgdmFsdWU6JXYgc2luZ2xlLXF1b3RlXSIsIG9yaWdpblZhbHVlKSwKICAgICksIHJpc2sudGl0bGVWZXJib3NlKHNwcmludGYoIueWkeS8vFNRTOazqOWFpe+8muOAkOWPguaVsO+8miV2WyV2XSDml6DovrnnlYzpl63lkIjjgJEiLHBhcmFtVHlwZSwgb3JpZ2luVmFsdWUpKSwgcmlzay50eXBlKCJzcWxpbmplY3Rpb24iKSwgcmlzay5wYXlsb2FkKGNvZGVjLlN0cmNvbnZRdW90ZShuZWdhdGl2ZVBheWxvYWQpKSwgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwgcmlzay5yZXF1ZXN0KG4xcnNwLlJlcXVlc3RSYXcpLCByaXNrLnJlc3BvbnNlKG4xcnNwLlJlc3BvbnNlUmF3KSkKICAgIHJldHVybiB0cnVlCn0KCi8qIOa1i+ivlSApJyDpl63lkIjnsbvlnosgKi8KZnVuYyBjaGVja1R5cGUzKHBhcmFtLCBvcmlnaW5WYWx1ZSwgb3JpZ2luUmVzcG9uc2UsIElzTnVtZXJpYyl7CiAgICBkZWZlciBmdW5jewogICAgICAgIGVyciA6PSByZWNvdmVyKCkKICAgICAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICAgICAgeWFraXRfb3V0cHV0KGVyci5FcnJvcigpKQogICAgICAgIH0KICAgIH0KICAgIGlmIElzTnVtZXJpY3sKICAgICAgICBwYXJhbVR5cGUgOj0gIuaVsOWtlyIKICAgICAgICByYW5kMSA9IHJhbmRuKDEsIDIwMDAwKQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9IHNwcmludGYoIiV2KScvKiovQU5ELyoqLycoJXYpJz0nKCV2Iiwgb3JpZ2luVmFsdWUsIHJhbmQxLCByYW5kMSkKICAgICAgICBuZWdhdGl2ZVBheWxvYWQgPSBzcHJpbnRmKCIldiknLyoqL0FORC8qKi8nKCV2KSc9JygldiIsIG9yaWdpblZhbHVlLCByYW5kMSwgcmFuZDErMSkKICAgIH1lbHNlewogICAgICAgIHBhcmFtVHlwZSA6PSAi5a2X56ym5LiyIgogICAgICAgIHJhbmRTdHJpbmcgOj0gcmFuZHN0cig0KQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9IHNwcmludGYoIiV2KScvKiovQU5ELyoqLycoJXYpJz0nKCV2Iiwgb3JpZ2luVmFsdWUsIHJhbmRTdHJpbmcsIHJhbmRTdHJpbmcpCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gc3ByaW50ZigiJXYpJy8qKi9BTkQvKiovJygldiknPScoJXYiLCBvcmlnaW5WYWx1ZSwgcmFuZFN0cmluZywgcmFuZHN0cig1KSkKICAgIH0KICAgIAoKICAgIHJlcyA9IG9yaWdpblJlc3BvbnNlCiAgICBfLCBib2R5T3JpZ2luID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChyZXMuUmVzcG9uc2VSYXcpCgogICAgcDFyc3AsIGVyciA6PSBwYXJhbS5GdXp6KHBvc2l0aXZlUGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgIHlha2l0X291dHB1dChzcHJpbnRmKCgicmVxdWVzdCBwb3NpdGl2ZSByc3AgZXJyb3I6ICVzIiksIGVycikpCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CiAgICBfLCBwQm9keSA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQocDFyc3AuUmVzcG9uc2VSYXcpCgogICAgbjFyc3AsIGVyciA6PSBwYXJhbS5GdXp6KG5lZ2F0aXZlUGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgIHlha2l0X291dHB1dChzcHJpbnRmKCJyZXNwb25zZSBuZWdhdGl2ZSByc3AgZXJyb3I6ICV2IiwgZXJyKSkKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KICAgIF8sIG5Cb2R5ID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChuMXJzcC5SZXNwb25zZVJhdykKCiAgICBpZiByZXMuUmVzcG9uc2VSYXcgPT0gbmlsIHx8IHAxcnNwLlJlc3BvbnNlUmF3ID09IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KCJyZXNwb25zZSBlbXB0eSIpCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CgogICAgb3BSZXN1bHQgOj0gc3RyLkNhbGNTaW1pbGFyaXR5KGJvZHlPcmlnaW4sIHBCb2R5KQogICAgCiAgICBpZiBvcFJlc3VsdCA8IFNJTUlMQVJJVFlfUklUSU8gewogICAgICAgIFJFQVNPTiA9IHNwcmludGYoIuWPguaVsOS4uiV277yM5YGH5a6a5ous5Y+35Y2V5byV5Y+36L6555WM77yMWyV2XeS4juWOn+WPguaVsOe7k+aenOS4jeebuOWQjCIsIHBhcmFtVHlwZSwgcG9zaXRpdmVQYXlsb2FkKQogICAgICAgIHlha2l0X291dHB1dChSRUFTT04pCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CgogICAgcG5SZXN1bHQgOj0gc3RyLkNhbGNTaW1pbGFyaXR5KHBCb2R5LCBuQm9keSkKIAogICAgaWYgcG5SZXN1bHQgPiBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICByZWFzb24gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOazqOWFpeajgOafpeWksei0pe+8muWOn+WboO+8mlsldl0g5LiOIFsldl0g57uT5p6c57G75Ly8L+ebuOWQjDog55u45Ly85bqm5Li677yaJXYiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCwgbmVnYXRpdmVQYXlsb2FkLCBwblJlc3VsdCkKICAgICAgICB5YWtpdF9vdXRwdXQocmVhc29uKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJolduWei1sldl0g5ous5Y+35Y2V5byV5Y+36Zet5ZCI44CRIiwgcGFyYW1UeXBlLCBvcmlnaW5WYWx1ZSkpCgogICAgcmlzay5OZXdSaXNrKHJlcy5VcmwsIHJpc2sudGl0bGUoCiAgICAgICAgc3ByaW50ZigiTWF5YmUgU1FMIEluamVjdGlvbjogW3BhcmFtIC0gdHlwZTpzdHIgdmFsdWU6JXYgc2luZ2xlLXF1b3RlXSIsIG9yaWdpblZhbHVlKSwKICAgICksIHJpc2sudGl0bGVWZXJib3NlKHNwcmludGYoIueWkeS8vFNRTOazqOWFpe+8muOAkOWPguaVsO+8miV2WyV2XSDmi6zlj7fljZXlvJXlj7fpl63lkIjjgJEiLHBhcmFtVHlwZSwgb3JpZ2luVmFsdWUpKSwgcmlzay50eXBlKCJzcWxpbmplY3Rpb24iKSwgcmlzay5wYXlsb2FkKGNvZGVjLlN0cmNvbnZRdW90ZShuZWdhdGl2ZVBheWxvYWQpKSwgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwgcmlzay5yZXF1ZXN0KG4xcnNwLlJlcXVlc3RSYXcpLCByaXNrLnJlc3BvbnNlKG4xcnNwLlJlc3BvbnNlUmF3KSkKICAgIHJldHVybiB0cnVlCn0KCi8qIOa1i+ivlSApIiDpl63lkIjnsbvlnosgKi8KZnVuYyBjaGVja1R5cGU0KHBhcmFtLCBvcmlnaW5WYWx1ZSwgb3JpZ2luUmVzcG9uc2UsIElzTnVtZXJpYyl7CiAgICAgICAgZGVmZXIgZnVuY3sKICAgICAgICBlcnIgOj0gcmVjb3ZlcigpCiAgICAgICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgICAgIHlha2l0X291dHB1dChlcnIuRXJyb3IoKSkKICAgICAgICB9CiAgICB9CiAgICBpZiBJc051bWVyaWN7CiAgICAgICAgcGFyYW1UeXBlIDo9ICLmlbDlrZciCiAgICAgICAgcmFuZDEgPSByYW5kbigxLCAyMDAwMCkKICAgICAgICBwb3NpdGl2ZVBheWxvYWQgPSBzcHJpbnRmKGAldikiLyoqL0FORC8qKi8iKCV2KSI9IigldmAsIG9yaWdpblZhbHVlLCByYW5kMSwgcmFuZDEpCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gc3ByaW50ZihgJXYpIi8qKi9BTkQvKiovIigldikiPSIoJXZgLCBvcmlnaW5WYWx1ZSwgcmFuZDEsIHJhbmQxKzEpCiAgICB9ZWxzZXsKICAgICAgICBwYXJhbVR5cGUgOj0gIuWtl+espuS4siIKICAgICAgICByYW5kU3RyaW5nIDo9IHJhbmRzdHIoNCkKICAgICAgICBwb3NpdGl2ZVBheWxvYWQgPSBzcHJpbnRmKGAldikiLyoqL0FORC8qKi8iKCV2KSI9IigldmAsIG9yaWdpblZhbHVlLCByYW5kU3RyaW5nLCByYW5kU3RyaW5nKQogICAgICAgIG5lZ2F0aXZlUGF5bG9hZCA9IHNwcmludGYoYCV2KSIvKiovQU5ELyoqLyIoJXYpIj0iKCV2YCwgb3JpZ2luVmFsdWUsIHJhbmRTdHJpbmcsIHJhbmRzdHIoNSkpCiAgICB9CiAgICAKCiAgICByZXMgPSBvcmlnaW5SZXNwb25zZQogICAgXywgYm9keU9yaWdpbiA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQocmVzLlJlc3BvbnNlUmF3KQoKICAgIHAxcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihwb3NpdGl2ZVBheWxvYWQpLkV4ZWNGaXJzdCgpCiAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoc3ByaW50ZigoInJlcXVlc3QgcG9zaXRpdmUgcnNwIGVycm9yOiAlcyIpLCBlcnIpKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQogICAgXywgcEJvZHkgPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHAxcnNwLlJlc3BvbnNlUmF3KQoKICAgIG4xcnNwLCBlcnIgOj0gcGFyYW0uRnV6eihuZWdhdGl2ZVBheWxvYWQpLkV4ZWNGaXJzdCgpCiAgICBpZiBlcnIgIT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoc3ByaW50ZigicmVzcG9uc2UgbmVnYXRpdmUgcnNwIGVycm9yOiAldiIsIGVycikpCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CiAgICBfLCBuQm9keSA9IHN0ci5TcGxpdEhUVFBIZWFkZXJzQW5kQm9keUZyb21QYWNrZXQobjFyc3AuUmVzcG9uc2VSYXcpCgogICAgaWYgcmVzLlJlc3BvbnNlUmF3ID09IG5pbCB8fCBwMXJzcC5SZXNwb25zZVJhdyA9PSBuaWwgewogICAgICAgIHlha2l0X291dHB1dCgicmVzcG9uc2UgZW1wdHkiKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIG9wUmVzdWx0IDo9IHN0ci5DYWxjU2ltaWxhcml0eShib2R5T3JpZ2luLCBwQm9keSkKICAgCiAgICBpZiBvcFJlc3VsdCA8IFNJTUlMQVJJVFlfUklUSU8gewogICAgICAgIFJFQVNPTiA9IHNwcmludGYoIuWPguaVsOS4uiV277yM5YGH5a6a5ous5Y+35Y+M5byV5Y+36L6555WM77yMWyV2XeS4juWOn+WPguaVsOe7k+aenOS4jeebuOWQjCIsIHBhcmFtVHlwZSwgcG9zaXRpdmVQYXlsb2FkKQogICAgICAgIHlha2l0X291dHB1dChSRUFTT04pCiAgICAgICAgcmV0dXJuIGZhbHNlCiAgICB9CgogICAgcG5SZXN1bHQgOj0gc3RyLkNhbGNTaW1pbGFyaXR5KHBCb2R5LCBuQm9keSkKICAgIAogICAgaWYgcG5SZXN1bHQgPiBTSU1JTEFSSVRZX1JJVElPIHsKICAgICAgICByZWFzb24gPSBzcHJpbnRmKCLlj4LmlbDkuLoldu+8jOazqOWFpeajgOafpeWksei0pe+8muWOn+WboO+8mlsldl0g5LiOIFsldl0g57uT5p6c57G75Ly8L+ebuOWQjDog55u45Ly85bqm5Li677yaJXYiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCwgbmVnYXRpdmVQYXlsb2FkLCBwblJlc3VsdCkKICAgICAgICB5YWtpdF9vdXRwdXQocmVhc29uKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJolduWei1sldl0g5ous5Y+35Y+M5byV5Y+36Zet5ZCI44CRIiwgcGFyYW1UeXBlLCBvcmlnaW5WYWx1ZSkpCgogICAgcmlzay5OZXdSaXNrKHJlcy5VcmwsIHJpc2sudGl0bGUoCiAgICAgICAgc3ByaW50ZigiTWF5YmUgU1FMIEluamVjdGlvbjogW3BhcmFtIC0gdHlwZTpzdHIgdmFsdWU6JXYgc2luZ2xlLXF1b3RlXSIsIG9yaWdpblZhbHVlKSwKICAgICksIHJpc2sudGl0bGVWZXJib3NlKHNwcmludGYoIueWkeS8vFNRTOazqOWFpe+8muOAkOWPguaVsO+8miV2WyV2XSDmi6zlj7flj4zlvJXlj7fpl63lkIjjgJEiLHBhcmFtVHlwZSwgb3JpZ2luVmFsdWUpKSwgcmlzay50eXBlKCJzcWxpbmplY3Rpb24iKSwgcmlzay5wYXlsb2FkKGNvZGVjLlN0cmNvbnZRdW90ZShuZWdhdGl2ZVBheWxvYWQpKSwgcmlzay5wYXJhbWV0ZXIocGFyYW0uTmFtZSgpKSwgcmlzay5yZXF1ZXN0KG4xcnNwLlJlcXVlc3RSYXcpLCByaXNrLnJlc3BvbnNlKG4xcnNwLlJlc3BvbnNlUmF3KSkKICAgIHJldHVybiB0cnVlCn0KCi8qIOa1i+ivlW9yZGVyIGJ55peg6Zet5ZCI57G75Z6LICovCmZ1bmMgY2hlY2tUeXBlT3JkZXJCeShwYXJhbSwgb3JpZ2luVmFsdWUsIG9yaWdpblJlc3BvbnNlLCBJc051bWVyaWMpewogICAgIGRlZmVyIGZ1bmN7CiAgICAgICAgZXJyIDo9IHJlY292ZXIoKQogICAgICAgIGlmIGVyciAhPSBuaWwgewogICAgICAgICAgICB5YWtpdF9vdXRwdXQoZXJyLkVycm9yKCkpCiAgICAgICAgfQogICAgfQoKICAgIGlmIElzTnVtZXJpY3sKICAgICAgICBwYXJhbVR5cGUgOj0gIuaVsOWtlyIKICAgICAgICByYW5kMSA9IHJhbmRuKDEsIDIwMDAwKQogICAgICAgIHBvc2l0aXZlUGF5bG9hZCA9ICIoc2VsZWN0LyoqLzEvKiovcmVnZXhwLyoqL2lmKDE9MSwxLDB4MDApKSIKICAgICAgICBuZWdhdGl2ZVBheWxvYWQgPSAiKHNlbGVjdC8qKi8xLyoqL3JlZ2V4cC8qKi9pZigxPTIsMSwweDAwKSkiCiAgICB9ZWxzZXsKICAgICAgICBwYXJhbVR5cGUgOj0gIuWtl+espuS4siIKICAgICAgICByYW5kU3RyaW5nIDo9IHJhbmRzdHIoNCkKICAgICAgICBwb3NpdGl2ZVBheWxvYWQgPSAiKHNlbGVjdC8qKi8xLyoqL3JlZ2V4cC8qKi9pZigxPTEsMSwweDAwKSkiCiAgICAgICAgbmVnYXRpdmVQYXlsb2FkID0gIihzZWxlY3QvKiovMS8qKi9yZWdleHAvKiovaWYoMT0yLDEsMHgwMCkpIgogICAgfQogICAgCgogICAgcmVzID0gb3JpZ2luUmVzcG9uc2UKICAgIF8sIGJvZHlPcmlnaW4gPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KHJlcy5SZXNwb25zZVJhdykKCiAgICBwMXJzcCwgZXJyIDo9IHBhcmFtLkZ1enoocG9zaXRpdmVQYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KHNwcmludGYoKCJyZXF1ZXN0IHBvc2l0aXZlIHJzcCBlcnJvcjogJXMiKSwgZXJyKSkKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KICAgIF8sIHBCb2R5ID0gc3RyLlNwbGl0SFRUUEhlYWRlcnNBbmRCb2R5RnJvbVBhY2tldChwMXJzcC5SZXNwb25zZVJhdykKCiAgICBuMXJzcCwgZXJyIDo9IHBhcmFtLkZ1enoobmVnYXRpdmVQYXlsb2FkKS5FeGVjRmlyc3QoKQogICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KHNwcmludGYoInJlc3BvbnNlIG5lZ2F0aXZlIHJzcCBlcnJvcjogJXYiLCBlcnIpKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQogICAgXywgbkJvZHkgPSBzdHIuU3BsaXRIVFRQSGVhZGVyc0FuZEJvZHlGcm9tUGFja2V0KG4xcnNwLlJlc3BvbnNlUmF3KQoKICAgIGlmIHJlcy5SZXNwb25zZVJhdyA9PSBuaWwgfHwgcDFyc3AuUmVzcG9uc2VSYXcgPT0gbmlsIHsKICAgICAgICB5YWtpdF9vdXRwdXQoInJlc3BvbnNlIGVtcHR5IikKICAgICAgICByZXR1cm4gZmFsc2UKICAgIH0KCiAgICBvcFJlc3VsdCA6PSBzdHIuQ2FsY1NpbWlsYXJpdHkoYm9keU9yaWdpbiwgcEJvZHkpCiAgICAKICAgIGlmIG9wUmVzdWx0IDwgU0lNSUxBUklUWV9SSVRJTyB7CiAgICAgICAgUkVBU09OID0gc3ByaW50Zigi5Y+C5pWw5Li6JXbvvIzlgYflrppPUkRFUiBCWeaXoOi+ueeVjO+8jFsldl3kuI7ljp/lj4LmlbDnu5PmnpzkuI3nm7jlkIwiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCkKICAgICAgICB5YWtpdF9vdXRwdXQoUkVBU09OKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHBuUmVzdWx0IDo9IHN0ci5DYWxjU2ltaWxhcml0eShwQm9keSwgbkJvZHkpCiAgICAKICAgIGlmIHBuUmVzdWx0ID4gU0lNSUxBUklUWV9SSVRJTyB7CiAgICAgICAgcmVhc29uID0gc3ByaW50Zigi5Y+C5pWw5Li6JXbvvIzml6DovrnnlYxPUkRFUiBCWeazqOWFpeajgOafpeWksei0pe+8muWOn+WboO+8mlsldl0g5LiOIFsldl0g57uT5p6c57G75Ly8L+ebuOWQjDog55u45Ly85bqm5Li677yaJXYiLCBwYXJhbVR5cGUsIHBvc2l0aXZlUGF5bG9hZCwgbmVnYXRpdmVQYXlsb2FkLCBwblJlc3VsdCkKICAgICAgICB5YWtpdF9vdXRwdXQocmVhc29uKQogICAgICAgIHJldHVybiBmYWxzZQogICAgfQoKICAgIHlha2l0X291dHB1dChzcHJpbnRmKCLnlpHkvLxTUUzms6jlhaXvvJrjgJDlj4LmlbDvvJolduWei1sldl0gT1JERVIgQlnml6DovrnnlYzpl63lkIjjgJEiLCBwYXJhbVR5cGUsIG9yaWdpblZhbHVlKSkKCiAgICByaXNrLk5ld1Jpc2socmVzLlVybCwgcmlzay50aXRsZSgKICAgICAgICBzcHJpbnRmKCJNYXliZSBTUUwgSW5qZWN0aW9uOiBbcGFyYW0gLSB0eXBlOnN0ciB2YWx1ZToldiBzaW5nbGUtcXVvdGVdIiwgb3JpZ2luVmFsdWUpLAogICAgKSwgcmlzay50aXRsZVZlcmJvc2Uoc3ByaW50Zigi55aR5Ly8U1FM5rOo5YWl77ya44CQ5Y+C5pWw77yaJXZbJXZdIE9SREVSIEJZ5peg6L6555WM6Zet5ZCI44CRIixwYXJhbVR5cGUsIG9yaWdpblZhbHVlKSksIHJpc2sudHlwZSgic3FsaW5qZWN0aW9uIiksIHJpc2sucGF5bG9hZChjb2RlYy5TdHJjb252UXVvdGUobmVnYXRpdmVQYXlsb2FkKSksIHJpc2sucGFyYW1ldGVyKHBhcmFtLk5hbWUoKSksIHJpc2sucmVxdWVzdChuMXJzcC5SZXF1ZXN0UmF3KSwgcmlzay5yZXNwb25zZShuMXJzcC5SZXNwb25zZVJhdykpCgogICAKICAgIGNvbmZpcm1QYXlsb2FkID0gc3ByaW50ZigiSUYoMT0xLCV2LCV2KSIsICJzbGVlcCgzKSIsIG9yaWdpblZhbHVlKQogICAgcmVzdWx0LCBfID0gcGFyYW0uRnV6eihjb25maXJtUGF5bG9hZCkuRXhlY0ZpcnN0KCkKICAgIGlmIHJlc3VsdC5EdXJhdGlvbk1zID4gMjUwMHsKICAgICAgICByaXNrLk5ld1Jpc2soCiAgICAgICAgICAgIHJlc3VsdC5VcmwsCiAgICAgICAgICAgIHJpc2suc2V2ZXJpdHkoImNyaXRpY2FsIiksCiAgICAgICAgICAgIHJpc2sudGl0bGUoc3RyLmYoIk9SREVSIEJZIFNRTCBJbmplY3Rpb246IFsldjoldl0iLCBwYXJhbS5OYW1lKCksIHBhcmFtLlZhbHVlKCkpKSwKICAgICAgICAgICAgcmlzay50aXRsZVZlcmJvc2Uoc3RyLmYoIuWtmOWcqE9SREVSIEJZIFNRTCDms6jlhaU6IFvlj4LmlbDlkI06JXYg5YC8OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpLAogICAgICAgICAgICByaXNrLnR5cGUoInNxbGluamVjdGlvbiIpLCAKICAgICAgICAgICAgcmlzay5yZXF1ZXN0KHJlc3VsdC5SZXF1ZXN0UmF3KSwKICAgICAgICAgICAgcmlzay5yZXNwb25zZShyZXN1bHQuUmVzcG9uc2VSYXcpLAogICAgICAgICAgICByaXNrLnBheWxvYWQoY29uZmlybVBheWxvYWQpLAogICAgICAgICAgICByaXNrLnBhcmFtZXRlcihwYXJhbS5OYW1lKCkpLAogICAgICAgICkKICAgICAgICB5YWtpdF9vdXRwdXQoc3RyLmYoIuWtmOWcqE9SREVSIEJZIFNRTCDms6jlhaU6IFvlj4LmlbDlkI06JXYg5YC8OiV2XSIsIHBhcmFtLk5hbWUoKSwgcGFyYW0uVmFsdWUoKSkpCiAgICAgICAgcmV0dXJuIHRydWUKICAgIH0KICAgIHJldHVybiB0cnVlIC8v5bC9566h5rKh5qOA5rWL5Ye655u05o6l5bu25pe277yM5L2G6L+Y5piv5qOA5rWL5Yiw6aG16Z2i5Y+Y5YyW77yM5Y+v6IO95pivT1JERVIgQlnms6jlhaUg6ZyA6KaB5Lq65bel5Yik5patCn0KCgoKX190ZXN0X18gPSBmdW5jKCkgewogICAgeWFraXRfb3V0cHV0KCLmtYvor5XkuK0uLi4iKQogICAgLy9yZXN1bHRzLCBlcnIgOj0geWFraXQuR2VuZXJhdGVZYWtpdE1JVE1Ib29rc1BhcmFtcygiR0VUIiwgImh0dHA6Ly94eHgueHh4Lnh4eC54eHg6ODk5MC9zcWxpL2V4YW1wbGU1LnBocD9pZD0yIikKICAgIHJlc3VsdHMsIGVyciA6PSB5YWtpdC5HZW5lcmF0ZVlha2l0TUlUTUhvb2tzUGFyYW1zKCJHRVQiLCAiaHR0cHM6Ly93d3cuYmFpZHUuY29tL3M/aWU9dXRmLTgmZj04JnJzdl9icD0xJnJzdl9pZHg9MSZ0bj1iYWlkdSZ3ZD1hYmMmZmVubGVpPTI1NiZyc3ZfcHE9MHhiNTZlYjE1YTAwMDAyY2QwJnJzdl90PTVkMWZKbDBPaEk1TWhSTGNWQ0FYVjRncjJmRSUyRmVQRXczMyUyRlhzRVJVaDBNOUtYNkR4YVV5dDBQV3FwUkQmcnFsYW5nPWVuJnJzdl9lbnRlcj0xJnJzdl9kbD10YiZyc3Zfc3VnMz00JnJzdl9zdWcxPTImcnN2X3N1Zzc9MTAxJnJzdl9zdWcyPTAmcnN2X2J0eXBlPWkmcHJlZml4c3VnPWFiYyZyc3A9NiZpbnB1dFQ9NDE1JnJzdl9zdWc0PTEzNDYiKQogICAgaWYgZXJyICE9IG5pbCB7CiAgICAgICAgeWFraXRfb3V0cHV0KCLnlJ/miJBtaXRt5Y+C5pWw5Ye66ZSZIikKICAgICAgICByZXR1cm4KICAgIH0KICAgIGlzSHR0cHMsIHVybCwgcmVxUmF3LCByc3BSYXcsIGJvZHkgPSByZXN1bHRzCgogICAgbWlycm9yRmlsdGVyZWRIVFRQRmxvdyhyZXN1bHRzLi4uKQogICAgeWFraXRfb3V0cHV0KCLmtYvor5XlrozmiJDvvIEiKQp9Ci8vX190ZXN0X18oKQ==`)

func TestEngine_Lexer(t *testing.T) {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(string(longCodeForParserLexer)))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	start := time.Now()
	for {
		tokenStream.Consume()
		if tokenStream.LT(1).GetTokenType() == antlr.TokenEOF {
			break
		}
		if time.Now().Sub(start).Seconds() > 10 {
			panic("lexer toooooooooo slow, 10s for " + fmt.Sprint(len(tokenStream.GetAllTokens())))
		}
	}
	if len(tokenStream.GetAllTokens()) < 8000 {
		panic("lexer failed")
	}
}

func TestEngine_LexerNParser(t *testing.T) {
	_marshallerTest(string(longCodeForParserLexer))
}

func TestEngine_CompileTest(t *testing.T) {
	compiler(`a = 1`)
}

func TestForAtFirstLineWithContinue(t *testing.T) {
	compiler(`for true{
    continue
}
print("Done")`)
}

func TestForAtFirstLineWithBreak(t *testing.T) {
	compiler(`for true{
    print(1)
    for true{
        print(2)
        for true{
            print(3)
            break
        }
        break
    }
    break
}
print("Done")`)
}

func TestExecutor_MapRangeAndMembercall(t *testing.T) {
	code := `
a = {1:1,2:2,3:3,4:4}
for i = range a {
	assert i in [1, 2, 3, 4]
}
a.Set(5, 5)
keys = a.Keys()
keys.Sort()
assert keys == [1, 2, 3, 4, 5], keys
`
	_marshallerTest(code)
	_formattest(code)
}

func TestExecutor_TryCatch(t *testing.T) {
	code := `
try{}catch e{}finally{}

a = 1
try{
	panic(123)
	a++
}catch e{
	a+=3
}
assert a == 4


a = 1
try{
	panic(123)
	a++
}catch e{
	a+=2
}finally{a++}
assert a == 4

try{}catch e{}finally{}
`
	_ = code
	_formattest(`fn{
		defer fn{
			id = recover();
			{
				if id != nil {
					print("Catch"); print(id)}
				}
			{
				print("FINALLY")
			}
		}
		{print("TRY")}
	}`)

	_marshallerTest(code)
}

func TestWrappedFunc(t *testing.T) {
	code := `func wrapped(){
    return 1,2,3
}
func wrapper(){
    return wrapped()
}

a,b,c = wrapper()
`
	NewExecutor(code).VM.DebugExec()
}

func TestSingleQuote(t *testing.T) {
	code := `
ab = '"'
assert ab == "\""[0]
assert ab == "\""
assert "\"" == ab
assert 'c' + '  ' == "c  "
assert "a" == 'a'
assert "abc" == 'abc'
assert "a" == '' + 'a'

ab = 'b'
assert "ab"[1] == ab

ab = 'ab'
assert ab == "ab"

ab = 'ab""'
assert ab == "ab" + '""'
ab = 'ab\''
assert ab == "ab'"
ab = 'ab\n'
assert ab == "ab\n"
ab = 'ab\\n'
a = "ab" + '\\' + 'n'
dump(a)
assert ab == a

ab = ''
assert ab == ""
ab = 'c'
assert 'c' == 99
dump(ab)
ab = "9999"
assert ab == "" + 99 + 99
dump(ab, "" + 99 + 99)
`
	NewExecutor(code).VM.DebugExec()
}

func TestHijackVMFrameMapMemberCaller(t *testing.T) {
	executedHook := false
	hijackedFuncHooked := false
	codes := compiler(`test.dump(111)`).GetOpcodes()
	ins := yakvm.New()
	ins.ImportLibs(buildinLib)
	ins.RegisterMapMemberCallHandler("test", "dump", func(i interface{}) interface{} {
		println("HOOKED!")
		executedHook = true
		return func(origin interface{}) interface{} {
			println("HOOKED in Executor!")
			hijackedFuncHooked = true
			return origin
		}
	})
	ins.ImportLibs(map[string]interface{}{
		"test": map[string]interface{}{
			"dump": func(i interface{}) interface{} {
				println("test.dump() executed")
				spew.Dump(i)
				return i
			},
		},
	})
	ins.Exec(context.Background(), func(frame *yakvm.Frame) {
		frame.Exec(codes)
	})
	if !executedHook {
		panic("EXEC HOOK FAILED")
	}

	if !hijackedFuncHooked {
		panic("EXEC HOOK")
	}
}

func TestExecWithContext(t *testing.T) {
	code := `
print("start")
sleep(0.5)
print("end")
`
	engine := New()
	engine.ImportLibs(buildinLib)
	engine.ImportLibs(map[string]interface{}{
		"sleep": func(n float64) {
			time.Sleep(time.Duration(n * float64(time.Second)))
		},
		"print": func(v interface{}) {
			fmt.Println(v)
		},
	})
	ctx5, _ := context.WithTimeout(context.Background(), 2*time.Second)
	err := engine.Eval(ctx5, code)
	if err != nil {
		t.Fatal(err)
	}
	ctx2, _ := context.WithTimeout(context.Background(), 400*time.Millisecond)
	err = engine.Eval(ctx2, code)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatal(utils2.Errorf("expect deadline exceeded error , but get : %v", err))
	}
}

// auto convert []interface to argument type
func TestFuncCallTypeAutoConvert(t *testing.T) {
	code := `
assert typeof([]).String() == "[]interface {}", "[] type error"
assert typeof([1]).String() == "[]int", "[1] type error"
assert typeof([""]).String() == "[]string", "[\"1\"] type error"

assert getStringSliceArgumentType([]) == "[]string", "auto convert [] to []string failed"
`
	engine := New()
	engine.ImportLibs(buildinLib)
	engine.ImportLibs(map[string]interface{}{
		"getStringSliceArgumentType": func(s []string) string {
			return reflect.TypeOf(s).String()
		},
	})
	if err := engine.Eval(context.Background(), code); err != nil {
		t.Fatal(err)
	}
}

/*
变量检查的粗略实现，需要实现数据流分析后实现更精确的检查
目标：通过静态代码分析检查出执行时可能会出现的错误，需要尽可能依据代码执行顺序检查

1. 编译时检查使用未声明的变量，但对于全局定义的变量允许在非立即执行的函数内部使用
*/
func TestExecWithStrictMode(t *testing.T) {
	for index, testCase := range [][]string{
		{`
test = b=>a(b)
a = a=>a
assert(test(1) == 1)
`, ""}, {`
test = b=>a
a = 1
assert(test(1) == 1)
`, ""}, {`
fn{
	assert(a == 1)	
}
a = 1
`, "undefined variable: a"}, {`
fn{
	fn{
		assert(a == 1)	
	}
}
a = 1
`, "undefined variable: a"}, {`
f(){
	fn{
		assert(a == 1)	
	}
}
a = 1
f()
`, ""}, {`
test = b=>a
assert(test(1) == 1)
`, "undefined variable: a"}, {`
if true {
	a = 1
}else{
	a = 2
}
assert(a == 1)
`, "undefined variable: a"},
	} {
		engine := New()
		engine.ImportLibs(map[string]interface{}{
			"print": func(v interface{}) {
				fmt.Println(v)
			},
			"assert": func(b bool) {
				if !b {
					panic("assert failed")
				}
			},
		})
		engine.strictMode = true
		err := engine.SafeEval(context.Background(), testCase[0])
		if err == nil && testCase[1] != "" {
			t.Fatal(utils2.Errorf("expect error `%s`, but get `nil`, index: %d", testCase[1], index))
		}
		if err != nil {
			if !strings.Contains(err.Error(), testCase[1]) {
				t.Fatal(utils2.Errorf("expect error `%s`, but get `%v`, index: %d", testCase[1], err, index))
			}
		}
	}
}

func TestNewExecutor_ArgNumberCheck(t *testing.T) {
	code := `
test()
`
	engine := New()
	engine.ImportLibs(map[string]interface{}{
		"test": func(i int) {
		},
	})
	err := engine.SafeEval(context.Background(), code)
	if err == nil {
		t.Fatal(utils2.Errorf("expect error `native func arg number error`, but get `nil`"))
	}
}

func TestFixIssues304(t *testing.T) {
	code := `
ni = []var([]var{[]var{123}})
`
	_marshallerTest(code)
}

func TestFixCallReturnForceConvert(t *testing.T) {
	code := `
t = timeNow()
sleep(1)
t2 = timeNow()
try {
	sub = t2.Sub(t)
	printf("duration: %s\n",sub.String())
	assert typeof(sub) != int
} catch e {
	panic("time.Duration force convert to int")
}
`
	_marshallerTest(code)
}

func TestFixForSingleConditionJump(t *testing.T) {
	code := `
a = 1 
for a < 10000 {
	a ++ 
}
`
	_marshallerTest(code)
	_formattest(code)
}

func TestFixFormatterComment(t *testing.T) {
	t.Run("double comment", func(t *testing.T) {
		code := `f1 = () => 1
f1()// 1
f1()// 2
`
		code, code2 := _formatCodeTest(code)
		if strings.Contains(code2, "// 1// 1") {
			t.Fatalf("comment formatter failed, double comment")
		}
	})

	t.Run("head comment", func(t *testing.T) {
		code := `// here is head comment
f1 = () => 1
f1()// 1
f1()// 2
`
		code, code2 := _formatCodeTest(code)
		if !strings.Contains(code2, "// here is head comment") {
			t.Fatalf("comment formatter failed, no head comment")
		}
	})

	t.Run("middle comment", func(t *testing.T) {
		code := `f1 = () => 1
f1()// 1
// middle comment
f1()// 2
`
		code, code2 := _formatCodeTest(code)
		if !strings.Contains(code2, "// middle comment") {
			t.Fatalf("comment formatter failed, no middle comment")
		}
	})
}

func TestFixEmptyAnonymousFuncReturn(t *testing.T) {
	code := `a = () => {}
assert a() == nil
b = () => {1:2}
assert b() == {1:2}`
	_marshallerTest(code)
}

func TestIincludeCycle(t *testing.T) {
	// include
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	cycleCode := fmt.Sprintf(`
include "%s"
`, file.Name())
	file.WriteString(cycleCode)
	defer os.Remove(file.Name())

	inputStream := antlr.NewInputStream(cycleCode)
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)
	vt := yakast.NewYakCompiler()
	vt.AntlrTokenStream = tokenStream
	p.AddErrorListener(vt.GetParserErrorListener())
	vt.VisitProgram(p.Program().(*yak.ProgramContext))
	if len(vt.GetCompileErrors()) <= 0 {
		t.Fatalf("expect compile error, but get nil")
	}

	if !strings.Contains(vt.GetCompileErrors()[0].Message, "include cycle not allowed") {
		t.Fatalf("expect inclue cycle error, but get %v", vt.GetCompileErrors()[0].Message)
	}
}

func TestFixForFastAssign(t *testing.T) {
	code := `
a = [4, 5, 6, 7]
count = 0
for line in a {
	a.Insert(3, 100)
	count++
	assert a.Count(100) == count
}
a.Insert(3, 100)
assert a.Count(100) == count +1
`
	_marshallerTest(code)
}

func TestFixForIntAlwaysRun(t *testing.T) {
	code := `
for in 0 {
	panic("should not run for in 0, but run")
}
`
	_marshallerTest(code)
}

func TestType(t *testing.T) {
	t.Run("var and any", func(t *testing.T) {
		codeTemplate := `
a = make(map[string]%s)
assert typeof(a) == map[string]%s
	`

		_marshallerTest(fmt.Sprintf(codeTemplate, "var", "var"))
		_marshallerTest(fmt.Sprintf(codeTemplate, "var", "any"))
		_marshallerTest(fmt.Sprintf(codeTemplate, "any", "any"))
	})

	t.Run("struct type", func(t *testing.T) {
		_marshallerTest(`
assert typeof([testIns]) == typeof(wantInsSlice)
`)
	})
}

func TestAutoTypeConvert(t *testing.T) {
	t.Run("plus", func(t *testing.T) {
		code := `
assert typeof(1 + 1.1) == typeof(2.1), "int + float == float failed"
assert typeof(1.1 + 1) == typeof(2.1), "float + int == float failed"
try {
	"a" + b"b"
} catch e {
	assert e != nil, "string + bytes shoule be failed"
} 

try {
	b"a" + "b"
} catch e {
	assert e != nil, "bytes + string shoule be failed"
} 
v = "你好"
assert v[0] == '你', "string[0] == char failed"
assert "你" + '好' == "你好","string + char == string failed"
assert '你'+"好" == "你好","char + string == string failed"
assert "1" + 1 == "11", "string + int == string failed"
assert 1 + "1" == "11", "int + string == string failed"
assert b"1" + 1 == b"11", "bytes + int == bytes failed"
assert 1 + b"1" == b"11", "int + bytes == bytes failed"
`

		_marshallerTest(code)
	})
}

func TestCancelCtx(t *testing.T) {
	code := `
wg = NewSizedWaitGroup(1)

for i=0 ; i < 20; i ++ {
    t = i
    wg.Add()
    go fn {
        defer wg.Done()
        try{
            sleep(0.5)
            println(t)
        }catch e {
            println(e)
        }
	}
}

wg.Wait()
`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_marshallerTestWithCtx(code, ctx, false)
	}()

	time.Sleep(5 * time.Second)
}

func TestOrderedMap(t *testing.T) {
	code := `
om = omap({"c":3, 4: 5})
om["a"] = 1
om.b = 2
assert om.a == 1
assert om.b == 2
assert om.c == 3
assert om["4"] == 5
om.Delete("a")
om.Delete("b")
om.Delete("c")
om.Delete("4")
assert om.a == nil
assert om["b"] == nil

for i in 100 {
	om[i] = i
}

for i in 100 {
	count = 0
	for k, v in om {
		assert k == string(count)
		assert v == count
		count++
	}
}
assert len(om) == 100

om2 = omap()
om2["a"] = 1
om2["b"] = 2
om2["c"] = 3
want = '{"a":1,"b":2,"c":3}'
for i in 100 {
	b = string(jsonMarshal(om2)~)
	assert b == want, b
}

assert len(testOrderedMap.StringMap(om2)) == 3
assert len(testOrderedMap.AnyMap(om2)) == 3
m = make(map[string]var)
m["a"] = 1
m2 = make(map[var]var)
m2[0] = 1
assert len(testOrderedMap.ToOrderedMap(m)) == 1
assert len(testOrderedMap.ToOrderedMap(m2)) == 1

om3 = omap({"0": 0, "1": 1, "2": 2})
for i in 100 {
	count = 0
	for k, v in om3 {
		assert k == string(count), "k[%s] != count[%d]" % [k, count]
		assert v == count, v, "v[%s] != count[%d]" % [v, count]
		count++
	}
}
`
	_marshallerTest(code)
}

func TestHereDoc(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		testStr := `qwer1234!@#$\r\n\t\v`
		code := fmt.Sprintf(`
a = <<<EOF
%s
EOF
b = %s
assert a == b, a
`, testStr, strconv.Quote(testStr))
		_marshallerTest(code)
		_formattest(code, true)
	})

	t.Run("CRLF-CRLF", func(t *testing.T) {
		code := "a=<<<TEST\r\na\r\nTEST; a += \"b\"; dump(a);assert a == \"ab\", a;"
		_marshallerTest(code)
		_formattest(code, true)
	})
	t.Run("CRLF-LF", func(t *testing.T) {
		code := "a=<<<TEST\r\na\nTEST\r\nTEST; dump(a);assert a == \"a\\nTEST\", a;"
		// showAst(code)
		_marshallerTest(code)
		_formattest(code, true)
	})
	t.Run("LF-LF", func(t *testing.T) {
		code := "a=<<<TEST\na\nTEST + \"b\"; dump(a);assert a == \"ab\", a;"
		_marshallerTest(code)
		_formattest(code, true)
	})
	t.Run("LF-CRLF", func(t *testing.T) {
		code := "a=<<<TEST\na\r\nTEST; dump(a);assert a == \"a\\r\", a;"
		// showAst(code)
		_marshallerTest(code)
		_formattest(code, true)
	})
	t.Run("has empty line arround", func(t *testing.T) {
		code := "a=<<<EOF\n\nasd\n\nEOF; dump(a); assert a == \"\\nasd\\n\", a;"
		_marshallerTest(code)
		_formattest(code, true)
	})
	t.Run("BUG", func(t *testing.T) {
		testStr := `qwer1234!@#$%\r\n\t\vCNM;;`
		code := fmt.Sprintf(`a = <<<NM
%s
NM;	
b = %s
assert a == b, a
`, testStr, strconv.Quote(testStr))
		_marshallerTest(code)
		_formattest(code, true)
	})
}

func TestUnicode(t *testing.T) {
	code := `
assert "\xE4\xBD\xA0\xE5\xA5\xBD" == "你好"
assert "\u4F60\u597D" == "你好"
`
	_marshallerTest(code)
	_formattest(code)
}

func TestAliasTypeOp(t *testing.T) {
	code := `
assert dur("100ms") < dur("200ms")
`
	_marshallerTest(code)
	_formattest(code)
}

func TestReturnInRangeBody(t *testing.T) {
	code := `
func a(){
    for i in 10 {
        return
    }
}
assert a() == nil
`
	_marshallerTest(code)
	_formattest(code)
}

func TestTypeCast(t *testing.T) {
	code := `
number = getNumber(1)
assert "Number" in typeof(number).String()
assert typeof(int(number)).String() == "int"
assert string(int(number)) == "1"
assert int(number)+1 == 2

number2 = dur("1s")
assert typeof(number2).String() == "time.Duration"
assert typeof(int(number2)).String() == "int"
assert string(int(number2)/1000/1000/1000) == "1"
assert (int(number2)+1)/1000/1000/1000 == 1
`
	_marshallerTest(code)
	_formattest(code)
}
