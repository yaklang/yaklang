package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func memcachedBinaryTest(magic, op byte, status uint16, opaque uint32, extras, key, value []byte) []byte {
	w := make([]byte, 24)
	w[0], w[1], w[4] = magic, op, byte(len(extras))
	binary.BigEndian.PutUint16(w[2:4], uint16(len(key)))
	binary.BigEndian.PutUint16(w[6:8], status)
	binary.BigEndian.PutUint32(w[8:12], uint32(len(extras)+len(key)+len(value)))
	binary.BigEndian.PutUint32(w[12:16], opaque)
	w = append(w, extras...)
	w = append(w, key...)
	return append(w, value...)
}
func memcachedTestExchange(t *testing.T, s *binMemcached, steps []sessionStep, chunk int) []map[string]any {
	t.Helper()
	var out []map[string]any
	var buffered [2][]byte
	for _, step := range steps {
		for off := 0; off < len(step.wire); {
			n := len(step.wire) - off
			if chunk > 0 {
				n = min(n, chunk)
			}
			buffered[step.dir] = append(buffered[step.dir], step.wire[off:off+n]...)
			off += n
			for len(buffered[step.dir]) > 0 {
				w := buffered[step.dir]
				size, err := memcachedFrameSize(w, 4096, 100)
				require.NoError(t, err)
				if size == 0 || size > len(w) {
					break
				}
				fields, err := s.consume(step.dir, w[:size])
				require.NoError(t, err)
				out = append(out, fields)
				buffered[step.dir] = w[size:]
			}
		}
	}
	require.Empty(t, buffered[0])
	require.Empty(t, buffered[1])
	return out
}
func TestMemcachedNativeTextRealFields(t *testing.T) {
	steps := []sessionStep{{0, []byte("set k1 5 0 5\r\nhello\r\n")}, {1, []byte("STORED\r\n")}, {0, []byte("get k1 nokey\r\n")}, {1, []byte("VALUE k1 5 5\r\nhello\r\nEND\r\n")}, {0, []byte("gets k1\r\n")}, {1, []byte("VALUE k1 5 5 2\r\nhello\r\nEND\r\n")}}
	for _, chunk := range []int{0, 1, 7, 64} {
		t.Run(fmt.Sprintf("chunk%d", chunk), func(t *testing.T) {
			s := newBinMemcached(0, "text-get-set", 100, nil)
			defer s.close()
			p := probeMemcached(steps[0].wire, 64)
			require.Equal(t, ProbeAccept, p.Verdict)
			fields := memcachedTestExchange(t, s, steps, chunk)
			require.Len(t, fields, 6)
			require.Equal(t, "set", fields[0]["Packet Name"])
			require.Equal(t, "k1", fields[0]["Key"])
			require.Equal(t, uint32(5), fields[0]["Flags"])
			require.Equal(t, int64(0), fields[0]["Expiration"])
			require.Equal(t, []byte("hello"), fields[0]["Value"])
			require.Equal(t, "set", fields[1]["In Reply To"])
			require.Equal(t, []string{"k1", "nokey"}, fields[2]["Keys"])
			values := fields[3]["Values"].([]map[string]any)
			require.Len(t, values, 1)
			require.Equal(t, "k1", values[0]["Key"])
			require.Equal(t, []byte("hello"), values[0]["Value"])
			require.Equal(t, "get", fields[3]["In Reply To"])
			require.Equal(t, true, fields[3]["Matched"])
			require.Equal(t, uint64(2), fields[5]["Values"].([]map[string]any)[0]["CAS"])
			require.Equal(t, 0, s.outstanding())
		})
	}
}
func TestMemcachedNativeBinaryOpaqueAndValues(t *testing.T) {
	key := []byte("bk")
	set := memcachedBinaryTest(128, 1, 0, 1, make([]byte, 8), key, []byte("binval"))
	stored := memcachedBinaryTest(129, 1, 0, 1, nil, nil, nil)
	binary.BigEndian.PutUint64(stored[16:24], 6)
	get := memcachedBinaryTest(128, 0, 0, 2, nil, key, nil)
	hit := memcachedBinaryTest(129, 0, 0, 2, []byte{0, 0, 0, 5}, nil, []byte("binval"))
	binary.BigEndian.PutUint64(hit[16:24], 6)
	missing := memcachedBinaryTest(128, 0, 0, 3, nil, []byte("missing"), nil)
	miss := memcachedBinaryTest(129, 0, 1, 3, nil, nil, []byte("Not found"))
	for _, chunk := range []int{0, 1, 7, 64} {
		t.Run(fmt.Sprintf("chunk%d", chunk), func(t *testing.T) {
			s := newBinMemcached(0, "binary-get-set", 100, nil)
			defer s.close()
			require.Equal(t, ProbeAccept, probeMemcached(set[:24], 64).Verdict)
			fields := memcachedTestExchange(t, s, []sessionStep{{0, set}, {1, stored}, {0, get}, {1, hit}, {0, missing}, {1, miss}}, chunk)
			require.Equal(t, uint64(1), fields[0]["Opaque"])
			require.Equal(t, key, fields[0]["Key"])
			require.Equal(t, []byte("binval"), fields[0]["Value"])
			require.Equal(t, "SET", fields[1]["In Reply To"])
			require.Equal(t, uint64(6), fields[1]["CAS"])
			require.Equal(t, uint32(5), fields[3]["Flags"])
			require.Equal(t, []byte("binval"), fields[3]["Value"])
			require.Equal(t, "GET", fields[3]["In Reply To"])
			require.Equal(t, key, fields[3]["Key"])
			require.Equal(t, 1, fields[5]["Status"])
			require.Equal(t, []byte("Not found"), fields[5]["Error Message"])
			require.Equal(t, 0, s.outstanding())
		})
	}
}
func TestMemcachedNativeAtomicBoundsDirectionAndOwnership(t *testing.T) {
	s := newBinMemcached(0, "text-get-set", 10, func(n int64) error {
		if n > 700 {
			return protocolError(ErrResourceExceeded, "test bytes")
		}
		return nil
	})
	key := strings.Repeat("k", 250)
	request := []byte("get " + key + "\r\n")
	fields, err := s.consume(0, request)
	require.NoError(t, err)
	require.Equal(t, int64(602), s.sessionBytes())
	fields["Keys"].([]string)[0] = "changed"
	for i := range request {
		request[i] = 'x'
	}
	_, err = s.consume(0, []byte("get "+key+"\r\n"))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	require.Equal(t, key, s.text[0].keys[0])
	_, err = s.consume(0, []byte("END\r\n"))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	_, err = s.consume(1, []byte("VALUE alien 0 0\r\n\r\nEND\r\n"))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	_, err = s.consume(1, []byte("END\r\n"))
	require.NoError(t, err)
	require.Equal(t, 0, s.outstanding())
	require.Equal(t, int64(336), s.sessionBytes())
	s.close()
	require.Nil(t, s.text)
	require.Nil(t, s.binary)
	require.Nil(t, s.reserveMemory)
	require.Zero(t, s.sessionBytes())
	_, err = s.consume(0, []byte("get key\r\n"))
	require.Error(t, err)
	s = newBinMemcached(0, "binary-get-set", 1, nil)
	get := memcachedBinaryTest(128, 0, 0, 77, nil, []byte("key"), nil)
	fields, err = s.consume(0, get)
	require.NoError(t, err)
	require.Equal(t, int64(339), s.sessionBytes())
	fields["Key"].([]byte)[0] = 'x'
	get[24] = 'x'
	require.Equal(t, []byte("key"), s.binary[0].request.key)
	_, err = s.consume(0, memcachedBinaryTest(128, 0, 0, 77, nil, []byte("key"), nil))
	require.Error(t, err)
	_, err = s.consume(0, memcachedBinaryTest(128, 0, 0, 78, nil, []byte("key"), nil))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	_, err = s.consume(1, memcachedBinaryTest(129, 1, 0, 77, nil, nil, nil))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	fields, err = s.consume(1, memcachedBinaryTest(129, 0, 1, 78, nil, nil, []byte("Not found")))
	require.NoError(t, err)
	require.Equal(t, false, fields["Matched"])
	require.Equal(t, "missing-request", fields["Association"])
	require.Equal(t, 1, s.outstanding())
	_, err = s.consume(1, memcachedBinaryTest(129, 0, 1, 77, nil, nil, []byte("Not found")))
	require.NoError(t, err)
	require.Equal(t, 0, s.outstanding())
	require.Equal(t, int64(336), s.sessionBytes())
	s.close()
	require.Nil(t, s.binary)
	require.Zero(t, s.sessionBytes())
}
func TestMemcachedNativeTextBinaryFramingNegatives(t *testing.T) {
	for _, wire := range [][]byte{[]byte("SET k 0 0 0\r\n\r\n"), []byte("GET / HTTP/1.1\r\n"), []byte("set k -1 0 0\r\n\r\n"), []byte("set k 0 0 1\r\naXX"), []byte("VALUE k 0 1\r\naXXEND\r\n"), []byte("VALUE k 0 0\r\n\r\nSTORED\r\n"), []byte("get\r\n"), []byte("set k 0 0 0 extra\r\n\r\n")} {
		_, err := memcachedFrameSize(wire, 4096, 100)
		require.Error(t, err, "%q", wire)
	}
	for _, wire := range [][]byte{[]byte("set k 0 0 4294967295\r\n"), []byte("get " + strings.Repeat("k", 251) + "\r\n"), bytes.Repeat([]byte{'x'}, 129)} {
		_, err := memcachedFrameSize(wire, 128, 100)
		require.Error(t, err)
	}
	req := memcachedBinaryTest(128, 0, 0, 1, nil, []byte("key"), nil)
	badExtra := bytes.Clone(req)
	badExtra[4] = 1
	badType := bytes.Clone(req)
	badType[5] = 1
	badKey := bytes.Clone(req)
	badKey[3] = 4
	badBody := bytes.Clone(req)
	binary.BigEndian.PutUint32(badBody[8:12], 0xffffffff)
	for _, wire := range [][]byte{badExtra, badType, badKey, badBody, memcachedBinaryTest(128, 1, 0, 1, nil, []byte("key"), nil), memcachedBinaryTest(129, 0, 0, 1, nil, nil, nil), memcachedBinaryTest(129, 1, 0, 1, nil, nil, []byte("extra"))} {
		_, err := memcachedFrameSize(wire, 4096, 100)
		require.Error(t, err)
		require.Equal(t, ProbeReject, probeMemcached(wire, 64).Verdict)
	}
	s := newBinMemcached(0, "text-get-set", 100, nil)
	raw := []byte("set k 0 0 7 noreply\r\na\r\nEND!\r\n")
	fields, err := s.consume(0, raw)
	require.NoError(t, err)
	require.Equal(t, []byte("a\r\nEND!"), fields["Value"])
	require.Equal(t, 0, s.outstanding())
	n, err := memcachedFrameSize(append(bytes.Clone(raw), []byte("get k\r\n")...), 4096, 100)
	require.NoError(t, err)
	require.Equal(t, len(raw), n)
	_, err = memcachedFrameSize([]byte("VALUE k 0 0\r\n\r\nVALUE j 0 0\r\n\r\nEND\r\n"), 4096, 1)
	require.Error(t, err)
}

