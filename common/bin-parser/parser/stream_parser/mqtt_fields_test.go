package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func mqttTestVector(b []byte) []byte {
	w := make([]byte, 2, len(b)+2)
	binary.BigEndian.PutUint16(w, uint16(len(b)))
	return append(w, b...)
}

func mqttTestPacket(header byte, chunks ...[]byte) []byte {
	body := bytes.Join(chunks, nil)
	w := []byte{header}
	n := len(body)
	for {
		v := byte(n % 128)
		n /= 128
		if n != 0 {
			v |= 128
		}
		w = append(w, v)
		if n == 0 {
			break
		}
	}
	return append(w, body...)
}

func mqttTestConnect(level int, flags byte, id string, extra ...[]byte) []byte {
	name := "MQTT"
	if level == 3 {
		name = "MQIsdp"
	}
	return mqttTestPacket(0x10, append(append(mqttTestVector([]byte(name)), byte(level), flags, 0, 60), mqttTestVector([]byte(id))...), bytes.Join(extra, nil))
}

func TestMQTTFieldsLayouts(t *testing.T) {
	for _, level := range []int{3, 4} {
		fixtures := map[byte][]byte{
			1: mqttTestConnect(level, 0xce, "sample", mqttTestVector([]byte("state/device")), mqttTestVector([]byte("offline")), mqttTestVector([]byte("sample-user")), mqttTestVector([]byte("sample-value"))),
			2: {0x20, 2, 0, 0},
			3: mqttTestPacket(0x35, mqttTestVector([]byte("sensor/temp")), []byte{0, 7, 0xff, 0, 1}),
			4: {0x40, 2, 0, 1}, 5: {0x50, 2, 0xff, 0xff}, 6: {0x62, 2, 0, 1}, 7: {0x70, 2, 0, 1},
			8:  mqttTestPacket(0x82, []byte{0, 1}, mqttTestVector([]byte("sensor/+")), []byte{1}, mqttTestVector([]byte("#")), []byte{2}),
			9:  {0x90, 5, 0, 1, 0, 1, 2},
			10: mqttTestPacket(0xa2, []byte{0, 1}, mqttTestVector([]byte("sensor/+")), mqttTestVector([]byte("#"))),
			11: {0xb0, 2, 0, 1}, 12: {0xc0, 0}, 13: {0xd0, 0}, 14: {0xe0, 0},
		}
		for typ, wire := range fixtures {
			t.Run(fmt.Sprintf("%d/%d", level, typ), func(t *testing.T) {
				f, info, err := decodeMQTTFields(wire, level)
				require.NoError(t, err)
				tlsCertificateTestCoverage(t, f, len(wire))
				require.Equal(t, uint64(typ), info["Packet Type"])
				require.Equal(t, level, info["Protocol Level Context"])
				require.Equal(t, true, info["Remaining Length Canonical"])
				for cut := 0; cut < len(wire); cut++ {
					_, _, err := decodeMQTTFields(wire[:cut], level)
					require.Error(t, err)
				}
				_, _, err = decodeMQTTFields(append(bytes.Clone(wire), 0), level)
				require.Error(t, err)
			})
		}
	}
}

func TestMQTTFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"3", "4"}, func(profile string) []byte {
		return mqttTestPacket(0x32, mqttTestVector([]byte("sample/topic")), []byte{0, 1, 0, 0xff})
	}, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
		return parseMQTTFields(n, process, int(profile[0]-'0'))
	})
}

