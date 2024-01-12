package yakgrpc

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GetSuggestion(local ypb.YakClient, typ, pluginType string, t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
	t.Log("========== get ", typ)
	ret, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   typ,
		YakScriptType: pluginType,
		YakScriptCode: code,
		Range:         Range,
	})
	log.Info(ret)
	if err != nil {
		t.Fatal(err)
	}
	// t.Log(ret)
	return ret
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	getCompletion := func(t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "completion", "yak", t, code, Range)
	}

	t.Run("check basic extern-lib completion", func(t *testing.T) {
		res := getCompletion(t, `
cli.
	`, &ypb.Range{
			Code:        "",
			StartLine:   2,
			StartColumn: 4,
			EndLine:     2,
			EndColumn:   4,
		})
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
		}
	})

	t.Run("check extern struct completion", func(t *testing.T) {
		res := getCompletion(t, `
prog = ssa.Parse("")~
prog.
		`, &ypb.Range{
			Code:        "prog.",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   6,
		})
		got := lo.Map(res.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.Label
		})
		log.Info("got: ", got)
		want := []string{"Programe", "Ref"}
		if utils.StringSliceContainsAll(got, want...) {
			t.Fatalf("want %v, but got %v", want, got)
		}
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	getHover := func(t *testing.T, code, typ string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "hover", typ, t, code, Range)
	}

	check := func(t *testing.T, code, typ string, Range *ypb.Range, want string) {
		req := getHover(t, code, typ, Range)
		log.Info(req.SuggestionMessage)
		if len(req.SuggestionMessage) != 1 {
			t.Fatal("should get 1 suggestion")
		}
		got := req.SuggestionMessage[0].Label
		if got != want {
			t.Fatalf("want %s, but get %s", want, got)
		}

	}

	t.Run("check mitm hover argument", func(t *testing.T) {
		check(t, `
		hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
			responseBytes, _ = codec.StrconvUnquote(flow.Response)
			a = flow.BeforeSave() //error
		}
		`,
			"mitm",
			&ypb.Range{
				Code:        "modify",
				StartLine:   2,
				StartColumn: 56,
				EndLine:     2,
				EndColumn:   62,
			},
			"```go\nfunc modify(r1 yakit.HTTPFlow) null\n```",
		)
	})

	t.Run("check extern lib hover", func(t *testing.T) {
		code := `
ssa.Parse(
    "var i = 0", 
    ssa.withLanguage(
        ssa.Javascript
    )
)
`

		{
			ssaRange := &ypb.Range{
				Code:        "ssa",
				StartLine:   2,
				StartColumn: 0,
				EndLine:     2,
				EndColumn:   3,
			}
			want := getExternLibDesc("ssa", "any")
			check(t, code, "yak", ssaRange, want)
		}
		{
			ssaParseRange := &ypb.Range{
				Code:        "ssa.Parse",
				StartLine:   2,
				StartColumn: 0,
				EndLine:     2,
				EndColumn:   9,
			}
			// 标准库函数
			funcDecl := getFuncDeclByName("ssa.Parse")
			desc := getFuncDeclDesc(funcDecl, "Parse")
			want := desc
			check(t, code, "yak", ssaParseRange, want)
		}
		{
			ssaParseRange := &ypb.Range{
				Code:        "ssa.Javascript",
				StartLine:   5,
				StartColumn: 9,
				EndLine:     5,
				EndColumn:   22,
			}
			// 标准库变量
			instance := getInstanceByName("ssa.Javascript")
			desc := getInstanceDesc(instance)
			want := desc
			check(t, code, "yak", ssaParseRange, want)
		}
	})
}
