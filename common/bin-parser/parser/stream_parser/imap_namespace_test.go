package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIMAPNamespaceFieldsGroupsExtensionsAndBoundaries(t *testing.T) {
	for _, wire := range []string{
		"* NAMESPACE ((\"\" \"/\")) NIL NIL\r\n",
		"* NAMESPACE NIL ((\"~\" NIL)) ((\"#shared/\" \"/\")(\"#other.\" \".\" \"X-AUDIT\" (\"one\" \"two\")))\r\n",
		"* NAMESPACE (({6}\r\nINBOX. \".\")) NIL NIL\r\n",
		"* namespace ((\"a\\\"/\" \"/\")) nil nil\r\n",
	} {
		fields, info, err := decodeIMAPFields([]byte(wire), "response")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(wire))
		require.Equal(t, "NAMESPACE", info["Response Name"])
		require.Equal(t, false, info["Mailbox Names Decoded"])
		for _, group := range []string{"Personal Namespaces", "Other Users Namespaces", "Shared Namespaces"} {
			require.Contains(t, info, group)
		}
		for cut := 0; cut < len(wire); cut++ {
			_, _, err := decodeIMAPFields([]byte(wire[:cut]), "response")
			require.Error(t, err, "prefix %d", cut)
		}
	}
	_, info, err := decodeIMAPFields([]byte("* NAMESPACE ((\"a\\\"/\" \"/\")) NIL NIL\r\n"), "response")
	require.NoError(t, err)
	entry := info["Personal Namespaces"].(map[string]any)["Entries"].([]map[string]any)[0]
	require.Equal(t, []byte("a\"/"), entry["Prefix"])
	wire := []byte("* NAMESPACE ((\"a\\\"/\" \"/\")) NIL NIL\r\n")
	var encoded []byte
	for _, span := range entry["Prefix Encoded Byte Ranges"].([][2]int) {
		encoded = append(encoded, wire[span[0]:span[1]]...)
	}
	require.Equal(t, entry["Prefix"], encoded)
	for _, bad := range []string{
		"* NAMESPACE () NIL NIL\r\n", "* NAMESPACE NIL NIL\r\n", "* NAMESPACE NIL NIL NIL NIL\r\n",
		"* NAMESPACE ((NIL \"/\")) NIL NIL\r\n", "* NAMESPACE ((\"\" \"ab\")) NIL NIL\r\n",
		"* NAMESPACE ((\"\" \"/\" \"X-AUDIT\" ())) NIL NIL\r\n",
		"* NAMESPACE ((\"\" \"/\") (\"other\" \"/\")) NIL NIL\r\n",
	} {
		_, _, err := decodeIMAPFields([]byte(bad), "response")
		require.Error(t, err, bad)
	}
	for _, command := range []string{"NAMESPACE", "IDLE"} {
		for _, tag := range []string{"a", "a]"} {
			_, _, err := decodeIMAPFields([]byte(tag+" "+command+"\r\n"), "command")
			require.NoError(t, err)
		}
		_, _, err := decodeIMAPFields([]byte("a "+command+" extra\r\n"), "command")
		require.Error(t, err)
	}
}

func FuzzIMAPNamespaceFields(f *testing.F) {
	f.Add([]byte("* NAMESPACE ((\"\" \"/\")) NIL NIL\r\n"))
	f.Add([]byte("* NAMESPACE NIL ((\"~\" NIL)) ((\"#shared/\" \"/\" \"X-AUDIT\" (\"one\")))\r\n"))
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 64<<10 || !bytes.HasPrefix(bytes.ToUpper(wire), []byte("* NAMESPACE ")) {
			return
		}
		fields, info, err := decodeIMAPFields(wire, "response")
		if err != nil {
			require.Nil(t, fields)
			require.Nil(t, info)
			return
		}
		tlsCertificateTestCoverage(t, fields, len(wire))
		require.Equal(t, "NAMESPACE", info["Response Name"])
		require.Equal(t, false, info["Session State Validated"])
	})
}
