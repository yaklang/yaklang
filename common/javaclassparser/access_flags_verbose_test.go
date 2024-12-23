package javaclassparser

import (
	"reflect"
	"testing"
)

func TestClassAccessFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    uint16
		wantFlag []string
		wantStr  string
	}{
		{
			name:     "公共类",
			input:    0x0001,
			wantFlag: []string{"public"},
			wantStr:  "public",
		},
		{
			name:     "公共final类",
			input:    0x0011,
			wantFlag: []string{"public", "final"},
			wantStr:  "public final",
		},
		{
			name:     "公共接口",
			input:    0x0201,
			wantFlag: []string{"public", "interface"},
			wantStr:  "public interface",
		},
		{
			name:     "公共抽象类",
			input:    0x0401,
			wantFlag: []string{"public", "abstract"},
			wantStr:  "public abstract",
		},
		{
			name:     "公共抽象接口",
			input:    0x0601,
			wantFlag: []string{"public", "interface"},
			wantStr:  "public interface",
		},
		{
			name:     "公共final枚举",
			input:    0x4011,
			wantFlag: []string{"public", "final", "enum"},
			wantStr:  "public final enum",
		},
		{
			name:     "公共枚举注解",
			input:    0x6001,
			wantFlag: []string{"public", "annotation", "enum"},
			wantStr:  "public annotation enum",
		},
		{
			name:     "抽象类",
			input:    0x0400,
			wantFlag: []string{"abstract"},
			wantStr:  "abstract",
		},
		{
			name:     "包可见类",
			input:    0x0000,
			wantFlag: []string{},
			wantStr:  "",
		},
		{
			name:     "公共注解",
			input:    0x2001,
			wantFlag: []string{"public", "annotation"},
			wantStr:  "public annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlag, gotStr := getClassAccessFlagsVerbose(tt.input)
			if !reflect.DeepEqual(gotFlag, tt.wantFlag) {
				t.Errorf("getClassAccessFlagsVerbose() 获取的标志 = %v, 期望标志 = %v", gotFlag, tt.wantFlag)
			}
			if gotStr != tt.wantStr {
				t.Errorf("getClassAccessFlagsVerbose() 获取的字符串 = %v, 期望字符串 = %v", gotStr, tt.wantStr)
			}
		})
	}
}
