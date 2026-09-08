//go:build linux

package scannode

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestManagedInputSearchContinuation(t *testing.T) {
	content := "needle first\nneedle second\nneedle third\n"
	command := managedInputBindFixture(t, "log_analysis", content)
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
		t.Fatal(err)
	}
	defer driver.bindings[0].InputWorkspace.Cleanup()
	runtime := driver.bindings[0].LegionResultRuntime.(*legionServerFocusRuntime)
	if err := runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, inputExecutionContract("source.search")); err != nil {
		t.Fatal(err)
	}
	path := command.InputManifest.Resources[0].RelativePath
	for _, capability := range []string{"source.search", "input.search"} {
		t.Run(capability, func(t *testing.T) {
			offset := int64(0)
			for i, want := range []string{"first", "second", "third"} {
				result, err := runtime.Execute(capability, map[string]any{"path": path, "query": "needle", "limit": 1, "offset": offset})
				if err != nil {
					t.Fatal(err)
				}
				key := "matches"
				if capability == "source.search" {
					key = "results"
				}
				matches, ok := result[key].([]map[string]any)
				if !ok || len(matches) != 1 || !strings.Contains(fmt.Sprint(matches[0]["content"]), want) || matches[0]["byte_offset"] != offset {
					t.Fatalf("page %d repeated or lost an original offset: %#v", i, result)
				}
				if i == 2 {
					if result["complete"] != true || result["truncated"] != false {
						t.Fatalf("last page incomplete: %#v", result)
					}
					break
				}
				next, ok := result["next_offset"].(int64)
				if !ok || next <= offset || result["next_path"] != path || result["complete"] != false {
					t.Fatalf("non-progressing continuation: %#v", result)
				}
				offset = next
			}
		})
	}
	for _, params := range []map[string]any{
		{"path": path, "offset": -1},
		{"path": path, "offset": len(content) + 1},
		{"path": "inputs", "offset": 1},
	} {
		params["query"] = "needle"
		if _, err := runtime.Execute("source.search", params); err == nil {
			t.Fatalf("invalid cursor accepted: %v", params)
		}
	}
}

func TestManagedInputSearchByteBudget(t *testing.T) {
	const budget = 64 << 10
	needle := "needle-across-page"
	content := strings.Repeat("x", budget-3) + needle + "\n" + strings.Repeat("x", budget)
	command := managedInputBindFixture(t, "log_analysis", content)
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, content) }))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
		t.Fatal(err)
	}
	defer driver.bindings[0].InputWorkspace.Cleanup()
	runtime := driver.bindings[0].LegionResultRuntime.(*legionServerFocusRuntime)
	if err := runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, inputExecutionContract("source.search")); err != nil {
		t.Fatal(err)
	}
	path := command.InputManifest.Resources[0].RelativePath
	first, err := runtime.Execute("source.search", map[string]any{"path": path, "query": needle, "case_sensitive": true, "max_scan_bytes": budget})
	if err != nil || first["complete"] != false || first["count"] != 0 || first["scanned_bytes"] != int64(budget) {
		t.Fatalf("scan budget ignored: %#v %v", first, err)
	}
	cursor := first["next_offset"].(int64)
	if cursor <= 0 || cursor > budget-3 {
		t.Fatalf("cross-page query overlap lost: %v", first)
	}
	second, err := runtime.Execute("source.search", map[string]any{"path": path, "query": needle, "case_sensitive": true, "offset": cursor, "max_scan_bytes": budget})
	if err != nil {
		t.Fatal(err)
	}
	matches := second["results"].([]map[string]any)
	if len(matches) != 1 || matches[0]["byte_offset"] != int64(budget-3) {
		t.Fatalf("cross-page match lost: %v", second)
	}
}
