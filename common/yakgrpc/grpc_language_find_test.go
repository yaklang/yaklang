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

func checkDefinition(t *testing.T, local ypb.YakClient, sourceCode, pluginType string, selectRange memedit.RangeIf, wantRanges ...memedit.RangeIf) {
	t.Helper()

	rsp := getFindDefinition(local, pluginType, t, sourceCode, RangeIfToGrpcRange(selectRange), "")

	require.NotNil(t, rsp)
	require.Len(t, rsp.Ranges, len(wantRanges))
	for i, wantRange := range wantRanges {
		require.Equal(t, wantRange, GrpcRangeToRangeIf(rsp.Ranges[i]))
	}
}

func checkReferences(t *testing.T, local ypb.YakClient, sourceCode, pluginType string, selectRange memedit.RangeIf, wantRanges []memedit.RangeIf) {
	t.Helper()

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
			newRangeFromText("2:1 2:1"),
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

	t.Run("return value", func(t *testing.T) {
		code := `func Error() {
err = ""
return err
}
`
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("2:1 2:3"),
			[]memedit.RangeIf{
				newRangeFromText("2:1 2:4"),
				newRangeFromText("3:8 3:11"),
			},
		)
	})

	t.Run("standard library function", func(t *testing.T) {
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

	t.Run("standard function", func(t *testing.T) {
		code := `println(1)
println(2)`

		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("1:1 1:7"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:8"),
				newRangeFromText("2:1 2:8"),
			},
		)
	})

	t.Run("standard function in different function", func(t *testing.T) {
		code := `println(1)
println(2)
func a() {
println(3)
}
func b() {
println(4)
}`

		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("1:1 1:7"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:8"),
				newRangeFromText("2:1 2:8"),
				newRangeFromText("4:1 4:8"),
				newRangeFromText("7:1 7:8"),
			},
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_Find_Phi(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	code := `a = 1
println(a)
if c {
a = 2
println(a)
}
println(a)`

	t.Run("def-1", func(t *testing.T) {
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("1:1 1:1"),
			newRangeFromText("1:1 1:2"),
		)
	})
	t.Run("def-2", func(t *testing.T) {
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("4:1 4:1"),
			newRangeFromText("4:1 4:2"),
		)
	})
	t.Run("def-phi", func(t *testing.T) {
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("7:9 7:9"),
			newRangeFromText("1:1 1:2"),
			newRangeFromText("4:1 4:2"),
		)
	})
	t.Run("use-1", func(t *testing.T) {
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("2:9 2:9"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:2"),
				newRangeFromText("2:9 2:10"),
				newRangeFromText("7:9 7:10"),
			},
		)
	})
	t.Run("use-2", func(t *testing.T) {
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("5:9 5:9"),
			[]memedit.RangeIf{
				newRangeFromText("4:1 4:2"),
				newRangeFromText("5:9 5:10"),
				newRangeFromText("7:9 7:10"),
			},
		)
	})
	t.Run("use-phi", func(t *testing.T) {
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("7:9 7:9"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:2"),
				newRangeFromText("2:9 2:10"),
				newRangeFromText("4:1 4:2"),
				newRangeFromText("5:9 5:10"),
				newRangeFromText("7:9 7:10"),
			},
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_Find_FreeValue(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	code := `a = 1
b = func() {
c = a + 1
d = a + 2
return a
}
e = func() {
q = a + 1
w = a + 2
return a
}
`

	t.Run("def", func(t *testing.T) {
		checkDefinition(t,
			local,
			code,
			"yak",
			newRangeFromText("5:8 5:9"),
			newRangeFromText("1:1 1:2"),
		)
	})

	t.Run("ref", func(t *testing.T) {
		checkReferences(t,
			local,
			code,
			"yak",
			newRangeFromText("5:8 5:9"),
			[]memedit.RangeIf{
				newRangeFromText("1:1 1:2"),
				newRangeFromText("3:5 3:6"),
				newRangeFromText("4:5 4:6"),
				newRangeFromText("5:8 5:9"),
				newRangeFromText("8:5 8:6"),
				newRangeFromText("9:5 9:6"),
				newRangeFromText("10:8 10:9"),
			},
		)
	})
}
