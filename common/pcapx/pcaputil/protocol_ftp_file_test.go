package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newFTPFileTestParser(t *testing.T, limit int) (*binParser, *binFlow) {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithProtocolFTPFileAssembly(limit)(c))
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	control := &binFlow{
		a: c.binParser, id: 1, transportFlow: &TrafficFlow{}, ftp: &binFTP{},
		endpoints: [2]string{"192.0.2.1:44000", "192.0.2.2:21"},
	}
	return c.binParser, control
}

func TestFTPFileAssemblyValidation(t *testing.T) {
	_, err := NewPacketAnalyzer(WithProtocolFTPFileAssembly(0))
	require.ErrorContains(t, err, "1..67108864")
	_, err = NewPacketAnalyzer(WithProtocolFTPFileAssembly(MaxFTPFileBytes + 1))
	require.ErrorContains(t, err, "1..67108864")
}

func ftpTestControl(t *testing.T, control *binFlow, dir int, raw, name, role, replyTo string, code int, ts time.Time) *ProtocolEvent {
	t.Helper()
	values := map[string]any{"Packet Name": name, "Role": role}
	if role == "reply" {
		values["Reply Code"] = code
		if replyTo != "" {
			values["In Reply To"] = replyTo
		}
	}
	event := &ProtocolEvent{Timestamp: ts, Raw: []byte(raw), Session: values}
	control.observeFTPDataControl(dir, event)
	return event
}

func runFTPFileTransfer(t *testing.T, command string, limit int, chunks [][]byte, completion int, gap bool) (*ProtocolEvent, *binFlow) {
	return runFTPFileTransferWithResume(t, command, limit, chunks, completion, gap, false)
}

func runFTPFileTransferWithResume(t *testing.T, command string, limit int, chunks [][]byte, completion int, gap, resume bool) (*ProtocolEvent, *binFlow) {
	t.Helper()
	a, control := newFTPFileTestParser(t, limit)
	defer a.closeFTPEndpoints()
	ts := time.Unix(100, 0)
	ftpTestControl(t, control, 0, "PASV\r\n", "PASV", "command", "", 0, ts)
	ftpTestControl(t, control, 1, "227 Entering Passive Mode (192,0,2,2,200,1)\r\n", "Reply", "reply", "PASV", 227, ts)
	if resume {
		ftpTestControl(t, control, 0, "REST 8\r\n", "REST", "command", "", 0, ts)
		ftpTestControl(t, control, 1, "350 Restart position accepted\r\n", "Reply", "reply", "REST", 350, ts)
	}
	ftpTestControl(t, control, 0, command+" report.txt\r\n", command, "command", "", 0, ts)

	data := &binFlow{
		a: a, id: 2, protocol: "ftp-data",
		endpoints: [2]string{"192.0.2.1:44001", "192.0.2.2:51201"},
	}
	data.ftpData = a.matchFTPData(data, ts)
	require.NotNil(t, data.ftpData)
	dataDir := 0
	if command == "RETR" {
		dataDir = 1
	}
	for i, chunk := range chunks {
		if gap && i == 1 {
			data.directions[dataDir].offset++
		}
		data.feed(dataDir, chunk, ts)
	}
	data.closeSession()

	var completionEvent *ProtocolEvent
	if completion != 0 {
		completionEvent = ftpTestControl(t, control, 1, "226 transfer complete\r\n", "Reply", "reply", command, completion, ts)
	}
	return completionEvent, data
}

func TestFTPFileExportRequiresSuccessfulCompletion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		completion int
	}{
		{"complete", 226},
		{"missing-final-reply", 0},
		{"failed-transfer", 426},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, _ := runFTPFileTransfer(t, "RETR", 1024, [][]byte{[]byte("abc"), []byte("def")}, tc.completion, false)
			if tc.completion != 226 {
				if event != nil {
					_, ok := event.Session["File Bytes"]
					require.False(t, ok)
				}
				return
			}
			require.Equal(t, []byte("abcdef"), event.Session["File Bytes"])
			require.Equal(t, "download", event.Session["File Transfer"])
			require.Equal(t, "RETR", event.Session["FTP Command"])
			require.EqualValues(t, 226, event.Session["FTP Completion Code"])
			require.Equal(t, "192.0.2.2:51201", event.Session["FTP File Source"])
			require.Equal(t, "192.0.2.1:44001", event.Session["FTP File Destination"])
			require.Len(t, event.Session["FTP File Sources"].([]map[string]any), 2)
		})
	}
}

func TestFTPFileExportUploadDirection(t *testing.T) {
	event, _ := runFTPFileTransfer(t, "STOR", 1024, [][]byte{[]byte("upload")}, 250, false)
	require.Equal(t, []byte("upload"), event.Session["File Bytes"])
	require.Equal(t, "upload", event.Session["File Transfer"])
	require.Equal(t, "STOR", event.Session["FTP Command"])
}

func TestFTPFileExportRejectsGapsAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		chunks [][]byte
		gap    bool
	}{
		{"offset-gap", 1024, [][]byte{[]byte("abc"), []byte("def")}, true},
		{"file-limit", 3, [][]byte{[]byte("abcd")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, _ := runFTPFileTransfer(t, "RETR", tc.limit, tc.chunks, 226, tc.gap)
			_, ok := event.Session["File Bytes"]
			require.False(t, ok)
		})
	}
}

func TestFTPFileExportRejectsNonFileCommands(t *testing.T) {
	event, _ := runFTPFileTransfer(t, "APPE", 1024, [][]byte{[]byte("append")}, 226, false)
	_, ok := event.Session["File Bytes"]
	require.False(t, ok)
}

func TestFTPFileExportZeroBytes(t *testing.T) {
	event, _ := runFTPFileTransfer(t, "RETR", 1024, nil, 226, false)
	_, ok := event.Session["File Bytes"]
	require.False(t, ok)
}

func TestFTPFileExportSkipsResumedPartialTransfer(t *testing.T) {
	event, _ := runFTPFileTransferWithResume(t, "RETR", 1024, [][]byte{[]byte("suffix")}, 226, false, true)
	_, ok := event.Session["File Bytes"]
	require.False(t, ok)
}
