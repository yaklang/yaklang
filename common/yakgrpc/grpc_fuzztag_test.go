package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGetAllFuzztagInfo(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	tags, err := client.GetAllFuzztagInfo(context.Background(), &ypb.GetAllFuzztagInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(tags)
}

func TestGenerateFuzztag(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	for i, testCase := range []struct {
		textRange *ypb.Range
		expect    string
		typ       string
	}{
		{
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 13,
				EndLine:     1,
				EndColumn:   13,
			},
			expect: "{{int(1-5)}}{{base64()}}",
		},
		{
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 1,
				EndLine:     1,
				EndColumn:   1,
			},
			expect: "{{base64()}}{{int(1-5)}}",
		},
		{
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 10,
				EndLine:     1,
				EndColumn:   12,
			},
			expect: "{{int(1-5{{base64()}}}",
		},
		{
			typ: "wrap",
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 10,
				EndLine:     1,
				EndColumn:   12,
			},
			expect: "{{int(1-5{{base64()})}}}",
		},
		{
			typ: "wrap",
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 1,
				EndLine:     1,
				EndColumn:   13,
			},
			expect: "{{base64({{int(1-5)}})}}",
		},
		{
			typ: "wrap",
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 12,
				EndLine:     1,
				EndColumn:   12,
			},
			expect: "{{int(1-5)}{{base64()}}}",
		},
		{
			typ: "wrap",
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 14,
				EndLine:     1,
				EndColumn:   14,
			},
			expect: "{{int(1-5)}}{{base64()}}",
		},
		{
			typ: "wrap",
			textRange: &ypb.Range{
				Code:        "{{int(1-5)}}",
				StartLine:   -1,
				StartColumn: 14,
				EndLine:     -1,
				EndColumn:   14,
			},
			expect: "{{base64()}}{{int(1-5)}}",
		},
		{
			textRange: &ypb.Range{
				Code:        "哈哈{{int(1-5)}}",
				StartLine:   1,
				StartColumn: 2,
				EndLine:     1,
				EndColumn:   2,
			},
			expect: "哈{{base64()}}哈{{int(1-5)}}",
		},
	} {
		t.Run(fmt.Sprintf("test%d", i), func(t *testing.T) {
			typ := testCase.typ
			if typ == "" {
				typ = "insert"
			}
			res, err := client.GenerateFuzztag(context.Background(), &ypb.GenerateFuzztagRequest{
				Name:  "base64",
				Type:  typ,
				Range: testCase.textRange,
			})
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, testCase.expect, res.Result)
		})
	}
}
