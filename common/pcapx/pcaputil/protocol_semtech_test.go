package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const semtechRXTest = `{"rxpk":[{"tmst":3512348611,"freq":866.349812,"chan":2,"rfch":0,"stat":1,"modu":"LORA","datr":"SF7BW125","codr":"4/6","rssi":-35,"lsnr":5.1,"size":3,"data":"AQID"}]}`
const semtechStatTest = `{"stat":{"time":"2014-01-12 08:59:28 GMT","lati":46.24,"long":3.2523,"alti":145,"rxnb":2,"rxok":2,"rxfw":2,"ackr":100,"dwnb":2,"txnb":2}}`

func semtechTestWire(kind byte, body string) []byte {
	w := []byte{2, 0x12, 0x34, kind}
	if kind == 0 || kind == 2 || kind == 5 {
		w = append(w, 1, 2, 3, 4, 5, 6, 7, 8)
	}
	return append(w, body...)
}

func TestSemtechCompleteObservedFields(t *testing.T) {
	w := semtechTestWire(0, semtechRXTest)
	f, err := decodeSemtechDatagram(w, 64)
	require.NoError(t, err)
	require.True(t, validSemtechDatagram(w))
	require.Equal(t, "PUSH_DATA", f["Packet Name"])
	require.Equal(t, "0102030405060708", f["Gateway EUI"])
	require.Equal(t, "1234", f["Token"])
	require.Equal(t, "observed", f["Context Level"])
	require.NotContains(t, f, "Association")
	p := f["RF Packets"].([]map[string]any)[0]
	for k, want := range map[string]any{"tmst": float64(3512348611), "freq": 866.349812, "chan": float64(2), "rfch": float64(0), "stat": float64(1), "modu": "LORA", "datr": "SF7BW125", "codr": "4/6", "rssi": float64(-35), "lsnr": 5.1, "size": float64(3), "data": "AQID", "Payload": []byte{1, 2, 3}} {
		require.Equal(t, want, p[k], k)
	}
	for i := range w {
		w[i] = 0
	}
	require.Equal(t, "AQID", p["data"])
	require.Equal(t, []byte{1, 2, 3}, p["Payload"])
	f, err = decodeSemtechDatagram(semtechTestWire(0, semtechStatTest), 64)
	require.NoError(t, err)
	s := f["Gateway Statistics"].(map[string]any)
	require.Equal(t, 46.24, s["lati"])
	require.Equal(t, float64(100), s["ackr"])
	for kind, name := range map[byte]string{1: "PUSH_ACK", 2: "PULL_DATA", 4: "PULL_ACK"} {
		f, err := decodeSemtechDatagram(semtechTestWire(kind, ""), 64)
		require.NoError(t, err)
		require.Equal(t, name, f["Packet Name"])
	}
	fsk := `{"rxpk":[{"tmst":4294967295,"freq":869.1,"chan":9,"rfch":1,"stat":-1,"modu":"FSK","datr":50000,"rssi":-75,"size":1,"data":"AA==","time":"2013-03-31T16:21:17.530974Z","tmms":1000}]}`
	f, err = decodeSemtechDatagram(semtechTestWire(0, fsk), 64)
	require.NoError(t, err)
	require.Equal(t, float64(50000), f["RF Packets"].([]map[string]any)[0]["datr"])
}

func TestSemtechMalformedAndUnsupportedControls(t *testing.T) {
	for _, body := range []string{
		`{"rxpk":[],"rxpk":[]}`, `{"rxpk":[{"data":"AQID","data":"AQID"}]}`, `{"rxpk":[]}`, `{"stat":null}`, `[]`, semtechRXTest + `{}`,
		strings.Replace(semtechRXTest, `"size":3`, `"size":4`, 1), strings.Replace(semtechRXTest, `"size":3,`, "", 1),
		strings.Replace(semtechRXTest, `"AQID"`, `"AB=="`, 1), strings.Replace(semtechRXTest, `"AQID"`, `"AQI"`, 1),
		strings.Replace(semtechRXTest, `3512348611`, `4294967296`, 1), strings.Replace(semtechRXTest, `3512348611`, `1.5`, 1),
		strings.Replace(semtechRXTest, `866.349812`, `1e309`, 1), strings.Replace(semtechRXTest, `"stat":1`, `"stat":2`, 1),
		strings.Replace(semtechRXTest, `"SF7BW125"`, `50000`, 1), strings.Replace(semtechRXTest, `"4/6"`, `"4/9"`, 1),
		strings.Replace(semtechRXTest, `"modu":"LORA"`, `"modu":"FSK"`, 1), strings.Replace(semtechRXTest, `"rssi":-35`, `"rssi":"-35"`, 1),
		strings.Replace(semtechStatTest, `46.24`, `90.1`, 1), strings.Replace(semtechStatTest, `"ackr":100`, `"ackr":101`, 1), strings.Replace(semtechStatTest, `"rxok":2`, `"rxok":-1`, 1),
		strings.Replace(semtechStatTest, `2014-01-12 08:59:28 GMT`, `2014-02-31 08:59:28 GMT`, 1),
	} {
		w := semtechTestWire(0, body)
		require.False(t, validSemtechDatagram(w), body)
		_, err := decodeSemtechDatagram(w, 64)
		require.Error(t, err, body)
	}
	for _, w := range [][]byte{semtechTestWire(1, "x"), semtechTestWire(2, "x"), semtechTestWire(4, "x"), semtechTestWire(0, ""), {2, 1, 2, 2}, {1, 1, 2, 1}, {2, 1, 2, 6}, semtechTestWire(3, `{"txpk":{}}`), semtechTestWire(5, "")} {
		require.False(t, validSemtechDatagram(w))
		_, err := decodeSemtechDatagram(w, 64)
		require.Error(t, err)
	}
}

