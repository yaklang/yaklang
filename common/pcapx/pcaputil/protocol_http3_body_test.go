package pcaputil

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func h3BodyTestFeed(q *binQUIC, dir int, sid, off uint64, wire []byte, fin bool) (map[string]any, map[string]any, error) {
	q.eventID++
	q.eventRefs = []PacketReference{{Number: q.eventID}}
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": sid, "Offset": off, "Stream Data": wire, "FIN": fin}
	info := map[string]any{}
	err := q.applyFrames(dir, quicSpaceApplication, []map[string]any{fr}, info, 1024)
	return fr, info, err
}

func TestHTTP3EqualPrefixRequiresFINAndEmptyFINCompletesIt(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	wire := append(h3BodyRequest("POST", "4"), h3Data([]byte("data"))...)
	fr, _, err := h3BodyTestFeed(q, 0, 0, 0, wire, false)
	require.NoError(t, err)
	require.Equal(t, "pending-FIN", fr["HTTP3 Message Conformance"].(map[string]any)["Length Check"])
	fr, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(wire)), nil, true)
	require.NoError(t, err)
	m := fr["HTTP3 Message Conformance"].(map[string]any)
	require.Equal(t, "matched", m["Length Check"])
	require.Equal(t, uint64(1), m["Header PDU ID"])
	require.Equal(t, uint64(4), m["Observed DATA Bytes"])
}

func FuzzHTTP3BodyLengthAndFINIsolation(f *testing.F) {
	f.Add(uint64(4), uint8(4), true)
	f.Add(uint64(3), uint8(4), false)
	f.Add(^uint64(0), uint8(0), true)
	f.Fuzz(func(t *testing.T, declared uint64, size uint8, fin bool) {
		n := int(size % 96)
		wire := append(h3BodyRequest("POST", strconv.FormatUint(declared, 10)), h3Data(make([]byte, n))...)
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 1, eventRefs: []PacketReference{{Number: 1}}}
		candidate := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": wire, "FIN": fin}
		other := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": h3RequestHeaders(), "FIN": true}
		err := q.applyFrames(0, quicSpaceApplication, []map[string]any{candidate, other}, map[string]any{}, 1024)
		wantError := uint64(n) > declared || fin && uint64(n) != declared
		if (err != nil) != wantError {
			t.Fatalf("body %d declared %d FIN %v: %v", n, declared, fin, err)
		}
		if err != nil && !isH3FieldStreamError(err) {
			t.Fatal("length discrepancy became a connection-wide error")
		}
		if other["HTTP3 Frames"] == nil {
			t.Fatal("message length discrepancy discarded another stream")
		}
	})
}

func h3BodyRequest(method, length string) []byte {
	fields := [][2]string{{":method", method}, {":scheme", "https"}, {":path", "/"}, {":authority", "example.com"}}
	if length != "-" {
		fields = append(fields, [2]string{"content-length", length})
	}
	return h3LiteralFields(fields...)
}

func TestHTTP3ContentLengthDiscrepanciesAreStreamLocal(t *testing.T) {
	for _, tt := range []struct {
		name, length string
		body         string
		fin          bool
	}{
		{"short-at-FIN", "5", "data", true},
		{"overrun-before-FIN", "3", "data", false},
		{"zero-with-content", "0", "x", true},
		{"huge-decimal-at-FIN", "1844674407370955161600000000", "", true},
		{"negative-length", "-1", "", true},
		{"plus-length", "+1", "", true},
		{"non-decimal", "1x", "", true},
		{"conflicting-list", "1, 2", "", true},
		{"empty-length", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			wire := append(h3BodyRequest("POST", tt.length), h3Data([]byte(tt.body))...)
			bad := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": wire, "FIN": tt.fin}
			good := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": h3RequestHeaders(), "FIN": true}
			q.eventID, q.eventRefs = 1, []PacketReference{{Number: 1}}
			info := map[string]any{}
			err := q.applyFrames(0, quicSpaceApplication, []map[string]any{bad, good}, info, 1024)
			require.Error(t, err, "literal DATA/FIN does not agree with declared body framing")
			require.True(t, isH3FieldStreamError(err), "message failure must not abort unrelated QUIC frames")
			require.NotEmpty(t, good["HTTP3 Frames"])
			require.Equal(t, true, info["HTTP3"])
			if tt.length == "-1" || tt.length == "+1" || tt.length == "1x" || tt.length == "1, 2" || tt.length == "" {
				hf := bad["HTTP3 Frames"].([]map[string]any)[0]
				require.Equal(t, "malformed", hf["Header Conformance Status"])
			}
		})
	}
}

