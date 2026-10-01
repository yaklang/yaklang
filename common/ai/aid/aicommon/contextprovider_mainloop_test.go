package aicommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextProviderMainLoopAttachmentDiffDoesNotEchoHelperQuery(t *testing.T) {
	const query = "HELPER_QUERY_MUST_NOT_RETURN_VIA_DIFF"
	path := filepath.Join(t.TempDir(), "changing.txt")
	require.NoError(t, os.WriteFile(path, []byte("BEFORE_FILE_CONTENT"), 0o600))
	manager := NewContextProviderManager()
	manager.RegisterTracedContent("file", FileContextProvider(path, query))
	for _, updated := range []bool{false, true} {
		if updated {
			require.NoError(t, os.WriteFile(path, []byte("AFTER_FILE_CONTENT"), 0o600))
		}
		// Rendering another context must not change this context's diff baseline.
		helper := manager.ExecuteWithNonce(nil, nil, "helper")
		require.Contains(t, helper, "User Prompt: "+query)
		main := manager.ExecuteMainLoopWithNonce(nil, nil, "main")
		require.NotContains(t, main, query)
		if updated {
			for _, rendered := range []string{helper, main} {
				require.Contains(t, rendered, "CHANGES_DIFF_")
				require.Contains(t, rendered, "BEFORE_FILE_CONTENT")
				require.Contains(t, rendered, "AFTER_FILE_CONTENT")
			}
		}
		require.NotContains(t, manager.ExecuteWithNonce(nil, nil, "helper"), "CHANGES_DIFF_")
		repeated := manager.ExecuteMainLoopWithNonce(nil, nil, "main")
		require.NotContains(t, repeated, query)
		require.NotContains(t, repeated, "CHANGES_DIFF_")
	}
}
