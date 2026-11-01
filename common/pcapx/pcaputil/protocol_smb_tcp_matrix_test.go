package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func smbTCPHandshake(clientSeq, serverSeq uint32) []tcpStep {
	return []tcpStep{
		{seq: clientSeq - 1, syn: true},
		{seq: serverSeq - 1, ack: clientSeq, syn: true, synack: true, reverse: true},
		{seq: clientSeq, ack: serverSeq},
	}
}

func smbTCPWrite(steps *[]tcpStep, seq *uint32, reverse bool, data []byte, cuts ...int) {
	if len(cuts) == 0 {
		cuts = []int{len(data)}
	}
	at := 0
	for _, n := range cuts {
		if at >= len(data) {
			return
		}
		end := at + n
		if end > len(data) {
			end = len(data)
		}
		*steps = append(*steps, tcpStep{seq: *seq, reverse: reverse, data: string(data[at:end])})
		*seq += uint32(end - at)
		at = end
	}
}

func smbTCPReplay(t *testing.T, port layers.TCPPort, steps []tcpStep, deferred bool, budget ...int) ([]*ProtocolEvent, BinParserStats, error) {
	t.Helper()
	var opts []CaptureOption
	if deferred {
		opts = append(opts, WithBinParserDeferred(true))
	}
	if len(budget) == 2 {
		opts = append(opts, WithProtocolBudget(budget[0], budget[1]))
	}
	return binReplay(t, binTestPcap(t, steps, port, false, true), 1, opts...)
}

func smbTCPOK(t *testing.T, port layers.TCPPort, steps []tcpStep, deferred bool) []*ProtocolEvent {
	t.Helper()
	events, stats, err := smbTCPReplay(t, port, steps, deferred)
	require.NoError(t, err)
	require.Zero(t, stats.BufferedBytes)
	return events
}

func smb2CreateCloseCompound() (request, response []byte, split int) {
	fid := bytes.Repeat([]byte{1}, 16)
	create := smb2Create(10, 0x11, 1, "a.txt")
	closeP := smb2Close(11, 0x11, 1, bytes.Repeat([]byte{0xff}, 16))
	cbody := append([]byte(nil), create[4:]...)
	for len(cbody)%8 != 0 {
		cbody = append(cbody, 0)
	}
	kbody := append([]byte(nil), closeP[4:]...)
	binary.LittleEndian.PutUint32(cbody[20:24], uint32(len(cbody)))
	binary.LittleEndian.PutUint32(kbody[16:20], smb2FlagRelated)
	binary.LittleEndian.PutUint32(kbody[36:40], ^uint32(0))
	binary.LittleEndian.PutUint64(kbody[40:48], ^uint64(0))
	copy(kbody[72:88], bytes.Repeat([]byte{0xff}, 16))
	request = smb2TCP(append(cbody, kbody...))
	createResp := smb2CreateResp(10, 0x11, 1, fid)[4:]
	closeResp := smb2CloseResp(11, 0x11, 1, nil)[4:]
	binary.LittleEndian.PutUint32(createResp[20:24], uint32(len(createResp)))
	binary.LittleEndian.PutUint32(closeResp[16:20], smb2FlagResp|smb2FlagRelated)
	response = smb2TCP(append(createResp, closeResp...))
	return request, response, 4 + len(createResp)
}

