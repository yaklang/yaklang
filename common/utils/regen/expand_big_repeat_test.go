package regen

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_expandBigRepeat(t *testing.T) {
	// [0-9a-z]{9999} -> 9x [0-9a-z]{1000} + [0-9a-z]{999} = 9999
	got := expandBigRepeat("[0-9a-z]{9999}")
	t.Logf("expanded: %s", got)
	// 应包含 9 个 {1000} 和 1 个 {999}
	n1000, n999 := 0, 0
	for i := 0; i <= len(got)-4; i++ {
		if got[i:i+4] == "1000" && (i == 0 || got[i-1] == '{') && (i+4 == len(got) || got[i+4] == '}') {
			n1000++
		}
	}
	for i := 0; i <= len(got)-3; i++ {
		if got[i:i+3] == "999" && (i == 0 || got[i-1] == '{') && (i+3 == len(got) || got[i+3] == '}') {
			n999++
		}
	}
	require.Equal(t, 9, n1000, "expanded: %s", got)
	require.Equal(t, 1, n999, "expanded: %s", got)
	// 展开后的模式应能被 Go 解析，且匹配长度恰好为 9999 的串
	re, err := regexp.Compile(got)
	require.NoError(t, err)
	// 生成一个并检查长度
	s, err := GenerateOne(got)
	require.NoError(t, err)
	require.True(t, re.MatchString(s), "generated %q should match expanded pattern", s)
	require.Len(t, s, 9999, "generated length should be 9999; expanded was: %s", got)
}

func Test_expandBigRepeat_EdgeCases(t *testing.T) {
	// escaped '{' should not be treated as quantifier
	pattern := `\{1001}`
	expanded := expandBigRepeat(pattern)
	require.Equal(t, pattern, expanded)

	// escaped sequences like \d should expand correctly
	pattern = `\d{1001}`
	expanded = expandBigRepeat(pattern)
	re, err := regexp.Compile(expanded)
	require.NoError(t, err)
	s, err := GenerateOne(expanded)
	require.NoError(t, err)
	require.True(t, re.MatchString(s))
	require.Len(t, s, 1001)

	// divisible case: no remainder and no {0}
	pattern = `a{2000}`
	expanded = expandBigRepeat(pattern)
	require.False(t, strings.Contains(expanded, "{0}"), "expanded: %s", expanded)
	require.Equal(t, 2, strings.Count(expanded, "{1000}"), "expanded: %s", expanded)
	s, err = GenerateOne(expanded)
	require.NoError(t, err)
	require.Len(t, s, 2000)

	// nested groups
	pattern = `(abc){1500}`
	expanded = expandBigRepeat(pattern)
	re, err = regexp.Compile(expanded)
	require.NoError(t, err)
	s, err = GenerateOne(expanded)
	require.NoError(t, err)
	require.True(t, re.MatchString(s))
	require.Len(t, s, 4500)

	// char class with leading ']' literal
	pattern = `[]a]{1001}`
	expanded = expandBigRepeat(pattern)
	re, err = regexp.Compile(expanded)
	require.NoError(t, err)
	s, err = GenerateOne(expanded)
	require.NoError(t, err)
	require.True(t, re.MatchString(s))
	require.Len(t, s, 1001)
	for _, r := range s {
		require.True(t, r == ']' || r == 'a')
	}

	// escaped backslash inside char class
	pattern = `[\\]{1001}`
	expanded = expandBigRepeat(pattern)
	re, err = regexp.Compile(expanded)
	require.NoError(t, err)
	s, err = GenerateOne(expanded)
	require.NoError(t, err)
	require.True(t, re.MatchString(s))
	require.Len(t, s, 1001)
	for _, r := range s {
		require.Equal(t, '\\', r)
	}
}
