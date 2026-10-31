package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

// Independent fixed-word constructors exercise mixed OEM/Unicode alignment;
// no external capture, secret, locale guess or parser encoder is used here.
func smb1FileWire(command byte, response bool, mid, uid, tid uint16) []byte {
	base := 47
	if command == 0x75 && response {
		base = 45
	}
	if command == 0x06 {
		base = 41
		if response {
			base = 39
		}
	}
	w := make([]byte, base)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', command})
	binary.LittleEndian.PutUint16(w[14:16], 0xc003) // Unicode without EXTENDED_SECURITY.
	binary.LittleEndian.PutUint16(w[16:18], 0x1234)
	binary.LittleEndian.PutUint16(w[30:32], 0x5678)
	binary.LittleEndian.PutUint16(w[28:30], tid)
	binary.LittleEndian.PutUint16(w[32:34], uid)
	binary.LittleEndian.PutUint16(w[34:36], mid)
	if response {
		w[13] = 0x80
	}
	text := func(s string) {
		if len(w)%2 != 0 {
			w = append(w, 0x9a)
		} // Alignment is not password/string data.
		for _, u := range append(utf16.Encode([]rune(s)), 0) {
			w = append(w, byte(u), byte(u>>8))
		}
	}
	if command == 0x75 {
		w[36], w[37] = 4, 0xff
		binary.LittleEndian.PutUint16(w[39:41], 0x1234) // Ignored terminal AndXOffset.
		if response {
			w[36] = 3
			binary.LittleEndian.PutUint16(w[41:43], 1)
			w = append(w, 'I', 'P', 'C', 0)
			text("NTFS")
		} else {
			binary.LittleEndian.PutUint16(w[41:43], 0)
			binary.LittleEndian.PutUint16(w[43:45], 0)
			text("\\\\server\\共享")
			w = append(w, '?', '?', '?', '?', '?', 0)
		}
	} else if response {
		w[36] = 0
	} else {
		w[36] = 1
		binary.LittleEndian.PutUint16(w[37:39], 0x16)
		w = append(w, 4)
		text("\\dir\\文件.txt")
	}
	binary.LittleEndian.PutUint16(w[base-2:base], uint16(len(w)-base))
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func TestSMB1FileCommandsPublicSessionProfiles(t *testing.T) {
	for _, command := range []byte{0x75, 0x06} {
		for _, response := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 4096} {
				t.Run(fmt.Sprintf("command%02x/response%v/chunk%d", command, response, chunk), func(t *testing.T) {
					s := smb1SetupSession(t)
					w := smb1FileWire(command, response, 4, 2048, 2048)
					require.Equal(t, ProbeAccept, s.Probe(w).Verdict)
					var events []*ProtocolEvent
					for at := 0; at < len(w); at += chunk {
						piece := w[at:min(at+chunk, len(w))]
						r := s.Feed(0, time.Unix(100, 0), piece)
						require.Equal(t, len(piece), r.Consumed)
						if r.Err != nil {
							require.Equal(t, ErrNeedMore, r.Err.Kind)
							require.True(t, r.NeedMore)
						}
						if at+chunk < len(w) {
							require.Empty(t, r.Events)
						} else {
							require.Nil(t, r.Err)
						}
						events = append(events, r.Events...)
					}
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, "decoded", e.Status)
					require.Equal(t, "smb", e.Protocol)
					require.Equal(t, w, e.Raw)
					require.Zero(t, e.ResponseTo)
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, command, fields["Header"].(map[string]any)["Command"])
					require.Equal(t, response, fields["Response"])
					name := "CIFSDirectTCPDelete"
					if command == 0x75 {
						name = "CIFSDirectTCPTreeConnectAndX"
					}
					if response {
						name += "Response"
					} else {
						name += "Request"
					}
					require.Equal(t, name, e.Entry)
					for _, key := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
						require.Equal(t, false, fields[key])
					}
					body := fields["Body"].(map[string]any)
					if command == 0x75 && !response {
						require.Equal(t, "\\\\server\\共享", body["Path"])
						require.Equal(t, []byte{}, body["Password"])
						require.Equal(t, []byte{0x9a}, body["Alignment Pad"])
					}
					if command == 0x75 && response {
						require.Equal(t, "IPC", body["Service"])
						require.Equal(t, "NTFS", body["Native File System"])
						require.Equal(t, []byte{0x9a}, body["Alignment Pad"])
					}
					if command == 0x06 && !response {
						require.Equal(t, "\\dir\\文件.txt", body["File Name"])
					}
					original := bytes.Clone(e.Raw)
					clear(w)
					require.Equal(t, original, e.Raw)
				})
			}
		}
	}
}
