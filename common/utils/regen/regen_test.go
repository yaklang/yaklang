package regen

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func testAllAndStream(t *testing.T, pattern string) {
	t.Helper()

	results, err := Generate(pattern)
	require.NoError(t, err)

	ch, cancel, err := GenerateStream(pattern)
	require.NoError(t, err)
	defer cancel()

	results2 := make([]string, 0, len(results))
	for result := range ch {
		results2 = append(results2, result)
	}

	require.ElementsMatch(t, results, results2)
}

func testOneAndStream(t *testing.T, pattern string) {
	t.Helper()

	re := regexp.MustCompile(pattern)
	result, err := GenerateOne(pattern)
	require.NoError(t, err)

	result2, err := GenerateOneStream(pattern)
	require.NoError(t, err)

	// we can't use require.Equal here because the order of the runes in the string is not guaranteed
	// require.Equal(t, result, result2)
	require.True(t, re.MatchString(result))
	require.True(t, re.MatchString(result2))
}

func testVisibleOneAndStream(t *testing.T, pattern string) {
	t.Helper()

	re := regexp.MustCompile(pattern)
	result, err := GenerateVisibleOne(pattern)
	require.NoError(t, err)

	result2, err := GenerateVisibleOneStream(pattern)
	require.NoError(t, err)

	// we can't use require.Equal here because the order of the runes in the string is not guaranteed
	// require.Equal(t, result, result2)
	require.True(t, re.MatchString(result))
	require.True(t, re.MatchString(result2))
}

func Test_Smoke_Generate(t *testing.T) {
	testAllAndStream(t, `aab`)
	testAllAndStream(t, `.?1*2+3{1}`)
	testAllAndStream(t, `22|33`)
	testAllAndStream(t, `(1|2){2,3}`)
}

func Test_Smoke_GenerateOne(t *testing.T) {
	testOneAndStream(t, `aab`)
	testOneAndStream(t, `.?1*2+3{1}`)
	testOneAndStream(t, `22|33`)
	testOneAndStream(t, `(1|2){2,3}`)
	testOneAndStream(t, `(?:ATGPlatform/([\d.]+))?`)
}

func Test_Smoke_GenerateVisibleOne(t *testing.T) {
	testVisibleOneAndStream(t, `aab`)
	testVisibleOneAndStream(t, `.?1*2+3{1}`)
	testVisibleOneAndStream(t, `22|33`)
	testVisibleOneAndStream(t, `(1|2){2,3}`)
	testVisibleOneAndStream(t, `(?:ATGPlatform/([\d.]+))?`)
}