func TestMemcachedNativeProbeRejectsObservedForeignPrefixes(t *testing.T) {
	for _, wire := range [][]byte{
		{0x80, 0x60},          // RTP dynamic payload type
		{0x80, 0, 0, 0},       // RTP PT0 with sequence zero
		{0x80, 1, 0, 0},       // Modbus transaction 0x8001 / protocol ID zero
		{0x80, 1, 0, 2, 0},    // SET missing flags/expiry extras
		{0x80, 0, 0, 2, 0, 1}, // reserved data type
		{0x80, 0, 0, 1, 0, 0, 0, 0, 0x12, 0x34, 0x56, 0x78}, // RTP SSRC resembles impossible GET body
	} {
		require.Equal(t, ProbeReject, probeMemcached(wire, 64).Verdict)
	}
	for _, op := range []byte{0, 1} {
		extra := []byte(nil)
		if op == 1 {
			extra = make([]byte, 8)
		}
		wire := memcachedBinaryTest(0x80, op, 0, 1, extra, []byte("key"), nil)
		for n := 1; n < 24; n++ {
			require.Equal(t, ProbeNeedMore, probeMemcached(wire[:n], 64).Verdict, "op=%d prefix=%d", op, n)
		}
		require.Equal(t, ProbeAccept, probeMemcached(wire[:24], 64).Verdict)
	}
}

