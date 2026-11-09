package core

import (
	"context"
	"testing"
)

func TestArtifactParseRecoveredFunctions(t *testing.T) {
	artifact := testArtifact(t)
	artifact.Image.Functions = []FunctionBoundary{
		{Address: 0x1000, Size: 4, Name: "first", Source: BoundarySymbol},
		{Address: 0x1004, Size: 4, Name: "second", Source: BoundaryEHFrame},
	}
	artifact.Parser = &stubParser{available: true}
	result, err := artifact.Parse(context.Background(), Region{Address: 0x1000, Offset: 0x10, Size: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.IR.Functions) != 2 {
		t.Fatalf("functions = %d", len(result.IR.Functions))
	}
	if result.IR.Functions[0].Name != "first" || result.IR.Functions[0].Boundary != BoundarySymbol {
		t.Fatalf("first function = %+v", result.IR.Functions[0])
	}
	if result.IR.Functions[1].Name != "second" || result.IR.Functions[1].Boundary != BoundaryEHFrame {
		t.Fatalf("second function = %+v", result.IR.Functions[1])
	}
}
