package pass

import (
	"embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

//go:embed testcases/*.json
var returnCases embed.FS

// returnCase is one pass fixture. The SSAIR is the input and the functions list
// the return type each of them must end up with, so the fixture stays data rather
// than a wall of text inside a test.
type returnCase struct {
	Name      string              `json:"name"`
	Note      string              `json:"note"`
	Pass      string              `json:"pass"`
	VoidCount int                 `json:"void_count"`
	SSAIR     []string            `json:"ssair"`
	Functions []returnExpectation `json:"functions"`
}

type returnExpectation struct {
	Name       string `json:"name"`
	ReturnType string `json:"return_type"`
}

func loadReturnCases(t *testing.T) []returnCase {
	t.Helper()
	entries, err := returnCases.ReadDir("testcases")
	if err != nil {
		t.Fatal(err)
	}
	var cases []returnCase
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := returnCases.ReadFile("testcases/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var loaded []returnCase
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		cases = append(cases, loaded...)
	}
	if len(cases) == 0 {
		t.Fatal("no testcases")
	}
	return cases
}

func TestReturnPassFixtures(t *testing.T) {
	for _, tc := range loadReturnCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			program := parseSSAForPass(t, tc.SSAIR...)
			switch tc.Pass {
			case "mark-void-returns":
				if count := MarkVoidReturns(program); count != tc.VoidCount {
					t.Fatalf("void functions = %d, want %d", count, tc.VoidCount)
				}
			case "refine-return-types":
				RefineReturnTypes(program)
			default:
				t.Fatalf("unknown pass %q", tc.Pass)
			}
			for _, want := range tc.Functions {
				function := findFunctionForPass(t, program, want.Name)
				if got := function.GetType().String(); !strings.Contains(got, "-> "+want.ReturnType) {
					t.Fatalf("%s return type = %s, want -> %s", want.Name, got, want.ReturnType)
				}
				if want.ReturnType != "null" {
					continue
				}
				// A void function keeps no return values.
				for _, blockID := range function.Blocks {
					block, ok := function.GetBasicBlockByID(blockID)
					if !ok || block == nil {
						continue
					}
					for _, instructionID := range block.Insts {
						instruction, ok := function.GetInstructionById(instructionID)
						if !ok || instruction == nil {
							continue
						}
						if ret, ok := instruction.(*ssa.Return); ok && len(ret.Results) != 0 {
							t.Fatalf("%s void return kept %d values", want.Name, len(ret.Results))
						}
					}
				}
			}
		})
	}
}
