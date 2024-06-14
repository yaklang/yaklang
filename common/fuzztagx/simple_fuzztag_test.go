package fuzztagx

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
	"testing"
)

func TestSimpleFuzzTag_Exec(t *testing.T) {
	for i, test := range []struct {
		code   string
		expect []string
	}{
		{ // 常规
			code:   "{{f2(f1(aaa))}}",
			expect: []string{"aaa12"},
		},
		{
			code:   "{{repeat(3)}}{{randstr()}}",
			expect: []string{"0", "0", "0"},
		},
		{ // dyn
			code:   "{{repeat(3)}}{{randstr::dyn()}}",
			expect: []string{"0", "1", "2"},
		},
		{ // 同步
			code:   "{{array::1(a|b|c)}}{{f1(array::1(a|b|c))}}",
			expect: []string{"aa1", "bb1", "cc1"},
		},
		{ // 同步
			code:   "{{array::1(a|b|c)}}{{f1::1(array(a|b|c))}}",
			expect: []string{"aa1", "bb1", "cc1"},
		},
		{ // 笛卡尔
			code:   "{{echo(echo(a)array(b|c))}}",
			expect: []string{"ab", "ac"},
		},
		{ // 同步
			code:   "{{array::1(a|b)}}{{array::1(echo(a|)array(b|c))}}",
			expect: []string{"aa", "bb", "a", "c"},
		},
		{ // 函数外存在字符串
			code:   "{{array::1(a|b)}}{{echo(123)a}}",
			expect: []string{"a{{echo(123)a}}", "b{{echo(123)a}}"},
		},
		{ // 函数名无效
			code:   "{{array::1(a|b)}}{{echo.a(123)}}",
			expect: []string{"a{{echo.a(123)}}", "b{{echo.a(123)}}"},
		},
		{ // 小括号转义
			code:   "{{array::1(a|b)}}{{echo(12\\(3)}}",
			expect: []string{"a12(3", "b12(3"},
		},
	} {
		t.Run(fmt.Sprintf("test: %d", i), func(t *testing.T) {
			iForRandStr := 0
			gener, err := NewGenerator(test.code, map[string]*parser.TagMethod{
				"echo": {
					Name: "echo",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						return []*parser.FuzzResult{parser.NewFuzzResultWithData(s)}, nil
					},
				},
				"repeat": {
					Name: "repeat",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						n, _ := strconv.Atoi(s)
						res := []*parser.FuzzResult{}
						for i := 0; i < n; i++ {
							res = append(res, parser.NewFuzzResultWithData(""))
						}
						return res, nil
					},
				},
				"array": {
					Name: "array",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						res := []*parser.FuzzResult{}
						for _, item := range strings.Split(s, "|") {
							res = append(res, parser.NewFuzzResultWithData(item))
						}
						return res, nil
					},
				},
				"randstr": {
					Name: "randstr",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						defer func() {
							iForRandStr++
						}()
						return []*parser.FuzzResult{parser.NewFuzzResultWithData(fmt.Sprint(iForRandStr))}, nil
					},
				},
				"f1": {
					Name: "f1",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						return []*parser.FuzzResult{parser.NewFuzzResultWithData(s + "1")}, nil
					},
				},
				"f2": {
					Name: "f2",
					Fun: func(s string) ([]*parser.FuzzResult, error) {
						return []*parser.FuzzResult{parser.NewFuzzResultWithData(s + "2")}, nil
					},
				},
			}, true, false)
			if err != nil {
				t.Fatal(err)
			}
			gener.Debug()
			for i := 0; gener.Next(); i++ {
				if string(gener.Result().GetData()) != test.expect[i] {
					t.Fatal(fmt.Errorf("expect: %s, got: %s", test.expect[i], string(gener.Result().GetData())))
				}
			}
		})
	}
}

var testMap1 = map[string]func(string) []string{
	"echo": func(i string) []string {
		return []string{i}
	},
	"array": func(i string) []string {
		return strings.Split(i, "|")
	},
	"get1": func(i string) []string {
		return []string{"1"}
	},
	"list": func(s string) []string {
		return strings.Split(s, "|")
	},
	"int": func(i string) []string {
		return funk.Map(utils.ParseStringToPorts(i), func(i int) string {
			return strconv.Itoa(i)
		}).([]string)
	},
	"panic": func(s string) []string {
		panic(s)
		return nil
	},
}