func TestMQTTFieldsVersionBoundaries(t *testing.T) {
	for _, spec := range []struct {
		wire     []byte
		ok3, ok4 bool
	}{
		{mqttTestConnect(3, 2, "sample"), true, false},
		{mqttTestConnect(4, 2, "sample"), false, true},
		{[]byte{0x6a, 2, 0, 1}, true, false},
		{mqttTestPacket(0x8a, []byte{0, 1}, mqttTestVector([]byte("a")), []byte{0}), true, false},
		{mqttTestPacket(0xaa, []byte{0, 1}, mqttTestVector([]byte("a"))), true, false},
		{[]byte{0x90, 3, 0, 1, 128}, false, true},
		{[]byte{0x20, 2, 1, 0}, false, true},
		{[]byte{0x20, 2, 1, 1}, false, false},
		{[]byte{0x20, 2, 2, 0}, false, false},
		{[]byte{0x20, 2, 0, 6}, false, false},
	} {
		for _, level := range []int{3, 4} {
			_, _, err := decodeMQTTFields(spec.wire, level)
			good := spec.ok3
			if level == 4 {
				good = spec.ok4
			}
			if good {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		}
	}
	for _, level := range []int{3, 4} {
		// Missing optional legacy fields are not fabricated as empty strings.
		_, info, err := decodeMQTTFields(mqttTestConnect(level, 0xc2, "sample"), level)
		if level == 3 {
			require.NoError(t, err)
			require.Equal(t, []string{"User Name", "Password"}, info["Legacy Omitted Fields"])
		} else {
			require.Error(t, err)
		}
		for _, id := range []string{"", "abcdefghijklmnopqrstuvwx"} {
			_, _, err := decodeMQTTFields(mqttTestConnect(level, 2, id), level)
			if level == 3 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		}
		_, _, err = decodeMQTTFields(mqttTestConnect(level, 0, ""), level)
		require.Error(t, err)
	}
	// Version 4 permits binary password and Will bytes, including NUL/0xff.
	_, _, err := decodeMQTTFields(mqttTestConnect(4, 0xc6, "sample", mqttTestVector([]byte("a")), mqttTestVector([]byte{0, 0xff}), mqttTestVector([]byte("u")), mqttTestVector([]byte{0xff, 0})), 4)
	require.NoError(t, err)
}

func TestMQTTFieldsInvalidAndResourceBounds(t *testing.T) {
	for _, wire := range [][]byte{
		{0, 0}, {0xf0, 0}, {0x11, 0}, {0xc1, 0}, {0xc0, 1, 0},
		{0xc0, 0x80}, {0xc0, 0x80, 0x80, 0x80, 0x80, 0}, {0xc0, 0xff, 0xff, 0xff, 0x7f},
		{0x36, 0}, {0x38, 0}, {0x40, 2, 0, 0}, {0x82, 2, 0, 1}, {0xa2, 2, 0, 1}, {0x90, 2, 0, 1},
		mqttTestPacket(0x30, mqttTestVector(nil)),
		mqttTestPacket(0x32, mqttTestVector([]byte("a")), []byte{0, 0}),
		mqttTestPacket(0x82, []byte{0, 1}, mqttTestVector([]byte("a")), []byte{3}),
		mqttTestPacket(0x82, []byte{0, 1}, mqttTestVector([]byte("a")), []byte{128}),
		{0x90, 3, 0, 1, 3}, {0x90, 3, 0, 1, 129},
	} {
		for _, level := range []int{3, 4} {
			_, _, err := decodeMQTTFields(wire, level)
			require.Error(t, err, "%x level %d", wire, level)
		}
	}
	for _, flags := range []byte{1, 8, 32, 0x40, 0x1c} {
		_, _, err := decodeMQTTFields(mqttTestConnect(4, flags, "sample"), 4)
		require.Error(t, err)
	}
	for _, topic := range []string{"", "a#", "a/#/b", "a+", "a/+b", "a\x00b", "\xc0\x80", "\xed\xa0\x80", "\xf4\x90\x80\x80"} {
		_, _, err := decodeMQTTFields(mqttTestPacket(0x82, []byte{0, 1}, mqttTestVector([]byte(topic)), []byte{0}), 4)
		require.Error(t, err)
	}
	for _, topic := range []string{"/", "+/+/", "a/#", "#", "设备/温度", "\ufeffvalue", "a\x01b"} {
		_, _, err := decodeMQTTFields(mqttTestPacket(0x82, []byte{0, 1}, mqttTestVector([]byte(topic)), []byte{0}), 4)
		require.NoError(t, err)
	}
	for _, topic := range []string{"a/+", "a/#"} {
		_, _, err := decodeMQTTFields(mqttTestPacket(0x30, mqttTestVector([]byte(topic))), 4)
		require.Error(t, err)
	}
	// Receiver-compatible 1..4-byte length decoding retains nonminimal
	// encodings explicitly. It is not MQTT 5's stricter integer contract.
	for _, wire := range [][]byte{{0xc0, 0x80, 0}, {0xc0, 0x80, 0x80, 0}, {0xc0, 0x80, 0x80, 0x80, 0}} {
		f, info, err := decodeMQTTFields(wire, 4)
		require.NoError(t, err)
		require.Equal(t, false, info["Remaining Length Canonical"])
		tlsCertificateTestCoverage(t, f, len(wire))
	}
	for _, count := range []int{1024, 1025} {
		for _, typ := range []byte{8, 9, 10} {
			item := []byte{0}
			header := typ << 4
			if typ != 9 {
				header |= 2
				item = mqttTestVector([]byte("a"))
				if typ == 8 {
					item = append(item, 0)
				}
			}
			_, _, err := decodeMQTTFields(mqttTestPacket(header, []byte{0, 1}, bytes.Repeat(item, count)), 4)
			if count == 1024 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		}
	}
	for _, size := range []int{127, 128, 16383, 16384, mqttFieldsMaxBytes - 4, mqttFieldsMaxBytes - 3} {
		wire := mqttTestPacket(0x30, mqttTestVector([]byte("a")), bytes.Repeat([]byte{0xa5}, size-3))
		f, info, err := decodeMQTTFields(wire, 4)
		if len(wire) <= mqttFieldsMaxBytes {
			require.NoError(t, err)
			require.Equal(t, size, info["Remaining Length"])
			tlsCertificateTestCoverage(t, f, len(wire))
		} else {
			require.Error(t, err)
		}
	}
	_, _, err := decodeMQTTFields([]byte{0xc0, 0}, 5)
	require.Error(t, err)
}
