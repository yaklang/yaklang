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
			// Java source enums are implicitly final/abstract; those modifiers are
			// illegal to write explicitly, so the verbose string drops them while the
			// raw flag list still reflects the bytecode access flags.
			name:     "公共final枚举",
			input:    0x4011,
			wantFlag: []string{"public", "final", "enum"},
			wantStr:  "public enum",
		},
		{
			name:     "公共枚举注解",
			input:    0x6001,
			wantFlag: []string{"public", "annotation", "enum"},
			wantStr:  "public enum",
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

// TestMethodAccessFlags locks the ACC_STRICT modifier rendering. The bit must surface as the Java
// keyword `strictfp`, never the bare `strict`: the latter is not a valid modifier token, so a
// `public strict double m()` signature failed syntax validation and the whole method (plus its
// stub) was dropped. Root cause of the FloatingIOWriter doubleValue()/floatValue() drop warnings.
func TestMethodAccessFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    uint16
		wantFlag []string
		wantStr  string
	}{
		{
			name:     "public strictfp",
			input:    0x0801, // ACC_PUBLIC | ACC_STRICT
			wantFlag: []string{"public", "strictfp"},
			wantStr:  "public strictfp",
		},
		{
			name:     "package-private strictfp",
			input:    0x0800, // ACC_STRICT only
			wantFlag: []string{"strictfp"},
			wantStr:  "strictfp",
		},
		{
			name:     "public static final",
			input:    0x0019, // ACC_PUBLIC | ACC_STATIC | ACC_FINAL
			wantFlag: []string{"public", "static", "final"},
			wantStr:  "public static final",
		},
		{
			// native is recorded in the flag list but intentionally omitted from the modifier
			// string (the dumper renders native methods specially), so only the access modifier
			// reaches the rendered prefix.
			name:     "public native",
			input:    0x0101, // ACC_PUBLIC | ACC_NATIVE
			wantFlag: []string{"public", "native"},
			wantStr:  "public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlag, gotStr := getMethodAccessFlagsVerbose(tt.input)
			if !reflect.DeepEqual(gotFlag, tt.wantFlag) {
				t.Errorf("getMethodAccessFlagsVerbose() 获取的标志 = %v, 期望标志 = %v", gotFlag, tt.wantFlag)
			}
			if gotStr != tt.wantStr {
				t.Errorf("getMethodAccessFlagsVerbose() 获取的字符串 = %v, 期望字符串 = %v", gotStr, tt.wantStr)
			}
		})
	}
}
