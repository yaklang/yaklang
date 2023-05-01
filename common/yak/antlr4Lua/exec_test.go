package antlr4Lua

//go test -timeout 30m -tags common/yak/antlr4Lua -v -count=1

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"yaklang/common/yak/antlr4yak/yakvm"
)

// init 暂时用来注入一些函数 lua规定变量名不能@开头，所以此处函数名若@开头表明其作用为替代一些yakvm中不存在的opcode的功能
func init() {
	//os.Setenv("LUA_DEBUG", "1")
	// 使用 yakvm.Import 是最原始的导入方法 yaklang的import分两种，一种是通过script_engine.go以类似于内置依赖库的形式导入
	// 另一种是在new Engine的时候导入eval等内置函数
	// 此处先加入一些常用的lua内置函数方便测试
	// http://www.lua.org/manual/5.3/manual.html

	// print (···)
	//Receives any number of arguments and prints their values to stdout, using the
	//tostring function to convert each argument to a string. print is not intended
	//for formatted output, but only as a quick way to show a value, for instance for
	//debugging. For complete control over the output, use string.format and io.write.
	// 原生lua的print会在多个参数中间加tab，这里问题不大go默认使用ws
	yakvm.Import("print", func(v ...interface{}) {
		toStr := func(x interface{}) (string, bool) {
			switch v := x.(type) {
			case string:
				return v, true
			case int:
				return strconv.Itoa(v), true
			case float64:
				if v == float64(int64(v)) {
					return fmt.Sprintf("%.1f", v), true
				}
				return fmt.Sprintf("%.14g", v), true
			case float32:
				if v == float32(int64(v)) {
					return fmt.Sprintf("%.1f", v), true
				}
				return fmt.Sprintf("%.14g", v), true
			}
			return "", false
		}
		for index, value := range v {
			formattedVal, ok := toStr(value)
			if ok {
				v[index] = formattedVal
			}
		}
		fmt.Println(v)
	})

	yakvm.Import("raw_print", func(v interface{}) {
		fmt.Println(v)
	})

	//assert (v [, message])
	//Calls error if the value of its argument v is false (i.e., nil or false);
	//otherwise, returns all its arguments. In case of error, message is the
	//error object; when absent, it defaults to "assertion failed!"
	yakvm.Import("assert", func(condition interface{}, message string) {
		if condition == nil {
			panic(message)
		}
		if boolean, ok := condition.(bool); ok {
			if !boolean {
				panic(message)
			}
		}
	})

	yakvm.Import("@pow", func(x interface{}, y interface{}) float64 {
		interfaceToFloat64 := func(a interface{}) (float64, bool) {
			switch v := a.(type) {
			case float64:
				return v, true
			case int:
				return float64(v), true
			case int64:
				return float64(v), true
			}
			return 0, false
		}
		index, ok1 := interfaceToFloat64(x)
		base, ok2 := interfaceToFloat64(y)
		if ok1 && ok2 {
			return math.Pow(base, index)
		} else {
			panic("attempt to pow a '" + reflect.TypeOf(base).String() + "' with a '" + reflect.TypeOf(index).String() + "'")
		}

	})

	yakvm.Import("@floor", func(x interface{}, y interface{}) float64 {
		interfaceToFloat64 := func(a interface{}) (float64, bool) {
			switch v := a.(type) {
			case float64:
				return v, true
			case int:
				return float64(v), true
			case int64:
				return float64(v), true
			}
			return 0, false
		}
		base, ok1 := interfaceToFloat64(x)
		index, ok2 := interfaceToFloat64(y)
		res := base / index
		if ok1 && ok2 {
			return math.Floor(res)
		} else {
			panic("attempt to floor a '" + reflect.TypeOf(base).String() + "' with a '" + reflect.TypeOf(index).String() + "'")
		}

	})

	yakvm.Import("tostring", func(x interface{}) string {
		switch v := x.(type) {
		case string:
			return v
		case int:
			return strconv.Itoa(v)
		case float64:
			if v == float64(int64(v)) {
				return fmt.Sprintf("%.1f", v)
			}
			return fmt.Sprintf("%.14g", v)
		case float32:
			if v == float32(int64(v)) {
				return fmt.Sprintf("%.1f", v)
			}
			return fmt.Sprintf("%.14g", v)
		default:
			panic(fmt.Sprintf("tostring() cannot convert %v", reflect.TypeOf(x).String()))

		}
	})

	yakvm.Import("@strcat", func(x interface{}, y interface{}) string {
		defer func() {
			if recover() != nil {
				panic(fmt.Sprintf("attempt to concatenate %v with %v", reflect.TypeOf(x).String(), reflect.TypeOf(y).String()))
			}
		}()
		toStr := func(x interface{}) string {
			switch v := x.(type) {
			case string:
				return v
			case int:
				return strconv.Itoa(v)
			case float64:
				return fmt.Sprintf("%.14g", v)
			case float32:
				return fmt.Sprintf("%.14g", v)
			default:
				panic(fmt.Sprintf("tostring() cannot convert %v", x))

			}
		}
		return toStr(x) + toStr(y)
	})

	yakvm.Import("@getlen-=", func(x interface{}) int {
		if str, ok := x.(string); ok {
			return len(str)
		}
		rk := reflect.TypeOf(x).Kind()
		if rk == reflect.Map {
			valueOfInputMap := reflect.ValueOf(x)
			if reflect.TypeOf(x).Key().Kind() != reflect.Int && reflect.TypeOf(x).Key().Kind() != reflect.Interface {
				return 0
			}
			mapLen := valueOfInputMap.Len()
			tblLen := 0

			for index := 1; index <= mapLen; index++ {
				value := valueOfInputMap.MapIndex(reflect.ValueOf(index))
				// 使用 reflect 访问 map 元素时，需要注意检查 MapIndex 返回的值是否可用，
				// 因为如果索引对应的键不存在，或者键对应的值为 nil，那么 MapIndex 方法会返回一个无效的 Value，
				// 不能直接使用，需要先检查一下
				if value.IsValid() && !value.IsZero() {
					tblLen++
				} else {
					return tblLen
				}
			}
			return tblLen
		}
		panic(fmt.Sprintf("attempt to get length of %v", reflect.TypeOf(x).String()))
	})

	yakvm.Import("next", func(x ...interface{}) (interface{}, interface{}) { // next(table[,index])
		if len(x) == 1 || x[1] == nil {
			keysString := make([]string, 0)
			keysMap := make(map[string]interface{})
			valueOfInputMap := reflect.ValueOf(x[0])
			iter := valueOfInputMap.MapRange()
			for iter.Next() {
				keyStr := yakvm.NewAutoValue(iter.Key().Interface()).String()
				keysString = append(keysString, keyStr)
				keysMap[keyStr] = iter.Key().Interface()
			}
			sort.Strings(keysString)
			return keysMap[keysString[0]], valueOfInputMap.MapIndex(reflect.ValueOf(keysMap[keysString[0]])).Interface()
		} else {
			keysString := make([]string, 0)
			keysMap := make(map[string]interface{})
			valueOfInputMap := reflect.ValueOf(x[0])
			indexToNext := reflect.ValueOf(x[1]).String()
			iter := valueOfInputMap.MapRange()
			for iter.Next() {
				keyStr := yakvm.NewAutoValue(iter.Key().Interface()).String()
				keysString = append(keysString, keyStr)
				keysMap[keyStr] = iter.Key().Interface()
			}
			sort.Strings(keysString)
			// map的key是唯一的
			for index, value := range keysString {
				if value == indexToNext {
					if index+1 == len(keysString) {
						return nil, nil
					} else {
						return keysMap[keysString[index+1]], valueOfInputMap.MapIndex(reflect.ValueOf(keysMap[keysString[index+1]])).Interface()
					}
				}
			}
			panic("`invalid key to 'next'`")
		}
		return nil, nil
	})

	yakvm.Import("error", func(x any) {
		panic(x)
	})

}

