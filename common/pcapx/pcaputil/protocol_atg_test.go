package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const atgTestValues = "447A00004478C00043FA0000422800003FC00000C080000000000000"

func atgTestResponse(selector string, records string) []byte {
	w := []byte("\x01i201" + selector + "2610091530" + records + "&&")
	var sum uint16
	for _, b := range w {
		sum += uint16(b)
	}
	return append(w, []byte(fmt.Sprintf("%04X\x03", -sum))...)
}

func TestATGInventoryFieldsAssociationAndOwnership(t *testing.T) {
	s := &binATG{clientKnown: true, clientDir: 0, maxElements: 64}
	q := []byte("\x01i20101")
	f, e := s.consume(0, q)
	require.NoError(t, e)
	require.Equal(t, 1, f["Outstanding Requests"])
	w := atgTestResponse("01", "011000507"+atgTestValues)
	f, e = s.consume(1, w)
	require.NoError(t, e)
	for k, v := range map[string]any{"Packet Name": "In-Tank Inventory Response", "Function": "i201", "Tank Selector": 1, "Date/Time": "2610091530", "Year Two Digits": 26, "Month": 10, "Day": 9, "Hour": 15, "Minute": 30, "Tank Count": 1, "Checksum Valid": true, "Unit System": "unobserved", "Association": "matched", "Request Tank Selector": 1, "Outstanding Requests": 0} {
		require.Equal(t, v, f[k], k)
	}
	row := f["Tanks"].([]map[string]any)[0]
	for k, v := range map[string]any{"Tank Number": 1, "Product Code": "1", "Status Bits": 5, "Delivery In Progress": true, "Leak Test In Progress": false, "Invalid Fuel Height Alarm": true, "Value Count": 7, "Volume": 1000.0, "TC Volume": 995.0, "Ullage": 500.0, "Height": 42.0, "Water": 1.5, "Temperature": -4.0, "Water Volume": 0.0} {
		require.Equal(t, v, row[k], k)
	}
	for i := range w {
		w[i] = 0
	}
	q[1] = 'X'
	require.Equal(t, "1", row["Product Code"])
	require.Equal(t, "2610091530", f["Date/Time"])
	require.Zero(t, s.outstanding())
	_, e = s.consume(0, []byte("\x01i20100"))
	require.NoError(t, e)
	f, e = s.consume(1, []byte("\x019999FF1B\x03"))
	require.NoError(t, e)
	require.Equal(t, "Command Rejected", f["Packet Name"])
	require.Equal(t, "matched", f["Association"])
	require.Zero(t, s.outstanding())
}

func TestATGNegativeClosedFieldsAndPending(t *testing.T) {
	valid := atgTestResponse("01", "011000507"+atgTestValues)
	for _, w := range [][]byte{valid[:len(valid)-1], append([]byte{0}, valid[1:]...), atgTestResponse("01", "011000507"+atgTestValues[:48]+"7FC00000"), atgTestResponse("01", "011000507"+atgTestValues[:48]+"7F800000"), atgTestResponse("01", "001000507"+atgTestValues), atgTestResponse("01", "011000508"+atgTestValues), atgTestResponse("02", "011000507"+atgTestValues), atgTestResponse("00", "011000507"+atgTestValues+"011000507"+atgTestValues), atgTestResponse("01", "011000507"+atgTestValues[:40])} {
		_, e := decodeATGInventory(w, 64)
		require.Error(t, e)
	}
	wrongChecksum := bytes.Clone(valid)
	wrongChecksum[26] = '1'
	_, e := decodeATGInventory(wrongChecksum, 64)
	require.Error(t, e)
	badDate := bytes.Replace(valid, []byte("2610091530"), []byte("2602311530"), 1)
	body := badDate[:len(badDate)-5]
	var sum uint16
	for _, b := range body {
		sum += uint16(b)
	}
	copy(badDate[len(badDate)-5:], fmt.Sprintf("%04X\x03", -sum))
	_, e = decodeATGInventory(badDate, 64)
	require.Error(t, e)
	s := &binATG{clientKnown: true, clientDir: 0, maxElements: 64}
	_, e = s.consume(0, []byte("\x01i20101"))
	require.NoError(t, e)
	_, e = s.consume(1, atgTestResponse("02", "021000007"+atgTestValues))
	require.Error(t, e)
	require.Equal(t, 1, s.outstanding(), "wrong echo must not consume pending")
	_, e = s.consume(1, wrongChecksum)
	require.Error(t, e)
	require.Equal(t, 1, s.outstanding())
	_, e = s.consume(0, valid)
	require.Error(t, e)
	require.Equal(t, 1, s.outstanding(), "wrong direction must not consume pending")
	_, e = s.consume(1, valid)
	require.NoError(t, e)
	require.Zero(t, s.outstanding())
	_, e = s.consume(1, valid)
	require.Error(t, e)
	_, e = (&binATG{maxElements: 64}).consume(0, []byte("\x01i20101"))
	require.Error(t, e)
}

