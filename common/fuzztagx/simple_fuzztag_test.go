package fuzztagx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"strconv"
	"strings"
	"testing"
)

func TestSimpleFuzzTag_Exec(t *testing.T) {
	for _, test := range []struct {
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
	} {
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
		}, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; gener.Next(); i++ {
			if string(gener.Result().GetData()) != test.expect[i] {
				t.Fatal("error")
			}
		}
	}
}
