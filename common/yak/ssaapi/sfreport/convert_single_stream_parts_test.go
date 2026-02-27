package sfreport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertSingleResultToSSAResultParts_NilResult(t *testing.T) {
	parts, err := ConvertSingleResultToSSAResultParts(nil, StreamPartsOptions{})
	require.NoError(t, err)
	assert.Nil(t, parts)
}

func TestConvertSingleResultToSSAResultPartsJSON_NilResult(t *testing.T) {
	raw, stats, err := ConvertSingleResultToSSAResultPartsJSON(nil, StreamPartsOptions{})
	require.NoError(t, err)
	assert.Empty(t, raw)
	require.NotNil(t, stats)
	assert.Equal(t, false, stats["has_payload"])
}

func TestNewStreamPartsOptions_Defaults(t *testing.T) {
	opts := NewStreamPartsOptions()
	assert.Equal(t, IRifyFullReportType, opts.ReportType)
	assert.True(t, opts.ShowDataflowPath)
	assert.True(t, opts.ShowFileContent)
	assert.True(t, opts.WithFile)
}

func TestDedupStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"b", "a", "b", "c", "a"}, []string{"a", "b", "c"}},
		{"with_spaces", []string{" a ", "a", " b"}, []string{"a", "b"}},
		{"with_empty", []string{"a", "", "  ", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupStrings(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

