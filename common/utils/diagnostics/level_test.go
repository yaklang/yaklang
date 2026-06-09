package diagnostics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelHighBlocksLowAndNormal(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	// minimum tier = high → trace/measure are below threshold and should not record.
	SetLevel(LevelHigh)

	require.NoError(t, Track("low-tier", func() error { return nil }))
	require.NoError(t, TrackLow("trace-tier", func() error { return nil }))
	require.Empty(t, DefaultRecorder().Snapshot(), "LevelNormal/LevelLow must not record when floor is LevelHigh")
}

func TestLevelHighRecordsHighTier(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	SetLevel(LevelHigh)
	require.NoError(t, TrackHigh("critical-only", func() error { return nil }))

	snaps := DefaultRecorder().Snapshot()
	require.Len(t, snaps, 1)
	require.Equal(t, "critical-only", snaps[0].Name)
	require.Equal(t, uint64(1), snaps[0].Count)
}

func TestLevelLowRecordsTraceAndMeasure(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	SetLevel(LevelLow)
	require.NoError(t, Track("measure-allowed", func() error { return nil }))
	require.NoError(t, TrackLow("trace-allowed", func() error { return nil }))

	names := map[string]bool{}
	for _, m := range DefaultRecorder().Snapshot() {
		names[m.Name] = true
	}
	require.True(t, names["measure-allowed"])
	require.True(t, names["trace-allowed"])
}

func TestLevelOffBlocksRecordingButRunsSteps(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	SetLevel(LevelOff)
	ran := false
	require.NoError(t, Track("off-tier", func() error {
		ran = true
		return nil
	}))
	require.True(t, ran)
	require.Empty(t, DefaultRecorder().Snapshot())
}

func TestEnabledTierOrdering(t *testing.T) {
	orig := GetLevel()
	defer SetLevel(orig)

	SetLevel(LevelNormal)
	require.False(t, Enabled(LevelLow), "trace tier below measure floor")
	require.True(t, Enabled(LevelNormal))
	require.True(t, Enabled(LevelHigh))

	SetLevel(LevelHigh)
	require.False(t, Enabled(LevelLow))
	require.False(t, Enabled(LevelNormal))
	require.True(t, Enabled(LevelHigh))
}
