package stream_parser

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func tlsServerHelloTestHandshake(extensions []byte, include bool) []byte {
	body := append([]byte{3, 3}, make([]byte, 32)...)
	body = append(body, 0, 0xc0, 0x2f, 0)
	if include {
		body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
		body = append(body, extensions...)
	}
	n := len(body)
	return append([]byte{2, byte(n >> 16), byte(n >> 8), byte(n)}, body...)
}

func tlsServerHelloTestExtension(typ uint16, body []byte) []byte {
	return append([]byte{byte(typ >> 8), byte(typ), byte(len(body) >> 8), byte(len(body))}, body...)
}

func tlsServerHelloTestRecord(hello, tail []byte) []byte {
	body := append(append([]byte(nil), hello...), tail...)
	return append([]byte{22, 3, 3, byte(len(body) >> 8), byte(len(body))}, body...)
}

func tlsServerHelloTestCoverage(t *testing.T, fields []tlsServerHelloField, size int) {
	t.Helper()
	var walk func([]tlsServerHelloField, int, int)
	walk = func(fs []tlsServerHelloField, start, end int) {
		at := start
		for _, f := range fs {
			require.Equal(t, at, f.Start, f.Name)
			require.GreaterOrEqual(t, f.End, f.Start, f.Name)
			if f.Type == "" {
				walk(f.Children, f.Start, f.End)
			}
			at = f.End
		}
		require.Equal(t, end, at)
	}
	walk(fields, 0, size)
}

func TestTLSServerHelloLayoutsAndExtensions(t *testing.T) {
	ext := func(typ uint16, body []byte) []byte { return tlsServerHelloTestExtension(typ, body) }
	var sct []byte
	sct = append(sct, 0)
	sct = append(sct, make([]byte, 32)...)
	sct = append(sct, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 4, 3, 0, 2, 0x30, 0)
	sctList := append([]byte{0, byte(len(sct) + 2), 0, byte(len(sct))}, sct...)
	extensions := []byte{}
	for _, e := range [][]byte{ext(0, nil), ext(11, []byte{2, 0, 2}), ext(15, []byte{2}), ext(16, []byte{0, 3, 2, 'h', '2'}), ext(18, sctList), ext(23, nil), ext(35, nil), ext(65281, []byte{2, 1, 2}), ext(65000, []byte{0xde, 0xad})} {
		extensions = append(extensions, e...)
	}
	keyshare := ext(51, append([]byte{0, 29, 0, 32}, make([]byte, 32)...))
	v13 := append(ext(43, []byte{3, 4}), keyshare...)
	for name, hello := range map[string][]byte{
		"no extensions":       tlsServerHelloTestHandshake(nil, false),
		"empty extensions":    tlsServerHelloTestHandshake(nil, true),
		"legacy extensions":   tlsServerHelloTestHandshake(extensions, true),
		"TLS13 keyshare":      tlsServerHelloTestHandshake(v13, true),
		"TLS13 psk":           tlsServerHelloTestHandshake(append(ext(41, []byte{0, 1}), ext(43, []byte{3, 4})...), true),
		"unknown SCT version": tlsServerHelloTestHandshake(ext(18, []byte{0, 3, 0, 1, 7}), true),
	} {
		t.Run(name, func(t *testing.T) {
			for _, record := range []bool{false, true} {
				wire := hello
				if record {
					wire = tlsServerHelloTestRecord(hello, []byte{11, 0, 0, 0, 14, 0})
				}
				fields, info, err := decodeTLSServerHello(wire, record)
				require.NoError(t, err)
				tlsServerHelloTestCoverage(t, fields, len(wire))
				require.Equal(t, record, info["ParsedFirstHandshakeOnly"])
				require.Equal(t, false, info["Handshake Completion Validated"])
				if record {
					require.Equal(t, 6, info["Following Handshake Byte Count"])
				}
				for cut := 0; cut < len(wire); cut++ {
					_, _, err = decodeTLSServerHello(wire[:cut], record)
					require.Error(t, err, "cut %d", cut)
				}
				_, _, err = decodeTLSServerHello(append(append([]byte(nil), wire...), 0), record)
				require.Error(t, err)
			}
		})
	}
}

