package imcontrol

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildFreeInputEventUsesStructuredAttachedResources(t *testing.T) {
	event := buildFreeInputEvent("描述这张图片", []string{"/tmp/a.jpg", " ", "/tmp/b.png"})
	if !event.GetIsFreeInput() {
		t.Fatal("event should be free input")
	}
	if event.GetFreeInput() != "描述这张图片" {
		t.Fatalf("FreeInput = %q", event.GetFreeInput())
	}
	if len(event.GetAttachedFilePath()) != 0 {
		t.Fatalf("AttachedFilePath should stay empty, got %+v", event.GetAttachedFilePath())
	}
	resources := event.GetAttachedResourceInfo()
	if len(resources) != 2 {
		t.Fatalf("AttachedResourceInfo len = %d, want 2", len(resources))
	}
	for i, wantPath := range []string{"/tmp/a.jpg", "/tmp/b.png"} {
		got := resources[i]
		if got.GetType() != aicommon.CONTEXT_PROVIDER_TYPE_FILE {
			t.Fatalf("resource[%d].Type = %q", i, got.GetType())
		}
		if got.GetKey() != aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH {
			t.Fatalf("resource[%d].Key = %q", i, got.GetKey())
		}
		if got.GetValue() != wantPath {
			t.Fatalf("resource[%d].Value = %q, want %q", i, got.GetValue(), wantPath)
		}
	}
}
