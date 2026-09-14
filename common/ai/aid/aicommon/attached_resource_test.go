package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttachedResourceHasType(t *testing.T) {
	require.True(t, NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, "{}").HasType(AttachedResourceTypeSelected))
	require.True(t, NewAttachedResource(" SELECTED ", "", "").HasType(AttachedResourceTypeSelected))

	httpFlow := NewAttachedResource("http_flow", "", `{"ids":[1]}`)
	require.True(t, httpFlow.HasType(AttachedResourceTypeHTTPFlowID, "httpflowid", "http_flow"))
	require.False(t, httpFlow.HasType(AttachedResourceTypeSelected))
}

func TestAttachedResourceHasKey(t *testing.T) {
	res := NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/a.yak")
	require.True(t, res.HasKey(CONTEXT_PROVIDER_KEY_FILE_PATH))
	require.False(t, res.HasKey(CONTEXT_PROVIDER_KEY_DIRECTORY_PATH))
}

func TestNonEmptyAttachedResourcesPreservesMalformedValues(t *testing.T) {
	malformed := NewAttachedResource("http_flow", "id", "not-an-id")
	valid := NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/a.yak")
	input := []*AttachedResource{nil, NewAttachedResource("http_flow", "id", " \t\n"), malformed, valid}
	require.Equal(t, []*AttachedResource{malformed, valid}, NonEmptyAttachedResources(input))
	require.Len(t, input, 4, "filter must not mutate the caller's attachments")
	_, err := ParseAttachedResourceData(malformed)
	require.Error(t, err, "malformed nonempty attachments must still be validated")
}
