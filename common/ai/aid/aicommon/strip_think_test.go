package aicommon

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripThinkTags(t *testing.T) {
	in := "<think>reasoning here</think>\n{\"@action\":\"finish\"}"
	out := StripThinkTags(in)
	require.Equal(t, "{\"@action\":\"finish\"}", out)
}

func TestStripThinkTagsReader(t *testing.T) {
	raw := "<think>abc</think>\n{\"@action\":\"read_file\",\"file\":\"/a\"}"
	r := NewStripThinkTagsReader(strings.NewReader(raw))
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "{\"@action\":\"read_file\",\"file\":\"/a\"}", string(got))
}

func TestStripThinkTagsReader_Chunked(t *testing.T) {
	raw := "<think>long reason</think>{\"@action\":\"finish\"}"
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < len(raw); i++ {
			_, _ = pw.Write([]byte{raw[i]})
		}
	}()
	got, err := io.ReadAll(NewStripThinkTagsReader(pr))
	require.NoError(t, err)
	require.Equal(t, "{\"@action\":\"finish\"}", string(got))
}
