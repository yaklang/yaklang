package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayFieldRegistryIsTypedSortedAndOwned(t *testing.T) {
	fields := RegisteredDisplayFields()
	require.Greater(t, len(fields), 60)
	previous := ""
	for _, field := range fields {
		require.Greater(t, field.Name, previous)
		previous = field.Name
		require.NotEmpty(t, field.Label)
		require.NotEmpty(t, field.Description)
		require.Contains(t, []string{"event", "pdu", "context", "derived"}, field.Origin)
		if field.Type == DisplayUnsigned {
			require.True(t, field.Bits > 0 && field.Bits <= 64)
		}
		got, ok := LookupDisplayField(field.Name)
		require.True(t, ok)
		require.Equal(t, field, got)
		filter, err := CompileDisplayFilter(field.Name)
		require.NoError(t, err)
		require.NotNil(t, filter)
	}
	d, ok := LookupDisplayField("wg.type")
	require.True(t, ok)
	require.Len(t, d.Values, 4)
	d.Values[0].Label = "edited"
	d2, _ := LookupDisplayField("wg.type")
	require.Equal(t, "Handshake Initiation", d2.Values[0].Label)
	fields[0].Name = "edited"
	require.NotEqual(t, fields[0], RegisteredDisplayFields()[0])
	_, ok = LookupDisplayField("no.such.field")
	require.False(t, ok)
}

func TestDisplayFieldProjectionHasProtocolScopeAndMissingValues(t *testing.T) {
	e := &ProtocolEvent{Protocol: "wireguard", Session: map[string]any{"Message Type": uint64(4), "Receiver Index": uint64(7), "Counter": uint64(0), "Authentication Verified": false, "Negotiated Version": uint16(0x304), "SNI": "poison.example", "Stream ID": uint32(9), "GRPC Status": "0", "DNS": map[string]any{"ID": uint16(7)}}}
	m := e.DisplayFields()
	require.Equal(t, []any{uint64(0)}, m["wg.counter"])
	require.Equal(t, []any{false}, m["wg.authentication.verified"])
	for _, key := range []string{"tls.record.authenticated", "tls.version", "tls.handshake.extensions_server_name", "http2.streamid", "grpc.status", "dns.id"} {
		require.NotContains(t, m, key)
	}
	for _, expr := range []string{`wg.authentication.verified == false`, `wg.counter == 0`, `not tls.record.authenticated`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		require.True(t, f.Match(e), expr)
	}
	for _, expr := range []string{`tls.record.authenticated == false`, `tls.version != 0`, `wg.sender != 0`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		require.False(t, f.Match(e), expr)
	}
	tls := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Negotiated Version": uint16(0), "Cipher Suite": uint16(0), "Record Version": uint16(0x303), "Authentication Verified": false}}
	require.NotContains(t, tls.DisplayFields(), "tls.version")
	require.NotContains(t, tls.DisplayFields(), "tls.handshake.ciphersuite")
	require.Equal(t, []any{uint64(0x303)}, tls.DisplayFields()["tls.record.version"])
	dns := &ProtocolEvent{Protocol: "dns", Session: map[string]any{"DNS": map[string]any{"Questions": []map[string]any{{"Name": "a.example"}}}}}
	require.NotContains(t, dns.DisplayFields(), "dns.qry.type", "missing is not a present nil field")
}

func TestDisplayFieldFilterRejectsInvalidTypeCombinations(t *testing.T) {
	for _, expr := range []string{
		`wg.counter contains "1"`, `dns.id in 192.0.2.0/24`, `ip.addr > 192.0.2.1`,
		`tls.record.authenticated > false`, `tls.record.authenticated == 0`,
		`dns.id == 65536`, `dns.id == -1`, `dns.id == 1.5`, `dns.id == 1/2`,
		`dns.id == "7"`, `pdu.id == 18446744073709551616`,
		`ip.addr == bad-address`, `ip.addr in 192.0.2.0/99`, `"dns.id" == 7`,
	} {
		_, err := CompileDisplayFilter(expr)
		require.Error(t, err, expr)
	}
	for _, expr := range []string{`dns.id == 0xffff`, `pdu.id == 18446744073709551615`, `ip.addr in 2001:db8::/32`, `protocol == "dns"`, `dns.qry.name contains "example"`} {
		_, err := CompileDisplayFilter(expr)
		require.NoError(t, err, expr)
	}
}

