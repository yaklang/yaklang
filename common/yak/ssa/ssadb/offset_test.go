package ssadb

import "testing"

func TestIrOffsetGetStartAndEndPositions_EmptyFileHash(t *testing.T) {
	t.Parallel()

	offset := &IrOffset{FileHash: ""}
	editor, start, end, err := offset.GetStartAndEndPositions()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if editor != nil || start != nil || end != nil {
		t.Fatalf("expected nil editor/start/end for empty file hash")
	}
}

func TestIrOffsetGetStartAndEndPositions_WhitespaceFileHash(t *testing.T) {
	t.Parallel()

	offset := &IrOffset{FileHash: "   "}
	editor, start, end, err := offset.GetStartAndEndPositions()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if editor != nil || start != nil || end != nil {
		t.Fatalf("expected nil editor/start/end for whitespace file hash")
	}
}

func TestIrOffsetGetStartAndEndPositions_NilReceiver(t *testing.T) {
	t.Parallel()

	var offset *IrOffset
	editor, start, end, err := offset.GetStartAndEndPositions()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if editor != nil || start != nil || end != nil {
		t.Fatalf("expected nil editor/start/end for nil receiver")
	}
}
