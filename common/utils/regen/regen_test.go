package regen

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func Test_ExampleGenerate(t *testing.T) {
	pattern := `(abc|bcd){2}`
	results, _ := Generate(pattern)

	fmt.Printf("%#v\n", results)
}

func Test_GenerateOne(t *testing.T) {
	pattern := `(?:ATGPlatform/([\d.]+))?`
	result, _ := GenerateOne(pattern)
	spew.Dump(result)
	result, _ = GenerateVisibleOne(pattern)
	spew.Dump(result)
}

func BenchmarkGenerate(b *testing.B) {
	pattern := `\w{3}`
	for i := 0; i < b.N; i++ {
		Generate(pattern)
	}
}

func TestGenerateVisibleOne(t *testing.T) {
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
			var err error
			for _, pattern := range tt.args.patterns {
				got, e := GenerateVisibleOne(pattern)
				t.Logf("pattern: %s, got: [%s]", pattern, got)
				if e != nil {
					err = e
				}
				gotAll = append(gotAll, got)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateVisibleOne() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for i, g := range gotAll {
				re := regexp.MustCompile(tt.wantRes[i])
				if !re.MatchString(g) {
					t.Errorf("GenerateVisibleOne() got = %v, which doesn't match the pattern %v", g, tt.wantRes[i])
				}
			}
		})
	}
}
