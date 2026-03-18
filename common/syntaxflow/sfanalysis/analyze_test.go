package sfanalysis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeBlankRuleProfiles(t *testing.T) {
	t.Run("editor allows blank rule", func(t *testing.T) {
		report := Analyze(context.Background(), " \n\t", DefaultOptions(ProfileEditor))
		require.True(t, report.IsBlank)
		require.Empty(t, report.SyntaxErrors)
		require.Nil(t, report.Quality)
	})

	t.Run("ai syntax allows blank rule without sample", func(t *testing.T) {
		report := Analyze(context.Background(), " \n\t", DefaultOptions(ProfileAISyntax))
		require.True(t, report.IsBlank)
		require.Empty(t, report.SyntaxErrors)
		require.Nil(t, report.Sample)
	})

	t.Run("ai syntax rejects blank rule during sample verify", func(t *testing.T) {
		opts := DefaultOptions(ProfileAISyntax)
		opts.VerifySampleCode = true
		opts.SampleCode = "package main\nfunc main() {}"
		opts.SampleLanguage = "golang"

		report := Analyze(context.Background(), " \n\t", opts)
		require.True(t, report.IsBlank)
		require.NotNil(t, report.Sample)
		require.False(t, report.Sample.Matched)
		require.Equal(t, ProblemTypeBlankRule, report.Sample.Error)
	})

	t.Run("quality rejects blank rule", func(t *testing.T) {
		report := Analyze(context.Background(), " \n\t", DefaultOptions(ProfileQuality))
		require.True(t, report.IsBlank)
		require.NotNil(t, report.Quality)
		require.Equal(t, MinScore, report.Quality.Score)
		require.Len(t, report.Quality.Problems, 5)
		require.Equal(t, ProblemTypeLackDescriptionField, report.Quality.Problems[0].Type)
		require.Equal(t, Warning, report.Quality.Problems[0].Severity)
		require.Equal(t, ProblemTypeLackSolutionField, report.Quality.Problems[1].Type)
		require.Equal(t, Warning, report.Quality.Problems[1].Severity)
		require.Equal(t, ProblemTypeMissingPositiveTestData, report.Quality.Problems[2].Type)
		require.Equal(t, Warning, report.Quality.Problems[2].Severity)
		require.Equal(t, ProblemTypeMissingNegativeTestData, report.Quality.Problems[3].Type)
		require.Equal(t, Warning, report.Quality.Problems[3].Severity)
		require.Equal(t, ProblemTypeMissingAlert, report.Quality.Problems[4].Type)
		require.Equal(t, Warning, report.Quality.Problems[4].Severity)
	})
}