// Test_BigRepeat_Over1000 验证用户写 {9999} 等 >1000 时能正确展开并生成
func Test_BigRepeat_Over1000(t *testing.T) {
	// expandBigRepeat 会把 [0-9a-z]{9999} 展开为多个 ≤1000 的重复，生成结果长度应为 9999
	result, err := GenerateOne(`[0-9a-z]{9999}`)
	require.NoError(t, err)
	require.Len(t, result, 9999)
	for _, r := range result {
		require.True(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z'), "rune %q not in [0-9a-z]", r)
	}
	// 小一点再测一发，确保 1001 也会被展开
	result2, err := GenerateOne(`a{1001}`)
	require.NoError(t, err)
	require.Len(t, result2, 1001)
	for _, r := range result2 {
		require.Equal(t, 'a', r)
	}
}

func Test_ExampleGenerateStream(t *testing.T) {
	pattern := `(1|2){2,3}`
	ch, cancel, err := GenerateStream(pattern)
	defer cancel()
	require.NoError(t, err)

	for result := range ch {
		fmt.Printf("%s\n", result)
	}
}

// func Test_GenerateOne(t *testing.T) {
// 	pattern := `(?:ATGPlatform/([\d.]+))?`
// 	result, _ := GenerateOne(pattern)
// 	spew.Dump(result)
// 	result, _ = GenerateVisibleOne(pattern)
// 	spew.Dump(result)
// }

func BenchmarkGenerate(b *testing.B) {
	pattern := `\w{3}`
	for i := 0; i < b.N; i++ {
		Generate(pattern)
	}
}

func Test_GenerateVisibleOne(t *testing.T) {
	type args struct {
		patterns []string
	}
	tests := []struct {
		name    string
		args    args
		wantRes []string
		wantErr bool
	}{
		{
			name:    "Test OpAnyCharNotNlVisibleOne",
			args:    args{patterns: []string{"."}},
			wantRes: []string{"."},
			wantErr: false,
		},
		{
			name:    "Test OpAnyCharVisibleOne",
			args:    args{patterns: []string{"(?s)."}},
			wantRes: []string{"(?s)."},
			wantErr: false,
		},
		{
			name:    "Test OpQuestVisibleOne",
			args:    args{patterns: []string{"a?"}},
			wantRes: []string{"a?"},
			wantErr: false,
		},
		{
			name:    "Test OpStarVisibleOne",
			args:    args{patterns: []string{"a*"}},
			wantRes: []string{"a*"},
			wantErr: false,
		},
		{
			name:    "Test OpPlusVisibleOne",
			args:    args{patterns: []string{"a+"}},
			wantRes: []string{"a+"},
			wantErr: false,
		},
		{
			name:    "Test OpRepeatVisibleOne",
			args:    args{patterns: []string{"a{2}"}},
			wantRes: []string{"a{2}"},
			wantErr: false,
		},
		{
			name:    "Test OpCharClassVisibleOne",
			args:    args{patterns: []string{"[ab]"}},
			wantRes: []string{"[ab]"},
			wantErr: false,
		},
		{
			name:    "Test OpConcatVisibleOne",
			args:    args{patterns: []string{"ab"}},
			wantRes: []string{"ab"},
			wantErr: false,
		},
		{
			name:    "Test OpAlternateVisibleOne",
			args:    args{patterns: []string{"a|b"}},
			wantRes: []string{"a|b"},
			wantErr: false,
		},
		{
			name:    "Test OpCaptureVisibleOne",
			args:    args{patterns: []string{"(a)"}},
			wantRes: []string{"(a)"},
			wantErr: false,
		},
		{
			name:    "Test Multiple",
			args:    args{patterns: []string{"\\s", "\\S", `(?:\.min)?\.css`}},
			wantRes: []string{" ", "[\\x21-\\x7E]", ".css"},
			wantErr: false,
		},
		{
			name:    "Test OpWordBoundary",
			args:    args{patterns: []string{`<[^>]{1,512}\bwire:`, `<iframe[^>]+\blocalfocus\b`}},
			wantRes: []string{"<[^>]{1,512}\\bwire:", "<iframe[^>]+\\blocalfocus\\b"},
			wantErr: false,
		},
		{
			name:    "Test Multiple",
			args:    args{patterns: []string{`(?:ATGPlatform/([\d.]+))?`}},
			wantRes: []string{"(?:ATGPlatform/([\\d.]+))?"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAll []string
			var gotStreamAll []string
			var err error

			for _, pattern := range tt.args.patterns {
				got, e := GenerateVisibleOne(pattern)
				t.Logf("pattern: %s, got: [%s]", pattern, got)
				if e != nil {
					err = e
				}
				gotAll = append(gotAll, got)

				got2, e := GenerateVisibleOneStream(pattern)
				t.Logf("pattern: %s, got: [%s]", pattern, got)
				if e != nil {
					err = e
				}
				gotStreamAll = append(gotStreamAll, got2)
			}

			if tt.wantErr {
				require.Error(t, err)
			}

			for i, g := range gotAll {
				re := regexp.MustCompile(tt.wantRes[i])
				require.Truef(t, re.MatchString(g), "GenerateVisibleOne() got = %v, which doesn't match the pattern %v", g, tt.wantRes[i])
			}
			for i, g := range gotStreamAll {
				re := regexp.MustCompile(tt.wantRes[i])
				require.Truef(t, re.MatchString(g), "GenerateVisibleOneStream() got = %v, which doesn't match the pattern %v", g, tt.wantRes[i])
			}
		})
	}
}
