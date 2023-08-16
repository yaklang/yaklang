// Package crawlerx
// @Author bcy2007  2023/8/1 11:36
package crawlerx

import (
	"github.com/stretchr/testify/assert"
	"net/url"
	"reflect"
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
