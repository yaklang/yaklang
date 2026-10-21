package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Part 6 v1.05.07 6.7.2.4 forbids sequence reuse for a TokenId.
// These requests reuse RequestId and handle only after a completed response.
func TestOPCUASelectedSequenceReplayExistingAPI(t *testing.T) {
	raw, err := uaReadBasic()
	require.NoError(t, err)
	var x map[string]string
	require.NoError(t, json.Unmarshal(raw, &x))
	w := map[string][]byte{}
	for _, k := range []string{"opn", "request", "response"} {
		w[k], err = hex.DecodeString(x[k])
		require.NoError(t, err)
	}
	for _, bad := range []bool{false, true} {
		name := "advancing-neighbor"
		if bad {
			name = "replayed-completed-response"
		}
		t.Run(name, func(t *testing.T) {
			next := bytes.Clone(w["request"])
			binary.LittleEndian.PutUint32(next[16:20], 3)
			rsp := bytes.Clone(w["response"])
			if !bad {
				binary.LittleEndian.PutUint32(rsp[16:20], 2)
			}
			steps := []sessionStep{{0, w["opn"]}, {0, w["request"]}, {1, w["response"]}, {0, next}, {1, rsp}}
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(sessionTestPCAP(t, steps, layers.TCPPort(4840), 0, false, false)), WithBinParserConfig(BinParserConfig{DecodeOPCUARead: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, events, 5)
			require.Equal(t, true, events[2].Session["Matched"])
			require.Equal(t, uint32(631), events[3].Session["Service Type ID"])
			if bad {
				require.NotEmpty(t, events[4].Error, "replayed old response must not associate with the new request")
				rocTypedError(t, "Desynchronized", events[4].sessionError)
				require.NotContains(t, events[4].Session, "Matched")
			} else {
				require.Empty(t, events[4].Error)
				require.Equal(t, true, events[4].Session["Matched"])
				require.Equal(t, events[2].Session["Read"], events[4].Session["Read"])
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
}

func TestOPCUASelectedSequenceIsolation(t *testing.T) {
	for _, profile := range []struct{ root, field string }{
		{"opcua-read-scalar", "Read"}, {"opcua-browse", "Browse"},
		{"opcua-write", "Write"}, {"opcua-browsenext", "BrowseNext"}, {"opcua-call", "Call"},
	} {
		t.Run(profile.field, func(t *testing.T) {
			raw, err := opcuaSelectedInput(profile.root, "material/cases.json")
			require.NoError(t, err)
			var cases []uaReadCase
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			require.NoError(t, d.Decode(&cases))
			wire := make([][]byte, 3)
			for i, step := range cases[0].Steps {
				wire[i], err = hex.DecodeString(step.Hex)
				require.NoError(t, err)
			}
			for _, changedToken := range []bool{false, true} {
				s := &binOPCUA{read: profile.field == "Read", browse: profile.field == "Browse", write: profile.field == "Write", next: profile.field == "BrowseNext", call: profile.field == "Call"}
				for i, w := range wire {
					out, err := s.consumeEvent(i/2, &ProtocolEvent{Raw: w}, 1024, 1<<20)
					require.NoError(t, err)
					requireExactProtocolFields(t, cases[0].Steps[i].Session, out)
				}
				request := bytes.Clone(wire[1])
				binary.LittleEndian.PutUint32(request[16:20], 3)
				_, err = s.consumeEvent(0, &ProtocolEvent{Raw: request}, 1024, 1<<20)
				require.NoError(t, err)
				require.Len(t, s.appPending, 1)
				stale := bytes.Clone(wire[2])
				if changedToken {
					binary.LittleEndian.PutUint32(stale[12:16], 14)
				}
				out, err := s.consumeEvent(1, &ProtocolEvent{Raw: stale}, 1024, 1<<20)
				rocTypedError(t, "Desynchronized", err)
				require.Nil(t, out)
				require.Empty(t, s.appPending)
				require.Empty(t, s.chunks)
				require.Equal(t, map[uaKey]uint32{{0, 6, 1}: 446}, s.pending)
				require.Equal(t, uint32(1), s.seq[1][6], "refused replay must not lower or advance the accepted sequence")
				// Accepted next observation cannot consume the invalidated association.
				late := bytes.Clone(wire[2])
				binary.LittleEndian.PutUint32(late[16:20], 2)
				out, err = s.consumeEvent(1, &ProtocolEvent{Raw: late}, 1024, 1<<20)
				require.NoError(t, err)
				require.Equal(t, false, out["Matched"])
				want := cloneSession(cases[0].Steps[2].Session)
				want["Sequence Number"] = uint32(2)
				want["Matched"] = false
				requireExactProtocolFields(t, want, out)
				// A new exchange can proceed with increasing sequences after retirement.
				binary.LittleEndian.PutUint32(request[16:20], 4)
				_, err = s.consumeEvent(0, &ProtocolEvent{Raw: request}, 1024, 1<<20)
				require.NoError(t, err)
				binary.LittleEndian.PutUint32(late[16:20], 3)
				out, err = s.consumeEvent(1, &ProtocolEvent{Raw: late}, 1024, 1<<20)
				require.NoError(t, err)
				want["Sequence Number"] = uint32(3)
				want["Matched"] = true
				requireExactProtocolFields(t, want, out)
				require.Empty(t, s.appPending)
			}
		})
	}
	// Default envelope observation remains tolerant, without claiming body semantics.
	raw, err := uaReadBasic()
	require.NoError(t, err)
	var x map[string]string
	require.NoError(t, json.Unmarshal(raw, &x))
	s := &binOPCUA{}
	for _, step := range []struct {
		key string
		dir int
	}{{"opn", 0}, {"request", 0}, {"response", 1}, {"request", 0}, {"response", 1}} {
		w, err := hex.DecodeString(x[step.key])
		require.NoError(t, err)
		out, err := s.consume(step.dir, w, 1024, 1<<20)
		require.NoError(t, err)
		require.NotContains(t, out, "Read")
		if step.key == "response" {
			require.Equal(t, true, out["Matched"])
		}
	}
}
