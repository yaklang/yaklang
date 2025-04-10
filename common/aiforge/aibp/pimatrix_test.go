package aibp

import (
	"testing"
)

func TestPIMatrix(t *testing.T) {
	forge := NewPIMatrixForge()
	forge.GenerateFirstPromptWithMemoryOption()
}
