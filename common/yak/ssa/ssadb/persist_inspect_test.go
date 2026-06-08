package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMeasureLargeIrCodeFields(t *testing.T) {
	ir := &IrCode{
		CodeID:           42,
		OpcodeName:       "Function",
		ExtraInformation: strings.Repeat("x", LargeIrCodeFieldSampleBytes+1),
	}
	fields := MeasureLargeIrCodeFields(ir, LargeIrCodeFieldSampleBytes)
	require.Len(t, fields, 1)
	require.Equal(t, "ExtraInformation", fields[0].Name)
}

func TestLogLargeIrCodeFieldsSample(t *testing.T) {
	ir := &IrCode{
		CodeID:     99,
		OpcodeName: "BasicBlock",
		String:     strings.Repeat("y", LargeIrCodeFieldSampleBytes+1),
	}
	require.True(t, LogLargeIrCodeFieldsSample(ir, "prog"))
}

func TestPrepareIrCodeForPersist_samplingBudget(t *testing.T) {
	oversized := strings.Repeat("z", LargeIrCodeFieldSampleBytes+1)
	var logged int
	for i := 0; i < maxLargeFieldLogsPerBatch+2; i++ {
		PrepareIrCodeForPersist(&IrCode{
			CodeID:     int64(i),
			OpcodeName: "Const",
			String:     oversized,
		}, "prog", &logged)
	}
	require.Equal(t, maxLargeFieldLogsPerBatch, logged)
}
