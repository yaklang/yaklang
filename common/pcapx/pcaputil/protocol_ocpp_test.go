package pcaputil

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ocppTestFrame(dir int, text string) []byte {
	raw := []byte{0x81, byte(len(text))}
	if len(text) >= 126 {
		raw = []byte{0x81, 126, 0, 0}
		binary.BigEndian.PutUint16(raw[2:], uint16(len(text)))
	}
	if dir == 0 {
		raw[1] |= 128
		raw = append(raw, 1, 2, 3, 4)
		for i, b := range []byte(text) {
			raw = append(raw, b^byte(i%4+1))
		}
	} else {
		raw = append(raw, text...)
	}
	return raw
}

func ocppTestSession(t *testing.T, offered, selected string) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("test complete") })
	req := "GET /ocpp/CP001 HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
	if offered != "" {
		req += "Sec-WebSocket-Protocol: " + offered + "\r\n"
	}
	r := s.Feed(0, time.Unix(1, 0), []byte(req+"\r\n"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n"
	if selected != "" {
		resp += "Sec-WebSocket-Protocol: " + selected + "\r\n"
	}
	r = s.Feed(1, time.Unix(2, 0), []byte(resp+"\r\n"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	return s
}

func TestProtocolSessionOCPP16BootHeartbeat(t *testing.T) {
	s := ocppTestSession(t, "ocpp1.6", "ocpp1.6")
	r := s.Feed(0, time.Unix(3, 0), ocppTestFrame(0, `[2,"1","BootNotification",{"chargePointVendor":"MVP","chargePointModel":"Sim"}]`))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	request := r.Events[0]
	app, ok := request.Session["OCPP"].(map[string]any)
	require.True(t, ok, "negotiated OCPP 1.6 must have independently decoded RPC fields")
	require.Equal(t, "BootNotification", app["Action"])
	require.Equal(t, "1", app["Unique ID"])
	require.Equal(t, map[string]any{"chargePointVendor": "MVP", "chargePointModel": "Sim"}, app["Payload"])
	r = s.Feed(1, time.Unix(4, 0), ocppTestFrame(1, `[3,"1",{"status":"Accepted","currentTime":"2026-10-08T16:00:00Z","interval":300}]`))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, request.ID, r.Events[0].ResponseTo)
	require.Equal(t, request.ID, r.Events[0].TransactionID)
	require.Equal(t, "matched", r.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}

func TestProtocolSessionOCPP16AssociationBoundaries(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		t.Run(string(rune('A'+chunk)), func(t *testing.T) {
			s := ocppTestSession(t, "ocpp1.6, other", "ocpp1.6")
			feed := func(dir int, text string) *ProtocolEvent {
				wire := ocppTestFrame(dir, text)
				var events []*ProtocolEvent
				step := chunk
				if step == 0 {
					step = len(wire)
				}
				for at := 0; at < len(wire); at += step {
					end := min(at+step, len(wire))
					r := s.Feed(dir, time.Unix(3, 0), wire[at:end])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
						require.Less(t, end, len(wire))
					}
					events = append(events, r.Events...)
				}
				require.Len(t, events, 1)
				return events[0]
			}
			status := func(e *ProtocolEvent, want string) {
				require.Equal(t, want, e.Session["OCPP"].(map[string]any)["Context Status"])
			}
			a := feed(0, `[2,"a","Heartbeat",{}]`)
			status(a, "observed")
			require.Zero(t, a.ResponseTo)
			require.Equal(t, a.ID, a.TransactionID)
			b := feed(0, `[2,"b","Heartbeat",{}]`)
			status(b, "observed")
			wrong := feed(0, `[3,"a",{"currentTime":"2026-10-08T16:00:00Z"}]`)
			status(wrong, "context-required")
			require.Zero(t, wrong.ResponseTo)
			require.Zero(t, wrong.TransactionID)
			response := feed(1, `[3,"b",{"currentTime":"2026-10-08T16:00:05Z"}]`)
			status(response, "matched")
			require.Equal(t, b.ID, response.ResponseTo)
			require.Equal(t, b.ID, response.TransactionID)
			duplicate := feed(0, `[2,"a","Heartbeat",{}]`)
			status(duplicate, "context-required")
			late := feed(1, `[3,"a",{"currentTime":"2026-10-08T16:00:00Z"}]`)
			status(late, "context-required")
			require.Zero(t, late.ResponseTo)
			bad := feed(0, `[2,"bad","Heartbeat",{}]`)
			status(bad, "observed")
			badresp := feed(1, `[3,"bad",{"currentTime":"2026-02-30T16:00:00Z"}]`)
			status(badresp, "malformed")
			require.Zero(t, badresp.ResponseTo)
			status(feed(1, `[3,"bad",{"currentTime":"2026-10-08T16:00:00Z"}]`), "context-required")
			status(feed(0, `[2,"b","Heartbeat",{}]`), "context-required")
			good := feed(0, `[2,"good","Heartbeat",{}]`)
			errReply := feed(1, `[4,"good","OccurenceConstraintViolation","",{"reason":"test"}]`)
			status(errReply, "matched")
			require.Equal(t, good.ID, errReply.ResponseTo)
			require.Equal(t, map[string]any{"reason": "test"}, errReply.Session["OCPP"].(map[string]any)["Error Details"])
			// Mutating returned Session/query results must not alter association or
			// the independently retained decoding projection.
			query, err := errReply.GetFields()
			require.NoError(t, err)
			query["OCPP"].(map[string]any)["Error Details"].(map[string]any)["reason"] = "mutated"
			errReply.Session["OCPP"].(map[string]any)["Error Details"].(map[string]any)["reason"] = "session mutated"
			again, err := errReply.GetFields()
			require.NoError(t, err)
			require.Equal(t, "test", again["OCPP"].(map[string]any)["Error Details"].(map[string]any)["reason"])
			status(feed(0, `[2,"neighbor","Heartbeat",{}]`), "observed")
			status(feed(1, `[3,"neighbor",{"currentTime":"2026-10-08T16:00:00Z"}]`), "matched")
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Nil(t, s.Close("again"))
		})
	}
}

func TestProtocolSessionOCPP16PayloadBoundaries(t *testing.T) {
	for _, test := range []struct{ name, text, status string }{
		{"missing-model", `[2,"x","BootNotification",{"chargePointVendor":"MVP"}]`, "malformed"},
		{"extra-heartbeat", `[2,"x","Heartbeat",{"extra":1}]`, "malformed"},
		{"null-heartbeat", `[2,"x","Heartbeat",null]`, "malformed"},
		{"duplicate-key", `[2,"x","BootNotification",{"chargePointVendor":"MVP","chargePointVendor":"other","chargePointModel":"Sim"}]`, "malformed"},
		{"wrong-case", `[2,"x","heartbeat",{}]`, "unsupported"},
		{"extra-envelope", `[2,"x","Heartbeat",{},0]`, "malformed"},
		{"long-id", `[2,"` + strings.Repeat("x", 37) + `","Heartbeat",{}]`, "malformed"},
		{"long-vendor", `[2,"x","BootNotification",{"chargePointVendor":"` + strings.Repeat("x", 21) + `","chargePointModel":"Sim"}]`, "malformed"},
		{"unicode-20", `[2,"x","BootNotification",{"chargePointVendor":"` + strings.Repeat("电", 20) + `","chargePointModel":"Sim"}]`, "observed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := ocppTestSession(t, "ocpp1.6", "ocpp1.6")
			r := s.Feed(0, time.Time{}, ocppTestFrame(0, test.text))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, test.status, r.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
			require.Zero(t, r.Events[0].ResponseTo)
			neighbor := s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"neighbor","Heartbeat",{}]`))
			require.Nil(t, neighbor.Err)
			require.Equal(t, "observed", neighbor.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
		})
	}
	for _, interval := range []string{"300", "3e2", "300.0", "-1", "9007199254740993"} {
		t.Run("interval-"+interval, func(t *testing.T) {
			s := ocppTestSession(t, "ocpp1.6", "ocpp1.6")
			r := s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"x","BootNotification",{"chargePointVendor":"MVP","chargePointModel":"Sim"}]`))
			require.Nil(t, r.Err)
			r = s.Feed(1, time.Time{}, ocppTestFrame(1, `[3,"x",{"status":"Pending","currentTime":"2026-10-08t16:00:00z","interval":`+interval+`}]`))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			app := r.Events[0].Session["OCPP"].(map[string]any)
			require.Equal(t, "matched", app["Context Status"])
			require.Equal(t, json.Number(interval), app["Payload"].(map[string]any)["interval"])
		})
	}
}

