package utils

import (
	"reflect"
	"sort"
	"testing"
)

func TestGetSameSubStringsCompatibility(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{nil, nil}, {[]string{"only"}, nil}, {[]string{"", ""}, nil},
		{[]string{"same", "same"}, nil}, {[]string{"ab", "cd"}, []string{}},
		{[]string{"你好abc", "你好xyz"}, []string{"你好"}},
		{[]string{"abcabc", "abcXabc"}, []string{"abc"}},
		{[]string{"aXXb", "aYYb", "aZZb"}, []string{"a", "b"}},
		{[]string{"aXXb", "aYYb", "aZZb", "aWWb"}, []string{"a", "b"}},
		{[]string{"aXXb", "aYYb", "aZZb", "none"}, []string{}},
	} {
		got := GetSameSubStrings(tc.in...)
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %#v want %#v", tc.in, got, tc.want)
		}
	}
}
