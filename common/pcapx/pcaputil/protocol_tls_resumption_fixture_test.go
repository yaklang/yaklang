package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// This oracle reads only immutable fixture bytes/expected facts. It does not
// call the production hello, PSK, metadata or handshake parsers, and never reads
// an actual ProtocolEvent to manufacture an expected value.
type tlsFrozenHello struct {
	sid      []byte
	version  uint16
	metadata map[string]any
}

func tlsFrozenOpaque(b []byte) map[string]any {
	return map[string]any{"Length": len(b), "SHA256": fmt.Sprintf("%x", sha256.Sum256(b))}
}

func tlsFrozenRows(t *testing.T, value any) []map[string]any {
	t.Helper()
	switch rows := value.(type) {
	case nil:
		return nil
	case []map[string]any:
		return rows
	case []any:
		out := make([]map[string]any, len(rows))
		for i, row := range rows {
			var ok bool
			out[i], ok = row.(map[string]any)
			require.True(t, ok)
		}
		return out
	default:
		t.Fatalf("unexpected frozen row type %T", value)
		return nil
	}
}

func tlsFrozenInt(t *testing.T, value any) int {
	t.Helper()
	switch value := value.(type) {
	case int:
		return value
	case byte:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float64:
		require.Equal(t, value, float64(int(value)))
		return int(value)
	default:
		t.Fatalf("unexpected frozen integer type %T", value)
		return 0
	}
}

func tlsFrozenHelloMetadata(t *testing.T, body []byte, client bool) tlsFrozenHello {
	t.Helper()
	require.GreaterOrEqual(t, len(body), 35)
	n := int(body[34])
	require.LessOrEqual(t, n, 32)
	at := 35 + n
	require.LessOrEqual(t, at, len(body))
	h := tlsFrozenHello{sid: bytes.Clone(body[35:at]), version: binary.BigEndian.Uint16(body), metadata: map[string]any{"Session ID": tlsFrozenOpaque(body[35:at])}}
	if client {
		require.LessOrEqual(t, at+2, len(body))
		n = int(binary.BigEndian.Uint16(body[at:]))
		at += 2
		require.GreaterOrEqual(t, n, 2)
		require.Zero(t, n%2)
		require.Less(t, at+n, len(body))
		at += n
		n = int(body[at])
		at++
		require.Greater(t, n, 0)
		require.LessOrEqual(t, at+n, len(body))
		at += n
	} else {
		require.LessOrEqual(t, at+3, len(body))
		at += 3
	}
	if at == len(body) {
		return h
	}
	require.LessOrEqual(t, at+2, len(body))
	n = int(binary.BigEndian.Uint16(body[at:]))
	at += 2
	require.Equal(t, len(body), at+n)
	seen := map[uint16]bool{}
	for at < len(body) {
		require.LessOrEqual(t, at+4, len(body))
		kind, n := binary.BigEndian.Uint16(body[at:]), int(binary.BigEndian.Uint16(body[at+2:]))
		at += 4
		require.False(t, seen[kind])
		seen[kind] = true
		require.LessOrEqual(t, at+n, len(body))
		value := body[at : at+n]
		at += n
		switch kind {
		case 43:
			if !client {
				require.Len(t, value, 2)
				h.version = binary.BigEndian.Uint16(value)
			}
		case 45:
			require.True(t, client)
			require.GreaterOrEqual(t, len(value), 2)
			require.Equal(t, int(value[0]), len(value)-1)
			h.metadata["PSK Key Exchange Modes Length"], h.metadata["PSK Key Exchange Modes"] = len(value)-1, bytes.Clone(value[1:])
		case 35:
			if client && len(value) > 0 {
				h.metadata["Session Ticket Offer"] = tlsFrozenOpaque(value)
			}
		case 41:
			t.Fatal("this historical fixture needs an explicit independently reviewed PSK offer oracle")
		}
	}
	return h
}

func tlsFrozenApplyMetadata(t *testing.T, rows []map[string]any, metadata []map[string]any) {
	t.Helper()
	require.Len(t, metadata, len(rows))
	for i, row := range rows {
		for key, value := range metadata[i] {
			require.NotContains(t, row, key, "the immutable old expectation must not be replaced")
			row[key] = value
		}
	}
}

