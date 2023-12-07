package yakgrpc

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GetSuggestion(local ypb.YakClient, typ string, t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
	t.Log("========== get ", typ)
	ret, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   typ,
		YakScriptType: "yak",
		YakScriptCode: code,
		Range:         Range,
	})
	if err != nil {
		t.Fatal(err)
	}
	// t.Log(ret)
	return ret
}

func TestLanguageSuggestionCompletion(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	getCompletion := func(t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "completion", t, code, Range)
	}

	t.Run("check basic extern-lib completion", func(t *testing.T) {
		res := getCompletion(t, `
cli.
	`, &ypb.Range{
			Code:        "",
			StartLine:   2,
			StartColumn: 4,
			EndLine:     0,
			EndColumn:   0,
		})
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
		}
	})
}
