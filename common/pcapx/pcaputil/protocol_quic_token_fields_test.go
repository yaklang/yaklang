package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
)

func TestNativeQUICTokenAllVarintWidthsOwnPrivateWireFacts(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, width := range []int{1, 2, 4, 8} {
			length := 31
			if width > 1 {
				length = 300
			}
			token := bytes.Repeat([]byte{0xff, 0, 0xc3}, (length+2)/3)[:length]
			wire := versionedTestInitial(t, version, []byte("seedcid8"), []byte("seedcid8"), []byte("clientid"), token, 0, 0, width)
			es, _ := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, wire}}, 14443), 1, false, false)
			require.Len(t, es, 1)
			e := es[0]
			require.Equal(t, "decoded", e.Status)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.Equal(t, false, e.Session["Peer Identity Verified"])
			fields, err := e.GetFields()
			require.NoError(t, err)
			rule, err := binparser.ParseStructured(wire, "application-layer.quic", "QUIC")
			require.NoError(t, err)
			require.Equal(t, rule["fields"], fields, "native and rule token boundaries agree for v%x width%d", version, width)
			projection := e.DisplayFieldValues()
			require.Equal(t, []any{uint64(length)}, projection.FieldMap()["quic.token_length"])
			require.Equal(t, []any{hex.EncodeToString(token)}, projection.FieldMap()["quic.token"])
			require.Empty(t, projection.FieldMap()["quic.retry_token"])
			for _, value := range projection.Values {
				if value.Name == "quic.token_length" {
					require.Equal(t, &DisplayFieldRange{Offset: 23, Length: width}, value.Range)
				}
				if value.Name == "quic.token" {
					require.Equal(t, &DisplayFieldRange{Offset: 23 + width, Length: length}, value.Range)
				}
			}
			e.Session["Initial Token"] = []byte("forged")
			require.Equal(t, projection, e.DisplayFieldValues(), "public snapshots cannot change retained token facts")
			e.Raw[23+width] ^= 1
			require.Empty(t, e.DisplayFields()["quic.token"], "wire mutation cannot borrow original token identity")
		}
	}
}

func FuzzQUICTokenVarintExtentAndIsolation(f *testing.F) {
	f.Add([]byte{0xff, 0, 0xc3}, uint8(2), false, uint16(0))
	f.Add([]byte{}, uint8(3), true, uint16(1))
	f.Add([]byte(nil), uint8(0), true, uint16(0))
	f.Add(bytes.Repeat([]byte{0xa5}, 20481), uint8(3), false, uint16(0))
	f.Fuzz(func(t *testing.T, token []byte, encoding uint8, v2 bool, trim uint16) {
		if len(token) > 20481 {
			return
		}
		width := []int{1, 2, 4, 8}[encoding&3]
		if width == 1 && len(token) >= 64 {
			return
		}
		version, first := quicVersion1, byte(0xc0)
		if v2 {
			version, first = quicVersion2, 0xd0
		}
		wire := binary.BigEndian.AppendUint32([]byte{first}, version)
		wire = append(wire, 0, 0)
		length := binary.BigEndian.AppendUint64(nil, uint64(len(token)))[8-width:]
		length[0] |= (encoding & 3) << 6
		wire = append(wire, length...)
		wire = append(wire, token...)
		wire = append(wire, 2, 0, 0)
		if trim != 0 {
			wire = wire[:len(wire)-1-int(trim)%len(wire)]
		}
		fields, err := quicWireEnvelope(wire)
		if trim != 0 || len(token) > 20<<10 {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		h, err := quicParseHeader(wire, true)
		require.NoError(t, err)
		facts, ranges := quicHeaderTokenFacts(h, wire)
		require.Equal(t, uint64(len(token)), facts["Initial Token Length"])
		require.True(t, bytes.Equal(token, facts["Initial Token"].([]byte)))
		require.Equal(t, []any{7, 7 + width}, ranges["Initial Token Length"])
		require.Equal(t, []any{7 + width, 7 + width + len(token)}, ranges["Initial Token"])
		require.NotContains(t, facts, "Retry Token")
		if len(token) > 0 {
			require.Equal(t, token, fields["QUICToken"].(map[string]any)["Token"])
			facts["Initial Token"].([]byte)[0] ^= 1
			require.Equal(t, token, wire[7+width:7+width+len(token)], "returned facts own their bytes")
		}
	})
}

func TestNativeQUICRetryTokenAndTagAreOpaqueWireFields(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		original, client, server := []byte("seedcid8"), []byte("clientid"), []byte("serverid")
		token := []byte{0xff, 0, 0xc3}
		first := versionedTestInitial(t, version, original, original, client, nil, 0, 0)
		retry := versionedTestRetry(version, original, client, server, token)
		es, _ := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, first}, {1, retry}}, 14443), 1, false, false)
		require.Len(t, es, 2)
		e := es[1]
		require.Equal(t, false, e.Session["Authentication Verified"])
		require.Equal(t, false, e.Session["Peer Identity Verified"])
		require.Equal(t, true, e.Session["Retry Integrity Verified"])
		fields := e.DisplayFields()
		require.Equal(t, []any{hex.EncodeToString(token)}, fields["quic.retry_token"])
		require.Equal(t, []any{hex.EncodeToString(retry[len(retry)-16:])}, fields["quic.retry_integrity_tag"])
		require.Empty(t, fields["quic.token_length"], "Retry has no Initial token length field")
		require.Empty(t, fields["quic.token"])
	}
}
