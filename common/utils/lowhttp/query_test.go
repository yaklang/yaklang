package lowhttp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQueryParams1(t *testing.T) {
	test := assert.New(t)
	params := NewQueryParams("a=1&b=2")
	params.Add("c", "3")
	test.Equal("a=1&b=2&c=3", params.Encode())

	params.Set("a", "22")
	test.Equal("a=22&b=2&c=3", params.Encode())

	params.Add("c", "4")
	test.Equal("a=22&b=2&c=3&c=4", params.Encode())

	params.Del("c")
	test.Equal("a=22&b=2", params.Encode())

	params.Remove("b")
	test.Equal("a=22", params.Encode())

	params.Add("c", "3")
	test.Equal("a=22&c=3", params.Encode())

	test.Equal("22", params.Get("a"))
	test.Equal("3", params.Get("c"))
	test.Equal("", params.Get("b"))

	params.Add("c", "5")
	test.Equal("a=22&c=3&c=5", params.Encode())
	test.Equal([]string{"3", "5"}, params.GetAll("c"))

	params.DisableAutoEncode(true).AppendRaw("ac2224*(&*&&*&*((&*")
	test.Equal("a=22&c=3&c=5&ac2224*(&*&&*&*((&*", params.Encode())
	params.Set("ccc", "&&&")
	params.DisableAutoEncode(false)
	test.Equal("a=22&c=3&c=5&ac2224*(&*&&*&*((&*&ccc=&&&", params.Encode())
	params.Set("ddd", "cc31==+&")
	test.Equal("a=22&c=3&c=5&ac2224*(&*&&*&*((&*&ccc=&&&&ddd=cc31%3D%3D%2B%26", params.Encode())
}
