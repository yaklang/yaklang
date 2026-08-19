package ssa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFullTypeNameTruncationObservable verifies that hitting the
// maxFullTypeNameEntries cap is recorded instead of being fully silent
// (review B5).
func TestFullTypeNameTruncationObservable(t *testing.T) {
	before := fullTypeNameTruncationCount()
	ft := NewFunctionType("", nil, nil, false)
	names := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		names = append(names, fmt.Sprintf("name-%d", i))
	}
	ft.SetFullTypeNames(names)
	require.Equal(t, maxFullTypeNameEntries, len(ft.GetFullTypeNames()))
	require.GreaterOrEqual(t, fullTypeNameTruncationCount(), before+1,
		"truncating the list must be observable through the counter")
}