func TestSemtechExactIntegerNumbers(t *testing.T) {
	for _, number := range []string{"3512348611.000000001", "-0.0000000000000000000000001", "1e-324", "4294967295.000000001", "1e2147483649", "1e-2147483649"} {
		body := strings.Replace(semtechRXTest, "3512348611", number, 1)
		_, err := decodeSemtechDatagram(semtechTestWire(0, body), 64)
		require.Error(t, err, number)
		var pe *ProtocolError
		require.True(t, errors.As(err, &pe))
		require.Equal(t, ErrMalformedMessage, pe.Kind)
	}
	for _, number := range []string{"3512348611.0", "35123486110e-1", "3.512348611e9"} {
		body := strings.Replace(semtechRXTest, "3512348611", number, 1)
		fields, err := decodeSemtechDatagram(semtechTestWire(0, body), 64)
		require.NoError(t, err, number)
		require.Equal(t, float64(3512348611), fields["RF Packets"].([]map[string]any)[0]["tmst"])
	}
	for _, number := range []string{"0e2147483649", "0.000e-2147483649"} {
		body := strings.Replace(semtechRXTest, "3512348611", number, 1)
		fields, err := decodeSemtechDatagram(semtechTestWire(0, body), 64)
		require.NoError(t, err, number)
		require.Equal(t, float64(0), fields["RF Packets"].([]map[string]any)[0]["tmst"])
	}
	for _, number := range []string{"0", "-0", "0.00e-100", "100e-2", "30.00e-1"} {
		require.True(t, semtechExactInteger(number), number)
	}
	for _, number := range []string{"0.01", "100e-3", "30.0001e-1", "1e-2147483649"} {
		require.False(t, semtechExactInteger(number), number)
	}
}

func TestSemtechBudgetAndDatagramBoundary(t *testing.T) {
	_, err := decodeSemtechDatagram(semtechTestWire(0, semtechRXTest), 2)
	require.Error(t, err)
	_, err = decodeSemtechDatagram(semtechTestWire(0, `{"x":[[[[[[[[[0]]]]]]]]]}`), 64)
	require.Error(t, err)
	_, err = decodeSemtechDatagram(semtechTestWire(0, strings.Repeat(" ", 65508)), 64)
	require.Error(t, err)
	w := semtechTestWire(0, semtechRXTest)
	for _, deferred := range []bool{false, true} {
		var es []*ProtocolEvent
		var stats ProtocolStats
		capture := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41707, 36539, w)
		require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
		require.Zero(t, stats.BufferedBytes)
		var ds []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "semtech-udp" && (e.Status == "decoded" || e.Status == "deferred") {
				ds = append(ds, e)
			}
		}
		require.Len(t, ds, 1)
		decoded, err := ds[0].Decode()
		require.NoError(t, err)
		require.Equal(t, "1234", decoded["fields"].(map[string]any)["Token"])
		split := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41707, 1700, w[:20], w[20:])
		es = nil
		require.NoError(t, ReplayPcap(bytes.NewReader(split), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
		for _, e := range es {
			require.NotEqual(t, "semtech-udp", e.Protocol, "separate UDP datagrams must not join")
		}
	}
	// Token bytes retain their observed wire order, without inferring integer endianness.
	w, _ = hex.DecodeString("02ab0101")
	f, err := decodeSemtechDatagram(w, 64)
	require.NoError(t, err)
	require.Equal(t, "ab01", f["Token"])
}