func TestHTTP3RepeatedLengthAndMissingMethodStayExplicit(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	head := h3LiteralFields([2]string{":method", "POST"}, [2]string{":scheme", "https"}, [2]string{":path", "/"}, [2]string{":authority", "example.com"}, [2]string{"content-length", "0004, 4"}, [2]string{"content-length", "4"})
	fr, _, err := h3BodyTestFeed(q, 0, 0, 0, append(head, h3Data([]byte("data"))...), true)
	require.NoError(t, err)
	m := fr["HTTP3 Message Conformance"].(map[string]any)
	require.Equal(t, "4", m["Declared Content-Length"])
	require.Equal(t, "matched", m["Length Check"])
	require.NotEmpty(t, m["Length Normalization"])
	q = &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	fr, _, err = h3BodyTestFeed(q, 1, 0, 0, h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "10"}), true)
	require.NoError(t, err, "missing request method cannot disprove a legitimate HEAD response")
	require.Equal(t, "unverified-request-method; HEAD response cannot be excluded", fr["HTTP3 Message Conformance"].(map[string]any)["Length Check"])
	before := q.initialMemory()
	q.streams[0].h3.body[1].length = string(make([]byte, 10000))
	require.GreaterOrEqual(t, q.initialMemory()-before, int64(9998), "canonical decimal state is charged to the native session budget")
}

func TestHTTP3ContentLengthWaitsForMissingStreamBytesBeforeFIN(t *testing.T) {
	for _, length := range []string{"6", "7"} {
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
		head := h3BodyRequest("POST", length)
		_, _, err := h3BodyTestFeed(q, 0, 0, 0, head, false)
		require.NoError(t, err)
		body := h3Data([]byte("abcdef"))
		_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(head)+4), body[4:], true)
		require.NoError(t, err, "observed FIN beyond an unfilled gap does not prove a short HTTP body")
		fr, _, err := h3BodyTestFeed(q, 0, 0, uint64(len(head)), body[:4], false)
		if length == "7" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.NotNil(t, fr["HTTP3 Message Conformance"])
			check := fr["HTTP3 Message Conformance"].(map[string]any)
			require.Equal(t, "matched", check["Length Check"])
			require.Equal(t, uint64(6), check["Observed DATA Bytes"])
		}
	}
}

func TestHTTP3ContentLengthExceptionsAndDirectionIsolation(t *testing.T) {
	for _, tt := range []struct {
		name, method, status, length, body string
		wantError                          bool
	}{
		{"HEAD-representation-length", "HEAD", "200", "17", "", false},
		{"HEAD-with-content", "HEAD", "200", "1", "x", true},
		{"304-representation-length", "GET", "304", "17", "", false},
		{"304-with-content", "GET", "304", "1", "x", true},
		{"204-no-content", "GET", "204", "-", "", false},
		{"204-forbidden-length-zero", "GET", "204", "0", "", true},
		{"204-with-content", "GET", "204", "-", "x", true},
		{"informational-forbidden-length", "GET", "103", "0", "", true},
		{"FIN-without-final-response", "GET", "103", "-", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, _, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest(tt.method, "-"), true)
			require.NoError(t, err)
			headers := [][2]string{{":status", tt.status}}
			if tt.length != "-" {
				headers = append(headers, [2]string{"content-length", tt.length})
			}
			wire := h3LiteralFields(headers...)
			if tt.body != "" {
				wire = append(wire, h3Data([]byte(tt.body))...)
			}
			fr, _, err := h3BodyTestFeed(q, 1, 0, 0, wire, true)
			if tt.wantError {
				require.Error(t, err)
				require.True(t, isH3FieldStreamError(err))
			} else {
				require.NoError(t, err)
				require.NotNil(t, fr["HTTP3 Message Conformance"])
			}
		})
	}
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	fr, _, err := h3BodyTestFeed(q, 0, 0, 0, append(h3BodyRequest("POST", "3"), h3Data([]byte("abc"))...), true)
	require.NoError(t, err)
	require.NotNil(t, fr["HTTP3 Message Conformance"])
	require.Equal(t, uint64(3), fr["HTTP3 Message Conformance"].(map[string]any)["Observed DATA Bytes"])
	fr, _, err = h3BodyTestFeed(q, 1, 0, 0, append(h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "4"}), h3Data([]byte("abcd"))...), true)
	require.NoError(t, err)
	require.NotNil(t, fr["HTTP3 Message Conformance"])
	require.Equal(t, uint64(4), fr["HTTP3 Message Conformance"].(map[string]any)["Observed DATA Bytes"])
}
