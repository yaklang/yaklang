package fuzztagx

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
	"testing"
)

func TestExec(t *testing.T) {
	var testMap = map[string]BuildInTagFun{
		"int": func(i string) []string {
			return []string{i}
		},
		"list": func(s string) []string {
			return strings.Split(s, "|")
		},
	}
	a, err := ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 3 {
		panic(a)
	}

	a, err = ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 3 {
		panic(a)
	}

	a, err = ExecuteWithStringHandler(`{{int::1({{list(aaa|ccc|ddd|eee)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 4 {
		panic(a)
	}

	a, err = ExecuteWithStringHandler(`{{int::3({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}

	a, err = ExecuteWithStringHandler(`{{int({{list(aaa|ccc|ddd)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}

	a, err = ExecuteWithStringHandler(`{{int({{list(aaa|ccc|ddd)}})}}{{int({{list(aaa|ccc|ddd)}})}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 9 {
		panic(a)
	}
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
		{"{{xx12:-_(____)____)}}[[[[}}", "{{xx12:-_(____)____)}}[[[[}}"},
		{"{{xx12:-_(____\\)____)}}[[[[}}", "[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{[[[[}}", "{[[[[}}"},
		{"{{xx12:-_(____)} }}____)}}[[[[}}", "{{xx12:-_(____)} }}____)}}[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{{[[[[}}", "{{[[[[}}"},
		{"{{xx12:-_(____\\)} }____)}}{{1[[[[}}", "{{1[[[[}}"},
		//{"{{xx12:-_(____\\)} }__)__)}}{{1[[[[}}", "{{xx12:-_(____\\)} }__)__)}}{{1[[[[}}"},
		{"{{xx12:-_(____\\)} }__\\)__)}}{{1[[[[}}", "{{1[[[[}}"},
		{"{{{{1[[[[}}", "{{{{1[[[[}}"},
		{"{{{{int}}{{1[[[[}}", "{{1{{1[[[[}}"},
		{"{{i{{int}}nt(1-2)}}", "{{i1nt(1-2)}}"},
		{"{{", "{{"},
		{"{{test(123123\\))}}", "123123)"},
		{"{{print(list{\\())}}", "{{print(list{\\())}}"},
		{"{{print(list{\\(\\))}}", ""},
		{"{{{test(123)}}", "{123"},
		// {"{{i{{int}}n{{int}}t(1-2)}}", "{{i1nt(1-2)}}"},
	} {
		t, r := v[0], v[1]
		spew.Dump(t)
		result, err := ExecuteWithStringHandler(t, map[string]BuildInTagFun{
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

	var testMap = map[string]BuildInTagFun{
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
		result, err := ExecuteWithStringHandler(t, testMap)
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
func TestExecuteWithHandlerEscaped(t *testing.T) {
	for _, v := range [][]string{
		{"{{test(123123\\))}}", "123123)"},
		{"\\){{test(123123\\))}}", "\\)123123)"},
		{"\\){{test(1{{test(\\)1)}}23123\\))}}", "\\)1)123123)"},
	} {
		t, r := v[0], v[1]
		result, err := ExecuteWithStringHandler(t, map[string]BuildInTagFun{
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

	var testMap = map[string]BuildInTagFun{
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
		result, err := ExecuteWithStringHandler(t, testMap)
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
func TestExecuteWithConciseTag(t *testing.T) {
	var testMap = map[string]BuildInTagFun{
		"print": func(i string) []string {
			return []string{i}
		},
		"list": func(s string) []string {
			return strings.Split(s, "|")
		},
	}
	a, err := ExecuteWithStringHandler(`{{print::out1(list(a|b))}}{{print::out1(list(a|b))}}`, testMap)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
	if len(a) != 2 {
		panic(a)
	}
}
func TestExecuteWithMultimethod(t *testing.T) {
	var m = map[string]BuildInTagFun{
		"s": func(s string) []string {
			return []string{s + "a"}
		},
	}

	res, err := ExecuteWithStringHandler(`{{  s()    s()   }}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || len(res[0]) != 2 {
		panic("{{s()s()}}")
	}

	res, err = ExecuteWithStringHandler(`{{  s(s())   }}`, m)
	spew.Dump(res)
	if err != nil {
		panic(err)
	}
	if len(res) < 1 || len(res[0]) != 2 {
		panic("{{  s(s())   }}")
	}
}
func TestExecuteWithNewLine(t *testing.T) {
	var m = map[string]BuildInTagFun{
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
