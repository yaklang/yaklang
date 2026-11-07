package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMB2BehaviorAndResourceProjection(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")

	ipc := smb2ChainFeed(t, session, 0, smb2TreeConnect(1, 0x11, `\\srv\IPC$`))
	require.Equal(t, "share.connect", ipc["Operation"])
	require.Equal(t, "named-pipe", ipc["Resource Kind"])
	require.Contains(t, ipc["Observed Behaviors"], "named-pipe-access")
	require.Contains(t, ipc["Observed Behaviors"], "rpc-endpoint")

	pipe := smb2ChainFeed(t, session, 0, smb2Create(2, 0x11, 1, `\pipe\svcctl`))
	require.Equal(t, "file.open", pipe["Operation"])
	require.Contains(t, pipe["Observed Behaviors"], "named-pipe-access")
	require.Contains(t, pipe["Observed Behaviors"], "service-control")
	require.Equal(t, `\pipe\svcctl`, pipe["Resource"])

	file := smb2ChainFeed(t, session, 0, smb2Create(3, 0x11, 1, `docs\report.txt`))
	require.Contains(t, file["Observed Behaviors"], "file-open")
	intent, _ := file["Access Intent"].([]string)
	require.Contains(t, intent, "read-data")
	require.Contains(t, intent, "read-attributes")
	require.Contains(t, intent, "synchronize")

	read := smb2ChainFeed(t, session, 1, smb2ReadResp(4, 0x11, 1, []byte("abcd")))
	require.Equal(t, "file.read", read["Operation"])
	require.Contains(t, read["Observed Behaviors"], "file-read")
	require.Equal(t, "STATUS_SUCCESS", read["Status Name"])
}

func TestSMB2QueryInfoBehaviorAndClassName(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	smb2ChainFeed(t, session, 0, smb2QueryRequest(21, 0x11, 1, 1, 5, nil))
	response := smb2QueryResponse(21, 0x11, 1, 0, smb2StandardOutput())
	info := smb2ChainFeed(t, session, 1, response)
	require.Equal(t, "file.query_info", info["Operation"])
	require.Contains(t, info["Observed Behaviors"], "file-metadata-query")
	require.Equal(t, "FileStandardInformation", info["Information Class Name"])

	sd := make([]byte, 20)
	binary.LittleEndian.PutUint16(sd[2:4], 0x8000) // SE_SELF_RELATIVE
	smb2ChainFeed(t, session, 0, smb2QueryRequest(22, 0x11, 1, 3, 0, nil))
	info = smb2ChainFeed(t, session, 1, smb2QueryResponse(22, 0x11, 1, 0, sd))
	require.Contains(t, info["Observed Behaviors"], "security-descriptor-query")
	require.Equal(t, "FileSecurityInformation", info["Information Class Name"])
}

func TestSMB2PendingStatusName(t *testing.T) {
	fid := bytes.Repeat([]byte{0x51}, 16)
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	require.Nil(t, session.Feed(0, time.Unix(1, 0), smb2Read(7, 0x11, 1, fid, 4)).Err)
	interim := smb2AsyncWire(smb2ErrorResponse(8, 7, 0x11, 1, 0x00000103, nil), 0x1234)
	info := smb2ChainFeed(t, session, 1, interim)
	require.Equal(t, "STATUS_PENDING", info["Status Name"])
	require.Equal(t, "file.read", info["Operation"])
}
