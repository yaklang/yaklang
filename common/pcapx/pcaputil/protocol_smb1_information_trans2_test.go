package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMB1Information2AndTransaction2SourceRecords(t *testing.T) {
	setReq := smb1LayoutFixture(t, "set-information2-request.bin")
	setResp := smb1LayoutFixture(t, "set-information2-response.bin")
	queryReq := smb1LayoutFixture(t, "query-information2-request.bin")
	queryResp := smb1LayoutFixture(t, "query-information2-response.bin")
	transSetReq := smb1LayoutFixture(t, "transaction2-set-file-request.bin")
	transSetResp := smb1LayoutFixture(t, "transaction2-set-file-response.bin")
	transQueryReq := smb1LayoutFixture(t, "transaction2-query-path-request.bin")
	transQueryResp := smb1LayoutFixture(t, "transaction2-query-path-response.bin")
	require.Equal(t, []int{53, 39, 41, 61, 119, 64, 112, 180}, []int{
		len(setReq), len(setResp), len(queryReq), len(queryResp), len(transSetReq), len(transSetResp), len(transQueryReq), len(transQueryResp),
	})

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	feed := func(dir int, pdu []byte) *ProtocolEvent {
		t.Helper()
		result := session.Feed(dir, time.Unix(int64(dir+1), 0), pdu)
		require.Nil(t, result.Err)
		require.Len(t, result.Events, 1)
		return result.Events[0]
	}

	set := feed(0, setReq)
	setBody := set.Session["Body"].(map[string]any)
	require.Equal(t, uint16(16384), setBody["FID"])
	require.Equal(t, []any{uint64(37), uint64(39)}, setBody["Wire Field Ranges"].(map[string]any)["FID"])
	require.Equal(t, uint16(0x350a), setBody["CreateDate"])
	require.Equal(t, uint16(0x5185), setBody["CreationTime"])
	require.Equal(t, uint16(0x34ac), setBody["LastAccessDate"])
	require.Equal(t, uint16(0x5185), setBody["LastAccessTime"])
	require.Equal(t, uint16(0x344b), setBody["LastWriteDate"])
	require.Equal(t, uint16(0x5185), setBody["LastWriteTime"])
	require.Equal(t, uint16(0), setBody["Byte Count"])
	require.NotContains(t, setBody, "AndX Command")
	require.False(t, set.Session["Active AndX Chain Parsed"].(bool))
	fid := smb1AliasContractValue(t, set, "smb.fid")
	require.Equal(t, uint64(16384), fid.Value)
	require.Equal(t, &DisplayFieldRange{Offset: 37, Length: 2}, fid.Range)

	setAck := feed(1, setResp)
	require.Equal(t, "matched observed request; authentication unverified", setAck.Session["Association Status"])
	setAckBody := setAck.Session["Body"].(map[string]any)
	require.Equal(t, byte(0), setAckBody["Word Count"])
	require.Equal(t, uint16(0), setAckBody["Byte Count"])
	require.NotContains(t, setAckBody, "FID")
	require.NotContains(t, setAckBody, "CreateDate")
	require.NotContains(t, setAck.Session, "Request Path")
	require.NotContains(t, setAck.Session, "Request File Name")

	query := feed(0, queryReq)
	require.Equal(t, uint16(16384), query.Session["Body"].(map[string]any)["FID"])
	require.Equal(t, []any{uint64(37), uint64(39)}, query.Session["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["FID"])
	require.NotContains(t, query.Session["Body"], "CreateDate")

	queried := feed(1, queryResp)
	queryBody := queried.Session["Body"].(map[string]any)
	require.Equal(t, setBody["CreateDate"], queryBody["CreateDate"])
	require.Equal(t, setBody["LastWriteTime"], queryBody["LastWriteTime"])
	require.NotEqual(t, setBody["Wire Field Ranges"].(map[string]any)["CreateDate"], queryBody["Wire Field Ranges"].(map[string]any)["CreateDate"])
	require.Equal(t, []any{uint64(37), uint64(39)}, queryBody["Wire Field Ranges"].(map[string]any)["CreateDate"])
	require.Equal(t, uint32(7), queryBody["FileDataSize"])
	require.Equal(t, uint32(8), queryBody["FileAllocationSize"])
	require.Equal(t, uint16(0x20), queryBody["FileAttributes"])
	require.Equal(t, []any{uint64(49), uint64(53)}, queryBody["Wire Field Ranges"].(map[string]any)["FileDataSize"])
	require.NotContains(t, queryBody, "FID")
	require.NotContains(t, queried.Session, "Request File Name")
	size := smb1AliasContractValue(t, queried, "smb.file_data_size")
	require.Equal(t, uint64(7), size.Value)
	require.Equal(t, &DisplayFieldRange{Offset: 49, Length: 4}, size.Range)

	// The 0xFF byte is the low half of the FID, not an AndX command.
	andx := bytes.Clone(setReq)
	andx[37] = 0xff
	andxEvent := feed(0, andx)
	require.Equal(t, uint16(0x40ff), andxEvent.Session["Body"].(map[string]any)["FID"])
	require.NotContains(t, andxEvent.Session["Body"], "AndX Command")

	// These words contain no SMB_STRING, so clearing Unicode does not change them.
	oem := bytes.Clone(setReq)
	oem[15] &^= 0x80
	oemEvent := feed(0, oem)
	require.Equal(t, setBody["CreateDate"], oemEvent.Session["Body"].(map[string]any)["CreateDate"])
	require.NotContains(t, oemEvent.Session["Body"], "File Name")

	transSet := feed(0, transSetReq)
	transBody := transSet.Session["Body"].(map[string]any)
	require.Equal(t, "complete in this record", transBody["Transaction Status"])
	require.Equal(t, "TRANS2_SET_FILE_INFORMATION", transBody["Setup Name"])
	require.Equal(t, uint16(8), transBody["Setup"])
	require.Equal(t, uint16(16384), transBody["FID"])
	require.Equal(t, uint16(2), transBody["Information Level"])
	require.Equal(t, "SMB_INFO_SET_EAS", transBody["Information Level Name"])
	require.Equal(t, []any{uint64(72), uint64(78)}, transBody["Wire Field Ranges"].(map[string]any)["Parameters"])
	require.Equal(t, []any{uint64(72), uint64(74)}, transBody["Wire Field Ranges"].(map[string]any)["FID"])
	require.Equal(t, []any{uint64(78), uint64(119)}, transBody["Wire Field Ranges"].(map[string]any)["Data"])
	require.Equal(t, byte(0), transBody["Name"])
	require.NotContains(t, transBody, "AndX Command")
	eas := transBody["Extended Attributes"].([]map[string]any)
	require.Len(t, eas, 2)
	require.Equal(t, "EAONE", eas[0]["Attribute Name"])
	require.Equal(t, []byte("EAONE\x00"), eas[0]["Attribute Name Raw Bytes"])
	require.Equal(t, []byte("VALUE1"), eas[0]["Attribute Value"])
	require.Equal(t, byte(5), eas[0]["Attribute Name Length"])
	require.Equal(t, uint16(6), eas[0]["Attribute Value Length"])
	require.Equal(t, []any{uint64(86), uint64(92)}, eas[0]["Wire Field Ranges"].(map[string]any)["Attribute Name Raw Bytes"])
	require.Equal(t, "SECONDEA", eas[1]["Attribute Name"])
	require.Equal(t, []byte("ValueTwo"), eas[1]["Attribute Value"])
	require.Equal(t, uint32(41), transBody["EA List Size"])

	transAck := feed(1, transSetResp)
	ackBody := transAck.Session["Body"].(map[string]any)
	require.Equal(t, "matched observed request; authentication unverified", transAck.Session["Association Status"])
	require.Equal(t, uint16(0), ackBody["Ea Error Offset"])
	require.Equal(t, []any{uint64(60), uint64(62)}, ackBody["Wire Field Ranges"].(map[string]any)["Ea Error Offset"])
	require.Equal(t, []any{uint64(60), uint64(62)}, ackBody["Wire Field Ranges"].(map[string]any)["Parameters"])
	require.NotContains(t, ackBody, "Data")
	require.NotContains(t, ackBody, "File Name")
	require.NotContains(t, ackBody, "Extended Attributes")
	require.Len(t, ackBody["Padding"].([]map[string]any), 2)
	offset := smb1AliasContractValue(t, transAck, "smb.ea_error_offset")
	require.Equal(t, uint64(0), offset.Value)
	require.Equal(t, &DisplayFieldRange{Offset: 60, Length: 2}, offset.Range)

	pathReq := feed(0, transQueryReq)
	pathBody := pathReq.Session["Body"].(map[string]any)
	require.Equal(t, "TRANS2_QUERY_PATH_INFORMATION", pathBody["Setup Name"])
	require.Equal(t, uint16(0x0107), pathBody["Information Level"])
	require.Equal(t, uint32(0), pathBody["Reserved"])
	require.Equal(t, "scan-getinfo.dat", pathBody["File Name"])
	require.Equal(t, []any{uint64(78), uint64(110)}, pathBody["Wire Field Ranges"].(map[string]any)["File Name"])
	require.Equal(t, []any{uint64(78), uint64(112)}, pathBody["Wire Field Ranges"].(map[string]any)["File Name Raw Bytes"])
	require.Equal(t, []any{uint64(72), uint64(112)}, pathBody["Wire Field Ranges"].(map[string]any)["Parameters"])
	require.NotContains(t, pathBody, "Data")
	require.Equal(t, "scan-getinfo.dat", smb1AliasContractValue(t, pathReq, "smb.file").Value)

	pathResp := feed(1, transQueryResp)
	respBody := pathResp.Session["Body"].(map[string]any)
	require.Equal(t, `\test\scan-getinfo.dat`, respBody["File Name"])
	require.NotEqual(t, pathBody["File Name"], respBody["File Name"])
	require.Equal(t, uint32(44), respBody["File Name Length"])
	require.Equal(t, []any{uint64(136), uint64(180)}, respBody["Wire Field Ranges"].(map[string]any)["File Name"])
	require.Equal(t, uint64(127996387300000000), respBody["Creation Time"])
	require.Equal(t, uint64(127918627300000000), respBody["Last Access Time"])
	require.Equal(t, uint64(127840867300000000), respBody["Last Write Time"])
	require.Equal(t, uint64(127763104962886250), respBody["Last Change Time"])
	require.Equal(t, uint32(32), respBody["Extended File Attributes"])
	require.Equal(t, int64(8), respBody["Allocation Size"])
	require.Equal(t, int64(7), respBody["End Of File"])
	require.Equal(t, uint32(1), respBody["Number Of Links"])
	require.Equal(t, byte(0), respBody["Delete Pending"])
	require.Equal(t, byte(0), respBody["Directory"])
	require.Equal(t, uint32(41), respBody["EA Size"])
	require.Equal(t, []any{uint64(64), uint64(180)}, respBody["Wire Field Ranges"].(map[string]any)["Data"])
	require.Contains(t, respBody["Response Parameter Status"], "ignored")
	require.NotContains(t, respBody, "Ea Error Offset")
}

func TestSMB1Transaction2DoesNotBorrowAnotherSetup(t *testing.T) {
	transSetReq := smb1LayoutFixture(t, "transaction2-set-file-request.bin")
	transQueryReq := smb1LayoutFixture(t, "transaction2-query-path-request.bin")
	transQueryResp := smb1LayoutFixture(t, "transaction2-query-path-response.bin")
	otherLevel := bytes.Clone(transQueryReq)
	otherLevel[72] = 0x02
	levelSession, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer levelSession.Close("test")
	levelRequest := levelSession.Feed(0, time.Unix(1, 0), otherLevel)
	require.Nil(t, levelRequest.Err)
	require.Equal(t, uint16(0x0102), levelRequest.Events[0].Session["Body"].(map[string]any)["Information Level"])
	require.Equal(t, "scan-getinfo.dat", levelRequest.Events[0].Session["Body"].(map[string]any)["File Name"])
	wrongLevel := levelSession.Feed(1, time.Unix(2, 0), transQueryResp)
	require.Nil(t, wrongLevel.Err)
	require.NotContains(t, wrongLevel.Events[0].Session["Body"], "File Name")
	require.NotContains(t, wrongLevel.Events[0].Session["Body"], "End Of File")
	require.Contains(t, wrongLevel.Events[0].Session["Body"].(map[string]any)["Data Status"], "not a selected")

	// Same command and MID as SET_FILE still must not read the query-path data layout.
	sameMID := bytes.Clone(transQueryResp)
	sameMID[34], sameMID[35] = transSetReq[34], transSetReq[35]
	setSession, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer setSession.Close("test")
	require.Nil(t, setSession.Feed(0, time.Unix(3, 0), transSetReq).Err)
	borrowed := setSession.Feed(1, time.Unix(4, 0), sameMID)
	require.Nil(t, borrowed.Err)
	require.Equal(t, "matched observed request; authentication unverified", borrowed.Events[0].Session["Association Status"])
	require.NotContains(t, borrowed.Events[0].Session["Body"], "File Name")
	require.Contains(t, borrowed.Events[0].Session["Body"].(map[string]any)["Data Status"], "no data")
	require.Equal(t, len(transQueryResp)-64, len(borrowed.Events[0].Session["Body"].(map[string]any)["Data"].([]byte)))
}

func TestSMB1Information2AndTransaction2Bounds(t *testing.T) {
	setReq := smb1LayoutFixture(t, "set-information2-request.bin")
	setResp := smb1LayoutFixture(t, "set-information2-response.bin")
	queryResp := smb1LayoutFixture(t, "query-information2-response.bin")
	transSetReq := smb1LayoutFixture(t, "transaction2-set-file-request.bin")
	transSetResp := smb1LayoutFixture(t, "transaction2-set-file-response.bin")
	transQueryReq := smb1LayoutFixture(t, "transaction2-query-path-request.bin")
	transQueryResp := smb1LayoutFixture(t, "transaction2-query-path-response.bin")
	refuse := func(pdu []byte) {
		t.Helper()
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer session.Close("test")
		result := session.Feed(0, time.Unix(1, 0), pdu)
		require.NotNil(t, result.Err)
		for _, event := range result.Events {
			require.NotContains(t, event.Session, "Body")
			require.NotContains(t, event.Session, "FileDataSize")
			require.NotContains(t, event.Session, "Extended Attributes")
		}
	}

	long := append(bytes.Clone(setResp), 0)
	long[3] = byte(len(long) - 4)
	binary.LittleEndian.PutUint16(long[37:39], 1)
	refuse(long)

	truncated := bytes.Clone(setReq)
	truncated = truncated[:len(truncated)-1]
	truncated[3] = byte(len(truncated) - 4)
	refuse(truncated)

	badCount := bytes.Clone(queryResp)
	badCount[36] = 0
	refuse(badCount)

	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	opened := session.Feed(0, time.Unix(1, 0), setReq)
	require.Nil(t, opened.Err)
	failed := bytes.Clone(queryResp)
	binary.LittleEndian.PutUint32(failed[9:13], 0xc0000022)
	failed[34], failed[35] = setReq[34], setReq[35]
	status := session.Feed(1, time.Unix(2, 0), failed)
	require.NotNil(t, status.Err)
	require.Equal(t, ErrUnsupportedFeature, status.Err.Kind)
	require.NotContains(t, status.Events[0].Session, "Body")
	require.Zero(t, status.Events[0].ResponseTo)
	require.NotContains(t, status.Events[0].Session, "FileDataSize")

	secondary := bytes.Clone(transSetResp)
	secondary[8] = 0x33
	secondaryResult := session.Feed(1, time.Unix(3, 0), secondary)
	require.NotNil(t, secondaryResult.Err)
	require.Equal(t, ErrUnsupportedFeature, secondaryResult.Err.Kind)
	require.NotContains(t, secondaryResult.Events[0].Session, "Ea Error Offset")

	overlap := bytes.Clone(transSetReq)
	overlap[61] = overlap[57]
	overlap[62] = overlap[58]
	refuse(overlap)

	outside := bytes.Clone(transSetReq)
	binary.LittleEndian.PutUint16(outside[39:41], 200)
	binary.LittleEndian.PutUint16(outside[59:61], 200)
	refuse(outside)

	named := bytes.Clone(transSetReq)
	named[69] = 1
	refuse(named)

	emptyParams := bytes.Clone(transQueryReq)
	binary.LittleEndian.PutUint16(emptyParams[37:39], 0)
	binary.LittleEndian.PutUint16(emptyParams[55:57], 0)
	binary.LittleEndian.PutUint16(emptyParams[57:59], 0)
	emptySession, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer emptySession.Close("test")
	empty := emptySession.Feed(0, time.Unix(4, 0), emptyParams)
	require.Nil(t, empty.Err)
	emptyBody := empty.Events[0].Session["Body"].(map[string]any)
	require.NotContains(t, emptyBody, "Parameters")
	require.NotContains(t, emptyBody, "File Name")
	require.NotContains(t, emptyBody, "Information Level")
	require.Equal(t, uint32(0x424d53ff), empty.Events[0].Session["Header"].(map[string]any)["Protocol ID"])
	require.Equal(t, []any{uint64(4), uint64(8)}, empty.Events[0].Session["Header"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Protocol ID"])

	oemPath := bytes.Clone(transQueryReq)
	oemPath[15] &^= 0x80
	oem := emptySession.Feed(0, time.Unix(5, 0), oemPath)
	require.Nil(t, oem.Err)
	require.NotContains(t, oem.Events[0].Session["Body"], "File Name")
	require.Contains(t, oem.Events[0].Session["Body"].(map[string]any)["File Name Status"], "OEM")
	require.Equal(t, uint16(0x0107), oem.Events[0].Session["Body"].(map[string]any)["Information Level"])

	nonASCII := bytes.Clone(transSetReq)
	nonASCII[86] = 0xe9
	ea := emptySession.Feed(0, time.Unix(6, 0), nonASCII)
	require.Nil(t, ea.Err)
	attrs := ea.Events[0].Session["Body"].(map[string]any)["Extended Attributes"].([]map[string]any)
	require.NotContains(t, attrs[0], "Attribute Name")
	require.Contains(t, attrs[0]["Attribute Name Status"], "OEM")
	require.Equal(t, "SECONDEA", attrs[1]["Attribute Name"])

	displaced := bytes.Clone(transQueryResp)
	displaced[47] = 1
	alone, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer alone.Close("test")
	require.Nil(t, alone.Feed(0, time.Unix(7, 0), transQueryReq).Err)
	moved := alone.Feed(1, time.Unix(8, 0), displaced)
	require.Nil(t, moved.Err)
	require.Contains(t, moved.Events[0].Session["Body"].(map[string]any)["Transaction Status"], "displaced")
	require.NotContains(t, moved.Events[0].Session["Body"], "File Name")
	require.NotContains(t, moved.Events[0].Session["Body"], "End Of File")

	unmatched, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer unmatched.Close("test")
	only := unmatched.Feed(1, time.Unix(9, 0), transQueryResp)
	require.Nil(t, only.Err)
	require.NotContains(t, only.Events[0].Session["Body"], "File Name")
	require.NotContains(t, only.Events[0].Session, "Ea Error Offset")
	require.Contains(t, only.Events[0].Session["Association Status"], "no matching")
}