func TestTLSServerHelloRejectsAndResourceBounds(t *testing.T) {
	ext := tlsServerHelloTestExtension
	for name, e := range map[string][]byte{
		"duplicate": append(ext(23, nil), ext(23, nil)...), "empty sct": ext(18, []byte{0, 0}),
		"truncated sct": ext(18, []byte{0, 3, 0, 1, 0}), "sct trailing": ext(18, []byte{0, 4, 0, 1, 7, 0}),
		"ALPN empty": ext(16, []byte{0, 1, 0}), "ALPN two": ext(16, []byte{0, 4, 1, 'a', 1, 'b'}),
		"ALPN short": ext(16, []byte{0, 3, 2, 'a'}), "point empty": ext(11, []byte{0}),
		"renegotiation length": ext(65281, []byte{1}), "heartbeat invalid": ext(15, []byte{0}),
		"heartbeat tail": ext(15, []byte{1, 0}), "server name nonempty": ext(0, []byte{0}),
		"ticket nonempty": ext(35, []byte{0}), "ems nonempty": ext(23, []byte{0}),
		"bad supported version": ext(43, []byte{3, 3}), "zero supported version": ext(43, []byte{0, 0}), "supported version short": ext(43, []byte{3}),
		"keyshare without version": ext(51, []byte{0, 29, 0, 1, 3}), "keyshare empty": append(ext(43, []byte{3, 4}), ext(51, []byte{0, 29, 0, 0})...),
		"TLS13 missing key or psk": ext(43, []byte{3, 4}),
		"TLS13 legacy extension":   append(append(ext(43, []byte{3, 4}), ext(41, []byte{0, 0})...), ext(23, nil)...),
		"extension envelope short": {0, 0, 0}, "extension value short": {0xff, 0x00, 0, 2, 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := decodeTLSServerHello(tlsServerHelloTestHandshake(e, true), false)
			require.Error(t, err)
		})
	}
	good := tlsServerHelloTestHandshake(nil, false)
	for at, value := range map[int]byte{0: 1, 1: 1, 4: 2, 5: 2, 38: 33} {
		wire := append([]byte(nil), good...)
		wire[at] = value
		_, _, err := decodeTLSServerHello(wire, false)
		require.Error(t, err, "at %d", at)
	}
	hrr, _ := hex.DecodeString("cf21ad74e59a6111be1d8c021e65b891c2a211167abb8c5e079e09e2c8a8339c")
	copy(good[6:38], hrr)
	_, _, err := decodeTLSServerHello(good, false)
	require.ErrorContains(t, err, "HelloRetryRequest")
	v13 := tlsServerHelloTestHandshake(append(ext(43, []byte{3, 4}), ext(41, []byte{0, 0})...), true)
	v13[41] = 1
	_, _, err = decodeTLSServerHello(v13, false)
	require.ErrorContains(t, err, "compression")
	v13[41] = 0
	record := tlsServerHelloTestRecord(v13, nil)
	record[2] = 1
	_, _, err = decodeTLSServerHello(record, true)
	require.ErrorContains(t, err, "legacy version")
	record = tlsServerHelloTestRecord(tlsServerHelloTestHandshake(nil, false), nil)
	for _, at := range []int{0, 1, 3, 4} {
		bad := append([]byte(nil), record...)
		bad[at] ^= 0x40
		_, _, err = decodeTLSServerHello(bad, true)
		require.Error(t, err)
	}
	// Exact mathematical maxima are admitted; one byte over is rejected before decoding.
	large := ext(65000, make([]byte, 65531))
	hello := tlsServerHelloTestHandshake(large, true)
	hello = append(append(append([]byte(nil), hello[:39]...), make([]byte, 32)...), hello[39:]...)
	hello[38] = 32
	n := len(hello) - 4
	hello[1] = byte(n >> 16)
	hello[2] = byte(n >> 8)
	hello[3] = byte(n)
	require.Len(t, hello, tlsServerHelloMaxHandshake)
	fields, _, err := decodeTLSServerHello(hello, false)
	require.NoError(t, err)
	tlsServerHelloTestCoverage(t, fields, len(hello))
	_, _, err = decodeTLSServerHello(append(hello, 0), false)
	require.Error(t, err)
	record = tlsServerHelloTestRecord(tlsServerHelloTestHandshake(nil, false), make([]byte, 16384-42))
	require.Len(t, record, tlsServerHelloMaxRecord)
	_, _, err = decodeTLSServerHello(record, true)
	require.NoError(t, err)
	_, _, err = decodeTLSServerHello(append(record, 0), true)
	require.Error(t, err)
	for _, count := range []int{1024, 1025} {
		var entries []byte
		for i := 0; i < count; i++ {
			entries = append(entries, ext(uint16(2000+i), nil)...)
		}
		_, _, err = decodeTLSServerHello(tlsServerHelloTestHandshake(entries, true), false)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "1024")
		}
		list := make([]byte, 2+count*3)
		binary.BigEndian.PutUint16(list, uint16(count*3))
		for at := 2; at < len(list); at += 3 {
			list[at+1] = 1
			list[at+2] = 7
		}
		_, _, err = decodeTLSServerHello(tlsServerHelloTestHandshake(ext(18, list), true), false)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "1024")
		}
	}
}
