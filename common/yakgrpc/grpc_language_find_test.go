package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GrpcRangeToRangeIf(r *ypb.Range) memedit.RangeIf {
	return memedit.NewRange(
		memedit.NewPosition(int(r.StartLine), int(r.StartColumn)), memedit.NewPosition(int(r.EndLine), int(r.EndColumn)),
	)
}

func newRangeFromText(text string) *memedit.Range {
	splited := strings.Split(text, " ")
	if len(splited) != 2 {
		return nil
	}
	start := strings.Split(splited[0], ":")
	if len(start) != 2 {
		return nil
	}
	pos1 := memedit.NewPosition(codec.Atoi(start[0]), codec.Atoi(start[1]))

	end := strings.Split(splited[1], ":")
	if len(end) != 2 {
		return nil
	}
	pos2 := memedit.NewPosition(codec.Atoi(end[0]), codec.Atoi(end[1]))

	return memedit.NewRange(pos1, pos2)
}

func getFind(local ypb.YakClient, typ, pluginType string, t *testing.T, code string, Range *ypb.Range, id string) *ypb.YaklangLanguageFindResponse {
	ret, err := local.YaklangLanguageFind(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   typ,
		YakScriptType: pluginType,
		YakScriptCode: code,
		Range:         Range,
		ModelID:       id,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ret
}

func getFindReferences(local ypb.YakClient, pluginType string, t *testing.T, code string, Range *ypb.Range, id string) *ypb.YaklangLanguageFindResponse {
	return getFind(local, "reference", pluginType, t, code, Range, id)
}

func getFindDefinition(local ypb.YakClient, pluginType string, t *testing.T, code string, Range *ypb.Range, id string) *ypb.YaklangLanguageFindResponse {
	return getFind(local, "definition", pluginType, t, code, Range, id)
}

func checkDefinition(t *testing.T, local ypb.YakClient, sourceCode, pluginType string, selectRange, wantRange memedit.RangeIf) {
	t.Helper()

	editor := memedit.NewMemEditor(sourceCode)
	defer editor.Release()

	rsp := getFindDefinition(local, pluginType, t, sourceCode, RangeIfToGrpcRange(selectRange), "")

	require.NotNil(t, rsp)
	require.Len(t, rsp.Ranges, 1)
	require.Equal(t, memedit.RangeIf(wantRange), GrpcRangeToRangeIf(rsp.Ranges[0]))
}

func checkReferences(t *testing.T, local ypb.YakClient, sourceCode, pluginType string, selectRange memedit.RangeIf, wantRanges []memedit.RangeIf) {
	t.Helper()

	editor := memedit.NewMemEditor(sourceCode)
	defer editor.Release()

	rsp := getFindReferences(local, pluginType, t, sourceCode, RangeIfToGrpcRange(selectRange), "")

	require.NotNil(t, rsp)
	require.Len(t, rsp.Ranges, len(wantRanges))
	for i, wantRange := range wantRanges {
		require.Equal(t, wantRange, GrpcRangeToRangeIf(rsp.Ranges[i]))
	}
}

func TestGRPCMUSTPASS_LANGUAGE_Find_Definition(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("variable", func(t *testing.T) {
		code := `var message = "Hello World."
message`
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("2:1 2:8"),
			newRangeFromText("1:5 1:12"),
		)
	})

	t.Run("function", func(t *testing.T) {
		code := `func main() {}
main()`
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("2:1 2:4"),
			newRangeFromText("1:6 1:10"),
		)
	})

	t.Run("member call", func(t *testing.T) {
		code := `var m = {}
m.a = 1
m.a`

		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("3:1 3:4"),
			newRangeFromText("2:1 2:4"),
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_Find_References(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("variable", func(t *testing.T) {
		code := `var a = 1
a`
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("2:1 2:2"),
			[]memedit.RangeIf{
				newRangeFromText("1:5 1:6"),
				newRangeFromText("2:1 2:2"),
			},
		)
	})

	t.Run("member call ", func(t *testing.T) {
		code := `var m = {}
m.a = 1
println(m.a)`
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("3:9 3:11"),
			[]memedit.RangeIf{
				newRangeFromText("2:1 2:4"),
				newRangeFromText("3:9 3:12"),
			},
		)
	})

	t.Run("function", func(t *testing.T) {
		code := `func Error() {
	return ""
}
a = Error()`
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("4:5 4:9"),
			[]memedit.RangeIf{
				newRangeFromText("1:6 1:11"),
				newRangeFromText("4:5 4:10"),
			},
		)
	})

	// 	t.Run("stdlib", func(t *testing.T) {
	// 		code := `ssa.Parse("")
	// ssa.Parse("")`
	// 		checkReferences(t,
	// 			local,
	// 			code,
	// 			"yak",
	// 			newRangeFromText("1:0 1:3"),
	// 			[]memedit.RangeIf{
	// 				newRangeFromText("1:0 1:3"),
	// 				newRangeFromText("2:0 2:3"),
	// 			},
	// 		)
	// 	})

	t.Run("stdlib function", func(t *testing.T) {
		code := `ssa.Parse("")
ssa.Parse("")`

		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("1:1 1:9"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:10"),
				newRangeFromText("2:1 2:10"),
			},
		)
	})
}
