package lowhttp

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"yaklang/common/utils"
)

func TestCookiesToString(t *testing.T) {
	cookies := []*http.Cookie{
		{
			Name:  "name1",
			Value: "value1",
		},
		{
			Name:  "name2",
			Value: "value2",
		},
	}
	result := CookiesToString(cookies)
	if result != "name1=value1; name2=value2" {
		t.Errorf("CookiesToString failed, result: %s", result)
	}
}

func TestAddOrUpgradeCookie(t *testing.T) {
	raw, err := AddOrUpgradeCookie([]byte(`GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 0

`), "a=b")
	if err != nil {
		t.Fatal(err)
		return
	}

	expected := []byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\nContent-Length: 0\r\nCookie: a=b\r\n\r\n")

	if !bytes.Equal(raw, expected) {
		t.Fatalf("expected %v, but got %v", string(expected), string(raw))
		return
	}

	raw, err = AddOrUpgradeCookie([]byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: c=d
Content-Length: 0

`), "a=b")
	if err != nil {
		t.Fatal(err)
		return
	}

	expected = []byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\nCookie: c=d; a=b\r\nContent-Length: 0\r\n\r\n")

	if !bytes.Equal(raw, expected) {
		t.Fatalf("expected %v, but got %v", string(expected), string(raw))
		return
	}
}

func TestUrlParamsJoin(t *testing.T) {
	for _, c := range []map[string]struct {
		url      string
		params   []interface{}
		expected string
	}{
		{
			"1": {
				url: "https://baidu.com",
				params: []interface{}{
					map[string]interface{}{
						"abc": 123,
					},
				},
				expected: "https://baidu.com/?abc=123",
			},
		},
		{
			"2": {
				url: "https://baidu.com/",
				params: []interface{}{
					map[string]interface{}{
						"abc": 123,
					},
				},
				expected: "https://baidu.com/?abc=123",
			},
		},
		{
			"3": {
				url: "https://baidu.com/abc",
				params: []interface{}{
					map[string]interface{}{
						"abc": 123,
					},
				},
				expected: "https://baidu.com/abc?abc=123",
			},
		},
		{
			"4": {
				url: "https://baidu.com/abc/",
				params: []interface{}{
					map[string]interface{}{
						"abc": 123,
					},
				},
				expected: "https://baidu.com/abc/?abc=123",
			},
		},
		{
			"5": {
				url: "https://baidu.com/abc/?a=1",
				params: []interface{}{
					map[string]interface{}{
						"abc": 123,
					},
				},
				expected: "https://baidu.com/abc/?a=1&abc=123",
			},
		},
		{
			"6": {
				url: "https://baidu.com/abc/?a=1",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "https://baidu.com/abc/?a=1&abc=1&abc=2",
			},
		},
		{
			"7": {
				url: "https://baidu.com/abc/?a=1&c=4444a",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "https://baidu.com/abc/?a=1&c=4444a&abc=1&abc=2",
			},
		},
		{
			"8": {
				url:      "https://baidu.com",
				params:   []interface{}{},
				expected: "https://baidu.com",
			},
		},
		{
			"9": {
				url:      "https://baidu.com/a",
				params:   []interface{}{},
				expected: "https://baidu.com/a",
			},
		},
		{
			"10": {
				url:      "https://baidu.com/a",
				params:   []interface{}{},
				expected: "https://baidu.com/a",
			},
		},
		{
			"11": {
				url: "a=1",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "a=1&abc=1&abc=2",
			},
		},
		{
			"12": {
				url: "a=1&c=1",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "a=1&c=1&abc=1&abc=2",
			},
		},
		{
			"13": {
				url: "a=1&c=1#123",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "a=1&c=1#123&abc=1&abc=2",
			},
		},
		{
			"14": {
				url: "https://baidu.com/?a=1&c=1#123",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "https://baidu.com/?a=1&c=1&abc=1&abc=2#123",
			},
		},
		{
			"15": {
				url: "",
				params: []interface{}{
					map[string]interface{}{
						"abc": []string{"1", "2"},
					},
				},
				expected: "abc=1&abc=2",
			},
		},
		{
			"16": {
				url: "",
				params: []interface{}{
					map[string]interface{}{
						"abc": "admin",
					},
				},
				expected: "abc=admin",
			},
		},
	} {
		for key, value := range c {
			println("开始测试案例：" + key)
			if ret := utils.UrlJoinParams(value.url, value.params...); ret != value.expected {
				fmt.Printf("expect: %v got: %v\n", value.expected, ret)
				t.FailNow()
			}
		}
	}
}