func TestMultilineComment(t *testing.T) {
	code := `--[[
 多行注释
assert(false, "多行注释bug")
 多行注释
 --]]`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestLongStr(t *testing.T) {
	code := `string3 = [["Lua 教程"]]
str = '"Lua 教程"'
a=[[=["lua"]=]]
print(a)
print(str)
print(string3)
assert(string3 == '"Lua 教程"', "LongString error")
assert(a == '=["lua"]=', "LongString error")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestOperatorPriority(t *testing.T) {
	code := `a = 20
b = 10
c = 15
d = 5

e = (a + b) * c / d;-- ( 30 * 15 ) / 5
print("(a + b) * c / d 运算值为  :",e )
assert(e==90.0, "OperatorPriority not right")

e = ((a + b) * c) / d; -- (30 * 15 ) / 5
print("((a + b) * c) / d 运算值为 :",e )
assert(e==90.0, "OperatorPriority not right")

e = (a + b) * (c / d);-- (30) * (15/5)
print("(a + b) * (c / d) 运算值为 :",e )
assert(e==90.0, "OperatorPrior	ity not right")

e = a + (b * c) / d;  -- 20 + (150/5)
print("a + (b * c) / d 运算值为   :",e )
assert(e==50.0, "OperatorPriority not right")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestRoundDiv(t *testing.T) {
	code := `
	res = 5//2
	assert(res==2, "round div failed")`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestPow(t *testing.T) {
	code := `
test1 = 0^0
test2 = 1^0
test3 = (-1^0)
test4 = (-2^3)
test5 = -2^2
test6 = (3^2)
test7 = 3.14 ^ 2
test8 = 3.14 ^ 3.14
test9 = 2 ^ 3 ^ 2
assert(test1==1.0, "TestPow1 failed")
assert(test2==1.0, "TestPow2 failed")
assert(test3==-1.0, "TestPow3 failed")
assert(test4==-8.0, "TestPow4 failed")
assert(test5==-4.0, "TestPow5 failed")
assert(test6==9.0, "TestPow6 failed")
assert(test7==9.8596, "TestPow7 failed")
assert(tostring(test8)=="36.337838880175", "TestPow8 failed")
print(test9)
assert(test9 == 512.0, "TestPow9 failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestConcatOperator(t *testing.T) {
	code := `
res = (3.14 ^ 3.14) .. (3.14 ^ 3.14)
assert(tostring(res)=="36.33783888017536.337838880175", "Concat Operator <..> failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestConcatOperatorBadCase(t *testing.T) {
	defer func() {
		if msg := recover(); msg != nil {
			if !strings.Contains(msg.(*yakvm.VMPanic).Error(), "Function with float") {
				panic("BUG: StrConcat operator <..> should only accept number and string but get " + msg.(*yakvm.VMPanic).Error())
			}
		} else {
			panic("StrConcat operator <..> should only accept number and string")
		}
	}()
	code := `
function func()

end
print(func..(3.14^3.14))
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestGetLenOperator(t *testing.T) {
	code := `
res = "Zhongli (Chinese: 钟离 Zhōnglí) is a playable Geo character in Genshin Impact. A consultant of the Wangsheng Funeral Parlor, he is later revealed to be the current vessel of the Geo Archon, Morax, who has decided to experience the world from the perspective of a mortal."
assert(#res==274, "Getlen Operator <#> failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestGetLenOperator1(t *testing.T) {
	code := `
assert(#"Zhongli (Chinese: 钟离 Zhōnglí) is a playable Geo character in Genshin Impact. A consultant of the Wangsheng Funeral Parlor, he is later revealed to be the current vessel of the Geo Archon, Morax, who has decided to experience the world from the perspective of a mortal."==274, "Getlen Operator <#> failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestGetLenOperatorForTbl(t *testing.T) {
	code := `
print(#{10, 20, 30, 40, 50,"asd",[7]="s"})
print(#{["x"]="s"})
print(#{10, 20, 30, -1, 50,"asd",["x"]="s"})

assert(#{10, 20, 30, 40, 50,"asd",[7]="s"} == 7, "Get length of tbl BUG")
assert(#{["x"]="s"} == 0, "Get length of tbl BUG")
assert(#{10, 20, 30, -1, 50,"asd",["x"]="s"} == 6, "Get length of tbl BUG")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestGetLenOperatorBadCase(t *testing.T) {
	defer func() {
		if msg := recover(); msg != nil {
			if !strings.Contains(msg.(*yakvm.VMPanic).Error(), "to get length of") {
				panic("BUG: Getlen operator <#> should only accept string but get " + msg.(*yakvm.VMPanic).Error())
			}
		} else {
			panic("Getlen operator <#> should only accept string")
		}
	}()
	code := `
res = 123
print(#res)
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarReAssign(t *testing.T) {
	code := `
	a=1
	a=2
	a=3
	assert(a==3, "var-reassignment failed")`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiAssignmentRightGreaterLeft(t *testing.T) {
	code := `
a, b = 1,2,3,4,5
raw_print(a)
assert(a==1, "multi-assignment failed a, b = 1,2,3,4,5 a should be 1")
assert(b==2, "multi-assignment failed a, b = 1,2,3,4,5 b should be 2")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiAssignmentLeftGreaterRight(t *testing.T) {
	code := `
a, b, c = 1,2
assert(a==1, "multi-assignment failed a,b,c = 1,2 a should be 1 ")
assert(b==2, "multi-assignment failed a,b,c = 1,2 b should be 2 ")
assert(c==nil, "multi-assignment failed a,b,c = 1,2 c should be nil ")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiAssignmentRightGreaterLeft1(t *testing.T) {
	code := `
function gen()
    return 1,2
end

function gen2()
    return 3,4
end
a, b, c = gen(),gen2(),"nop"
assert(a==1, "multi-assignment failed a,b,c = 1,2 a should be 1 ")
assert(b==3, "multi-assignment failed a,b,c = 1,2 b should be 3 ")
assert(c=="nop", "multi-assignment failed a,b,c = 1,2 c should be nop ")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiAssignmentRightGreaterLeft2(t *testing.T) {
	code := `
function gen()
    return 1,2
end

function gen2()
    return 3,4
end
a=gen(),gen2()

c = gen(),3
print(a)
print(b)
print(c)
assert(a==1, "multi-assignment failed a,b,c = 1,2 a should be 1 ")
assert(b==nil, "multi-assignment failed a,b,c = 1,2 b should be nil ")
assert(c==1, "multi-assignment failed a,b,c = 1,2 c should be 1 ")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiAssignmentLeftGreaterRight1(t *testing.T) {
	code := `
function gen()
    return 1,2
end

a,b=gen()

print(a)
print(b)
assert(a==1, "multi-assignment failed a,b,c = 1,2 a should be 1 ")
assert(b==2, "multi-assignment failed a,b,c = 1,2 b should be 2 ")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestEqualAssign(t *testing.T) {
	code := `function gen()
    return 1,2
end

function gen2()
    return 3,4
end
a,b=gen(), gen2()
assert(a==1, "multi-assignment failed a,b,c = 1,2 a should be 1 ")
assert(b==3, "multi-assignment failed a,b,c = 1,2 b should be 3 ")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestBasicFunctionWithoutReturn(t *testing.T) {

	code := `function test2()
    print("hello test2")
end
test2()
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestBasicFunctionWithReturn(t *testing.T) {

	code := `function test2()
    return 0
end
assert(test2()==0, "function with return not work")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarDefNormal(t *testing.T) {

	code := `function a()
    assert(t==2, "default global def failed")
end
t = 2
a()
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarDefInFunc(t *testing.T) {

	code := `function test2()
	test = "inner"
    print("hello test2")
end
test2()
assert(test=="inner", "var def failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFuncDefInFunc(t *testing.T) {
	code := `function test2()
    function test()
        inner = "hello test inner"
        print("escaped success")
    end
    outer = "hello test outer"
end
test2()
test()
assert(inner == "hello test inner", "func def in func failed")
assert(outer == "hello test outer", "func def in func failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiLabelNotAllowed(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			message := fmt.Sprintf("%s", e)
			if !strings.Contains(message, `label 'cx' already defined`) {
				panic(e)
			}
		}
	}()
	code := `function test2()
    print("hello test2")
end
::cx::
::cx::
test2()
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestGotoLabelNotVisible(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			message := fmt.Sprintf("%s", e)
			if !strings.Contains(message, `no visible label 'cx' for <goto>`) {
				panic(e)
			}
		}
	}()
	code := `function test2()
	::cx::
    print("hello test2")
end
test2()
goto cx
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestClosure(t *testing.T) {
	code := `
a = "global"
function gen()
ff = function() return (a) end
return ff
end

showA = gen();
assert(showA()=="global","closure failed")
a = "block"
assert(showA()=="block","closure failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestDoEndBasic(t *testing.T) {
	code := `
do
    print("In Do-End")
    do
        print("In nested Do-End")
        do
            print("In double nested Do-End")
            do
                inside = "Play Dead Inside"
                print("In triple nested Do-End")
            end
			print(inside)
            assert(inside=="Play Dead Inside", "do-end scope not right<not defined as global>")
        end
		print(inside)
        assert(inside=="Play Dead Inside", "do-end scope not right<not defined as global>")
    end
    print(inside)
    assert(inside=="Play Dead Inside", "do-end scope not right<not defined as global>")
end

print("Outside Do-End")
print(inside)
assert(inside=="Play Dead Inside", "do-end scope not right<not defined as global>")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestDoEnd(t *testing.T) {
	code := `
local a, b = 1, 10

do
   assert(a==1, "local in do end failed 1")
   local a
   assert(a==nil, "local in do end failed 2")
end

assert(a==1, "local in do end failed 3")
assert(b==10, "local in do end failed 4")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestWhileWithoutBreak(t *testing.T) {
	code := `
a=10
while( a < 20 )
do
   print(a)
   a = a + 1
end
assert(a==20, "basic while loop bug without break")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestWhileWithBreak(t *testing.T) {
	code := `
a=10
while( a < 20 )
do
   print(a)
   a = a + 1
   break
end
assert(a==11, "basic while loop bug with break")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultipleWhileWithBreak(t *testing.T) {
	code := `
a = 10
while (a < 20) do
    print(a)
    a = a + 1
    while (a < 20) do
        print(a)
        a = a + 1
        while (a < 20) do
            print(a)
            a = a + 1
            while (a < 20) do
                print(a)
                a = a + 1
                break
            end
            break
        end
        break
    end
    break
end
assert(a == 14, "basic while loop bug with break")

	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestBasicRepeatUntil(t *testing.T) {
	code := `
--[ 变量定义 --]
a = 10
--[ 执行循环 --]
repeat
   print("a的值为:", a)
   a = a + 1
until( a > 15 )
assert(a==16, "basic repeat-until loop bug without break")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestBasicRepeatUntilWithBreak(t *testing.T) {
	code := `
--[ 变量定义 --]
a = 10
--[ 执行循环 --]
repeat
   print("a的值为:", a)
   a = a + 1
   break
until( a > 15 )
assert(a==11, "basic repeat-until loop bug with break")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestNilIf(t *testing.T) {
	code := `
if(condition) then
    assert(false, "nil in if-condition should be equivalent false")
end   
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestNilIf1(t *testing.T) {
	code := `
local condition
if(condition) then
 assert(false, "nil in if-condition should be equivalent to false")
end
`
	NewLuaSnippetExecutor(code).SmartRun()
}

// Fixed: 在lua里if 0 和 if (0) 都被认为是 true 这个和yak的jmpIfFalse等跳转的opcode的跳转逻辑不同 暂时没处理
func TestIfWithZero(t *testing.T) {
	code := `
--[ 0 为 true ]
a = false
if(0)
then
	a = true
   print("0 为 true")
end
assert(a, "0 in if-condition should be equivalent to true")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestElseIf(t *testing.T) {
	code := `
--[ 定义变量 --]
a = 100

--[ 检查布尔条件 --]
if( a == 10 )
then
   --[ 如果条件为 true 打印以下信息 --]
   assert(false, "basic else-if failed")
elseif( a == 20 )
then  
   --[ if else if 条件为 true 时打印以下信息 --]
   assert(false, "basic else-if failed")
elseif( a == 30 )
then
   --[ if else if condition 条件为 true 时打印以下信息 --]
  assert(false, "basic else-if failed")
else
   --[ 以上条件语句没有一个为 true 时打印以下信息 --]
   print("没有匹配 a 的值" )
end
print("a 的真实值为: ", a )
assert(a==100, "basic else-if failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestElseIf1(t *testing.T) {
	code := `
--[ 定义变量 --]
a = 100;
b = 200;
ok = false;
--[ 检查条件 --]
if( a == 100 )
then
   --[ if 条件为 true 时执行以下 if 条件判断 --]
   if( b == 200 )
   then
      --[ if 条件为 true 时执行该语句块 --]
      print("a 的值为 100 b 的值为 200" );
      ok = true

   end
end
assert(ok, "basic else-if failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestNot(t *testing.T) {
	code := `a = true
b = true

if ( a and b )
then
   print("a and b - 条件为 true" )
end

if ( a or b )
then
   print("a or b - 条件为 true" )
end

print("---------分割线---------" )

-- 修改 a 和 b 的值
a = false
b = true

if ( a and b )
then
   print("a and b - 条件为 true" )
else
   print("a and b - 条件为 false" )
end

if ( not( a and b) )
then
   print("not( a and b) - 条件为 true" )
else
   print("not( a and b) - 条件为 false" )
end`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIfAnd(t *testing.T) {
	code := `
print(false and true)
print(false or true)
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIfWithAnd(t *testing.T) {
	code := `
if (2 == 1 and assert(false, "if with and not short cut"))
    do
        assert(false, "if with and failed")
    end
end
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIfWithAndOr(t *testing.T) {
	code := `
check = false
if (2 == 1 and assert(false, "if with and or bug not short cut")) or (2 == 2 and 3 == 3) or assert(false, "if with and or bug not short cut") then
    do
        check = true
    end
end
assert(check, "if with and or failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIfWithAndOr1(t *testing.T) {
	code := `
check = true
if (2 == 1 and 2 == 1) and (2 == 2 and 3 == 3) then
    do
        check = false
    end
end
assert(check, "if with and or failed")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestForWithoutStep(t *testing.T) {
	code := `
cnt = 0
for x = 0, 5 do
	assert(x==cnt, "for without step bug")
    cnt = cnt + 1
    print(x)
end
assert(x==nil, "for var escaped")
assert(cnt == 6, "for without step bug 'cnt' not right")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestForWithStep(t *testing.T) {
	code := `
cnt = 0
loop_cnt = 0
for x = 0, 5, 2 do
	assert(x==cnt, "for with step bug")
	cnt = cnt + 2
	loop_cnt = loop_cnt + 1
    print(x)
end
assert(loop_cnt == 3, "for with step bug 'cnt' not right")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestForNotLoop(t *testing.T) {
	code := `
for x = 100, 5, 2 do
    assert(false, "BUG: for-loop which should not be looped get looped")
end
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestForWithNegStep(t *testing.T) {
	code := `
cnt = 6
for x = 6, 5, -1 do
	assert(x==cnt, "for with neg-step bug")
	cnt = cnt - 1
    print(x)
end
assert(cnt==4, "for with neg-step bug 'cnt' not right")

	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFunctionAutoOmitExtraValue(t *testing.T) {
	code := `function test(raw)
    return raw
end

assert(test(1,2,3,4,4,4,5,5,5,6)==1, "auto omit function param when call bug")
assert(test()==nil, "auto omit function param when call bug")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFunctionVisibility(t *testing.T) {
	code := `
function make()
    function global()
        print("FUNCTION SHOULD ESCAPED")
    end
    local function native()
        print("FUNCTION SHOULD NOT ESCAPED")
    end
end
make()
print(native)
assert(global~=nil, "FUNCTION SHOULD ESCAPED")
assert(native==nil, "FUNCTION SHOULD NOT ESCAPED")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

// Fixed: 目前多变量赋值行为还是有问题，这个主要是因为yak的opCode行为和lua不一致导致的 此前采用的办法在面对函数这种运行时动态赋值无效 tweak op_assign
func TestFunctionWithMultipleReturnVar(t *testing.T) {
	code := `
function gen()
   return 1,2,3
end
a,b,c,d=gen()
raw_print(a)
raw_print(b)
raw_print(c)
raw_print(d)
assert(a==1, "error in multiple return var1")
assert(b==2, "error in multiple return var2")
assert(c==3, "error in multiple return var3")
assert(d==nil, "error in multiple return var4")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestPrint(t *testing.T) {
	code := `
print(1)
raw_print(1)
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestOpAssign(t *testing.T) {
	code := `
a,b = 1,2
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestLocalFunc(t *testing.T) {
	code := `
a = "gg"
function gen()
local a="global"
ff = function() return (a) end
return ff
end

showA = gen();
assert(showA()=="global","closure failed1")
a = "block"
assert(showA()=="global","closure failed2")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestLocalVar(t *testing.T) {
	code := `

function gen()
local a="global"
end

showA = gen();
assert(a==nil,"closure failed1")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestLocalVisibility(t *testing.T) {
	code := `
do
local a=1
do
assert(a==1, "local visibility bug")
end
end
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestMultiLocal(t *testing.T) {
	code := `function gen()
    local a = 1
    local a
    assert(a==nil,"re-declare failed1")
end

gen()`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestLocalWithConst(t *testing.T) {
	//这个错误应该是编译时检查的
	defer func() {
		if msg := recover(); msg != nil {
			if !strings.Contains(msg.(string), "attempt to assign to const variable 'a'") {
				panic("BUG: const violation panic message not right get " + msg.(string))
			}
		} else {
			panic("const should not allowed later assign")
		}
	}()
	code := `
local a <const> =1
a=2
print(a)
print(a)
`

	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarExchange(t *testing.T) {
	code := `a = {1,2,3,4,20}
a[1],a[2], a[3] = a[4],a[3], a[2]

raw_print(a[1])
raw_print(a[2])
raw_print(a[3])
assert(a[1]==4, "Var Exchange failed")
assert(a[2]==3, "Var Exchange failed")
assert(a[3]==2, "Var Exchange failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarExchange1(t *testing.T) {
	code := `a = 1
a = a + 1

raw_print(a)
assert(a==2, "Var Exchange failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestVarExchange2(t *testing.T) {
	code := `
function test(a)
a=1+a
assert(a==2, "Var Exchange failed")
end
test(1)

`
	NewLuaSnippetExecutor(code).SmartRun()
}

// SOLVED： 现在yakvm的opcode不支持 map[nil]的写法
func TestTableConstructor(t *testing.T) {
	code := `
function f()
    return "test"
end
  a = { [f()] = g; "x", "y"; x = 1, f(), [30] = 23; 45 ,4,"lua"}
assert(a["test"] == nil, "tbl ctor bug 1")
assert(a[1] == "x", "tbl ctor bug 2")
assert(a[2] == "y", "tbl ctor bug 3")
assert(a[3] == f(), "tbl ctor bug 4")
assert(a[30] == 23, "tbl ctor bug 5")
assert(a[4] == 45, "tbl ctor bug 6")
assert(a.x == 1, "tbl ctor bug 7")
	`
	NewLuaSnippetExecutor(code).SmartRun()
}

// SOLVED： 现在yakvm的opcode不支持 map[nil]的写法
func TestTableConstructor1(t *testing.T) {
	code := `
a = {
    x = 1
}
assert(a.x == 1, "TestTableConstructor1 failed1")
a.x = 2
print(a.x)
assert(a.x == 2, "TestTableConstructor1 failed2")

	`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestAssignNil(t *testing.T) {
	code := `
a=nil
assert(a==nil, "TestAssignNil failed")
`
	NewLuaSnippetExecutor(code).SmartRun()

}

func TestTblAssign(t *testing.T) {
	code := `
a={}
a.x=1
print(a.x)
assert(a.x==1, "TestTblAssign failed")
`
	NewLuaSnippetExecutor(code).SmartRun()

}

func TestBasicTbl(t *testing.T) {
	code := `
-- 简单的 table
mytable = {}
print("mytable 的类型是 ","table")

mytable[1]= "Lua"
mytable["wow"] = "修改前"
print("mytable 索引为 1 的元素是 ", mytable[1])

assert(mytable[1]=="Lua", "TestBasicTbl failed1")

print("mytable 索引为 wow 的元素是 ", mytable["wow"])
assert(mytable["wow"]=="修改前", "TestBasicTbl failed2")

-- alternatetable和mytable的是指同一个 table
alternatetable = mytable

print("alternatetable 索引为 1 的元素是 ", alternatetable[1])
assert(alternatetable[1]=="Lua", "TestBasicTbl failed3")
print("mytable 索引为 wow 的元素是 ", alternatetable["wow"])
assert(alternatetable["wow"]=="修改前", "TestBasicTbl failed4")
alternatetable["wow"] = "修改后"

print("mytable 索引为 wow 的元素是 ", mytable["wow"])
assert(alternatetable["wow"]=="修改后", "TestBasicTbl failed5")

-- 释放变量
alternatetable = nil
print("alternatetable 是 ", alternatetable)
assert(alternatetable==nil, "TestBasicTbl failed6")

-- mytable 仍然可以访问
print("mytable 索引为 wow 的元素是 ", mytable["wow"])
assert(mytable["wow"]=="修改后", "TestBasicTbl failed7")
mytable = nil
print("mytable 是 ", mytable)
assert(mytable==nil, "TestBasicTbl failed8")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestEmbeddedTbl(t *testing.T) {
	code := `
Account = {
    balance = 0,
	privatePocket = {balance = 100}, 
    withdraw = function(self, v)
        self.balance = self.balance - v
    end
}
Account.balance = 0
Account.privatePocket.balance = 1000
print(Account.balance)
print(Account.privatePocket.balance)
assert(Account.privatePocket.balance==1000, "TestEmbeddedTbl failed")
assert(Account.balance==0, "TestEmbeddedTbl failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestTblNoneExist(t *testing.T) {
	code := `
a={}
print(a[nil])
print(a[notDefine])

assert(a[nil]==nil, "TestTblNoneExist failed1")
assert(a[notDefine]==nil, "TestTblNoneExist failed2")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFuncLocal1(t *testing.T) {
	code := `
local function ez(n)
n=n+1
return n+1
end
print(ez(1))
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFunc2(t *testing.T) {
	code := `
n=1
n=n+1
assert(n==2, "TestFunc2 failed")
print(n)
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFib(t *testing.T) {
	code := `function fib (n)
	if n == 0 then
		return 0
	elseif n == 1 then
		return 1
	end
	return fib(n-1) + fib(n-2)
end

function fibr (n0, n1, c)
	if c == 0 then
		return n0
	elseif c == 1 then
		return n1
	end
	return fibr(n1, n0+n1, c-1)
end

function fibl (n)
	if n == 0 then
		return 0
	elseif n == 1 then
		return 1
	end
	local n0, n1 = 0, 1
	for i = n, 2, -1 do
		local tmp = n0 + n1
		n0 = n1
		n1 = tmp
	end
	return n1
end

print(fib(20))
print(fibr(0, 1, 20))
print(fibl(20))
assert(fib(20)==6765, "fib failed")
assert(fibr(0, 1, 20)==6765, "fibr failed")
assert(fibl(20)==6765, "fibl failed")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestFunctionWithEllipsis(t *testing.T) {
	code := `function add(...)
    local s = 0
    --for i, v in ipairs {...} do -- > {...} 表示一个由所有变长参数构成的数组  
      --  s = s + v
    -- end
s=25
    return s
end
print(add(3, 4, 5, 6, 7)) --->25

function average(...)
    result = 0
    local arg = {...}    --> arg 为一个表，局部变量
    -- for i,v in ipairs(arg) do
       -- result = result + v
    -- end
result = 33
    print("总共传入 " .. #arg .. " 个数")  --> #数组变量名 可以获取数组长度，但是如果这个数组中包含nil，那么需要通过select获取
    return result/#arg
end
 
print("平均值为",average(10,5,3,4,5,6)) ---> 5.5
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestBasic(t *testing.T) {
	code := `
currentNumber = 1
currentNumber = currentNumber + 1
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIterStateLess(t *testing.T) {
	code := `
function square(iteratorMaxCount, currentNumber)
    if currentNumber < iteratorMaxCount then
        currentNumber = currentNumber + 1
        return currentNumber, currentNumber * currentNumber
    end
end

cnt = 1
for i, n in square, 3, 0 do
    if cnt == 1 then
        assert(i == 1 and n == 1, "TestNoneStateIter failed cnt1")
    else
        if cnt == 2 then
            assert(i == 2 and n == 4, "TestNoneStateIter failed cnt2")
        else
            if cnt == 3 then
                assert(i == 3 and n == 9, "TestNoneStateIter failed cnt3")
            end
        end

    end
    cnt = cnt + 1
    print(i, n)
end

`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIterMultiStates(t *testing.T) {
	code := `array = {"Google", "Runoob"}

function elementIterator(collection)
    local index = 0
    local count = #collection
    -- 闭包函数
    return function()
        index = index + 1
        if index <= count then
            --  返回迭代器的当前元素
            return collection[index]
        end
    end
end

cnt = 1

for element in elementIterator(array) do
    if cnt == 2 then
        assert(element == "Runoob", "TestIterMultiStates failed cnt 2")
    end
    if cnt == 1 then
        cnt = cnt + 1
        assert(element == "Google", "TestIterMultiStates failed cnt 1")
    end

    print(element)
end
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIterWithIPairs(t *testing.T) {
	code := `array = {"Google", "Runoob"}
test1, test2 = false, false
for key, value in ipairs(array) do
    if key == 1 then
        assert(value == "Google", "TestIterWithIPairs failed1")
        test1 = true
    end

    if value == "Runoob" then
        assert(key == 2, "TestIterWithIPairs failed2")
        test2 = true
    end

    print(key, value)
end
assert(test1 and test2, "TestIterWithIPairs not iterate all elems")

`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIterWithPairs(t *testing.T) {
	code := `-- create a table with some key-value pairs
local myTable = {
    name = "John",
    age = 25,
    city = "New York"
}
myTable.name = 1

test1, test2, test3 = false, false, false
-- iterate over all key-value pairs in the table
for key, value in pairs(myTable) do
    if key == "name" then
        assert(value == 1, "IterWithPairs failed1")
        test1 = true
    end

    if key == "age" then
        assert(value == 25, "IterWithPairs failed2")
        test2 = true
    end

    if key == "city" then
        assert(value == "New York", "IterWithPairs failed3")
        test3 = true
    end

    print(key .. " = " .. value)
end
assert(test1 and test2 and test3, "TestIterWithPairs Not Iterate all elem!")
`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestIterWithCustom(t *testing.T) {
	code := `-- 创建迭代器的工厂函数
function kvpair(tbl)
    -- 迭代器为next
    return next, tbl, nil
end
test1, test2, test3 = false, false, false
-- 泛型for
local tbl = {
    nickname = "alice",
    gender = 1
}

for k, v in kvpair(tbl) do
    if k == "nickname" then
        assert(v == "alice", "custom iterator failed1")
        test1 = true
    end

    if k == "gender" then
        assert(v == 1, "custom iterator failed2")
        test2 = true
    end

    print(k, v)
end
assert(test1 and test2, "TestIterWithCustom not iterate all elems")

`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestOOB(t *testing.T) {
	code := `
Account = {
    balance = 0,
    withdraw = function(self, v)
        self.balance = self.balance - v
    end
}

function Account:deposit(v)
    self.balance = self.balance + v
end

Account.deposit(Account, 200.00)
print(Account.balance)
assert(Account.balance==200, "oop bug1")

Account:withdraw(100.00)
print(Account.balance)

assert(Account.balance==100, "oop bug2")

`
	NewLuaSnippetExecutor(code).SmartRun()
}

func TestOOPEmbedded(t *testing.T) {
	code := `
Account = {
    balance = 0,
    privatePocket = {
        balance = 100,
        ["loopSuperPrivatePocket"] = {
            balance = 10000
        }
    },
    withdraw = function(self, v)
        self.balance = self.balance - v
    end
}

function Account:deposit(v, msg)
    self.balance = self.balance + v
    print("i have save ", msg)
    print(msg)
end

function Account.privatePocket:peekBalance()
    assert(self.balance == 100, "bug 1")
end

function Account.privatePocket.loopSuperPrivatePocket:peekBalance()
    assert(self.balance == 10000, "bug 2")
end

Account.deposit(Account, 1, "1")
Account:deposit(1, "1")
assert(Account.balance == 2, "bug0")
Account.privatePocket:peekBalance()
Account.privatePocket.peekBalance(Account.privatePocket)
Account.privatePocket.loopSuperPrivatePocket:peekBalance()
Account.privatePocket.loopSuperPrivatePocket:peekBalance(Account.privatePocket.loopSuperPrivatePocket)
`
	NewLuaSnippetExecutor(code).SmartRun()
}
