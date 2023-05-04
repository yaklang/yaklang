package tests

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"math"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

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
	yakvm.Import("assert", func(x ...interface{}) {
		if len(x) == 2 {
			assert := func(condition interface{}, message string) {
				if condition == nil {
					panic(message)
				}
				if boolean, ok := condition.(bool); ok {
					if !boolean {
						panic(message)
					}
				}
			}
			assert(x[0], x[1].(string))
		} else {
			condition := x[0]
			if condition == nil {
				panic("assertion failed")
			}
			if boolean, ok := condition.(bool); ok {
				if !boolean {
					panic("assertion failed")
				}
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

	yakvm.Import("@getlen", func(x interface{}) int {
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

}

// TODO: FIX LABEL
//func Test_GOTO(t *testing.T) {
//	code := `
//	local function testG (a)
//	  if a == 1 then
//		goto l1
//		error("should never be here!")
//	  elseif a == 2 then goto l2
//	  elseif a == 3 then goto l3
//	  elseif a == 4 then
//		goto l1  -- go to inside the block
//		error("should never be here!")
//		::l1:: a = a + 1   -- must go to 'if' end
//	  else
//		goto l4
//		::l4a:: a = a * 2; goto l4b
//		error("should never be here!")
//		::l4:: goto l4a
//		error("should never be here!")
//		::l4b::
//	  end
//	  do return a end
//	  ::l2:: do return "2" end
//	  ::l3:: do return "3" end
//	  ::l1:: return "1"
//	end
//
//	assert(testG(1) == "1")
//	assert(testG(2) == "2")
//	assert(testG(3) == "3")
//	assert(testG(4) == 5)
//	assert(testG(5) == 10)`
//	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
//}

func Test_BUG_5_2_Beta(t *testing.T) {
	code := `local function foo ()
  local a
  return function ()
    local b
    a, b = 3, 14    -- local and upvalue have same index
    return a, b
  end
end

local a, b = foo()()
assert(a == 3 and b == 14, "Test_BUG_5_2_Beta failed")

print('OK')`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

func TestUpValue(t *testing.T) {
	code := `do
  local a,i,j,b
  a = {'a', 'b'}; i=1; j=2; b=a
  local function foo ()
    i, a[i], a, j, a[j], a[i+j] = j, i, i, b, j, i
  end
  foo()
  assert(i == 2 and b[1] == 1 and a == 1 and j == b and b[2] == 2 and
         b[3] == 1)
  local t = {}
  (function (a) t[a], a = 10, 20  end)(1);
  assert(t[1] == 10, "failed")
end`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

// test conflicts in multiple assignment
func Test1(t *testing.T) {
	code := `do
  local a,i,j,b
  a = {'a', 'b'}; i=1; j=2; b=a
  i, a[i], a, j, a[j], a[i+j] = j, i, i, b, j, i
  assert(i == 2 and b[1] == 1 and a == 1 and j == b and b[2] == 2 and
         b[3] == 1)
  a = {}
  local function foo ()    -- assigining to upvalues
    b, a.x, a = a, 10, 20
  end
  foo()
  assert(a == 20 and b.x == 10,"failed")
end
`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

// test conflicts in multiple assignment
func Test2(t *testing.T) {
	code := `do
  local a,i,j,b
  a = {'a', 'b'}; i=1; j=2; b=a
  i, a[i], a, j, a[j], a[i+j] = j, i, i, b, j, i
  assert(i == 2 and b[1] == 1 and a == 1 and j == b and b[2] == 2 and
         b[3] == 1)
  a = {}
  local function foo ()    -- assigining to upvalues
    b, a.x, a = a, 10, 20
  end
  foo()
  assert(a == 20 and b.x == 10,"failed")
end
`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

// testing local-function recursion
func Test3(t *testing.T) {
	code := `fact = false
do
  local res = 1
  local function fact (n)
    if n==0 then return res
    else return n*fact(n-1)
    end
  end
  assert(fact(5) == 120)
end
assert(fact == false)`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

// testing declarations 这里因为目前实现不完整有删减
func Test4(t *testing.T) {
	code := `
 a = {i = 10}
self = 20
function a:x (x) return x+self.i end
function a.y (x) return x+self end

assert(a:x(1)+10 == a.y(1))

a.t = {i=-100}
a["t"].x = function (self, a,b) return self.i+a+b end

assert(a.t:x(2,3) == -95)

do
  local a = {x=0}
  function a:add (x) self.x, a.y = self.x+x, 20; return self end
  assert(a:add(10):add(20):add(30).x == 60 and a.y == 20)
end

local a = {b={c={}}}

function a.b.c.f1 (x) return x+1 end
function a.b.c:f2 (x,y) self[x] = y end
assert(a.b.c.f1(4) == 5)
a.b.c:f2('k', 12); assert(a.b.c.k == 12)

print('+')

t = nil   -- 'declare' t
function f(a,b,c) local d = 'a'; t={a,b,c,d} end

f(      -- this line change must be valid
  1,2)
assert(t[1] == 1 and t[2] == 2 and t[3] == nil and t[4] == 'a')
f(1,2,   -- this one too
      3,4)
assert(t[1] == 1 and t[2] == 2 and t[3] == 3 and t[4] == 'a')
function deep (n)
  if n>0 then deep(n-1) end
end
deep(10)
deep(180)


print"testing tail calls"

function deep (n) if n>0 then return deep(n-1) else return 101 end end
assert(deep(30000) == 101)
a = {}
function a:deep (n) if n>0 then return self:deep(n-1) else return 101 end end
assert(a:deep(30000) == 101)

do   -- tail calls x varargs
  local function foo (x, ...) local a = {...}; return x, a[1], a[2] end

  local function foo1 (x) return foo(10, x, x + 1) end

  local a, b, c = foo1(-2)
  assert(a == 10 and b == -2 and c == -1)

  a, b = (function () return foo() end)()
  assert(a == nil and b == nil)

  local X, Y, A
  local function foo (x, y, ...) X = x; Y = y; A = {...} end
  local function foo1 (...) return foo(...) end

  local a, b, c = foo1()
  assert(X == nil and Y == nil and #A == 0)

  a, b, c = foo1(10)
  assert(X == 10 and Y == nil and #A == 0)

  a, b, c = foo1(10, 20)
  assert(X == 10 and Y == 20 and #A == 0)

  a, b, c = foo1(10, 20, 30)
  assert(X == 10 and Y == 20 and #A == 1 and A[1] == 30)
end
`
	antlr4Lua.NewLuaSnippetExecutor(code).SmartRun()
}

func Test5(t *testing.T) {
	code := `do -- tail calls x varargs
    local function foo(x, ...)
        local a = {...};
        print(a[1])
		print(a[2])
        return x, a[1], a[2]
    end

    local function foo1(x)
        return foo(10, x, x + 1)
    end

    local a, b, c = foo1(-2)
	print(a)
print(b)
print(c)
end`
	antlr4Lua.NewLuaSnippetExecutor(code).Run()
}