func tlsFrozenExpectedMetadata(t *testing.T, c tlsAlertCase, target tlsAlertStep, want map[string]any) map[string]any {
	t.Helper()
	if target.Wire == "" || want == nil {
		return want
	}
	var pending [2][]byte
	var client *tlsFrozenHello
	clientDir := -1
	var context map[string]any
	for _, step := range c.Steps {
		isTarget := step.Wire == target.Wire && step.At == target.At && step.Direction == target.Direction && step.Offset == target.Offset
		var metadata []map[string]any
		if step.Error == "" && step.Wire != "" {
			wire := wrapperWire(t, step.Wire)
			if len(wire) >= 5 && wire[0] == 22 && step.Fields["Content Visibility"] == "cleartext" {
				pending[step.Direction] = append(pending[step.Direction], wire[5:]...)
				oldRows := tlsFrozenRows(t, step.Fields["Handshake Messages"])
				for len(pending[step.Direction]) >= 4 {
					b := pending[step.Direction]
					size := 4 + (int(b[1]) << 16) + (int(b[2]) << 8) + int(b[3])
					if size > len(b) {
						break
					}
					i := len(metadata)
					require.Less(t, i, len(oldRows))
					require.Equal(t, int(b[0]), tlsFrozenInt(t, oldRows[i]["Type"]))
					add := map[string]any{}
					if b[0] == 1 || b[0] == 2 {
						h := tlsFrozenHelloMetadata(t, b[4:size], b[0] == 1)
						add = h.metadata
						if b[0] == 1 {
							client = &h
							clientDir = step.Direction
							context = nil
						} else if client != nil {
							require.NotEqual(t, clientDir, step.Direction)
							echo := bytes.Equal(client.sid, h.sid)
							context = map[string]any{"Evidence Scope": "current connection hello observations", "Certificate Trust": "not-evaluated", "Session ID Echo": echo}
							if h.version >= 0x300 && h.version <= 0x303 {
								context["Session ID Resumption Indicated"] = len(client.sid) > 0 && len(h.sid) > 0 && echo
							}
							if h.version == 0x304 {
								context["Compatibility Session ID Echo"], context["PSK Selected"] = echo, false
							}
						}
					}
					metadata = append(metadata, add)
					pending[step.Direction] = b[size:]
				}
				require.Len(t, metadata, len(oldRows), "literal clear handshake bytes must match the original expected message list")
			}
		}
		if isTarget {
			if len(metadata) > 0 {
				tlsFrozenApplyMetadata(t, tlsFrozenRows(t, want["Handshake Messages"]), metadata)
			}
			if context != nil {
				require.NotContains(t, want, "Session Establishment")
				want["Session Establishment"] = context
			}
			return want
		}
	}
	t.Fatal("target expectation is not bound to a literal case step")
	return nil
}

func tlsFrozenQUICMetadata(t *testing.T, session map[string]any) {
	t.Helper()
	rows := tlsFrozenRows(t, session["TLS Handshakes"])
	if len(rows) == 0 {
		return
	}
	var data []byte
	for _, frame := range tlsFrozenRows(t, session["Frames"]) {
		if frame["Frame Type"] != "CRYPTO" {
			continue
		}
		off := tlsFrozenInt(t, frame["Offset"])
		var value []byte
		switch b := frame["Crypto Data"].(type) {
		case string:
			var err error
			value, err = base64.StdEncoding.DecodeString(b)
			require.NoError(t, err)
		case []byte:
			value = bytes.Clone(b)
		default:
			t.Fatalf("missing immutable CRYPTO plaintext %T", frame["Crypto Data"])
		}
		require.Equal(t, len(data), off, "historical completed handshake must have literal contiguous plaintext")
		data = append(data, value...)
	}
	var metadata []map[string]any
	for _, row := range rows {
		require.GreaterOrEqual(t, len(data), 4)
		size := 4 + (int(data[1]) << 16) + (int(data[2]) << 8) + int(data[3])
		require.LessOrEqual(t, size, len(data))
		kind := tlsFrozenInt(t, row["Type"])
		require.Equal(t, kind, int(data[0]))
		require.Equal(t, tlsFrozenInt(t, row["Length"]), size-4)
		add := map[string]any{}
		if kind == 1 || kind == 2 {
			add = tlsFrozenHelloMetadata(t, data[4:size], kind == 1).metadata
		}
		metadata = append(metadata, add)
		data = data[size:]
	}
	tlsFrozenApplyMetadata(t, rows, metadata)
}
