package pcaputil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func pimTestValues(snapshot DisplayFieldSnapshot, name string) []DisplayFieldValue {
	var values []DisplayFieldValue
	for _, value := range snapshot.Values {
		if value.Name == name {
			values = append(values, value)
		}
	}
	return values
}

func TestPIMIdenticalRepeatedAddressesOwnDifferentExactRanges(t *testing.T) {
	wire := []byte{0x20, 0, 0, 0, 0, 24, 0, 12, 1, 0, 192, 0, 2, 1, 1, 0, 192, 0, 2, 1}
	events := pimReplay(t, pimControlFrame(t, wire, false))
	require.Len(t, events, 1)
	event := events[0]
	values := pimTestValues(event.DisplayFieldValues(), "pim.address_list")
	require.Len(t, values, 2)
	for i, offset := range []int{10, 16} {
		require.Equal(t, "192.0.2.1", values[i].Value)
		require.Equal(t, &DisplayFieldRange{Offset: offset, Length: 4}, values[i].Range)
		require.Equal(t, []byte{192, 0, 2, 1}, event.Raw[offset:offset+4])
	}
	families := pimTestValues(event.DisplayFieldValues(), "pim.addr_address_family")
	require.Len(t, families, 2)
	require.Equal(t, &DisplayFieldRange{Offset: 8, Length: 1}, families[0].Range)
	require.Equal(t, &DisplayFieldRange{Offset: 14, Length: 1}, families[1].Range)

	before := event.DisplayFieldValues()
	event.Protocol, event.Status, event.Completeness = "tls", "limited", "evicted"
	event.ID, event.FlowID, event.Length = 999, 777, 1
	event.Source, event.Destination = "forged", "forged"
	event.Domain.Interface++
	event.SourceBytes.SHA256 = "forged"
	event.Session = map[string]any{"Options": []map[string]any{{"Address": "203.0.113.99"}}}
	event.Fields = map[string]any{"Version": uint64(1)}
	event.Raw = bytes.Repeat([]byte{0xff}, 30)
	require.Equal(t, before, event.DisplayFieldValues(), "public caches do not replace ingress-owned occurrences or their source identity")

	returned := event.DisplayFieldValues()
	for _, field := range returned.Values {
		if field.Range != nil {
			field.Range.Offset = 0
		}
	}
	returned.Source.PacketRefs[0].Number++
	require.Equal(t, before, event.DisplayFieldValues(), "returned spans and packet refs are independent owned copies")
}

func TestPIMJoinAndPruneAliasesBindTheirOwnRepeatedSource(t *testing.T) {
	wire := []byte{0x23, 0, 0, 0, 1, 0, 192, 0, 2, 2, 0, 1, 0, 210,
		1, 0, 0, 32, 239, 1, 2, 3, 0, 1, 0, 1,
		1, 0, 4, 32, 192, 0, 2, 10, 1, 0, 7, 32, 192, 0, 2, 10}
	event := pimReplay(t, pimControlFrame(t, wire, false))[0]
	snapshot := event.DisplayFieldValues()
	for name, offsets := range map[string][]int{"pim.source": {30, 38}, "pim.join_ip": {30}, "pim.prune_ip": {38}} {
		values := pimTestValues(snapshot, name)
		require.Len(t, values, len(offsets), name)
		for i, offset := range offsets {
			require.Equal(t, "192.0.2.10", values[i].Value)
			require.Equal(t, &DisplayFieldRange{Offset: offset, Length: 4}, values[i].Range, name)
		}
	}
	rpt := pimTestValues(snapshot, "pim.source_addr.flags.r")
	require.Len(t, rpt, 2)
	require.Equal(t, false, rpt[0].Value)
	require.Equal(t, true, rpt[1].Value)
	require.Equal(t, &DisplayFieldRange{Offset: 28, Length: 1}, rpt[0].Range)
	require.Equal(t, &DisplayFieldRange{Offset: 36, Length: 1}, rpt[1].Range)
}

func TestPIMRegisterVersionAndAdvertisedAddressesHaveDistinctSpans(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		event := pimReplay(t, pimTestFrame(t, v6, true, false))[0]
		version := pimTestValues(event.DisplayFieldValues(), "pim.ip_version")
		require.Len(t, version, 1)
		require.Equal(t, &DisplayFieldRange{Offset: 8, Length: 1}, version[0].Range)
		require.Equal(t, uint64(event.Raw[8]>>4), version[0].Value)
		sourceOffset, groupOffset, width := 20, 24, 4
		if v6 {
			sourceOffset, groupOffset, width = 16, 32, 16
		}
		for name, offset := range map[string]int{"pim.observed.register.source": sourceOffset, "pim.observed.register.group": groupOffset} {
			values := pimTestValues(event.DisplayFieldValues(), name)
			require.Len(t, values, 1)
			require.Equal(t, &DisplayFieldRange{Offset: offset, Length: width}, values[0].Range)
		}
		checksum := pimTestValues(event.DisplayFieldValues(), "pim.observed.checksum_verified")
		require.Len(t, checksum, 1)
		require.Nil(t, checksum[0].Range, "computed integrity is not a raw boolean byte")
	}
}

func TestPIMNativePartialTypedFieldsAlwaysHaveAnOwnedWireSpan(t *testing.T) {
	wire := []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105, 0, 2, 0, 4, 0x80, 5, 0, 10,
		0, 24, 0, 12, 1, 0, 192, 0, 2, 1, 1, 0, 192, 0, 2, 1}
	for cut := 4; cut <= len(wire); cut++ {
		event := pimReplay(t, pimControlFrame(t, wire[:cut], false))[0]
		for _, field := range event.DisplayFieldValues().Values {
			if !strings.HasPrefix(field.Name, "pim.") || field.Origin != "pdu" {
				continue
			}
			require.NotNil(t, field.Range, "cut%d %s", cut, field.Name)
			require.GreaterOrEqual(t, field.Range.Offset, 0)
			require.Positive(t, field.Range.Length)
			require.LessOrEqual(t, field.Range.Offset+field.Range.Length, len(event.Raw))
		}
	}
}