func TestATGBudgetFIFOAndFraming(t *testing.T) {
	s := &binATG{clientKnown: true, clientDir: 1, maxElements: 8}
	for i := 0; i < 8; i++ {
		_, e := s.consume(1, []byte(fmt.Sprintf("\x01i201%02d", i+1)))
		require.NoError(t, e)
	}
	_, e := s.consume(1, []byte("\x01i20109"))
	require.Error(t, e)
	require.Equal(t, 8, s.outstanding())
	for i := 0; i < 8; i++ {
		f, e := s.consume(0, atgTestResponse(fmt.Sprintf("%02d", i+1), fmt.Sprintf("%02d1000007", i+1)+atgTestValues))
		require.NoError(t, e)
		require.Equal(t, i+1, f["Request Tank Selector"])
	}
	require.Zero(t, s.outstanding())
	w := atgTestResponse("01", "011000507"+atgTestValues)
	_, e = decodeATGInventory(w, 1)
	require.Error(t, e)
	for i := 0; i < len(w); i++ {
		n, e := atgFrameSize(w[:i], false, 1024)
		require.NoError(t, e)
		require.Zero(t, n)
	}
	n, e := atgFrameSize(append(bytes.Clone(w), w...), false, 1024)
	require.NoError(t, e)
	require.Equal(t, len(w), n)
	_, e = atgFrameSize(w, false, len(w)-1)
	require.Error(t, e)
	n, e = atgFrameSize([]byte("\x01i20100\x01i20101"), true, 1024)
	require.NoError(t, e)
	require.Equal(t, 7, n)
	for _, size := range []int{1, 6} {
		_, e = atgFrameSize([]byte("\x01i20100"), true, size)
		require.Error(t, e)
		pe, ok := e.(*ProtocolError)
		require.True(t, ok)
		require.Equal(t, ErrResourceExceeded, pe.Kind)
	}
	n, e = atgFrameSize([]byte("\x01i20100"), true, 7)
	require.NoError(t, e)
	require.Equal(t, 7, n)
}

func TestATGNativeSplitDeferredAndPublicSession(t *testing.T) {
	wires := []sessionStep{{0, []byte("\x01i20101")}, {1, atgTestResponse("01", "011000507"+atgTestValues)}}
	for _, chunk := range []int{1, 7, 1000} {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
		require.NoError(t, e)
		var events []*ProtocolEvent
		for _, step := range wires {
			for at := 0; at < len(step.wire); {
				n := min(chunk, len(step.wire)-at)
				r := s.Feed(step.dir, time.Unix(1, 0), step.wire[at:at+n])
				events = append(events, r.Events...)
				at += n
			}
		}
		events = append(events, s.Close("test")...)
		require.Zero(t, s.Stats().BufferedBytes)
		decoded := []*ProtocolEvent{}
		for _, e := range events {
			if e.Protocol == "atg" && e.Status == "decoded" {
				decoded = append(decoded, e)
			}
		}
		require.Len(t, decoded, 2)
		require.Equal(t, "matched", decoded[1].Session["Association"])
		for _, deferred := range []bool{false, true} {
			var es []*ProtocolEvent
			var stats ProtocolStats
			pcap := sessionTestPCAP(t, wires, 41001, chunk, false, false)
			require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithTCPReassemblyWorkers(1), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Zero(t, stats.BufferedBytes)
			ds := []*ProtocolEvent{}
			for _, e := range es {
				if e.Protocol == "atg" && (e.Status == "decoded" || e.Status == "deferred") {
					ds = append(ds, e)
				}
			}
			require.Len(t, ds, 2)
			d, e := ds[1].Decode()
			require.NoError(t, e)
			f := d["fields"].(map[string]any)
			require.Equal(t, "matched", f["Association"])
			require.Equal(t, 0, f["Outstanding Requests"])
			for i := range pcap {
				pcap[i] = 0
			}
			require.Equal(t, "2610091530", f["Date/Time"])
		}
	}
}

func TestATGUnknownRoleAndWrongDirection(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, e)
	r := s.Feed(0, time.Unix(1, 0), []byte("\x01i20101"))
	es := append(r.Events, s.Close("test")...)
	for _, event := range es {
		require.False(t, event.Protocol == "atg" && event.Status == "decoded", "unknown role cannot decode a request")
	}
	require.Zero(t, s.Stats().BufferedBytes)
	state := &binATG{clientDir: 0, clientKnown: true, maxElements: 64}
	_, e = state.consume(0, []byte("\x01i20101"))
	require.NoError(t, e)
	_, e = state.consume(1, []byte("\x01i20101"))
	require.Error(t, e)
	require.Equal(t, 1, state.outstanding(), "server-origin query must not consume request")
	framed, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, e)
	framed.Feed(0, time.Unix(1, 0), []byte("\x01i20101"))
	partial := framed.Feed(1, time.Unix(1, 0), []byte("\x01i20101"))
	require.Empty(t, partial.Events, "server prefix can belong to a valid split response; wait for ETX")
	require.Equal(t, 1, framed.(*captureSession).f.atg.outstanding(), "partial wrong-direction prefix must not consume query")
	closed := framed.Close("test")
	var incomplete bool
	for _, event := range closed {
		if event.Protocol == "atg" && event.Status == "incomplete" {
			incomplete = true
		}
	}
	require.True(t, incomplete, "EOF proves this server response is incomplete")
	closeErr := sessionErrorFromEvents(closed)
	require.NotNil(t, closeErr)
	require.Equal(t, ErrNeedMore, closeErr.Kind)
	require.Zero(t, framed.Stats().BufferedBytes)
	// Midstream capture has no SYN-derived client role.
	var events []*ProtocolEvent
	pcap := binTestPcap(t, []tcpStep{{seq: 100, data: "\x01i20101"}, {seq: 200, data: string(atgTestResponse("01", "011000507"+atgTestValues)), reverse: true}}, 41001, false, true)
	require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	for _, event := range events {
		require.False(t, event.Protocol == "atg" && (event.Status == "decoded" || event.Status == "deferred"), "midstream role cannot be invented")
	}
}
