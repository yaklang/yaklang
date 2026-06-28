package javaclassparser

import (
	"os"
	"testing"
)

// TestRuntimeInvisibleAnnotationsRoundTrip locks Bug AF: RuntimeInvisibleAnnotations
// (RetentionPolicy.CLASS) shares the RuntimeVisibleAnnotationsAttribute struct, so re-marshalling a
// parsed class used to hard-code the "RuntimeVisibleAnnotations" attribute-name index, silently
// flipping invisible annotations to RUNTIME-visible (retention widened, bytes diverged). The seed
// `testdata/invisible_anno.class` carries a single class-level @ClassRetained(CLASS) annotation.
// After parse -> Bytes() -> re-parse the annotation must still be flagged invisible.
func TestRuntimeInvisibleAnnotationsRoundTrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/invisible_anno.class")
	if err != nil {
		t.Fatalf("read seed class failed: %v", err)
	}

	obj, err := NewClassParser(raw).Parse()
	if err != nil {
		t.Fatalf("parse seed class failed: %v", err)
	}
	if got := countInvisibleAnnoAttrs(obj.Attributes); got != 1 {
		t.Fatalf("expected 1 invisible-annotation attribute after initial parse, got %d", got)
	}
	if got := countVisibleAnnoAttrs(obj.Attributes); got != 0 {
		t.Fatalf("seed must not carry RUNTIME-visible annotations, got %d", got)
	}

	// Re-marshal and re-parse; the invisible flag must survive (i.e. the correct attribute-name
	// index was written back, not RuntimeVisibleAnnotations).
	reparsed, err := NewClassParser(obj.Bytes()).Parse()
	if err != nil {
		t.Fatalf("re-parse marshalled class failed: %v", err)
	}
	if got := countInvisibleAnnoAttrs(reparsed.Attributes); got != 1 {
		t.Fatalf("invisible annotation lost across marshal round-trip (Bug AF): got %d invisible attrs", got)
	}
	if got := countVisibleAnnoAttrs(reparsed.Attributes); got != 0 {
		t.Fatalf("invisible annotation was re-marshalled as RUNTIME-visible (Bug AF): got %d visible attrs", got)
	}
}

func countInvisibleAnnoAttrs(attrs []AttributeInfo) int {
	n := 0
	for _, a := range attrs {
		if anno, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok && anno.IsInvisible {
			n++
		}
	}
	return n
}

func countVisibleAnnoAttrs(attrs []AttributeInfo) int {
	n := 0
	for _, a := range attrs {
		if anno, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok && !anno.IsInvisible {
			n++
		}
	}
	return n
}
