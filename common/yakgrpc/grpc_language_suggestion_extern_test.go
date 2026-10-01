package yakgrpc

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func languageSuggestionExternRefs(prog *ssaapi.Program) map[string][]int64 {
	refs := make(map[string][]int64, len(prog.Program.ExternInstance)+1)
	for name := range prog.Program.ExternInstance {
		refs[name] = nil
		for _, value := range prog.Ref(name) {
			refs[name] = append(refs[name], value.GetId())
		}
		sort.Slice(refs[name], func(i, j int) bool { return refs[name][i] < refs[name][j] })
	}
	for _, value := range prog.Ref("marker") {
		refs["marker"] = append(refs["marker"], value.GetId())
	}
	return refs
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionExternSharedProgramImmutable(t *testing.T) {
	cases := []struct {
		name    string
		parse   func() (*ssaapi.Program, error)
		witness string
	}{
		{
			name: "default yak externs",
			parse: func() (*ssaapi.Program, error) {
				return static_analyzer.SSAParse("marker = 1\nprintln(marker)", "yak")
			},
			witness: "println",
		},
		{
			name: "mitm map extern",
			parse: func() (*ssaapi.Program, error) {
				return static_analyzer.SSAParse("marker = 1\nprintln(marker)", "mitm")
			},
			witness: "println",
		},
		{
			name: "function parameters and scalar types",
			parse: func() (*ssaapi.Program, error) {
				return ssaapi.Parse("marker = 1\ncompletionAuditFunction(\"x\", true)", ssaapi.WithExternValue(map[string]any{
					"completionAuditNumber":   7,
					"completionAuditText":     "value",
					"completionAuditBool":     true,
					"completionAuditBytes":    []byte("value"),
					"completionAuditFunction": func(string, bool) string { return "" },
					"completionAuditVariadic": func(string, ...int) {},
					"completionAuditCallback": func(func(string, []byte) bool) {},
					"completionAuditNil":      nil,
					"$completionAuditHidden":  "internal",
				}))
			},
			witness: "completionAuditFunction",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := tc.parse()
			require.NoError(t, err)
			witnesses := prog.Ref(tc.witness)
			require.Len(t, witnesses, 1)
			hoverBefore := OnHover(prog, tc.witness, false, witnesses[0].GetRange(), witnesses[0])
			require.Len(t, hoverBefore, 1)
			require.Contains(t, hoverBefore[0].Label, tc.witness)
			hoverSnapshot := proto.Clone(hoverBefore[0]).(*ypb.SuggestionDescription)
			instructionCount := prog.Program.Cache.CountInstruction()
			refsBefore := languageSuggestionExternRefs(prog)
			expectedFilter := map[string]struct{}{"localOnly": {}}
			for name := range prog.Program.ExternInstance {
				if !strings.HasPrefix(name, "$") {
					expectedFilter[name] = struct{}{}
				}
			}
			filtered := map[string]struct{}{"localOnly": {}}
			first := completionExternValues(prog, filtered)
			require.Equal(t, expectedFilter, filtered, "the caller's existing filters and all public extern filters must survive")
			require.Equal(t, instructionCount, prog.Program.Cache.CountInstruction(), "even the first completion must not register instructions in the analyzed program")
			require.Equal(t, refsBefore, languageSuggestionExternRefs(prog), "completion must not add extern symbols to the source indexes")
			expected := make(map[string]*ypb.SuggestionDescription, len(first))
			for _, item := range first {
				require.NotNil(t, item)
				require.Nil(t, expected[item.Label], "extern completion must not duplicate a label")
				require.False(t, strings.HasPrefix(item.Label, "$"))
				expected[item.Label] = proto.Clone(item).(*ypb.SuggestionDescription)
			}
			if tc.name == "function parameters and scalar types" {
				for label, description := range map[string]string{
					"completionAuditNumber": "number", "completionAuditText": "string",
					"completionAuditBool": "bool", "completionAuditBytes": "[]byte",
				} {
					require.True(t, proto.Equal(&ypb.SuggestionDescription{
						Label: label, Description: description, InsertText: label, Kind: CompletionKindVariable,
					}, expected[label]), "extern scalar type changed: %s", label)
				}
				for label, snippet := range map[string]string{
					"completionAuditFunction": "completionAuditFunction(${1:string}, ${2:boolean})",
					"completionAuditVariadic": "completionAuditVariadic(${1:string}, ${2:...int})",
					"completionAuditCallback": "completionAuditCallback(${1:func(string, []uint8) bool})",
				} {
					require.True(t, proto.Equal(&ypb.SuggestionDescription{
						Label: label, InsertText: snippet, Kind: CompletionKindFunction,
					}, expected[label]), "extern function parameters changed: %s", label)
				}
				require.NotContains(t, expected, "completionAuditNil")
				require.NotContains(t, expected, "$completionAuditHidden")
			} else {
				printlnDoc := doc.GetDefaultDocumentHelper().Functions["println"]
				require.NotNil(t, printlnDoc)
				require.True(t, proto.Equal(&ypb.SuggestionDescription{
					Label: "println", Description: printlnDoc.Document,
					InsertText: printlnDoc.VSCodeSnippets, Kind: CompletionKindFunction,
				}, expected["println"]), "documented extern completion must retain its authoritative snippet and documentation")
				if tc.name == "mitm map extern" {
					require.True(t, proto.Equal(&ypb.SuggestionDescription{
						Label: "MITM_PARAMS", Description: "map[string]string",
						InsertText: "MITM_PARAMS", Kind: CompletionKindVariable,
					}, expected["MITM_PARAMS"]), "MITM_PARAMS must retain its map type")
				}
			}

			const workers = 32
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(workers)
			errors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				go func() {
					defer wg.Done()
					<-start
					for repeat := 0; repeat < 4; repeat++ {
						filter := map[string]struct{}{"localOnly": {}}
						items := completionExternValues(prog, filter)
						if len(items) != len(expected) || len(filter) != len(expectedFilter) {
							errors <- fmt.Errorf("concurrent completion changed suggestion or filter count")
							return
						}
						for name := range expectedFilter {
							if _, ok := filter[name]; !ok {
								errors <- fmt.Errorf("concurrent completion omitted filter %s", name)
								return
							}
						}
						seen := make(map[string]bool, len(items))
						for _, item := range items {
							if item == nil || seen[item.Label] || !proto.Equal(expected[item.Label], item) {
								errors <- fmt.Errorf("concurrent completion changed a field or duplicated a suggestion")
								return
							}
							seen[item.Label] = true
						}
						if len(items) > 0 {
							items[0].InsertText = "one request's snippet"
						}
					}
				}()
			}
			close(start)
			wg.Wait()
			close(errors)
			for err := range errors {
				require.NoError(t, err)
			}
			require.Equal(t, instructionCount, prog.Program.Cache.CountInstruction())
			require.Equal(t, refsBefore, languageSuggestionExternRefs(prog))
			hoverAfter := OnHover(prog, tc.witness, false, witnesses[0].GetRange(), witnesses[0])
			require.Len(t, hoverAfter, 1)
			require.True(t, proto.Equal(hoverSnapshot, hoverAfter[0]), "completion must not change hover signatures or documentation")
			for _, item := range first {
				require.True(t, proto.Equal(expected[item.Label], item), "one concurrent request changed another request's suggestion")
			}
		})
	}
}
