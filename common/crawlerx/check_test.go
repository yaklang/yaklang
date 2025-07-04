// Package crawlerx
// @Author bcy2007  2023/8/1 11:36
package crawlerx

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestUrlRepeatCheckGenerator(t *testing.T) {
	testURL := "https://www.abc.com?id=3&value=4"
	type args struct {
		level       repeatLevel
		extraParams []string
		arg         string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "extreme level check",
			args: args{extremeLevel, []string{}, testURL},
			want: "https://www.abc.com",
		},
		{
			name: "low level check",
			args: args{lowLevel, []string{}, testURL},
			want: "https://www.abc.com?id=3&value=4",
		},
		{
			name: "mid level check",
			args: args{midLevel, []string{}, testURL},
			want: "https://www.abc.com?id&value",
		},
		{
			name: "normal1",
			args: args{lowLevel, []string{}, "http://testphp.vulnweb.com/"},
			want: "http://testphp.vulnweb.com/",
		},
		{
			name: "normal2",
			args: args{extremeLevel, []string{}, "http://testphp.vulnweb.com/"},
			want: "http://testphp.vulnweb.com/",
		},
		{
			name: "extra test",
			args: args{lowLevel, []string{"id"}, testURL},
			want: "https://www.abc.com?value=4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFunction := urlRepeatCheckGenerator(tt.args.level, tt.args.extraParams...)
			if got := testFunction(tt.args.arg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("urlRepeatCheckGenerator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUrlQuery(t *testing.T) {
	test := assert.New(t)
	urlStr := "https://www.abc.com?id=3&value=4&test=5&data=xxx"
	urlObj, _ := url.Parse(urlStr)
	query, _ := GetSortedQuery(urlObj.RawQuery)
	length := len(query)
	var queryItem []string
	for i := 0; i < length-1; i += 2 {
		queryItem = append(queryItem, query[i]+"="+query[i+1])
	}
	test.Equal("id=3&value=4&test=5&data=xxx", strings.Join(queryItem, "&"))
}

func TestGeneralSubDomainRange(t *testing.T) {
	type args struct {
		targetUrl string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"case1", args{targetUrl: "https://www.test.com/abc/def/"}, []string{"https://www.test.com/abc/def/"}},
		{"case2", args{targetUrl: "https://www.test.com/abc/def"}, []string{"https://www.test.com/abc/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, generalSubDomainRange(tt.args.targetUrl), "generalSubDomainRange(%v)", tt.args.targetUrl)
		})
	}
}

func TestBoardDomainCompilerStr(t *testing.T) {
	type args struct {
		arg string
	}
	var testUrl = "4dogs.cn"
	testFunc := func(targetUrl string) bool {
		reg, err := regexp.Compile(fmt.Sprintf(boardDomainCompilerStr, testUrl))
		if err != nil {
			t.Fatal(err)
			return false
		}
		return reg.MatchString(targetUrl)
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"case1", args{"http://www.4dogs.cn/abc"}, true},
		{"case2", args{"http://test.4dogs.cn/abc"}, true},
		{"case3", args{"https://www.4dogs.cn/abc"}, true},
		{"case4", args{"https://www4dogs.cn/abc"}, false},
		{"case5", args{"https://4dogs.cn/abc"}, true},
		{"case6", args{"https://www.4dogs.cnabc"}, false},
		{"case7", args{"https://abc/www.4dogs.cn/"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := testFunc(tt.args.arg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("urlRepeatCheckGenerator() = %v, want %v", got, tt.want)
			}
		})
	}
}
