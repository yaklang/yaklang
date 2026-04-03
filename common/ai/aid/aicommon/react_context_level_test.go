package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewGradientPromptFallback_UsesProfilesSequentially(t *testing.T) {
	base := GetModelContextProfile(ModelContextLevelStandard)
	profiles := BuildGradientModelContextProfiles(base)
	require.NotEmpty(t, profiles)

	var usedLevels []string
	fallback := NewGradientPromptFallback(base, func(profile ModelContextProfile) (string, error) {
		usedLevels = append(usedLevels, profile.Level)
		return profile.Level, nil
	})
	require.NotNil(t, fallback)

	for idx, profile := range profiles {
		got, err := fallback(100, 200, idx)
		require.NoErrorf(t, err, "fallback should succeed at level %d", idx)
		require.Equal(t, profile.Level, got)
	}

	_, err := fallback(100, 200, len(profiles))
	require.ErrorIs(t, err, ErrPromptFallbackNoMoreProfiles)
	require.Len(t, usedLevels, len(profiles))
}
