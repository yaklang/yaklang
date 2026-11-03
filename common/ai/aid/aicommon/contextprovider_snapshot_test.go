package aicommon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextProviderSnapshot_EmptyChangeAndRecovery(t *testing.T) {
	manager := NewContextProviderManager()
	var content string
	var providerErr error
	manager.RegisterTracedContent("snapshot", func(AICallerConfigIf, *Emitter, string) (string, error) {
		return content, providerErr
	})
	for _, state := range []struct {
		content string
		err     error
	}{
		{"", nil}, {" \n\t", nil}, {"BEFORE_CONTENT", nil},
		{"AFTER_CONTENT", nil}, {"", nil}, {"", errors.New("read failed")},
		{"RECOVERED_CONTENT", nil}, {"", nil},
	} {
		content, providerErr = state.content, state.err
		for _, render := range []func() string{
			func() string { return manager.ExecuteWithNonce(nil, nil, "helper") },
			func() string { return manager.ExecuteMainLoopWithNonce(nil, nil, "main") },
		} {
			got := render()
			require.NotContains(t, got, "CHANGES_DIFF_")
			if providerErr != nil {
				require.Contains(t, got, "read failed", "current failures remain visible")
			} else if content == "" || content == " \n\t" {
				require.Empty(t, got, "empty providers must omit their tags as well as their bodies")
			} else {
				require.Contains(t, got, content)
				if content != "BEFORE_CONTENT" {
					require.NotContains(t, got, "BEFORE_CONTENT")
				}
				require.NotContains(t, got, "read failed", "recovery must not repeat stale errors")
			}
		}
	}
}
