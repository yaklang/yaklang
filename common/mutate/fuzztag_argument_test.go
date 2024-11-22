package mutate

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseBuildinTagArgumentTypes(t *testing.T) {
	tags := GetAllFuzztags()
	for _, tag := range tags {
		if tag.ArgumentDescription == "" {
			//println(tag.TagName, "no argument")
			continue
		}
		_, err := ParseFuzztagArgumentTypes(tag.ArgumentDescription)
		if err != nil {
			t.Fatal(err)
		}
		testTags, err := GenerateExampleTags(tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, testTag := range testTags {
			renderRes := MutateQuick(testTag)
			if len(renderRes) == 0 || renderRes[0] == testTag {
				t.Fatalf("tag %s render failed", tag.TagName)
			}
		}

	}
}
func TestParseFuzztagArgumentTypes(t *testing.T) {
	tests := []struct {
		name   string
		args   string
		expect string
	}{
		{
			name:   "test all type",
			args:   `{{string(abc:字符串)}}{{enum({{string(low:低)}}{{string(high:高)}}:low:风险等级)}}`,
			expect: `{{Name: string, Default: abc, Description: 字符串 Separator: [,] IsOptional: false IsList: false}},{{Name: Enum, Default: low, Description: 风险等级 Select: {{Name: string, Default: low, Description: 低 Separator: [,] IsOptional: false IsList: false}},{{Name: string, Default: high, Description: 高 Separator: [,] IsOptional: false IsList: false}} IsOptional: false IsList: false}}`,
		},
		{
			name:   "test sep",
			args:   `{{string_split(abc:字符串)}}{{number_contact(1:开始)}}{{number(10:结束)}}`,
			expect: `{{Name: string, Default: abc, Description: 字符串 Separator: [|] IsOptional: false IsList: false}},{{Name: number, Default: 1, Description: 开始 Separator: [-] IsOptional: false IsList: false}},{{Name: number, Default: 10, Description: 结束 Separator: [,] IsOptional: false IsList: false}}`,
		},
		{
			name:   "test optional type",
			args:   `{{optional(string(abc:可选字符串))}}`,
			expect: `{{Name: string, Default: abc, Description: 可选字符串 Separator: [,] IsOptional: true IsList: false}}`,
		},
		{
			name:   "test list type",
			args:   `{{list(string(abc:可选字符串))}}`,
			expect: `{{Name: string, Default: abc, Description: 可选字符串 Separator: [,] IsOptional: false IsList: true}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFuzztagArgumentTypes(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tt.expect, DumpFuzztagArgumentTypes(got))
		})
	}
}
