package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeIrCodeForPersist_truncatesOversizedText(t *testing.T) {
	ir := &IrCode{
		String: strings.Repeat("x", maxSQLiteBindTextBytes+1024),
	}
	SanitizeIrCodeForPersist(ir)
	require.LessOrEqual(t, len(ir.String), maxSQLiteBindTextBytes)
	require.True(t, strings.HasSuffix(ir.String, irCodeTruncatedSuffix))
}

func TestSanitizeIrCodeForPersist_capsInt64Slice(t *testing.T) {
	items := make(Int64Slice, maxIrCodeSliceEntries+100)
	for i := range items {
		items[i] = int64(i)
	}
	ir := &IrCode{FormalArgs: items}
	SanitizeIrCodeForPersist(ir)
	require.Len(t, ir.FormalArgs, maxIrCodeSliceEntries)
}

func TestSanitizeIrCodeForPersist_nilSafe(t *testing.T) {
	require.NotPanics(t, func() {
		SanitizeIrCodeForPersist(nil)
	})
}
