package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"testing"
)

func TestDNSContextSharedCapacity(t *testing.T) {
	query := dnsOwnedQueryInput(t)
	reply := bytes.Clone(query)
	binary.BigEndian.PutUint16(reply[2:], 0x8000)
	_, dhcp := dhcpOwnedInputs(t)
	var capture []byte
	add := func(src, dst string, sp, dp uint16, ts uint32, wire []byte) {
		p := t20UDPCapture(t, net.ParseIP(src), net.ParseIP(dst), sp, dp, wire)
		binary.LittleEndian.PutUint32(p[24:], ts)
		if capture == nil {
			capture = append(capture, p[:24]...)
		}
		capture = append(capture, p[24:]...)
	}
	add("192.0.2.10", "192.0.2.53", 41707, 53, 100, query)
	add("192.0.2.11", "192.0.2.67", 68, 67, 101, dhcp[0])
	add("192.0.2.53", "192.0.2.10", 53, 41707, 102, reply)
	var events []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	a.conf.binParser.budget.MaxCollectionElements = 1
	r, err := NewCaptureReader(bytes.NewReader(capture))
	require.NoError(t, err)
	for range 3 {
		raw, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		require.NoError(t, a.Feed(raw, ci, r.LinkType()))
		clear(raw)
		a.conf.binParser.dnsMu.Lock()
		count := len(a.conf.binParser.dns.pending) + len(a.conf.binParser.lan.pending)
		a.conf.binParser.dnsMu.Unlock()
		require.LessOrEqual(t, count, 1, "separate storage must preserve the shared collection cap")
	}
	require.Len(t, events, 3)
	first, err := events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, dnsOwnedQueryFields(query), first)
	second, err := events[1].GetFields()
	require.NoError(t, err)
	require.Equal(t, dhcpOwnedFields(0), second)
	require.Zero(t, events[1].TransactionID)
	require.Zero(t, events[1].ResponseTo)
	want := dnsOwnedQueryFields(query)
	want["Flags"], want["Response"] = uint16(0x8000), true
	third, err := events[2].GetFields()
	require.NoError(t, err)
	require.Equal(t, want, third)
	require.Equal(t, events[0].ID, events[2].ResponseTo)
	require.Equal(t, events[0].ID, events[2].TransactionID)
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Len(t, events, 3)
	require.Zero(t, a.conf.binParser.buffered.Load())
}

func TestDNSContextNewConnectionAfterHistoryPressure(t *testing.T) {
	q := dnsQuery(0x7123, "generation.example", 1)
	other := bytes.Clone(q)
	binary.BigEndian.PutUint16(other[2:], 0x7124)
	fresh := bytes.Clone(q)
	binary.BigEndian.PutUint16(fresh[2:], 0x7125)
	response := bytes.Clone(fresh)
	binary.BigEndian.PutUint16(response[4:], 0x8000)
	first := sessionTestPCAP(t, []sessionStep{{0, q}, {0, other}}, 53, 0, false, false)
	second := sessionTestPCAP(t, []sessionStep{{0, fresh}, {1, response}}, 53, 0, false, false)
	require.Equal(t, first[:24], second[:24])
	for at := 24; at < len(second); {
		sec := binary.LittleEndian.Uint32(second[at:])
		binary.LittleEndian.PutUint32(second[at:], sec+1000)
		n := int(binary.LittleEndian.Uint32(second[at+8:]))
		at += 16 + n
	}
	capture := append(bytes.Clone(first), second[24:]...)
	var events []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) {
		if e.Protocol == "dns" {
			events = append(events, e)
		}
	}))
	require.NoError(t, err)
	a.conf.binParser.budget.MaxCollectionElements = 1
	r, err := NewCaptureReader(bytes.NewReader(capture))
	require.NoError(t, err)
	for {
		raw, ci, er := r.ReadPacketData()
		if er == io.EOF {
			break
		}
		require.NoError(t, er)
		require.NoError(t, a.Feed(raw, ci, r.LinkType()))
		clear(raw)
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Len(t, events, 4)
	require.Zero(t, events[1].TransactionID)
	require.NotEqual(t, events[0].FlowID, events[2].FlowID)
	require.NotZero(t, events[2].ID)
	require.Equal(t, events[2].ID, events[3].ResponseTo, "a new observed TCP generation must recover after the earlier bounded context closes")
	require.Equal(t, events[2].ID, events[3].TransactionID)
	require.Equal(t, "matched", events[3].Session["Association Status"])
	require.Equal(t, uint16(0x7125), events[2].Session["DNS"].(map[string]any)["ID"])
	require.Equal(t, uint16(0x7125), events[3].Session["DNS"].(map[string]any)["ID"])
	require.Zero(t, a.conf.binParser.buffered.Load())
}