func TestSMB2TCPStateMatrix(t *testing.T) {
	req := smb2NegotiateReq(0x0311)
	resp := smb2NegotiateResp(0x0311)
	require.Greater(t, len(resp), len(req))
	for _, deferred := range []bool{false, true} {
		t.Run(boolName("deferred", deferred), func(t *testing.T) {
			t.Run("nbss-tcp-smb-boundaries", func(t *testing.T) {
				name := append([]byte{32}, bytes.Repeat([]byte{'E', 'B'}, 16)...)
				name = append(name, 0)
				sessionReq := append(append([]byte{0x81, 0, 0, 68}, name...), name...)
				positive := []byte{0x82, 0, 0, 0}
				require.NotEqual(t, len(sessionReq), len(req))
				steps := smbTCPHandshake(100, 500)
				client, server := uint32(100), uint32(500)
				smbTCPWrite(&steps, &client, false, sessionReq, 10, 30, len(sessionReq))
				smbTCPWrite(&steps, &server, true, positive, 1, len(positive))
				smbTCPWrite(&steps, &client, false, req, 3, 17, len(req))
				smbTCPWrite(&steps, &server, true, resp)
				events := smbTCPOK(t, 139, steps, deferred)
				var nbss, request, answer *ProtocolEvent
				for _, event := range events {
					if event.Protocol == "nbss" && nbss == nil {
						nbss = event
					}
					if event.Protocol != "smb2" || event.Session["Packet Name"] != "NEGOTIATE" {
						continue
					}
					if event.Session["Response"] == true {
						answer = event
					} else if request == nil {
						request = event
					}
				}
				require.NotNil(t, nbss)
				require.NotNil(t, request)
				require.NotNil(t, answer)
				require.Equal(t, len(sessionReq), nbss.Length)
				require.NotEqual(t, 10, nbss.Length)
				require.GreaterOrEqual(t, len(nbss.SourceBytes.PacketRefs), 2)
				require.Equal(t, len(req), request.Length)
				require.NotEqual(t, 3, request.Length)
				require.Equal(t, uint64(len(sessionReq)), request.Offset)
				require.GreaterOrEqual(t, len(request.SourceBytes.PacketRefs), 3)
				require.Equal(t, "NetBIOS Session Service", request.Session["Carrier"])
				require.Equal(t, uint64(len(positive)), answer.Offset)
				span := answer.Session["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Dialect"].([]any)
				require.Equal(t, []byte{0x11, 0x03}, answer.Raw[span[0].(uint64):span[1].(uint64)])
			})

			t.Run("reorder-retransmit-hole-truncation-overlap", func(t *testing.T) {
				steps := smbTCPHandshake(100, 500)
				steps = append(steps, tcpStep{seq: 120, data: string(req[20:])}, tcpStep{seq: 100, data: string(req[:20])})
				events := smbTCPOK(t, 445, steps, deferred)
				require.Equal(t, 1, smbCount(events, "NEGOTIATE"))

				again := smbTCPHandshake(100, 500)
				again = append(again, tcpStep{seq: 100, data: string(req)}, tcpStep{seq: 100, data: string(req)})
				events = smbTCPOK(t, 445, again, deferred)
				require.Equal(t, 1, smbCount(events, "NEGOTIATE"))

				hole := smbTCPHandshake(100, 500)
				hole = append(hole, tcpStep{seq: 100, data: string(req[:8])}, tcpStep{seq: 120, data: string(req[8:])})
				events, _, err := smbTCPReplay(t, 445, hole, deferred)
				require.Error(t, err)
				require.Contains(t, err.Error(), "unfilled sequence gap")
				require.Equal(t, 0, smbCount(events, "NEGOTIATE"))

				short := smbTCPHandshake(100, 500)
				short = append(short, tcpStep{seq: 100, data: string(req[:len(req)-1])}, tcpStep{seq: uint32(100 + len(req) - 1), fin: true})
				events = smbTCPOK(t, 445, short, deferred)
				require.Equal(t, 0, smbCount(events, "NEGOTIATE"))
				require.Equal(t, "incomplete", events[len(events)-1].Status)

				conflict := bytes.Repeat([]byte{'X'}, 10)
				overlap := smbTCPHandshake(100, 500)
				overlap = append(overlap, tcpStep{seq: 110, data: string(conflict)}, tcpStep{seq: 100, data: string(req)})
				events, _, err = smbTCPReplay(t, 445, overlap, deferred)
				require.Error(t, err)
				require.Contains(t, err.Error(), "conflict")
				for _, event := range events {
					if event.Protocol == "smb2" && (event.Status == "decoded" || event.Status == "deferred") {
						require.Equal(t, req, event.Raw)
					}
					require.NotContains(t, string(event.Raw), string(conflict))
				}
			})

			t.Run("syn-fin-rst-generation", func(t *testing.T) {
				alone, _, err := smbTCPReplay(t, 445, []tcpStep{{seq: 99, syn: true, data: string(req)}}, deferred)
				require.NoError(t, err)
				require.Equal(t, 1, smbCount(alone, "NEGOTIATE"))
				steps := smbTCPHandshake(100, 500)
				client, server := uint32(100), uint32(500)
				smbTCPWrite(&steps, &client, false, req)
				steps = append(steps, tcpStep{seq: 999, syn: true})
				smbTCPWrite(&steps, &server, true, resp)
				events := smbTCPOK(t, 445, steps, deferred)
				require.Equal(t, 2, smbCount(events, "NEGOTIATE"))
				require.Equal(t, "NEGOTIATE", events[1].Session["Matched Request"])
				require.Equal(t, events[0].FlowID, events[1].FlowID)

				closed := smbTCPHandshake(100, 500)
				c, s := uint32(100), uint32(500)
				smbTCPWrite(&closed, &c, false, req)
				smbTCPWrite(&closed, &s, true, resp)
				closed = append(closed,
					tcpStep{seq: c, fin: true}, tcpStep{seq: s, fin: true, reverse: true},
					tcpStep{seq: 8000, syn: true}, tcpStep{seq: 9000, ack: 8001, syn: true, synack: true, reverse: true}, tcpStep{seq: 8001, ack: 9001},
					tcpStep{seq: 9001, reverse: true, data: string(resp)},
					tcpStep{seq: 8001, data: string(req)},
					tcpStep{seq: 9001 + uint32(len(resp)), reverse: true, data: string(resp)},
				)
				events = smbTCPOK(t, 445, closed, deferred)
				require.Equal(t, 5, smbCount(events, "NEGOTIATE"))
				require.Equal(t, "NEGOTIATE", events[1].Session["Matched Request"])
				require.NotEqual(t, events[0].FlowID, events[2].FlowID)
				require.NotEqual(t, events[0].TCPFlowID, events[2].TCPFlowID)
				require.Equal(t, true, events[2].Session["Unmatched"])
				require.Equal(t, "NEGOTIATE", events[4].Session["Matched Request"])
				require.Equal(t, events[3].FlowID, events[4].FlowID)

				reset := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&reset, &c, false, req)
				smbTCPWrite(&reset, &s, true, resp)
				reset = append(reset, tcpStep{seq: c, rst: true}, tcpStep{seq: c, data: string(req)})
				events = smbTCPOK(t, 445, reset, deferred)
				require.Equal(t, 2, smbCount(events, "NEGOTIATE"))
			})

			t.Run("mid-response-async-cancel-compound", func(t *testing.T) {
				reused := smbTCPHandshake(100, 500)
				client := uint32(100)
				smbTCPWrite(&reused, &client, false, req)
				smbTCPWrite(&reused, &client, false, req)
				events := smbTCPOK(t, 445, reused, deferred)
				decoded := 0
				for _, event := range events {
					if event.Status == "decoded" || event.Status == "deferred" {
						if event.Session["Packet Name"] == "NEGOTIATE" && event.Session["Response"] != true {
							decoded++
						}
					}
				}
				require.Equal(t, 1, decoded)

				dup := smbTCPHandshake(100, 500)
				c, s := uint32(100), uint32(500)
				smbTCPWrite(&dup, &c, false, req)
				smbTCPWrite(&dup, &s, true, resp)
				smbTCPWrite(&dup, &s, true, resp)
				events = smbTCPOK(t, 445, dup, deferred)
				matched := 0
				for _, event := range events {
					if event.Session["Matched Request"] == "NEGOTIATE" {
						matched++
					}
				}
				require.Equal(t, 1, matched)
				require.Equal(t, true, events[len(events)-1].Session["Unmatched"])

				fid := bytes.Repeat([]byte{9}, 16)
				read5 := smb2Read(5, 0x11, 1, fid, 4)
				read6 := smb2Read(6, 0x11, 1, fid, 4)
				late6 := smb2ReadResp(6, 0x11, 1, []byte("late"))
				late5 := smb2ReadResp(5, 0x11, 1, []byte("yes"))
				bad := smb2ReadResp(5, 0x11, 2, []byte("no"))
				late := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&late, &c, false, read5)
				smbTCPWrite(&late, &c, false, read6)
				smbTCPWrite(&late, &s, true, late6)
				smbTCPWrite(&late, &s, true, bad)
				smbTCPWrite(&late, &s, true, late5)
				events = smbTCPOK(t, 445, late, deferred)
				require.Equal(t, "READ", events[2].Session["Matched Request"])
				require.Equal(t, true, events[3].Session["Unmatched"])
				require.Equal(t, []byte("yes"), events[4].Session["Body"].(map[string]any)["Data"])
				require.Equal(t, uint64(5), events[4].Session["Message ID"])

				pending := smb2ReadResp(7, 0x11, 1, nil)
				binary.LittleEndian.PutUint32(pending[12:16], 0x00000103)
				binary.LittleEndian.PutUint32(pending[20:24], smb2FlagResp|smb2FlagAsync)
				binary.LittleEndian.PutUint64(pending[36:44], 0xfeed0001)
				wrong := smb2ReadResp(7, 0x11, 1, []byte("wrong"))
				binary.LittleEndian.PutUint32(wrong[20:24], smb2FlagResp|smb2FlagAsync)
				binary.LittleEndian.PutUint64(wrong[36:44], 0xfeed0002)
				final := smb2ReadResp(7, 0x11, 1, []byte("final"))
				binary.LittleEndian.PutUint32(final[20:24], smb2FlagResp|smb2FlagAsync)
				binary.LittleEndian.PutUint64(final[36:44], 0xfeed0001)
				async := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&async, &c, false, smb2Read(7, 0x11, 1, fid, 4))
				smbTCPWrite(&async, &s, true, pending)
				smbTCPWrite(&async, &s, true, wrong)
				smbTCPWrite(&async, &s, true, final)
				events = smbTCPOK(t, 445, async, deferred)
				require.Equal(t, true, events[1].Session["Operation Pending"])
				require.Equal(t, uint64(0xfeed0001), events[1].Session["Async ID"])
				require.Equal(t, true, events[2].Session["Unmatched"])
				require.Equal(t, "READ", events[3].Session["Matched Request"])
				require.Equal(t, []byte("final"), events[3].Session["Body"].(map[string]any)["Data"])

				cancel := smb2Cancel(8, 0x11, 1)
				binary.LittleEndian.PutUint16(cancel[70:72], 0x8000)
				cancelled := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&cancelled, &c, false, smb2Read(8, 0x11, 1, fid, 4))
				smbTCPWrite(&cancelled, &c, false, cancel)
				smbTCPWrite(&cancelled, &s, true, smb2ReadResp(8, 0x11, 1, []byte("kept")))
				events = smbTCPOK(t, 445, cancelled, deferred)
				require.Equal(t, "CANCEL", events[1].Session["Packet Name"])
				require.Equal(t, false, events[1].Session["Response Expected"])
				require.Equal(t, uint64(8), events[1].Session["Cancellation Target Message ID"])
				require.Equal(t, uint16(0x8000), events[1].Session["Body"].(map[string]any)["Reserved"])
				require.Equal(t, "READ", events[2].Session["Matched Request"])
				require.Equal(t, []byte("kept"), events[2].Session["Body"].(map[string]any)["Data"])

				reply := smb2Cancel(8, 0x11, 1)
				binary.LittleEndian.PutUint32(reply[20:24], smb2FlagResp)
				refused := smbTCPHandshake(100, 500)
				c = 100
				smbTCPWrite(&refused, &c, false, reply)
				events = smbTCPOK(t, 445, refused, deferred)
				require.NotEqual(t, "decoded", events[0].Status)
				require.NotEqual(t, "deferred", events[0].Status)
				require.NotContains(t, events[0].Session, "Cancellation Target Message ID")

				compoundReq, compoundResp, split := smb2CreateCloseCompound()
				early := smb2CloseResp(11, 0x11, 1, nil)
				ordered := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&ordered, &c, false, compoundReq)
				smbTCPWrite(&ordered, &s, true, early)
				smbTCPWrite(&ordered, &s, true, smb2CreateResp(10, 0x11, 1, fid))
				events = smbTCPOK(t, 445, ordered, deferred)
				require.Equal(t, "missing-request-file-context", events[1].Session["File Context Status"])
				require.NotContains(t, events[1].Session, "Inherited File ID")
				require.NotContains(t, events[1].Session, "Request File ID")
				require.Equal(t, "CREATE", events[2].Session["Matched Request"])

				inherited := smbTCPHandshake(100, 500)
				c, s = 100, 500
				smbTCPWrite(&inherited, &c, false, compoundReq)
				inherited = append(inherited, tcpStep{seq: s + uint32(split), reverse: true, data: string(compoundResp[split:])}, tcpStep{seq: s, reverse: true, data: string(compoundResp[:split])})
				events = smbTCPOK(t, 445, inherited, deferred)
				second := events[1].Session["Commands"].([]map[string]any)[1]
				require.Equal(t, true, second["Inherited File ID"])
				require.GreaterOrEqual(t, len(events[1].SourceBytes.PacketRefs), 2)
				idRange := second["Wire Field Ranges"].(map[string]any)["Session ID"].([]any)
				require.GreaterOrEqual(t, idRange[0].(uint64), uint64(split))
			})

			t.Run("budget-and-owned-bytes", func(t *testing.T) {
				want := "decoded"
				if deferred {
					want = "deferred"
				}
				exact := smbTCPHandshake(100, 500)
				exact = append(exact, tcpStep{seq: 100, data: string(req)})
				events, stats, err := smbTCPReplay(t, 445, exact, deferred, len(req), len(req)+8192)
				require.NoError(t, err)
				require.Equal(t, 1, smbCount(events, "NEGOTIATE"))
				require.Zero(t, stats.BufferedBytes)
				owned := append([]byte(nil), events[0].Raw...)
				encoded, err := json.Marshal(events[0])
				require.NoError(t, err)
				require.Contains(t, string(encoded), "NEGOTIATE")
				var copied ProtocolEvent
				require.NoError(t, json.Unmarshal(encoded, &copied))
				require.Equal(t, owned, copied.Raw)
				events[0].Session["Packet Name"] = "EDITED"
				events[0].Fields = map[string]any{"edited": true}
				copied.Session["Packet Name"] = "COPY"
				fresh, err := events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, "NEGOTIATE", fresh["Packet Name"])
				require.Equal(t, owned, events[0].Raw)
				require.Equal(t, owned, copied.Raw)

				short := smbTCPHandshake(100, 500)
				short = append(short, tcpStep{seq: 100, data: string(req)})
				events, _, err = smbTCPReplay(t, 445, short, deferred, len(req)-1, len(req)+8192)
				require.NoError(t, err)
				require.Equal(t, 0, smbCount(events, "NEGOTIATE"))
				require.Equal(t, "limited", events[0].Status)

				other := smbTCPHandshake(100, 500)
				c, s := uint32(100), uint32(500)
				smbTCPWrite(&other, &c, false, req)
				smbTCPWrite(&other, &s, true, resp)
				events, stats, err = smbTCPReplay(t, 445, other, deferred, len(req), len(resp)+8192)
				require.NoError(t, err)
				require.Equal(t, req, events[0].Raw)
				require.Equal(t, want, events[0].Status)
				require.NotEqual(t, events[0].Raw, events[len(events)-1].Raw)
				require.Equal(t, "limited", events[len(events)-1].Status)
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}

func boolName(name string, value bool) string {
	if value {
		return name
	}
	return "eager"
}
