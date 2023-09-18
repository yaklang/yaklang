package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
	"testing"
)

func TestPerformance(t *testing.T) {
	methods := map[string]func(string) []string{
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
	}
	invokeRecord := map[string]int{}
	for k, v := range methods {
		k := k
		v := v
		methods[k] = func(s string) []string {
			invokeRecord[k]++
			return v(s)
		}
	}
	for _, v := range [][]any{
		{"{{raw::raw({{aaa()}})}}", map[string]int{"raw": 1}},
		{"aaa{{raw::raw({{aaa()}})}}aaa{{repeat(3)}}", map[string]int{"raw": 3, "repeat": 1}},
		{"{{randstr::rep()}}{{repeat(10)}}", map[string]int{"randstr": 1, "repeat": 1}},
		{"{{randstr()}}{{repeat(10)}}", map[string]int{"randstr": 10, "repeat": 1}},
		{"{{array(a|b|c)}}{{repeat(2)}}", map[string]int{"array": 2, "repeat": 1}},
	} {
		t, r := v[0].(string), v[1].(map[string]int)
		invokeRecord = map[string]int{}
		_, err := ExecuteWithStringHandler(t, methods)
		if err != nil {
			panic(err)
		}
		if len(invokeRecord) != len(r) {
			panic("TestPerformance failed")
		}
		for k, v := range invokeRecord {
			if r[k] != v {
				panic("TestPerformance failed")
			}
		}
	}
}