// 同步渲染数量测试
func TestSyncRender1(t *testing.T) {
	for i, testcase := range [][2]any{
		{
			"{{echo({{list::1(aaa|ccc|ddd)}}{{list::1(aaa|ccc|ddd)}})}}",
			3,
		},
		{
			"{{echo::1({{list(aaa|ccc)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
			3,
		},
		{
			"{{echo::1({{list(aaa|ccc|ddd)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
			3,
		},
		{
			"{{echo::1({{list(aaa|ccc|ddd|eee)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
			4,
		},
		{
			"{{echo::3({{list(aaa|ccc|ddd)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
			9,
		},
		{
			"{{echo({{list(aaa|ccc|ddd)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
			9,
		},
		{
			"{{echo({{list(aaa|ccc|ddd)}})}}{{echo({{list(aaa|ccc|ddd)}})}}",
			9,
		},
	} {
		spew.Dump(testcase[0].(string))
		result, err := ExecuteSimpleTagWithStringHandler(testcase[0].(string), testMap1)
		if err != nil {
			panic(err)
		}
		if len(result) != testcase[1].(int) {
			t.Fatal(utils.Errorf("testcase %d error,got: length %d, expect length: %d", i, len(result), testcase[1].(int)))
		}
	}
	result, err := ExecuteSimpleTagWithStringHandler("{{echo::1({{array::1(a|b)}})}}", testMap1)
	if err != nil {
		panic(err)
	}
	if result[0] != "a" || result[1] != "b" {
		panic("test sync render error")
	}
}

// 畸形测试
func TestDeformityTag1(t *testing.T) {
	for _, v := range [][]string{
		{"{{get1(1-29)}}", "1"},
		{"{{i$$$$$nt(1-29)}}", "{{i$$$$$nt(1-29)}}"},
		{"{{xx12}}", ""},
		{"{{xx12:}}", ""},
		{"{{xx12:-_}}", ""},
		{"{{xx12:-_[[[[}}", "{{xx12:-_[[[[}}"},
		{"{{xx12:-_[}}[[[}}", "{{xx12:-_[}}[[[}}"},
		{"{{xx12:-_}}[[[[}}", "[[[[}}"},
		{"{{xx12:-_(1)}}[[[[}}", "[[[[}}"},
		{"{{xx12:-_:::::::(2)}}[[[[}}", "[[[[}}"},
		{"{{xx12:-_()}}[[[[}}", "[[[[}}"},
		//{"{{xx12:-_(____)____)}}[[[[}}", "{{xx12:-_(____)____)}}[[[[}}"}, // {{xx12:-_(____)____)}}应该被正确解析
		{"{{xx12:-_(____\\)____)}}[[[[}}", "[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{[[[[}}", "{[[[[}}"},
		{"{{xx12:-_(____)} }}____)}}[[[[}}", "{{xx12:-_(____)} }}____)}}[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{{[[[[}}", "{{[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{{1[[[[}}", "{{1[[[[}}"},
		//{"{{xx12:-_(____\\)} }__)__)}}{{1[[[[}}", "{{xx12:-_(____\\)} }__)__)}}{{1[[[[}}"},
		{"{{xx12:-_(____\\)} }__\\)__)}}{{1[[[[}}", "{{1[[[[}}"},
		{"{{{{1[[[[}}", "{{{{1[[[[}}"},
		{"{{{{get1}}{{1[[[[}}", "{{1{{1[[[[}}"},
		{"{{i{{get1}}nt(1-2)}}", ""},
		{"{{", "{{"},
		//{"{{echo(123123\\))}}", "123123)"}, // 括号不需要转义
		//{"{{print(list{\\())}}", "{{print(list{\\())}}"},
		//{"{{print(list{\\(\\))}}", ""},
		{"{{{echo(123)}}", "{123"},
		// {"{{i{{get1}}n{{get1}}t(1-2)}}", "{{i1nt(1-2)}}"},
	} {
		t, r := v[0], v[1]
		spew.Dump(t)
		result, err := ExecuteSimpleTagWithStringHandler(t, testMap1)
		if err != nil {
			panic(err)
		}
		if len(result) <= 0 {
			panic(1)
		}
		if result[0] != r {
			m := fmt.Sprintf("got: %v expect: %v", strconv.Quote(result[0]), strconv.Quote(r))
			panic(m)
		}
	}
}

