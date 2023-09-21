package antlr4Lua

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua/luaast"
	"os"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"math"
	"reflect"
	"sort"
	"strconv"
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
	Import("print", func(v ...interface{}) {
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

	Import("raw_print", func(v interface{}) {
		fmt.Println(v)
	})

	//assert (v [, message])
	//Calls error if the value of its argument v is false (i.e., nil or false);
	//otherwise, returns all its arguments. In case of error, message is the
	//error object; when absent, it defaults to "assertion failed!"
	
	//todo
	//https://www.lua.org/pil/8.3.html
	Import("assert", func(condition ...interface{}) {
		if condition == nil {
			panic("assert nil")
		}
		if len(condition) == 2 {
			Assert := func(condition interface{}, message string){
				if condition == nil {
					panic(message)
				}
				if boolean, ok := condition.(bool); ok {
					if !boolean {
						panic(message)
					}
				}
			}
			Assert(condition[0], condition[1].(string))
		} else {
			Assert := func(condition interface{}) {
				if boolean, ok := condition.(bool); ok {
					if !boolean {
						panic("assert failed")
					}
				}
			}
			Assert(condition)
		}
	})

	Import("@pow", func(x interface{}, y interface{}) float64 {
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

	Import("@floor", func(x interface{}, y interface{}) float64 {
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
		if ok1 && ok2 && (index != 0){
			res := base / index
			return math.Floor(res)
		} else if (index == 0) &&  ok1 && ok2{
			panic("dividend can't be zero!")
		} else {
			panic("attempt to floor a '" + reflect.TypeOf(base).String() + "' with a '" + reflect.TypeOf(index).String() + "'")
		}

	})

	Import("tostring", func(x interface{}) string {
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

	Import("@strcat", func(x interface{}, y interface{}) string {
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

	Import("@getlen", func(x interface{}) int {
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

	Import("next", func(x ...interface{}) (interface{}, interface{}) { // next(table[,index])
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

	Import("error", func(x any) {
		panic(x)
	})



}


var buildinLib = make(map[string]interface{})

func Import(name string, f interface{}) {
	buildinLib[name] = f
}

type LuaSnippetExecutor struct {
	sourceCode string
	engine     *Engine
	translator *luaast.LuaTranslator
}

func NewLuaSnippetExecutor(code string) *LuaSnippetExecutor {
	e := New()
	e.ImportLibs(buildinLib)
	return &LuaSnippetExecutor{sourceCode: code, engine: e, translator: &luaast.LuaTranslator{}}
}

func (l *LuaSnippetExecutor) Run() {
	err := l.engine.Eval(context.Background(), l.sourceCode)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

func (l *LuaSnippetExecutor) Debug() {
	l.engine.debug = true
	err := l.engine.Eval(context.Background(), l.sourceCode)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

// SmartRun SmartRun() will choose Run() or Debug() depending on the environment setting `LUA_DEBUG`
func (l *LuaSnippetExecutor) SmartRun() {
	if os.Getenv("LUA_DEBUG") != "" {
		l.Debug()
	} else {
		l.Run()
	}
}
