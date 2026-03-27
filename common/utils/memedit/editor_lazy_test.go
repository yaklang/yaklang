package memedit

import "testing"

func TestMemEditorLineMappingsLazyRebuild(t *testing.T) {
	editor := NewMemEditor("hello\nworld")
	if editor.lineMappingsReady {
		t.Fatalf("line mappings should start lazy")
	}
	if editor.lineLensMap != nil || editor.lineStartOffsetMap != nil {
		t.Fatalf("line mappings should not be allocated before use")
	}

	if got := editor.GetLineCount(); got != 2 {
		t.Fatalf("GetLineCount() = %d, want 2", got)
	}
	if !editor.lineMappingsReady {
		t.Fatalf("line mappings should be ready after first access")
	}

	if err := editor.InsertAtOffset(0, "x"); err != nil {
		t.Fatalf("InsertAtOffset() failed: %v", err)
	}
	if editor.lineMappingsReady {
		t.Fatalf("line mappings should be invalidated after source mutation")
	}

	if got, err := editor.GetOffsetByPositionWithError(2, 3); err != nil || got != 9 {
		t.Fatalf("GetOffsetByPositionWithError() = (%d, %v), want (9, nil)", got, err)
	}
	if !editor.lineMappingsReady {
		t.Fatalf("line mappings should rebuild on demand after mutation")
	}
}