// Tag名后换行
func TestNewLineAfterTagName1(t *testing.T) {
	var m = map[string]func(string) []string{
		"s": func(s string) []string {
			return []string{s + "a"}
		},
	}

	res, err := ExecuteSimpleTagWithStringHandler(`{{s 
() }}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || res[0] != "a" {
		panic("exec with new line error")
	}
}

func TestExecuteBug11(t *testing.T) {
	var m = map[string]func(string) []string{
		"int": func(s string) []string {
			return []string{s}
		},
	}

	res, err := ExecuteSimpleTagWithStringHandler(`{{int::aaa(1)}} {{int::aaa(1)}} {{int::aaa(1)}}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || res[0] != "1 1 1" {
		panic("error")
	}
}

// 转义
func TestEscape1(t *testing.T) {
	for _, v := range [][]string{
		{"{{echo(\\{{)}})}}", "{{)}}"},
		{"\\{{echo(1)}}", "\\1"},                   // 标签外不转义
		{"\\{{echo(1)\\}}", "\\{{echo(1)\\}}"},     // {{之后开始tag语法，需要转义，\}}转义后不能作为标签闭合符号，导致标签解析失败，原文输出
		{"\\{{echo(1)\\}}}}", "\\{{echo(1)\\}}}}"}, // 标签解析成功，但由于标签内数据`echo(1)}`编译失败，导致原文输出
		//{`{{echo({{echo(\\\\)}})}}`, `\\`},         // 多层标签嵌套转义
		//{`{{echo({{echo(\\)}})}}`, `\\`}, // \不转义
		{`{{echo(C:\Abc\tmp)}}`, `C:\Abc\tmp`},
	} {
		res, err := ExecuteSimpleTagWithStringHandler(v[0], map[string]func(string2 string) []string{
			"echo": func(s string) []string {
				return []string{s}
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res[0] != v[1] {
			t.Fatal(spew.Sprintf("expect: %s, got: %s", v[1], res[0]))
		}
	}
}

func _TestMagicLabel1(t *testing.T) {
	checkSameString := func(s []string) bool {
		set := utils.NewSet[string]()
		for _, v := range s {
			set.Add(v)
		}
		return len(set.List()) == 1
	}
	_ = checkSameString
	for _, v := range [][]any{
		{"{{randstr()}}{{repeat(10)}}", func(s []string) bool {
			return true
		}},
		{"{{randstr::dyn()}}{{repeat(10)}}", func(s []string) bool {
			return len(s) == 10 && s[0] != s[1]
		}},
		{"{{array::1(a|b)}}{{array::1(a|b|c)}}", []string{"aa", "bb", "c"}},
		{"{{array::1::rep(a|b)}}{{array::1(a|b|c)}}", []string{"aa", "bb", "bc"}},
		{"{{array::1(a|b|c)}}{{array::1::rep(a|b)}}", []string{"aa", "bb", "cb"}},
	} {
		t, r := v[0], v[1]
		spew.Dump(t)
		result, err := ExecuteSimpleTagWithStringHandler(t.(string), map[string]func(string) []string{
			"array": func(s string) []string {
				return strings.Split(s, "|")
			},
			"raw": func(s string) []string {
				return []string{s}
			},
			"randstr": func(s string) []string {
				return []string{utils.RandStringBytes(10)}
			},
			"repeat": func(s string) []string {
				res := make([]string, 0)
				n, err := strconv.Atoi(s)
				if err != nil {
					return res
				}

				for range make([]int, n) {
					res = append(res, "")
				}
				return res
			},
		})
		if err != nil {
			panic(err)
		}
		spew.Dump(result)
		switch ret := r.(type) {
		case string:
			if result[0] != r {
				m := fmt.Sprintf("got: %v expect: %v", strconv.Quote(result[0]), strconv.Quote(ret))
				panic(m)
			}
		case []string:
			if len(result) != len(ret) {
				panic("check failed")
			}
			for i, v := range result {
				if v != ret[i] {
					panic("check failed")
				}
			}
		case func([]string) bool:
			if !ret(result) {
				panic("check failed")
			}
		default:
			panic("unknown type")
		}
	}
}

func TestRawTag1(t *testing.T) {
	for _, v := range [][]string{
		{"{{=asdasd=}}", "asdasd"},                                  // 常规
		{`\{{=hello{{=hello\{{=world=}}`, `\{{=hellohello{{=world`}, // 测试 raw tag转义
	} {
		res, err := ExecuteSimpleTagWithStringHandler(v[0], map[string]func(string2 string) []string{
			"echo": func(s string) []string {
				return []string{s}
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res[0] != v[1] {
			t.Fatal(spew.Sprintf("expect: %s, got: %s", v[1], res[0]))
		}
	}
}
func TestMutiTag1(t *testing.T) {
	for _, v := range [][]string{
		{"{{echo({{={{echo\\(\\)}}=}})}}", "{{echo()}}"}, // 常规
		//{`{{echo({{=}}=}})}}`, `}}`}, // 测试嵌套（raw标签应该屏蔽所有语法）
	} {
		res, err := ExecuteSimpleTagWithStringHandler(v[0], map[string]func(string2 string) []string{
			"echo": func(s string) []string {
				return []string{s}
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res[0] != v[1] {
			t.Fatal(spew.Sprintf("expect: %s, got: %s", v[1], res[0]))
		}
	}
}

// 测试标签执行出错的情况
func TestErrors1(t *testing.T) {
	// 执行出错的几种情况：标签编译错误（返回原文）、未找到函数名（生成空？）、函数内部执行出错继续生成
	res, err := ExecuteSimpleTagWithStringHandler("{{panic(error}}", testMap1)
	if err != nil {
		t.Fatal(err)
	}
	if res[0] != "{{panic(error}}" {
		t.Fatal("expect `{{panic(error}}`")
	}

	res, err = ExecuteSimpleTagWithStringHandler("{{aaa}}", testMap1)
	if err != nil {
		t.Fatal(err)
	}
	if res[0] != "" {
		t.Fatal("expect ``")
	}

	res, err = ExecuteSimpleTagWithStringHandler("{{echo(a{{panic(error)}}b)}}", testMap1)
	if res[0] != "ab" {
		t.Fatal("expect `ab`")
	}
}
