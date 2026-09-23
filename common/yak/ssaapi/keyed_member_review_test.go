package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveKeyedMembersDoesNotIncludePrefixSiblings(t *testing.T) {
	prog, err := Parse(`obj = {"foo": "safe", "foo-other": "tainted", "foo-destructor": "data"}`)
	require.NoError(t, err)
	objects := prog.Ref("obj")
	require.Len(t, objects, 1)
	obj := objects[0]
	for _, pair := range obj.GetMembers() {
		if text, ok := constKeyText(pair[0]); ok && text == "foo" {
			matched := resolveKeyedMembers(obj, pair[0])
			require.Len(t, matched, 1, "a field name prefix is not a dataflow relationship")
			require.Equal(t, pair[1].GetId(), matched[0].GetId())
			return
		}
	}
	t.Fatal("foo member not found")
}
