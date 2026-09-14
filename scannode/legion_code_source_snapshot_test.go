package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegionCodeSourceSnapshotRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		rejected      bool
	}{
		{"exact bytes", "package main\r\n// 中文\r\n\tfunc main() {}\n", false},
		{"limit", strings.Repeat("a", maxInlineCodeSourceSnapshotBytes), false},
		{"oversize", strings.Repeat("a", maxInlineCodeSourceSnapshotBytes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			publisher := &recordingAIFocusRiskPublisher{}
			rawSink, err := newLegionAIFocusResultSink(publisher, "snapshot-bind", validCodeAuditResultContext())
			if err != nil {
				t.Fatal(err)
			}
			if err := rawSink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(strings.Repeat("a", 40), strings.Repeat("b", 64)); err != nil {
				t.Fatal(err)
			}
			runtime := &legionServerFocusRuntime{ctx: context.Background(), sink: rawSink.(aiFocusAssetResultSink), workspace: &legionCodeWorkspaceRuntime{root: root, spec: validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)}}
			activateTestLegionCodeWorkspaceFocusTurn(t, runtime)
			finding := validCodeFinding()
			finding.File, finding.StartLine, finding.EndLine = "main.go", 1, 1
			raw, _ := json.Marshal(finding)
			params := map[string]any{}
			if err := json.Unmarshal(raw, &params); err != nil {
				t.Fatal(err)
			}
			params["source_snapshot"] = map[string]any{"path": "main.go", "content": "model fabricated source", "sha256": strings.Repeat("f", 64)}
			_, err = runtime.Execute(serverFocusCapabilitySubmitCodeFinding, params)
			if tc.rejected {
				if err == nil || !strings.Contains(err.Error(), "64 KiB") || len(publisher.risks) != 0 {
					t.Fatalf("oversize file must reject finding before publication: err=%v risks=%d", err, len(publisher.risks))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(publisher.risks) != 1 {
				t.Fatal("finding not published")
			}
			var published aiFocusCodeFinding
			if err := json.Unmarshal(publisher.risks[0].raw, &published); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%x", sha256.Sum256([]byte(tc.content)))
			if published.SourceSnapshot == nil || published.SourceSnapshot.Path != "main.go" || published.SourceSnapshot.Content != tc.content || published.SourceSnapshot.SHA256 != want {
				t.Fatalf("source bytes or digest changed: %#v", published.SourceSnapshot)
			}
		})
	}
}

func TestLegionCodeSourceSnapshotSafety(t *testing.T) {
	root := t.TempDir()
	w := &legionCodeWorkspaceRuntime{root: root}
	for _, content := range []string{"\xff\n", "hello\x00world"} {
		if err := os.WriteFile(filepath.Join(root, "binary"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := w.captureSourceSnapshot("binary")
		if err == nil || snapshot != nil || !strings.Contains(err.Error(), "UTF-8") {
			t.Fatalf("non-text should reject snapshot: %#v %v", snapshot, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "source"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", "/etc/passwd", "missing", "link", "outside/secret", "."} {
		if snapshot, err := w.captureSourceSnapshot(path); err == nil {
			t.Fatalf("unsafe path %q accepted: %#v", path, snapshot)
		}
	}
}

func TestLegionCodeSourceSnapshotSinkValidation(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "snapshot-validation", validCodeAuditResultContext())
	if err != nil {
		t.Fatal(err)
	}
	if err := rawSink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(strings.Repeat("a", 40), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	sink := rawSink.(aiFocusCodeResultSink)
	for _, tc := range []struct{ name, path, content, hash string }{
		{"mismatched path", "other.go", "text", fmt.Sprintf("%x", sha256.Sum256([]byte("text")))},
		{"traversal", "../main.go", "text", ""},
		{"digest", "src/main.go", "text", strings.Repeat("a", 64)},
		{"size", "src/main.go", strings.Repeat("a", maxInlineCodeSourceSnapshotBytes+1), ""},
		{"nul", "src/main.go", "\x00", ""},
		{"utf8", "src/main.go", "\xff", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finding := validCodeFinding()
			finding.SourceSnapshot = &aiFocusCodeSourceSnapshot{Path: tc.path, Content: tc.content, SHA256: tc.hash}
			if _, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", finding); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
	if len(publisher.risks) != 0 {
		t.Fatal("invalid snapshot was published")
	}
	if _, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", validCodeFinding()); err != nil {
		t.Fatalf("historical finding rejected: %v", err)
	}
}
