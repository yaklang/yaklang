package pcaputil

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Generate complete transport envelopes around arbitrary success bodies, after
// establishing real pending requests. This reaches body/context arithmetic and
// connection state, rather than hoping random bytes discover an SMB signature.
func FuzzSMB2StatefulBodySources(f *testing.F) {
	fid := bytes.Repeat([]byte{1}, 16)
	for kind, wire := range [][]byte{smb2NegotiateResp(0x0202), smb2CreateResp(3, 17, 7, fid), smb2CloseResp(4, 17, 7, nil), smb2QueryResponse(5, 17, 7, 0, []byte{1, 2, 3, 4})} {
		f.Add(uint8(kind), bytes.Clone(wire[68:]), uint8(7))
	}
	f.Add(uint8(1), []byte{89, 0}, uint8(1))
	f.Fuzz(func(t *testing.T, kind uint8, body []byte, chunk uint8) {
		if len(body) > 2048 {
			return
		}
		session, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: 128 << 10, MaxCollectionElements: 64})
		require.NoError(t, err)
		smb2ChainFeed(t, session, 0, smb2NegotiateReq(0x0202))
		smb2ChainFeed(t, session, 1, smb2NegotiateResp(0x0202))
		command, mid, sid, tid := uint16(0), uint64(1), uint64(0), uint32(0)
		switch kind % 4 {
		case 0:
			smb2ChainFeed(t, session, 0, smb2NegotiateReq(0x0202))
		case 1:
			command, mid, sid, tid = 5, 3, 17, 7
			smb2ChainFeed(t, session, 0, smb2Create(3, 17, 7, "fuzz.txt"))
		case 2:
			command, mid, sid, tid = 6, 4, 17, 7
			smb2ChainFeed(t, session, 0, smb2Create(3, 17, 7, "fuzz.txt"))
			smb2ChainFeed(t, session, 1, smb2CreateResp(3, 17, 7, fid))
			smb2ChainFeed(t, session, 0, smb2Close(4, 17, 7, fid))
		case 3:
			command, mid, sid, tid = 16, 5, 17, 7
			smb2ChainFeed(t, session, 0, smb2QueryRequest(5, 17, 7, 1, 0x22, nil))
		}
		wire := smb2TCP(append(smb2Hdr(command, smb2FlagResp, mid, sid, tid), body...))
		for at := 0; at < len(wire); {
			end := min(len(wire), at+int(chunk)+1)
			result := session.Feed(1, time.Unix(1, 0), wire[at:end])
			for _, event := range result.Events {
				if event.Status == "decoded" {
					smb2FuzzOwnedRanges(t, event.Session, len(event.Raw))
				}
				owned := bytes.Clone(event.Raw)
				before, err := json.Marshal(event.Session)
				require.NoError(t, err, "compound maps cannot contain cycles")
				for i := at; i < end; i++ {
					wire[i] ^= 0xff
				}
				require.Equal(t, owned, event.Raw, "emitted sources cannot borrow producer input")
				after, err := json.Marshal(event.Session)
				require.NoError(t, err)
				require.Equal(t, before, after, "owned field bytes cannot borrow the input")
			}
			at = end
		}
		session.Close("FIN")
		require.Zero(t, session.Stats().BufferedBytes)
		require.LessOrEqual(t, session.Stats().PeakBufferedBytes, int64(128<<10))
		require.Nil(t, session.(*captureSession).f.a.err.Load(), "malformed bodies must not use panic recovery")
	})
}

func smb2FuzzOwnedRanges(t *testing.T, value any, size int) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if ranges, ok := value["Wire Field Ranges"].(map[string]any); ok {
			for _, forbidden := range []string{"Effective Session ID", "Effective Tree ID", "Request File ID", "File Context", "File Context Source Message ID", "Request Info Type", "Request File Info Class", "Request Additional Information", "Request Query Flags", "Request Output Buffer Length", "Request Message ID"} {
				require.NotContains(t, ranges, forbidden)
			}
			for _, span := range ranges {
				pair, ok := span.([]any)
				require.True(t, ok)
				require.Len(t, pair, 2)
				lo, lok := displayUint(pair[0])
				hi, hok := displayUint(pair[1])
				require.True(t, lok && hok && lo <= hi && hi <= uint64(size))
			}
		}
		for name, child := range value {
			if name != "Wire Field Ranges" {
				smb2FuzzOwnedRanges(t, child, size)
			}
		}
	case []map[string]any:
		for _, child := range value {
			smb2FuzzOwnedRanges(t, child, size)
		}
	case []any:
		for _, child := range value {
			smb2FuzzOwnedRanges(t, child, size)
		}
	}
}