func TestMemcachedNativeBinaryGETPublicCompatibility(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	request := memcachedBinaryTest(128, 0, 0, 1, nil, []byte("foo"), nil)
	r := s.Feed(0, time.Unix(1, 0), request)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	fields, err := r.Events[0].GetFields()
	require.NoError(t, err)
	for name, value := range map[string]uint64{
		"Magic": 128, "Opcode": 0, "Key Length": 3, "Extras Length": 0,
		"Data Type": 0, "VBucket ID": 0, "Total Body Length": 3,
		"Opaque": 1, "CAS": 0,
	} {
		require.Equal(t, value, fields[name], "existing GET field %s", name)
	}
	require.Equal(t, []byte("foo"), fields["Key"])
	response := memcachedBinaryTest(129, 0, 0, 1, []byte{0, 0, 0, 7}, nil, []byte("value"))
	r = s.Feed(1, time.Unix(2, 0), response)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, true, r.Events[0].Session["Matched"])
	require.Equal(t, "GET", r.Events[0].Session["In Reply To"])
	require.Equal(t, []byte("value"), r.Events[0].Session["Value"])
	s.Close("EOF")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMemcachedNativePublicIngress(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7, 64} {
			t.Run(fmt.Sprintf("chunk%d-deferred%v", chunk, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				defer s.Close("EOF")
				var events []*ProtocolEvent
				for _, step := range []sessionStep{{0, []byte("set k1 5 0 5\r\nhello\r\n")}, {1, []byte("STORED\r\n")}, {0, []byte("get k1\r\n")}, {1, []byte("VALUE k1 5 5\r\nhello\r\nEND\r\n")}} {
					for off := 0; off < len(step.wire); {
						n := len(step.wire) - off
						if chunk > 0 {
							n = min(n, chunk)
						}
						r := s.Feed(step.dir, time.Unix(1, 0), step.wire[off:off+n])
						require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
						events = append(events, r.Events...)
						off += n
					}
				}
				require.Len(t, events, 4)
				for _, e := range events {
					require.Equal(t, "memcached", e.Protocol)
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.NotEmpty(t, fields)
				}
				require.Equal(t, "get", events[3].Session["In Reply To"])
			})
		}
	}
}
func TestMemcachedNativeLongTextPublicIngress(t *testing.T) {
	longKey := strings.Repeat("l", 250)
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, chunk := range []int{0, 1, 7, 64} {
		t.Run(fmt.Sprintf("chunk%d", chunk), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
			require.NoError(t, err)
			var events []*ProtocolEvent
			for _, step := range []sessionStep{{0, []byte("set " + longKey + " 37 120 8\r\nmvp-data\r\n")}, {1, []byte("STORED\r\n")}, {0, []byte("get " + first + " " + second + "\r\n")}, {1, []byte("VALUE " + first + " 37 8\r\nmvp-data\r\nEND\r\n")}} {
				for off := 0; off < len(step.wire); {
					n := len(step.wire) - off
					if chunk > 0 {
						n = min(n, chunk)
					}
					r := s.Feed(step.dir, time.Unix(1, 0), step.wire[off:off+n])
					require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
					events = append(events, r.Events...)
					off += n
				}
			}
			require.Len(t, events, 4)
			require.Equal(t, longKey, events[0].Session["Key"])
			require.Equal(t, []string{first, second}, events[2].Session["Keys"])
			require.Equal(t, first, events[3].Session["Values"].([]map[string]any)[0]["Key"])
			require.Equal(t, true, events[3].Session["Matched"])
			require.Equal(t, 0, events[3].Session["Pending"])
			require.Empty(t, s.Close("EOF"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

// assertMemcachedMVPEvents accepts a case's expected.json directly so the sealed
// corpus replay matrix can share the independently generated semantic oracle.
func assertMemcachedMVPEvents(t testing.TB, expected json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Protocol     string           `json:"expected_protocol"`
		Forbidden    []string         `json:"forbidden_protocols"`
		MessageCount int              `json:"message_count"`
		Fields       []map[string]any `json:"exact_session_fields"`
		Raw          []string         `json:"exact_raw_hex"`
		Assertion    map[string]any   `json:"assertion"`
	}
	require.NoError(t, json.Unmarshal(expected, &a))
	var decoded []*ProtocolEvent
	var response *ProtocolEvent
	malformed, incomplete := false, false
	for _, e := range events {
		for _, forbidden := range a.Forbidden {
			require.NotEqual(t, forbidden, e.Protocol, "forbidden admission status=%s", e.Status)
		}
		if e.Protocol != "memcached" {
			continue
		}
		malformed = malformed || e.Status == "malformed"
		incomplete = incomplete || e.Status == "incomplete"
		if e.Status == "decoded" {
			decoded = append(decoded, e)
			if e.Session["Role"] == "response" {
				response = e
			}
		}
	}
	if a.Protocol != "" {
		require.Len(t, decoded, a.MessageCount)
		require.False(t, malformed, "positive fixture malformed")
		require.False(t, incomplete, "positive fixture left incomplete state")
		require.Len(t, a.Fields, len(decoded), "every complete message needs semantic oracle")
		require.Len(t, a.Raw, len(decoded), "every complete message needs source-order oracle")
		for i, e := range decoded {
			fields, err := e.GetFields()
			require.NoError(t, err)
			assertMVPJSONFields(t, a.Fields[i], fields)
			require.Equal(t, a.Raw[i], hex.EncodeToString(e.Raw), "message source order %d", i)
		}
		require.NotNil(t, response)
		require.Equal(t, 0, response.Session["Pending"])
	}
	if a.Assertion["no_decoded_response_or_storage_message"] == true {
		require.Empty(t, decoded, "truncated message decoded")
		require.True(t, incomplete || len(events) == 0, "truncated message must stay incomplete or unadmitted")
	}
	if matched, ok := a.Assertion["response_Matched"].(bool); ok {
		require.NotNil(t, response)
		require.Equal(t, matched, response.Session["Matched"])
		require.Equal(t, a.Assertion["response_Association"], response.Session["Association"])
		require.Equal(t, 1, response.Session["Pending"])
		require.True(t, incomplete, "unmatched opaque must preserve outstanding request until close")
	}
	if a.Assertion["last_response_status"] == "malformed" {
		require.True(t, malformed, "invalid association response accepted")
		for _, e := range decoded {
			require.NotEqual(t, "response", e.Session["Role"], "malformed response consumed outstanding request")
		}
	}
}

func assertMVPJSONFields(t testing.TB, expected, fields map[string]any) {
	t.Helper()
	for key, want := range expected {
		if strings.HasSuffix(key, " hex") {
			value, ok := fields[strings.TrimSuffix(key, " hex")].([]byte)
			require.True(t, ok, "byte field %s missing", key)
			require.Equal(t, want, hex.EncodeToString(value), key)
			continue
		}
		got, ok := fields[key]
		require.True(t, ok, "semantic field %s missing", key)
		wantJSON, err := json.Marshal(want)
		require.NoError(t, err)
		gotJSON, err := json.Marshal(got)
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), string(gotJSON), key)
	}
}

func TestMemcachedRepeatedRequestedKeys(t *testing.T) {
	for _, command := range []string{"get", "gets"} {
		s := newBinMemcached(0, "text-get-set", 8, nil)
		_, err := s.consume(0, []byte(command+" k k\r\n"))
		require.NoError(t, err)
		cas := ""
		if command == "gets" {
			cas = " 42"
		}
		item := "VALUE k 0 1" + cas + "\r\nv\r\n"
		wire := []byte(item + item + "END\r\n")
		fields, err := s.consume(1, wire)
		require.NoError(t, err, command)
		require.Equal(t, 2, fields["Value Count"])
		require.Equal(t, true, fields["Matched"])
		require.Zero(t, s.outstanding())
		for i := range wire {
			wire[i] = 0
		}
		values := fields["Values"].([]map[string]any)
		require.Equal(t, []byte("v"), values[0]["Value"])
		require.Equal(t, []byte("v"), values[1]["Value"])
		s.close()
	}
	// Repeated replies are valid only up to the observed request multiplicity.
	s := newBinMemcached(0, "text-get-set", 8, nil)
	defer s.close()
	_, err := s.consume(0, []byte("get k\r\n"))
	require.NoError(t, err)
	_, err = s.consume(1, []byte("VALUE k 0 1\r\nv\r\nVALUE k 0 1\r\nv\r\nEND\r\n"))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding(), "excess reply must not consume the request")
	_, err = s.consume(1, []byte("VALUE k 0 1\r\nv\r\nEND\r\n"))
	require.NoError(t, err)
	require.Zero(t, s.outstanding())
	limited := newBinMemcached(0, "text-get-set", 1, nil)
	defer limited.close()
	_, err = limited.consume(0, []byte("get k k\r\n"))
	require.Error(t, err)
	require.Zero(t, limited.outstanding())
}
