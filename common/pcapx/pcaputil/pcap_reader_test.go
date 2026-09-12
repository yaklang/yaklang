package pcaputil

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func classicFixture(order binary.ByteOrder, nano bool, snap uint32, payloads ...[]byte) []byte {
	var out bytes.Buffer
	h := make([]byte, 24)
	magic := uint32(0xa1b2c3d4)
	if nano {
		magic = 0xa1b23c4d
	}
	order.PutUint32(h, magic)
	order.PutUint16(h[4:], 2)
	order.PutUint16(h[6:], 4)
	order.PutUint32(h[16:], snap)
	order.PutUint32(h[20:], 1)
	out.Write(h)
	for i, data := range payloads {
		order.PutUint32(h, uint32(1700000000+i))
		order.PutUint32(h[4:], 123456)
		order.PutUint32(h[8:], uint32(len(data)))
		order.PutUint32(h[12:], uint32(len(data)))
		out.Write(h[:16])
		out.Write(data)
	}
	return out.Bytes()
}

func TestClassicPcapReader(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, nano := range []bool{false, true} {
			t.Run(order.String()+"/"+map[bool]string{false: "micro", true: "nano"}[nano], func(t *testing.T) {
				// Cross both the refill boundary and the large-record fallback.
				var packets [][]byte
				for _, size := range []int{0, 63, 1460, 256<<10 - 1, 256 << 10, 256<<10 + 1, 1024, 7} {
					packets = append(packets, bytes.Repeat([]byte{byte(size)}, size))
				}
				fixture := classicFixture(order, nano, 1<<20, packets...)
				r, err := newClassicPcapReader(bytes.NewReader(fixture))
				require.NoError(t, err)
				ref, err := pcapgo.NewReader(bytes.NewReader(fixture))
				require.NoError(t, err)
				for range packets {
					want, wantCI, err := ref.ReadPacketData()
					require.NoError(t, err)
					got, gotCI, err := r.read()
					require.NoError(t, err)
					wantCI.Timestamp = wantCI.Timestamp.Local()
					require.Equal(t, wantCI, gotCI)
					require.Equal(t, want, got)
				}
				_, _, err = r.read()
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

func TestClassicPcapTruncationAndLengths(t *testing.T) {
	full := classicFixture(binary.LittleEndian, false, 65535, []byte("abcdef"))
	for end := 25; end < len(full); end++ {
		r, err := newClassicPcapReader(bytes.NewReader(full[:end]))
		require.NoError(t, err)
		_, _, err = r.read()
		require.ErrorIs(t, err, io.ErrUnexpectedEOF, "truncation at %d", end)
	}
	for _, tc := range []struct{ captured, length uint32 }{{65536, 65536}, {6, 5}, {0xffffffff, 0xffffffff}} {
		bad := bytes.Clone(full)
		binary.LittleEndian.PutUint32(bad[32:], tc.captured)
		binary.LittleEndian.PutUint32(bad[36:], tc.length)
		r, err := newClassicPcapReader(bytes.NewReader(bad))
		require.NoError(t, err)
		_, _, err = r.read()
		require.ErrorContains(t, err, "invalid pcap record length")
		require.Empty(t, r.large)
	}
	for _, snap := range []uint32{0, 0xffffffff} {
		_, err := newClassicPcapReader(bytes.NewReader(classicFixture(binary.LittleEndian, false, snap)))
		require.Error(t, err)
	}
	// A complete record header with no payload is a damaged file, not EOF.
	name := filepath.Join(t.TempDir(), "missing-payload.pcap")
	require.NoError(t, os.WriteFile(name, full[:40], 0600))
	require.Error(t, OpenPcapFile(name))
}

func TestClassicPcapNativeParityAndHandleFilter(t *testing.T) {
	source, err := os.ReadFile(makeTestCapture(t, "ipv4"))
	require.NoError(t, err)
	ref, err := pcapgo.NewReader(bytes.NewReader(source))
	require.NoError(t, err)
	var packets [][]byte
	for {
		raw, _, err := ref.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		packets = append(packets, raw)
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, nano := range []bool{false, true} {
			name := filepath.Join(t.TempDir(), "time.pcap")
			require.NoError(t, os.WriteFile(name, classicFixture(order, nano, 65535, packets...), 0600))
			var times [2][]time.Time
			for path := range times {
				opts := []CaptureOption{WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
					times[path] = append(times[path], f.Timestamp)
				})}
				if path == 1 {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				require.NoError(t, OpenPcapFile(name, opts...))
			}
			require.Equal(t, times[1], times[0])
			require.Len(t, times[0], 3)
		}
	}
	var received int
	require.NoError(t, OpenPcapFile(makeTestCapture(t, "ipv4"),
		WithNetInterfaceCreated(func(h *PcapHandleWrapper) { require.NoError(t, h.SetBPFFilter("udp")) }),
		WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) })))
	require.Zero(t, received, "exposed native handle filters must be honored")
}
