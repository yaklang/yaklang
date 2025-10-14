package fuzztagx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

var testMap = map[string]func(string) []string{
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
func TestSyncRender(t *testing.T) {
	for i, testcase := range [][2]any{
		//{
		//	"{{echo::1({{list(aaa|ccc)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
		//	3,
		//},
		//{
		//	"{{echo::1({{list(aaa|ccc|ddd)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
		//	3,
		//},
		//{
		//	"{{echo::1({{list(aaa|ccc|ddd|eee)}})}}{{echo::1({{list(aaa|ccc|ddd)}})}}",
		//	4,
		//},
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
		result, err := ExecuteWithStringHandler(testcase[0].(string), testMap)
		if err != nil {
			panic(err)
		}
		if len(result) != testcase[1].(int) {
			t.Fatal(utils.Errorf("testcase %d error,got: length %d, expect length: %d", i, len(result), testcase[1].(int)))
		}
	}
	result, err := ExecuteWithStringHandler("{{echo::1({{array::1(a|b)}})}}", testMap)
	if err != nil {
		panic(err)
	}
	if result[0] != "a" || result[1] != "b" {
		panic("test sync render error")
	}
}

// 畸形测试
func TestDeformityTag(t *testing.T) {
	for _, v := range [][]string{
		{"{{echo(${<{{echo(a)}}})}}", "${<a}"},
		{"{{echo({{{echo(a)}}})}}", "{a}"},
		{"{{echo({{{echo(a)}}}})}}", "{a}}"},
		{`{{echo(\{{1{{echo(a)}}}})}}`, "{{1a}}"}, // why
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
		result, err := ExecuteWithStringHandler(t, testMap)
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
func TestNewLineAfterTagName(t *testing.T) {
	var m = map[string]func(string) []string{
		"s": func(s string) []string {
			return []string{s + "a"}
		},
	}

	res, err := ExecuteWithStringHandler(`{{s 
() }}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || res[0] != "a" {
		panic("exec with new line error")
	}
}

func TestExecuteBug1(t *testing.T) {
	var m = map[string]func(string) []string{
		"int": func(s string) []string {
			return []string{s}
		},
	}

	res, err := ExecuteWithStringHandler(`{{int::aaa(1)}} {{int::aaa(1)}} {{int::aaa(1)}}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || res[0] != "1 1 1" {
		panic("error")
	}
}

// 转义
func TestEscape(t *testing.T) {
	for _, v := range [][]string{
		{"{{echo(\\{{)}})}}", "{{)}}"},
		{"\\{{echo(1)}}", "\\1"},                   // 标签外不转义
		{"\\{{echo(1)\\}}", "\\{{echo(1)\\}}"},     // {{之后开始tag语法，需要转义，\}}转义后不能作为标签闭合符号，导致标签解析失败，原文输出
		{"\\{{echo(1)\\}}}}", "\\{{echo(1)\\}}}}"}, // 标签解析成功，但由于标签内数据`echo(1)}`编译失败，导致原文输出
		//{`{{echo({{echo(\\\\)}})}}`, `\\`},         // 多层标签嵌套转义
		{`{{echo({{echo(\\)}})}}`, `\\`}, // \不转义
		{`{{echo(C:\Abc\tmp)}}`, `C:\Abc\tmp`},
	} {
		res, err := ExecuteWithStringHandler(v[0], map[string]func(string2 string) []string{
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

func TestMagicLabel(t *testing.T) {
	checkSameString := func(s []string) bool {
		set := utils.NewSet[string]()
		for _, v := range s {
			set.Add(v)
		}
		return len(set.List()) == 1
	}
	_ = checkSameString
	for i, v := range [][]any{
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
		t.Run(fmt.Sprintf("test: %d", i), func(t *testing.T) {
			code, r := v[0], v[1]
			spew.Dump(t)
			result, err := ExecuteWithStringHandler(code.(string), map[string]func(string) []string{
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
		})
	}
}

func TestRawTag(t *testing.T) {
	for _, v := range [][]string{
		{"{{=asdasd=}}", "asdasd"},                                  // 常规
		{`\{{=hello{{=hello\{{=world=}}`, `\{{=hellohello{{=world`}, // 测试 raw tag转义
	} {
		res, err := ExecuteWithStringHandler(v[0], map[string]func(string2 string) []string{
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
func TestMutiTag(t *testing.T) {
	for _, v := range [][]string{
		{"{{echo({{={{echo()}}=}})}}", "{{echo()}}"}, // 常规
		//{`{{echo({{=}}=}})}}`, `}}`}, // 测试嵌套（raw标签应该屏蔽所有语法）
	} {
		res, err := ExecuteWithStringHandler(v[0], map[string]func(string2 string) []string{
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
func TestErrors(t *testing.T) {
	// 执行出错的几种情况：标签编译错误（返回原文）、未找到函数名（生成空？）、函数内部执行出错继续生成
	res, err := ExecuteWithStringHandler("{{panic(error}}", testMap)
	if err != nil {
		t.Fatal(err)
	}
	if res[0] != "{{panic(error}}" {
		t.Fatal("expect `{{panic(error}}`")
	}

	res, err = ExecuteWithStringHandler("{{aaa}}", testMap)
	if err != nil {
		t.Fatal(err)
	}
	if res[0] != "" {
		t.Fatal("expect ``")
	}

	res, err = ExecuteWithStringHandler("{{echo(a{{panic(error)}}b)}}", testMap)
	if res[0] != "ab" {
		t.Fatal("expect `ab`")
	}
}
func TestDynTag(t *testing.T) {
	nodes, err := ParseFuzztag(`{{list}}{{append1({{randstr}})}}`, false)
	if err != nil {
		t.Fatal(err)
	}
	generator := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{
		"list": {
			Fun: func(n string) ([]*parser.FuzzResult, error) {
				return []*parser.FuzzResult{parser.NewFuzzResultWithData("1"), parser.NewFuzzResultWithData("2")}, nil
			},
		},
		"append1": {
			Fun: func(n string) ([]*parser.FuzzResult, error) {
				return []*parser.FuzzResult{parser.NewFuzzResultWithData(n + "1")}, nil
			},
		},
		"randstr": {
			IsDyn: true,
			Fun: func(n string) ([]*parser.FuzzResult, error) {
				return []*parser.FuzzResult{parser.NewFuzzResultWithData(utils.RandStringBytes(5))}, nil
			},
		},
	})
	n := 0
	for generator.Next() {
		if n > 2 {
			t.Fatal("test dyn tag failed")
		}

		if generator.Error != nil {
			t.Fatal(generator.Error)
		}
		println(string(generator.Result().GetData()))
		n++
	}
	if n != 2 {
		t.Fatal("test dyn tag failed")
	}
}
func TestTagArgument(t *testing.T) {
	res, err := ExecuteWithStringHandler("{{array({{int(1-2)}},a)}}", map[string]func(string2 string) []string{
		"array": func(s string) []string {
			return strings.Split(s, ",")
		},
		"int": func(s string) []string {
			return []string{"1", "2"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "1a2a", strings.Join(res, ""))
}

func TestYieldFun(t *testing.T) {
	code := `{{genStringList}}`
	nodes, err := ParseFuzztag(code, false)
	if err != nil {
		t.Fatal(err)
	}
	i := 0
	var finished bool
	generator := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{
		"genStringList": &parser.TagMethod{
			YieldFun: func(ctx context.Context, params string, yield func(*parser.FuzzResult)) error {
				for ; i < 10; i++ {
					yield(parser.NewFuzzResultWithData(strconv.Itoa(i)))
				}
				finished = true
				return nil
			},
		},
	})
	for i := 0; i < 3; i++ {
		generator.Next()
	}
	assert.Equal(t, "2", string(generator.Result().GetData()))
	assert.Equal(t, 3, i)
	generator.Cancel()
	generator.Wait()
	assert.Equal(t, true, finished)
}

func TestFuzztagGeneratorCancel(t *testing.T) {
	code := `{{genStringList}}`
	nodes, err := ParseFuzztag(code, false)
	if err != nil {
		t.Fatal(err)
	}
	sleepTime := 1 * time.Second
	generator := parser.NewGenerator(nil, nodes, map[string]*parser.TagMethod{
		"genStringList": &parser.TagMethod{
			YieldFun: func(ctx context.Context, params string, yield func(*parser.FuzzResult)) error {
				yield(parser.NewFuzzResultWithData("1"))
				time.Sleep(sleepTime)
				return nil
			},
		},
	})
	generator.Next()
	timeStart := time.Now()
	generator.Cancel()
	generator.Wait()
	timeEnd := time.Now()

	delta := timeEnd.Sub(timeStart)
	assert.True(t, delta >= sleepTime && delta <= sleepTime+time.Millisecond*500)
}

func TestSyncRender2(t *testing.T) {
	s := "{{int::1(1-10)}}-{{int::1(1-10)}}-{{int::1(1-10)}}-{{int::2(1-15)}}-{{int::2(1-15)}}"
	res, err := ExecuteWithStringHandler(s, testMap)
	require.NoError(t, err)
	require.Len(t, res, 10*15)
	for _, v := range res {
		splited := strings.Split(v, "-")
		for _, integer := range splited {
			i, err := strconv.Atoi(integer)
			require.NoError(t, err)
			require.GreaterOrEqual(t, i, 1)
			require.LessOrEqual(t, i, 15)
		}
	}
}

func TestSyncRender3(t *testing.T) {
	s := "{{int::1(1-10)}}-{{int::1(1-15)}}"
	res, err := ExecuteWithStringHandler(s, testMap)
	require.NoError(t, err)
	require.Len(t, res, 15)
	count := 0
	for _, v := range res {
		count++
		splited := strings.Split(v, "-")
		require.Len(t, splited, 2)
		if count > 10 {
			require.Equal(t, "", splited[0], "sync tag out index error")
		} else {
			require.Equal(t, strconv.Itoa(count), splited[0])
		}
		require.Equal(t, strconv.Itoa(count), splited[1])
	}
}

func TestSyncRender4(t *testing.T) {
	s := "{{array(a|b|c)}}"
	echoTag := fmt.Sprintf("{{echo(%s)}}", s)
	res, err := ExecuteWithStringHandlerEx(echoTag+echoTag, testMap, func(generator *parser.Generator) {
		generator.SetTagsSync(true)
	})
	require.NoError(t, err)
	require.Equal(t, strings.Join(res, ""), "aabbcc")
}
