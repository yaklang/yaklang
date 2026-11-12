package ssa2source_test

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
)

//go:embed testcases/*.json
var testcases embed.FS

type sourceCase struct {
	Name      string   `json:"name"`
	Note      string   `json:"note"`
	SSAIR     []string `json:"ssair"`
	Source    []string `json:"source"`
	Forbidden []string `json:"forbidden"`
}

func loadSourceCases(t *testing.T) []sourceCase {
	t.Helper()
	entries, err := testcases.ReadDir("testcases")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sourceCase
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := testcases.ReadFile("testcases/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var single sourceCase
		if err := json.Unmarshal(data, &single); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		if single.Name == "" {
			single.Name = strings.TrimSuffix(entry.Name(), ".json")
		}
		cases = append(cases, single)
	}
	return cases
}

func TestSSAIRToSourceFixtures(t *testing.T) {
	cases := loadSourceCases(t)
	if len(cases) == 0 {
		t.Fatal("no testcases")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			program, err := ssairtext.Parse(strings.Join(tc.SSAIR, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			text, err := ssa2source.Render(context.Background(), program)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range tc.Source {
				if !strings.Contains(text, fragment) {
					t.Fatalf("source lacks %q:\n%s", fragment, text)
				}
			}
			for _, fragment := range tc.Forbidden {
				if strings.Contains(text, fragment) {
					t.Fatalf("source keeps %q:\n%s", fragment, text)
				}
			}
		})
	}
}

func TestRenderRejectsNilAndCancellation(t *testing.T) {
	if _, err := ssa2source.Render(context.Background(), nil); err == nil {
		t.Fatal("nil program accepted")
	}
	program, err := ssairtext.Parse("sub_0x1000\ntype: () -> void\nentry_0:\n\tret")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ssa2source.Render(ctx, program); err == nil {
		t.Fatal("cancellation ignored")
	}
}
