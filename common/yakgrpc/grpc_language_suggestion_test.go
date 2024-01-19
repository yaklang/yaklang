package yakgrpc

import (
	"context"
	"strings"
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
		want := []string{"Program", "Ref"}
		if !utils.StringSliceContainsAll(got, want...) {
			t.Fatalf("want %v, but got %v", want, got)
		}
	})

	t.Run("check anyonmous field struct completion", func(t *testing.T) {
		res := getCompletion(t, `
rsp, err = http.Request("GET", "https://baidu.com")
rsp.
		`, &ypb.Range{
			Code:        "rsp.",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   4,
		})
		got := lo.Map(res.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.Label
		})
		log.Info("got: ", got)
		want := []string{"Response", "Body", "Status", "Data"}
		if !utils.StringSliceContainsAll(got, want...) {
			t.Fatalf("want %v, but got %v", want, got)
		}
	})
}

var local ypb.YakClient = nil

func CheckHover(t *testing.T) func(t *testing.T, code, typ string, Range *ypb.Range, want string, subStr ...bool) {
	if local == nil {
		var err error
		local, err = NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
	}

	getHover := func(t *testing.T, code, typ string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "hover", typ, t, code, Range)
	}
	check := func(t *testing.T, code, typ string, Range *ypb.Range, want string, sub ...bool) {
		subStr := false
		for _, v := range sub {
			if v {
				subStr = true
				break
			}
		}

		req := getHover(t, code, typ, Range)
		log.Info(req.SuggestionMessage)
		if len(req.SuggestionMessage) != 1 {
			t.Fatal("should get 1 suggestion")
		}
		got := req.SuggestionMessage[0].Label
		if subStr {
			if !strings.Contains(got, want) {
				t.Fatalf("want %s, but get %s", want, got)
			}
		} else {
			if got != want {
				t.Fatalf("want %s, but get %s", want, got)
			}
		}
	}
	return check
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Basic(t *testing.T) {
	check := CheckHover(t)

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
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_ExternLib(t *testing.T) {
	check := CheckHover(t)
	code := `
prog  = ssa.Parse(
    "", 
    ssa.withLanguage(
        ssa.Javascript
    )
)~
prog.Packages
`

	t.Run("check extern lib hover", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "ssa",
			StartLine:   2,
			StartColumn: 8,
			EndLine:     2,
			EndColumn:   11,
		}
		want := getExternLibDesc("ssa", "any")
		check(t, code, "yak", ssaRange, want)
	})

	t.Run("check extern lib method hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "ssa.Parse",
			StartLine:   2,
			StartColumn: 8,
			EndLine:     2,
			EndColumn:   17,
		}
		// 标准库函数
		funcDecl := getFuncDeclByName("ssa.Parse")
		desc := getFuncDeclDesc(funcDecl, "Parse")
		want := desc
		check(t, code, "yak", ssaParseRange, want)
	})

	t.Run("check extern lib instance hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "ssa.Javascript",
			StartLine:   5,
			StartColumn: 8,
			EndLine:     5,
			EndColumn:   22,
		}
		// 标准库变量
		instance := getInstanceByName("ssa.Javascript")
		desc := getConstInstanceDesc(instance)
		want := desc
		check(t, code, "yak", ssaParseRange, want)
	})
	t.Run("check extern lib method return hover", func(t *testing.T) {
		progRange := &ypb.Range{
			Code:        "prog",
			StartLine:   2,
			StartColumn: 0,
			EndLine:     2,
			EndColumn:   4,
		}
		want := `func (Program) Ref(name string) Value`
		check(t, code, "yak", progRange, want, true)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_ExternLib_InLoop(t *testing.T) {
	check := CheckHover(t)
	code := `for a {
prog = ssa.Parse(
	"", 
	ssa.withLanguage(
		ssa.Javascript
	)
)~
}`
	t.Run("check extern lib hover", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "ssa",
			StartLine:   2,
			StartColumn: 8,
			EndLine:     2,
			EndColumn:   11,
		}
		want := getExternLibDesc("ssa", "any")
		check(t, code, "yak", ssaRange, want)
	})

	t.Run("check extern lib method hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "ssa.Parse",
			StartLine:   2,
			StartColumn: 8,
			EndLine:     2,
			EndColumn:   17,
		}
		// 标准库函数
		funcDecl := getFuncDeclByName("ssa.Parse")
		desc := getFuncDeclDesc(funcDecl, "Parse")
		want := desc
		check(t, code, "yak", ssaParseRange, want)
	})

	t.Run("check extern lib instance hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "ssa.Javascript",
			StartLine:   5,
			StartColumn: 8,
			EndLine:     5,
			EndColumn:   22,
		}
		// 标准库变量
		instance := getInstanceByName("ssa.Javascript")
		desc := getConstInstanceDesc(instance)
		want := desc
		check(t, code, "yak", ssaParseRange, want)
	})
	t.Run("check extern lib method return hover", func(t *testing.T) {
		progRange := &ypb.Range{
			Code:        "prog",
			StartLine:   2,
			StartColumn: 0,
			EndLine:     2,
			EndColumn:   4,
		}
		want := `func (Program) Ref(name string) Value`
		check(t, code, "yak", progRange, want, true)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_StructMemberAndMethod(t *testing.T) {
	check := CheckHover(t)
	code := `rsp, err = http.Request("GET", "https://baidu.com")
rsp.Status
rsp.Data()`
	t.Run("check member hover", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "rsp.Status",
			StartLine:   2,
			StartColumn: 0,
			EndLine:     2,
			EndColumn:   10,
		}
		want := "```go\n" + `field Status string` + "\n```"
		check(t, code, "yak", ssaRange, want)
	})

	t.Run("check method hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "rsp.Data",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   8,
		}
		// 标准库函数
		want := "```go\n" + `func Data() string` + "\n```"
		check(t, code, "yak", ssaParseRange, want)
	})
}
