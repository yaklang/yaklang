package linkprep

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareForLink_emptyManifest_noop(t *testing.T) {
	td := t.TempDir()
	in := []string{filepath.Join(td, "a.a"), filepath.Join(td, "b.a")}
	out, cleanup, err := PrepareForLink(PrepareInput{
		Archives: append([]string{}, in...),
		Manifest: nil,
		WorkDir:  td,
	})
	require.NoError(t, err)
	require.Equal(t, in, out)
	cleanup()
}
