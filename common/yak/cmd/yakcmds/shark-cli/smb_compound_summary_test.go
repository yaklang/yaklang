package sharkcli

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func smb2CompoundTestUTF16(s string) []byte {
	out := make([]byte, len(s)*2)
	for i := range s {
		out[i*2] = s[i]
	}
	return out
}

func TestSMB2CompoundSummaryAggregatesEveryCommand(t *testing.T) {
	path := smb2CompoundTestUTF16(`\\srv\share`)
	tcBody := make([]byte, 8+len(path))
	binary.LittleEndian.PutUint16(tcBody[0:], 9)
	binary.LittleEndian.PutUint16(tcBody[4:], 72)
	binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
	copy(tcBody[8:], path)
	tree := smbDetailTestHeader(3, 0, 1, 0, 0, tcBody)
	for len(tree)%8 != 0 {
		tree = append(tree, 0)
	}
	binary.LittleEndian.PutUint32(tree[20:24], uint32(len(tree)))

	name := smb2CompoundTestUTF16("file.txt")
	createBody := make([]byte, 56+len(name))
	binary.LittleEndian.PutUint16(createBody[0:], 57)
	binary.LittleEndian.PutUint32(createBody[4:], 2)
	binary.LittleEndian.PutUint32(createBody[24:], 0x00120189)
	binary.LittleEndian.PutUint32(createBody[36:], 1)
	binary.LittleEndian.PutUint16(createBody[44:], 120)
	binary.LittleEndian.PutUint16(createBody[46:], uint16(len(name)))
	copy(createBody[56:], name)
	create := smbDetailTestHeader(5, 0, 2, 0, 0, createBody)

	raw := append(append([]byte(nil), tree...), create...)
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	result := session.Feed(0, time.Unix(1, 0), smbDetailTestDirect(raw))
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 1)
	event := result.Events[0]
	require.Equal(t, true, event.Session["Compound"])
	require.Equal(t, "compound", event.Session["Operation"])
	require.Contains(t, event.Session["Compound Operations"], "share.connect")
	require.Contains(t, event.Session["Compound Operations"], "file.open")
	require.Contains(t, event.Session["Observed Behaviors"], "share-connect")
	require.Contains(t, event.Session["Observed Behaviors"], "file-open")

	summary := applicationEventSummary(event)
	require.Contains(t, summary, "SMB2 compound · 2 commands")
	require.Contains(t, summary, "TREE_CONNECT")
	require.Contains(t, summary, "CREATE")
	require.Contains(t, summary, "share-connect")
	require.Contains(t, summary, "file-open")
}
