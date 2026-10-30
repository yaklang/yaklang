package pcapdb

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCAPDBDcerpcAuthTrailerKeepsFollowingCalls(t *testing.T) {
	capture := filepath.Join("..", "..", "..", "testdata", "enterprise-traffic-analysis", "lateral-movement-psexec", "capture.pcapng")
	m := testManager(t)
	db, err := m.GetOrCreate(capture, WithProtocols(true), WithBatchSize(8))
	require.NoError(t, err)

	messages, err := db.QueryProtocols(QueryProtocol("dcerpc"), QueryWithFields(true), QueryLimit(100))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(messages), 20, "authenticated svcctl calls must become protocol messages")

	var foundBind, foundAuth3, foundRequest, foundResponse bool
	for _, message := range messages {
		require.Empty(t, message.Error, "DCE/RPC authentication must not terminate the stream")
		ptype, ok := testSessionUint(message.Session["PType"])
		if !ok {
			continue
		}
		switch ptype {
		case 11:
			if message.Session["Authenticated"] == true {
				foundBind = true
			}
		case 16:
			foundAuth3 = true
			ntlm, ok := message.Session["NTLMSSP"].(map[string]any)
			require.True(t, ok, "NTLM Type3 identity fields must be indexed")
			require.NotEmpty(t, ntlm["User"])
		case 0:
			if op, ok := testSessionUint(message.Session["OpNum"]); ok && op == 15 {
				foundRequest = true
			}
		case 2:
			if message.Session["Association Status"] == "matched" {
				foundResponse = true
			}
		}
	}
	require.True(t, foundBind, "svcctl bind must be indexed")
	require.True(t, foundAuth3, "NTLM AUTH3 must be indexed")
	require.True(t, foundRequest, "svcctl OpenSCManager request must be indexed")
	require.True(t, foundResponse, "svcctl response must be matched and indexed")

	var sawOpen, sawCreate bool
	for _, message := range messages {
		switch message.Session["Packet Name"] {
		case "svcctl.ROpenSCManagerW":
			sawOpen = true
		case "svcctl.RCreateServiceWOW64W":
			sawCreate = true
		}
		require.NotContains(t, message.Summary, "unknown.op_")
	}
	require.True(t, sawOpen)
	require.True(t, sawCreate)

	smb, err := db.QueryProtocols(QueryProtocol("smb2"), QueryLimit(200))
	require.NoError(t, err)
	var sawAdmin, sawIPC, sawExe, sawPipe, sawRead, sawWrite bool
	for _, message := range smb {
		summary := message.Summary
		sawAdmin = sawAdmin || strings.Contains(summary, `ADMIN$`)
		sawIPC = sawIPC || strings.Contains(summary, `IPC$`)
		sawExe = sawExe || strings.Contains(summary, "PSEXESVC.exe")
		sawPipe = sawPipe || strings.Contains(summary, "PSEXESVC")
		sawRead = sawRead || strings.Contains(summary, "SMB2 READ") && strings.Contains(summary, "len=")
		sawWrite = sawWrite || strings.Contains(summary, "SMB2 WRITE") && strings.Contains(summary, "len=")
		require.NotContains(t, summary, "\\x")
	}
	require.True(t, sawAdmin && sawIPC && sawExe && sawPipe && sawRead && sawWrite)
}

func testSessionUint(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint8:
		return uint64(n), true
	case uint16:
		return uint64(n), true
	case uint32:
		return uint64(n), true
	case uint64:
		return n, true
	case int:
		return uint64(n), true
	case int32:
		return uint64(n), true
	case int64:
		return uint64(n), true
	case uint:
		return uint64(n), true
	case float64:
		return uint64(n), true
	case json.Number:
		value, err := strconv.ParseUint(string(n), 10, 64)
		return value, err == nil
	default:
		return 0, false
	}
}
