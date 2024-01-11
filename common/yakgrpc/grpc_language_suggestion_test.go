package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
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
			Code:        "flow",
			StartLine:   2,
			StartColumn: 28,
			EndLine:     2,
			EndColumn:   32,
		})
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
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
