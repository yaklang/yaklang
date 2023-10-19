package standard_parser

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fuzztagx"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
	"testing"
)

func TestExecuteWithRam(t *testing.T) {
	var testMap = map[string]func(string) []string{
		"int": func(i string) []string {
			return []string{i}
		},
		"list": func(s string) []string {
			return strings.Split(s, "|")
		},
	}
	a, err := fuzztagx.ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 3 {
		panic(a)
	}

	a, err = fuzztagx.ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 3 {
		panic(a)
	}

	a, err = fuzztagx.ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc|ddd|eee)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 4 {
		panic(a)
	}

	a, err = fuzztagx.ExecuteWithStringHandler(`{{int::3({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}

	a, err = fuzztagx.ExecuteWithStringHandler(`{{int({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}

	a, err = fuzztagx.ExecuteWithStringHandler(`{{int({{list(aaa|ccc|ddd)}})}}{{int({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}
}

func TestExecute(t *testing.T) {
	var testMap = map[string]func(string) []string{
		"int": func(i string) []string {
			return funk.Map(utils.ParseStringToPorts(i), func(i int) string {
				return strconv.Itoa(i)
			}).([]string)
		},
		"test1": func(s string) []string {
			return []string{
				"test1(asdfasdfas)",
				"test1(asdfasdfas)",
				"test1(asdfasdfas)",
				"test1(asdfasdfas)",
			}
		},
		"test": func(s string) []string {
			return []string{
				"WRPA:" + s,
			}
		},
		"punc": func(s string) []string {
			return []string{s + "PUNC"}
		},
	}
	a, err := fuzztagx.ExecuteWithStringHandler(
		//`{{int(1-2)}}abc{{int(1-5)}}`,
		//`abc{{in}}^t()))1{{a111-5)}}`,
		`{{xx12:-_(____\)____)}}[[[[}}`,
		testMap,
	)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}

func TestExecuteWithHandler(t *testing.T) {
	for _, v := range [][]string{
		{"{{int(1-29)}}", "1"},
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
		{"{{{{int}}{{1[[[[}}", "{{1{{1[[[[}}"},
		//{"{{i{{int}}nt(1-2)}}", "{{i1nt(1-2)}}"}, // 不允许渲染函数名
		{"{{", "{{"},
		//{"{{test(123123\\))}}", "123123)"}, // 括号不需要转义
		//{"{{print(list{\\())}}", "{{print(list{\\())}}"},
		//{"{{print(list{\\(\\))}}", ""},
		{"{{{test(123)}}", "{123"},
		// {"{{i{{int}}n{{int}}t(1-2)}}", "{{i1nt(1-2)}}"},
	} {
		t, r := v[0], v[1]
		spew.Dump(t)
		result, err := fuzztagx.ExecuteWithStringHandler(t, map[string]func(string) []string{
			"int": func(s string) []string {
				return []string{"1"}
			},
			"test": func(s string) []string {
				return []string{s}
			},
		})
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

	var testMap = map[string]func(string) []string{
		"int": func(i string) []string {
			return funk.Map(utils.ParseStringToPorts(i), func(i int) string {
				return strconv.Itoa(i)
			}).([]string)
		},
	}
	for _, v := range [][]string{
		{"{{int(1-29)}}", "29"},
		{"{{int(1-29)}}==={{int(1-29}}", fmt.Sprint(29)},
		{"{{int(1-29)}}==={{int(1-29)}}", fmt.Sprint(29 * 29)},
		{"{{int(1-29)}}==={{int(1-2)}}", fmt.Sprint(29 * 2)},
		{"{{int(1-29)}}==={{int(1)}}", fmt.Sprint(29)},
	} {
		t, r := v[0], v[1]
		result, err := fuzztagx.ExecuteWithStringHandler(t, testMap)
		if err != nil {
			panic(err)
		}
		if len(result) <= 0 {
			panic(1)
		}
		rStr := fmt.Sprint(len(result))
		if rStr != r {
			m := fmt.Sprintf("got: %v expect: %v", strconv.Quote(rStr), strconv.Quote(r))
			panic(m)
		}
	}

}

func TestExecuteWithNewLine(t *testing.T) {
	var m = map[string]func(string) []string{
		"s": func(s string) []string {
			return []string{s + "a"}
		},
	}

	res, err := fuzztagx.ExecuteWithStringHandler(`{{s 
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

	res, err := fuzztagx.ExecuteWithStringHandler(`{{int::aaa(1)}} {{int::aaa(1)}} {{int::aaa(1)}}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || res[0] != "1 1 1" {
		panic("error")
	}
}

func TestExecuteBug_Execute(t *testing.T) {
	var m = map[string]func(string) []string{
		"int": func(s string) []string {
			return []string{s}
		},
		"expr:a": func(s string) []string {
			if s != "base64(111) " {
				panic(1)
			}
			return []string{"ccc"}
		},
	}

	res, err := fuzztagx.ExecuteWithStringHandler(`{{expr:a(base64(111) )}}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if res[0] != "ccc" {
		panic("PANIC!")
	}
}
func TestExecutePrefixTag(t *testing.T) {
	var m = map[string]func(string) []string{
		"expr:a": func(s string) []string {
			return []string{s}
		},
	}
	testData := []string{
		"base64(111)",
		" base64(111)",
		" base64(111) ",
		" base64(base64(111)) ",
		"base64(111))))",
		"base64((111)",
	}
	for _, d := range testData {
		println(d)
		res, err := fuzztagx.ExecuteWithStringHandler(fmt.Sprintf(`{{expr:a(%s)}}`, d), m)
		if err != nil {
			panic(utils.Errorf("test data [%v] error: %v", d, err))
		}
		if len(res) == 0 {
			panic("generate error")
		}
		if res[0] != d {
			panic("TestExecutePrefixTag failed")
		}
	}
}
