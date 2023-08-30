package utils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
)

func TestParseHostToString(t *testing.T) {
	cases := map[string]string{
		"baidu.com":       "baidu.com",
		"1.1.1.1":         "1.1.1.1",
		"::":              "[::]",
		"::fbff":          "[::fbff]",
		"ffff:ffff::fbff": "[ffff:ffff::fbff]",
	}

	for i, expect := range cases {
		assert.Equal(t, expect, ParseHostToAddrString(i))
	}

	for i := 0; i < 120; i++ {
		println(GetRandomIPAddress())
	}
}

func TestSplitHostsAndPorts(t *testing.T) {
	p, pub := SplitHostsToPrivateAndPublic("baidu.com,127.0.0.1,192.168.1.1,8.8.8.8")
	spew.Dump(p)
	spew.Dump(pub)
	if len(p) != 2 {
		panic(1)
	}
}

func TestParseStringToHosts(t *testing.T) {
	cases := map[string][]string{
		//"1.1.1.1-3":          {"1.1.1.1", "1.1.1.2", "1.1.1.3"},
		"1.1.1.1/30":         {"1.1.1.1", "1.1.1.0", "1.1.1.2", "1.1.1.3"},
		"baidu.com":          {"baidu.com"},
		"1.1.1.1,2.2.2.0/31": {"1.1.1.1", "2.2.2.0", "2.2.2.1"},
		"1.1.1.1-3":          {"1.1.1.1", "1.1.1.2", "1.1.1.3"},
		"1.1.1.1,[::1],::1":  {"1.1.1.1", "::1"},
	}

	for input, expected := range cases {
		flag := false
		for _, result := range ParseStringToHosts(input) {
			if !StringArrayContains(expected, result) {
				t.Logf("go result: %s expected: %s got %v", result, expected, result)
				t.FailNow()
			} else {
				flag = true
			}
		}

		if !flag {
			t.Logf("empty result for %s", input)
			t.FailNow()
		}
	}
}

func TestParseStringToPorts(t *testing.T) {
	cases := map[string][]int{
		"1,2,3,4-6":         {1, 2, 3, 4, 5, 6},
		"-5":                {1, 2, 3, 4, 5},
		"1,2,3,4,1,2,3,1-3": {1, 2, 3, 4},
		"65534-":            {65534, 65535},
	}

	for input, expected := range cases {
		results := ParseStringToPorts(input)
		t.Log(results)

		flag := false
		for _, result := range results {
			if !IntArrayContains(expected, result) {
				t.Logf("go result: %v expected: %v", result, expected)
				t.FailNow()
			} else {
				flag = true
			}
		}

		if !flag {
			t.Logf("empty result for %s", input)
			t.FailNow()
		}
	}

}

func TestIPv6Use(t *testing.T) {
	addr := "[::]:8089"
	lis, err := net.Listen("tcp6", addr)
	assert.Nil(t, err)
	defer lis.Close()

	addr = "[::]:8093"
	lis, err = net.Listen("tcp", addr)
	assert.Nil(t, err)
	defer lis.Close()

	i := net.ParseIP("::ff")
	assert.NotNil(t, i)
}

func TestFixForParseIP(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1": "1.1.1.1",
		"[::]":    "::",
		"::ff":    "::ff",
		"[::ff]":  "::ff",
	}
	for i, e := range cases {
		ip := net.ParseIP(FixForParseIP(i))
		assert.NotNil(t, ip)
		assert.Equal(t, e, ip.String())
	}
}

func TestIsIPv6(t *testing.T) {
	cases := map[string]bool{
		"1.1.1.1":  false,
		"::":       true,
		"ffff::ff": true,
		"[::]":     true,
	}
	for i, e := range cases {
		assert.Equal(t, e, IsIPv6(i), "case for: %s", i)
	}
}

func TestHostsFilter(t *testing.T) {
	type filterCase struct {
		exclude []string
		target  string
		result  bool
	}
	cases := []filterCase{
		{
			exclude: []string{"1.1.1.1-3"},
			target:  "1.1.1.1",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1-3"},
			target:  "1.1.1.2",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1-3"},
			target:  "1.1.1.3",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1-3"},
			target:  "1.1.1.4",
		},

		{
			exclude: []string{"1.1.1.1/24"},
			target:  "1.1.1.1",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1/24"},
			target:  "1.1.1.12",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1/24"},
			target:  "1.1.1.111",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1/24,2.2.2.2-4"},
			target:  "1.1.1.145",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1/24,1.1.1.1-3"},
			target:  "1.1.1.122",
			result:  true,
		},
		{
			exclude: []string{"1.1.1.1/24,1.1.1.1-3"},
			target:  "1.2.1.122",
		},
		{
			exclude: []string{"1.1.1.1/24,1.1.1.1-3"},
			target:  "13.2.1.122",
		},
	}

	for _, fCase := range cases {
		filter := NewHostsFilter(fCase.exclude...)
		if !assert.Equal(t, fCase.result, filter.Contains(fCase.target),
			"input: %v exclude: %v expect: %v", fCase.target,
			fCase.exclude, fCase.result) {
			t.FailNow()
		}
	}
}

func TestPortsFilter(t *testing.T) {
	type portFilterCase struct {
		input   int
		exclude []string
		result  bool
	}

	cases := []portFilterCase{
		{
			input: 44, exclude: []string{"44-46"}, result: true,
		},
		{
			input: 45, exclude: []string{"44-46"}, result: true,
		},
		{
			input: 46, exclude: []string{"44-46"}, result: true,
		},
		{
			input: 47, exclude: []string{"44-46"},
		},
		{
			input: 47, exclude: []string{"-44"}, result: false,
		},
		{
			input: 1, exclude: []string{"-44"}, result: true,
		},
		{
			input: 44, exclude: []string{"-44"}, result: true,
		},
		{
			input: 44, exclude: []string{"44,10000-"}, result: true,
		},
		{
			input: 10043, exclude: []string{"44,10000-"}, result: true,
		},
		{
			input: 9999, exclude: []string{"44,10000-"}, result: false,
		},
	}

	for _, c := range cases {
		p := NewPortsFilter(c.exclude...)
		assert.Equal(t, c.result, p.Contains(c.input), "input: %v exclude: %v", c.input, c.exclude)
	}
}
