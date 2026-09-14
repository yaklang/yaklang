//go:build linux

package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestManagedInputReadEvidenceSurvivesLaterActions(t *testing.T) {
	content := "observed-first-event\n" + strings.Repeat("ordinary heartbeat日志\n", 1000) + "observed-last-event"
	command := managedInputBindFixture(t, "non_log_evidence_task", content)
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, content) }))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
		t.Fatal(err)
	}
	defer driver.bindings[0].InputWorkspace.Cleanup()
	runtime := driver.bindings[0].LegionResultRuntime.(*legionServerFocusRuntime)
	cfg := &aicommon.Config{SessionPromptState: aicommon.NewSessionPromptState()}
	opts, err := managedInputTools(runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, inputExecutionContract("source.read", "source.list")); err != nil {
		t.Fatal(err)
	}
	path := command.InputManifest.Resources[0].RelativePath
	for _, capability := range []string{"source.read", "input.read"} {
		if _, err := runtime.Execute(capability, map[string]any{"path": path, "max_bytes": len(content)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.Execute("input.list", map[string]any{"path": "inputs"}); err != nil {
		t.Fatal(err)
	}
	saved := cfg.GetSessionEvidenceRendered()
	for _, want := range []string{"observed-first-event", "observed-last-event", path, command.InputManifest.ManifestId, "untrusted", "omitted"} {
		if !strings.Contains(saved, want) {
			t.Fatalf("read evidence unavailable after later action: missing %q", want)
		}
	}
	if len(saved) > 11000 {
		t.Fatalf("evidence is not bounded: %d bytes", len(saved))
	}
	if strings.Count(saved, "[id:") != 1 {
		t.Fatal("legacy and generic reads duplicated the same observation")
	}
	store := aicommon.UnmarshalEvidenceStore(cfg.GetSessionPromptState().GetSessionEvidence())
	var observed struct {
		Segments []struct {
			Offset  int    `json:"offset"`
			Content string `json:"content"`
		} `json:"segments"`
	}
	encoded := store.Items[0].Content[strings.IndexByte(store.Items[0].Content, '{'):]
	if err := json.Unmarshal([]byte(encoded), &observed); err != nil {
		t.Fatal(err)
	}
	kept := 0
	for _, segment := range observed.Segments {
		if !utf8.ValidString(segment.Content) || segment.Content != content[segment.Offset:segment.Offset+len(segment.Content)] {
			t.Fatal("excerpt corrupted original bytes or offsets")
		}
		kept += len(segment.Content)
	}
	if kept > 8192 {
		t.Fatal("excerpt exceeded raw byte budget")
	}
	if _, err := runtime.Execute("input.read", map[string]any{"path": "../private"}); err == nil {
		t.Fatal("unsafe read succeeded")
	}
	if cfg.GetSessionEvidenceRendered() != saved {
		t.Fatal("failed read changed evidence")
	}
}