func TestDisplayFieldTLS13OpaqueAndAuthenticatedInnerTypesAreDistinct(t *testing.T) {
	e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Negotiated Version": uint16(0x304), "Record Type": byte(23), "Authentication Verified": false, "Inner Content Type": byte(22)}}
	m := e.DisplayFields()
	require.Equal(t, []any{uint64(23)}, m["tls.record.wire_type"])
	require.Equal(t, []any{uint64(23)}, m["tls.record.opaque_type"])
	require.NotContains(t, m, "tls.record.content_type", "encrypted outer type is not authenticated plaintext")
	e.Session["Authentication Verified"] = true
	m = e.DisplayFields()
	require.Equal(t, []any{uint64(22)}, m["tls.record.content_type"])
	for _, v := range e.DisplayFieldValues().Values {
		if v.Name == "tls.record.content_type" {
			require.Equal(t, "derived", v.Origin)
			require.Nil(t, v.Range)
		}
	}
	e.Session["Negotiated Version"] = uint16(0x303)
	m = e.DisplayFields()
	require.Equal(t, []any{uint64(23)}, m["tls.record.content_type"])
	require.NotContains(t, m, "tls.record.opaque_type")
	e.Session["Negotiated Version"] = uint16(0)
	m = e.DisplayFields()
	require.Equal(t, []any{uint64(23)}, m["tls.record.wire_type"])
	require.NotContains(t, m, "tls.record.content_type")
	require.NotContains(t, m, "tls.record.opaque_type")
}

func TestDisplayFieldUint64AndJSONNeverRoundToFloat(t *testing.T) {
	for _, value := range []uint64{1<<53 + 1, 1<<60 + 7, math.MaxUint64} {
		e := &ProtocolEvent{Protocol: "wireguard", Session: map[string]any{"Counter": value}}
		f, err := CompileDisplayFilter(fmt.Sprintf("wg.counter == %d", value))
		require.NoError(t, err)
		require.True(t, f.Match(e))
		encoded, err := json.Marshal(e)
		require.NoError(t, err)
		var restored ProtocolEvent
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&restored))
		require.True(t, f.Match(&restored))
		require.Equal(t, []any{value}, restored.DisplayFields()["wg.counter"])
		snapshotJSON, err := json.Marshal(e.DisplayFieldValues())
		require.NoError(t, err)
		var snapshot DisplayFieldSnapshot
		require.NoError(t, json.Unmarshal(snapshotJSON, &snapshot))
		require.Equal(t, []any{value}, snapshot.FieldMap()["wg.counter"], "exact_value survives a float-based JSON consumer")
		if value != math.MaxUint64 {
			wrong, err := CompileDisplayFilter(fmt.Sprintf("wg.counter == %d", value+1))
			require.NoError(t, err)
			require.False(t, wrong.Match(e))
		}
	}
	f, err := CompileDisplayFilter(`not wg.counter`)
	require.NoError(t, err)
	e := &ProtocolEvent{Protocol: "wireguard", Session: map[string]any{"Counter": float64(1<<60 + 7)}}
	s := e.DisplayFieldValues()
	require.True(t, s.Incomplete)
	require.NotContains(t, s.FieldMap(), "wg.counter")
	result := f.EvaluateEvents([]*ProtocolEvent{e})
	require.False(t, result.Complete)
	require.False(t, result.Matched)
	require.False(t, f.Match(e))
}

func TestDisplayFieldSourcesAreLogicalOwnedAndBounded(t *testing.T) {
	w := make([]byte, 148)
	w[0] = 1
	binary.LittleEndian.PutUint32(w[4:], 7)
	e := &ProtocolEvent{Protocol: "wireguard", Raw: w, ID: 9, SourceBytes: ByteSource{Kind: "reassembled", PacketRefs: []PacketReference{{Number: 4}}, ParentPDUs: []uint64{3}}, Session: map[string]any{"Message Type": uint64(1), "Sender Index": uint64(7), "Wire Field Ranges": map[string]any{"Message Type": []any{uint64(0), uint64(4)}, "Sender Index": []any{uint64(4), uint64(8)}}}}
	s := e.DisplayFieldValues()
	require.False(t, s.Incomplete)
	require.False(t, s.Truncated)
	for _, v := range s.Values {
		if v.Name == "wg.sender" {
			require.Equal(t, &DisplayFieldRange{Offset: 4, Length: 4}, v.Range)
			require.Equal(t, uint64(7), v.Value)
			v.Range.Offset = 100
		}
	}
	s.Source.PacketRefs[0].Number = 100
	s.Source.ParentPDUs[0] = 100
	require.Equal(t, uint64(4), e.SourceBytes.PacketRefs[0].Number)
	require.Equal(t, uint64(3), e.SourceBytes.ParentPDUs[0])
	for _, v := range e.DisplayFieldValues().Values {
		if v.Name == "wg.sender" {
			require.Equal(t, 4, v.Range.Offset)
		}
	}
	e.Session["Wire Field Ranges"].(map[string]any)["Sender Index"] = []any{uint64(4), uint64(10000)}
	for _, v := range e.DisplayFieldValues().Values {
		if v.Name == "wg.sender" {
			require.Nil(t, v.Range, "bad metadata cannot invent a frame span")
		}
	}
	context := &ProtocolEvent{Protocol: "ssh", Raw: w, Session: map[string]any{"Kex Algorithm": "curve25519-sha256", "Wire Field Ranges": map[string]any{"Kex Algorithm": []any{uint64(1), uint64(3)}}}}
	for _, v := range context.DisplayFieldValues().Values {
		if v.Name == "ssh.kex.selected" {
			require.Equal(t, "context", v.Origin)
			require.Nil(t, v.Range)
		}
	}
}