// RFC8259 section 8.2 permits escape grammar that is not Unicode. Keep the
// interoperable profile strict rather than silently replacing identity bytes.
func TestProtocolSessionOCPP16UnicodeIdentity(t *testing.T) {
	for _, value := range []string{`\ud800`, `\udc00`, `\ud800A`, `\ud800\ud800`} {
		t.Run(value, func(t *testing.T) {
			s := ocppTestSession(t, "ocpp1.6", "ocpp1.6")
			defer s.Close("done")
			r := s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"`+value+`","Heartbeat",{}]`))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, "malformed", r.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
			require.Zero(t, r.Events[0].TransactionID)
			require.Zero(t, r.Events[0].ResponseTo)
			normal := s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"\ud83d\ude00","Heartbeat",{}]`))
			require.Nil(t, normal.Err)
			require.Equal(t, "observed", normal.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
			reply := s.Feed(1, time.Time{}, ocppTestFrame(1, `[3,"😀",{"currentTime":"2026-10-08T16:30:00Z"}]`))
			require.Nil(t, reply.Err)
			require.Equal(t, "matched", reply.Events[0].Session["OCPP"].(map[string]any)["Context Status"])
			require.Equal(t, normal.Events[0].ID, reply.Events[0].ResponseTo)
			require.Equal(t, normal.Events[0].ID, reply.Events[0].TransactionID)
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestProtocolSessionOCPP16RequiresNegotiation(t *testing.T) {
	s := ocppTestSession(t, "ocpp1.6", "")
	r := s.Feed(0, time.Time{}, ocppTestFrame(0, `[2,"1","Heartbeat",{}]`))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.NotContains(t, r.Events[0].Session, "OCPP")
}

func TestProtocolSessionOCPP16InvalidSubprotocolSelection(t *testing.T) {
	for _, c := range []struct{ offer, selected string }{
		{"other", "ocpp1.6"}, {"ocpp1.6", "ocpp1.6, other"}, {"bad/choice", "bad/choice"}, {"ocpp1.6, ocpp1.6", "ocpp1.6"}, {"ocpp1.6, \"other\"", "ocpp1.6"},
	} {
		t.Run(c.offer+"->"+c.selected, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			req := "GET /ocpp/CP001 HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: " + c.offer + "\r\n\r\n"
			resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\nSec-WebSocket-Protocol: " + c.selected + "\r\n\r\n"
			require.Nil(t, s.Feed(0, time.Time{}, []byte(req)).Err)
			r := s.Feed(1, time.Time{}, []byte(resp))
			require.NotNil(t, r.Err)
			require.Nil(t, s.(*captureSession).f.ws)
			s.Close("complete")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestProtocolSessionOCPP16ResourceRelease(t *testing.T) { testOCPPResourceRelease(t, "1.6") }

func testOCPPResourceRelease(t *testing.T, version string) {
	for _, kind := range []string{"elements", "depth", "ids", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			budget := DefaultParserBudget()
			if kind == "elements" {
				budget.MaxCollectionElements = 3
			}
			if kind == "depth" {
				budget.MaxRecursionDepth = 1
			}
			if kind == "ids" {
				budget.MaxCollectionElements = 6
			}
			if kind == "bytes" {
				budget.MaxBufferedBytes = 3000
				budget.MaxMessageBytes = 512
				budget.MaxFrameBytes = 512
			}
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			defer s.Close("done")
			req := "GET /ocpp/CP001 HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: ocpp1.6\r\n\r\n"
			resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\nSec-WebSocket-Protocol: ocpp1.6\r\n\r\n"
			require.Nil(t, s.Feed(0, time.Time{}, []byte(req)).Err)
			require.Nil(t, s.Feed(1, time.Time{}, []byte(resp)).Err)
			refused := false
			for i := 0; i < 40; i++ {
				r := s.Feed(0, time.Time{}, ocppTestFrame(0, fmt.Sprintf(`[2,"%d","Heartbeat",{}]`, i)))
				if r.Err != nil {
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					require.Len(t, r.Events, 1)
					_, err = r.Events[0].GetFields()
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrResourceExceeded, typed.Kind)
					r.Events[0].Status = "decoded"
					r.Events[0].Fields = map[string]any{"forged": true}
					r.Events[0].Structured = map[string]any{"fields": r.Events[0].Fields}
					_, err = r.Events[0].GetFields()
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrResourceExceeded, typed.Kind)
					refused = true
					break
				}
				require.Len(t, r.Events, 1)
				request := r.Events[0]
				r = s.Feed(1, time.Time{}, ocppTestFrame(1, fmt.Sprintf(`[3,"%d",{"currentTime":"2026-10-08T16:00:00Z"}]`, i)))
				if r.Err != nil {
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					refused = true
					break
				}
				require.Len(t, r.Events, 1)
				require.Equal(t, request.ID, r.Events[0].ResponseTo)
			}
			require.True(t, refused, "the configured bound must actually refuse")
			require.Nil(t, s.(*captureSession).f.ws, "resource refusal releases WebSocket and all OCPP IDs")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("complete"))
			require.Nil(t, s.Close("again"))
		})
	}
}
