package aiknowledge

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeBusinessErrorCodes(t *testing.T) {
	_, opts, _, _ := fixture(t)
	create(t, opts, "docs")
	for _, test := range []struct {
		name string
		code string
		call func() error
	}{
		{"missing scope", "INVALID_ARGUMENT", func() error { _, err := Query(nil, "", 512, opts...); return err }},
		{"missing base", "NOT_FOUND", func() error { _, err := Query("missing", "", 512, opts...); return err }},
		{"missing entry", "NOT_FOUND", func() error {
			_, err := Query("docs", "", 512, append(opts, WithMode("get"), WithEntryID("missing"))...)
			return err
		}},
		{"get missing base", "NOT_FOUND", func() error { _, err := Get("missing", opts...); return err }},
		{"duplicate base", "ALREADY_EXISTS", func() error { _, err := Add("docs", opts...); return err }},
		{"empty entry", "INVALID_ARGUMENT", func() error { _, err := AddEntry("docs", "", "", opts...); return err }},
		{"invalid build", "INVALID_ARGUMENT", func() error { _, err := Build("docs", "x", append(opts, WithBuildMode("invalid"))...); return err }},
		{"cancelled", "CANCELLED", func() error { return fmt.Errorf("wrapped: %w", context.Canceled) }},
		{"deadline", "DEADLINE_EXCEEDED", func() error { return context.DeadlineExceeded }},
		{"execution failure", "OPERATION_FAILED", func() error { return fmt.Errorf("embedding unavailable") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			require.Error(t, err)
			result := ErrorResult(err)
			require.Equal(t, false, result["success"])
			require.Equal(t, test.code, result["error_code"])
			require.Equal(t, err.Error(), result["message"])
		})
	}
	require.Nil(t, ErrorResult(nil))
}
