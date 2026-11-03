package aicommon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestContextProviderSnapshot_EmptyFileOmitsMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context.txt")
	for _, content := range []string{"", " \n\t"} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		for name, provider := range map[string]ContextProvider{
			"path":   FileContextProvider(path),
			"inline": FileContentContextProvider(content),
		} {
			t.Run(name, func(t *testing.T) {
				manager := NewContextProviderManager()
				manager.RegisterTracedContent("file", provider)
				require.Empty(t, manager.ExecuteWithNonce(nil, nil, "helper"))
				require.Empty(t, manager.ExecuteMainLoopWithNonce(nil, nil, "main"))
			})
		}
	}

	// Preserve a truncated preview even if its first bytes are all whitespace:
	// the unread portion may contain actual data.
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(" ", MaxFileContentSize)+"UNREAD_DATA"), 0600))
	rendered, err := FileContextProvider(path)(nil, nil, "file")
	require.NoError(t, err)
	require.Contains(t, rendered, "File content truncated")
}
