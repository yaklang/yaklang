package yakgrpc

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
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
		if !utils.StringSliceContainsAll(got, want...) {
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

	t.Run("check mitm hover", func(t *testing.T) {
		test := assert.New(t)
		res := getHover(t, `
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
			})
		log.Info(res.SuggestionMessage)
		test.Equal(1, len(res.SuggestionMessage))
		test.Equal("```go\nfunc modify(r1 github.com/yaklang/yaklang/common/yakgrpc/yakit.HTTPFlow) null\n```", res.SuggestionMessage[0].Label)
	})
}
