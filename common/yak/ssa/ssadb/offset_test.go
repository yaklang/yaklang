package ssadb

import (
	"errors"
	"testing"
)

func TestIrOffsetGetStartAndEndPositions_EmptyFileHash(t *testing.T) {
	t.Parallel()

	offset := &IrOffset{FileHash: ""}
	editor, start, end, err := offset.GetStartAndEndPositions()
	if !errors.Is(err, ErrSourceRangeAbsent) {
		t.Fatalf("expected absent source range, got editor=%v start=%v end=%v err=%v", editor, start, end, err)
	}
}

func TestIrOffsetGetStartAndEndPositions_WhitespaceFileHash(t *testing.T) {
	t.Parallel()

	offset := &IrOffset{FileHash: "   "}
	editor, start, end, err := offset.GetStartAndEndPositions()
	if !errors.Is(err, ErrSourceRangeAbsent) {
		t.Fatalf("expected absent source range, got editor=%v start=%v end=%v err=%v", editor, start, end, err)
	}
}

func TestIrOffsetGetStartAndEndPositions_NilReceiver(t *testing.T) {
	t.Parallel()

	var offset *IrOffset
	editor, start, end, err := offset.GetStartAndEndPositions()
	if !errors.Is(err, ErrSourceRangeAbsent) {
		t.Fatalf("expected absent source range, got editor=%v start=%v end=%v err=%v", editor, start, end, err)
	}
}
