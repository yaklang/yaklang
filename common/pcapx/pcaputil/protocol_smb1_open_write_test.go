package pcaputil

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func smb1LayoutFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	data, err := trafficfixture.ReadFile(filepath.Join(filepath.Dir(file), "smb1layout", name))
	require.NoError(t, err)
	return data
}

func TestSMB1NTCreateAndWriteAndXSourceRecords(t *testing.T) {
	createReq := smb1LayoutFixture(t, "nt-create-request.bin")
	createResp := smb1LayoutFixture(t, "nt-create-response.bin")
	writeReq := smb1LayoutFixture(t, "write-andx-request.bin")
	writeResp := smb1LayoutFixture(t, "write-andx-response.bin")
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	opened := session.Feed(0, time.Unix(1, 0), createReq)
	require.Nil(t, opened.Err)
	body := opened.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, "scan-getinfo.dat", body["File Name"])
	require.Equal(t, uint16(32), body["Name Length"])
	require.Equal(t, uint32(2032127), body["Desired Access"])
	require.Equal(t, byte(255), body["AndX Command"])
	require.Equal(t, []any{uint64(88), uint64(122)}, body["Wire Field Ranges"].(map[string]any)["File Name Raw Bytes"])
	require.NotContains(t, body["Wire Field Ranges"], "File Name")

	created := session.Feed(1, time.Unix(2, 0), createResp)
	require.Nil(t, created.Err)
	response := created.Events[0].Session
	require.Equal(t, "matched observed request; authentication unverified", response["Association Status"])
	require.Equal(t, uint16(16384), response["Body"].(map[string]any)["FID"])
	require.Equal(t, uint32(2), response["Body"].(map[string]any)["Create Action"])
	require.Equal(t, int64(127996387040000000), response["Body"].(map[string]any)["Creation Time"])
	require.Equal(t, byte(0), response["Body"].(map[string]any)["Directory"])
	require.Equal(t, uint16(16384), response["Observed FID"])
	require.Equal(t, "scan-getinfo.dat", response["Request File Name"])

	written := session.Feed(0, time.Unix(3, 0), writeReq)
	require.Nil(t, written.Err)
	writeBody := written.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, []byte("abc\x00\x00\x00\x00"), writeBody["Data"])
	require.Equal(t, uint16(7), writeBody["Data Length Low"])
	require.Equal(t, uint16(0), writeBody["Data Length High"])
	require.Equal(t, uint32(0), writeBody["Offset Low"])
	require.Equal(t, uint32(0), writeBody["Offset High"])
	require.Equal(t, uint16(16384), writeBody["FID"])
	require.NotContains(t, writeBody, "File Name")
	require.Equal(t, []any{uint64(67), uint64(74)}, writeBody["Wire Field Ranges"].(map[string]any)["Data"])
	require.NotContains(t, writeBody["Wire Field Ranges"], "Offset")

	ack := session.Feed(1, time.Unix(4, 0), writeResp)
	require.Nil(t, ack.Err)
	ackBody := ack.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, uint16(7), ackBody["Count Low"])
	require.Equal(t, uint16(0), ackBody["Count High"])
	require.Equal(t, uint16(65535), ackBody["Available"])
	require.NotContains(t, ackBody, "File Name")
	require.NotContains(t, ackBody, "Data")
	require.NotContains(t, ack.Events[0].Session, "Request File Name")

	// Reusing the create MID for a write does not borrow the open filename.
	reused := bytes.Clone(writeReq)
	reused[34] = createReq[34]
	reused[35] = createReq[35]
	again := session.Feed(0, time.Unix(5, 0), reused)
	require.Nil(t, again.Err)
	require.NotContains(t, again.Events[0].Session["Body"], "File Name")

	short := bytes.Clone(createReq)
	short = short[:len(short)-1]
	short[3] = byte(len(short) - 4)
	refused := session.Feed(0, time.Unix(6, 0), short)
	require.NotNil(t, refused.Err)
}