func TestDisplayFieldRepeatedAndPacketUnionSemantics(t *testing.T) {
	e := &ProtocolEvent{Protocol: "dns", Session: map[string]any{"DNS": map[string]any{"Questions": []map[string]any{{"Name": "a.example", "Type": uint16(1)}, {"Name": "b.example", "Type": uint16(28)}}}}}
	for _, tc := range []struct {
		expr string
		want bool
	}{{`dns.qry.type == 28`, true}, {`dns.qry.type != 1`, false}, {`not (dns.qry.type == 1)`, false}, {`dns.qry.type != 16`, true}} {
		f, err := CompileDisplayFilter(tc.expr)
		require.NoError(t, err)
		require.Equal(t, tc.want, f.Match(e))
	}
	tls := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": true}}
	http := &ProtocolEvent{Protocol: "http", Raw: []byte("GET / HTTP/1.1\r\n\r\n")}
	f, err := CompileDisplayFilter(`tls.version == 0x304 and http.request.method == "GET"`)
	require.NoError(t, err)
	require.False(t, f.Match(tls))
	require.False(t, f.Match(http))
	require.True(t, f.MatchEvents([]*ProtocolEvent{tls, http}))
	ip := &ProtocolEvent{Source: "[2001:0DB8:0:0::1]:10", Destination: "[2001:db8::2]:20"}
	f, err = CompileDisplayFilter(`ip.addr == 2001:db8::1 and ip.addr in 2001:db8::/32`)
	require.NoError(t, err)
	require.True(t, f.Match(ip))
}

func TestDisplayFieldProjectionBudgetCannotBecomeNegativeEvidence(t *testing.T) {
	questions := make([]map[string]any, maxDisplayFieldValues+1)
	for i := range questions {
		questions[i] = map[string]any{"Name": "a.example", "Type": uint16(1)}
	}
	e := &ProtocolEvent{Protocol: "dns", Session: map[string]any{"DNS": map[string]any{"Questions": questions}}}
	s := e.DisplayFieldValues()
	require.True(t, s.Truncated)
	require.LessOrEqual(t, len(s.Values), maxDisplayFieldValues)
	require.Equal(t, []any{"dns"}, s.FieldMap()["protocol"], "event metadata survives projection limits")
	f, err := CompileDisplayFilter(`not tls.version`)
	require.NoError(t, err)
	result := f.EvaluateEvents([]*ProtocolEvent{e})
	require.False(t, result.Complete)
	require.Contains(t, result.Reason, "budget")
	require.False(t, f.Match(e))
}

func TestDisplayFieldInvalidScalarsAndPacketUnionHaveExplicitLimits(t *testing.T) {
	f, err := CompileDisplayFilter(`not wg.counter`)
	require.NoError(t, err)
	for _, value := range []any{math.NaN(), math.Inf(1), float64(-1), float64(0.5), "18446744073709551616"} {
		e := &ProtocolEvent{Protocol: "wireguard", Session: map[string]any{"Counter": value}}
		require.True(t, e.DisplayFieldValues().Incomplete)
		require.False(t, f.EvaluateEvents([]*ProtocolEvent{e}).Complete)
	}
	e := &ProtocolEvent{Protocol: "ssh", Session: map[string]any{"Identification": string([]byte{255})}}
	require.True(t, e.DisplayFieldValues().Incomplete, "invalid UTF-8 cannot silently change during JSON export")
	packets := make([]*ProtocolEvent, maxDisplayFieldValues)
	for i := range packets {
		packets[i] = &ProtocolEvent{ID: uint64(i + 1), Protocol: "dns"}
	}
	result := f.EvaluateEvents(packets)
	require.False(t, result.Complete)
	require.Contains(t, result.Reason, "union budget")
	f, err = CompileDisplayFilter(`ip.addr in ::ffff:192.0.2.0/120`)
	require.NoError(t, err)
	require.True(t, f.Match(&ProtocolEvent{Source: "192.0.2.1:22"}))
}

func FuzzTypedDisplayFieldProjection(f *testing.F) {
	f.Add(`wg.counter == 18446744073709551615`, uint64(18446744073709551615))
	f.Add(`dns.id contains "x"`, uint64(0))
	f.Fuzz(func(t *testing.T, expr string, counter uint64) {
		if len(expr) > 8192 {
			return
		}
		e := &ProtocolEvent{Protocol: "wireguard", Session: map[string]any{"Counter": counter}}
		filter, err := CompileDisplayFilter(expr)
		if err == nil {
			_ = filter.Match(e)
		}
		encoded, err := json.Marshal(e.DisplayFieldValues())
		require.NoError(t, err)
		require.False(t, strings.Contains(string(encoded), "NaN"))
	})
}
