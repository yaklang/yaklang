package netstackvm

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"golang.org/x/sys/unix"
)

func captureControl(level, typ int32, payload []byte) []byte {
	b := make([]byte, unix.CmsgSpace(len(payload)))
	offset := unix.SizeofCmsghdr - 8
	if offset == 8 {
		binary.NativeEndian.PutUint64(b[:8], uint64(unix.CmsgLen(len(payload))))
	} else {
		binary.NativeEndian.PutUint32(b[:4], uint32(unix.CmsgLen(len(payload))))
	}
	binary.NativeEndian.PutUint32(b[offset:], uint32(level))
	binary.NativeEndian.PutUint32(b[offset+4:], uint32(typ))
	copy(b[unix.CmsgLen(0):], payload)
	return b
}

func captureAux(status uint32, length int) []byte {
	b := make([]byte, 20)
	binary.NativeEndian.PutUint32(b[:4], status)
	binary.NativeEndian.PutUint32(b[4:8], uint32(length))
	binary.NativeEndian.PutUint32(b[8:12], uint32(length))
	binary.NativeEndian.PutUint16(b[14:16], 14)
	return b
}

func TestLinuxCaptureAuxdataTrustBoundaries(t *testing.T) {
	raw := tcpCapture(t, 2).Data()
	ci := gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw), InterfaceIndex: 7}
	good := captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, captureAux(unix.TP_STATUS_CSUMNOTREADY, len(raw)))
	for _, tc := range []struct {
		name       string
		oob        []byte
		flags      int
		packetType uint8
		want       bool
	}{
		{"partial", good, 0, unix.PACKET_HOST, true},
		{"validated", captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, captureAux(unix.TP_STATUS_CSUM_VALID, len(raw))), 0, unix.PACKET_HOST, true},
		{"outgoing", good, 0, unix.PACKET_OUTGOING, false},
		{"missing", nil, 0, unix.PACKET_HOST, false},
		{"malformed", good[:len(good)-5], 0, unix.PACKET_HOST, false},
		{"short-aux", captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, make([]byte, 19)), 0, unix.PACKET_HOST, false},
		{"duplicate-aux", append(append([]byte(nil), good...), good...), 0, unix.PACKET_HOST, false},
		{"control-truncated", good, unix.MSG_CTRUNC, unix.PACKET_HOST, false},
		{"packet-truncated", good, unix.MSG_TRUNC, unix.PACKET_HOST, false},
		{"length-mismatch", captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, captureAux(unix.TP_STATUS_CSUMNOTREADY, len(raw)+1)), 0, unix.PACKET_HOST, false},
		{"no-checksum-status", captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, captureAux(unix.TP_STATUS_USER, len(raw))), 0, unix.PACKET_HOST, false},
		{"conflicting-status", captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, captureAux(unix.TP_STATUS_CSUM_VALID|unix.TP_STATUS_CSUMNOTREADY, len(raw))), 0, unix.PACKET_HOST, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, got, err := linuxCaptureMetadata(raw, ci, tc.oob, tc.flags, tc.packetType)
			require.NoError(t, err)
			require.Equal(t, raw, data, "offload does not rewrite checksums")
			require.Equal(t, tc.want, len(got.AncillaryData) > 0)
		})
	}
}

func TestLinuxCaptureRestoresOffloadedVLANAndTimestamp(t *testing.T) {
	raw := tcpCapture(t, 2).Data()
	ci := gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw), InterfaceIndex: 7}
	for _, vlan := range []uint16{0, 100} {
		t.Run(fmt.Sprint(vlan), func(t *testing.T) {
			aux := captureAux(unix.TP_STATUS_VLAN_VALID|unix.TP_STATUS_VLAN_TPID_VALID|unix.TP_STATUS_CSUMNOTREADY, len(raw))
			binary.NativeEndian.PutUint16(aux[16:18], vlan)
			binary.NativeEndian.PutUint16(aux[18:20], 0x88a8)
			stamp := make([]byte, 16)
			binary.NativeEndian.PutUint64(stamp[:8], 1700000000)
			binary.NativeEndian.PutUint64(stamp[8:], 123456)
			oob := append(captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, aux), captureControl(unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, stamp)...)
			data, got, err := linuxCaptureMetadata(raw, ci, oob, 0, unix.PACKET_HOST)
			require.NoError(t, err)
			require.Equal(t, time.Unix(1700000000, 123456), got.Timestamp)
			require.Equal(t, ci.Length+4, got.Length)
			require.Equal(t, got.Length, got.CaptureLength)
			packet := gopacket.NewPacket(data, layers.LinkTypeEthernet, gopacket.Default)
			require.Equal(t, vlan, packet.Layer(layers.LayerTypeDot1Q).(*layers.Dot1Q).VLANIdentifier)
			require.NotNil(t, packet.Layer(layers.LayerTypeTCP))
			require.Len(t, got.AncillaryData, 1)
			// A second offloaded tag must preserve the original inner VLAN tag.
			aux = captureAux(unix.TP_STATUS_VLAN_VALID, len(data))
			double, _, err := linuxCaptureMetadata(data, got, captureControl(unix.SOL_PACKET, unix.PACKET_AUXDATA, aux), 0, unix.PACKET_HOST)
			require.NoError(t, err)
			decoded := gopacket.NewPacket(double, layers.LinkTypeEthernet, gopacket.Default)
			var tags int
			for _, layer := range decoded.Layers() {
				if layer.LayerType() == layers.LayerTypeDot1Q {
					tags++
				}
			}
			require.Equal(t, 2, tags)
			require.NotNil(t, decoded.Layer(layers.LayerTypeTCP))
		})
	}
}

// Opt-in regression against an ordinary Linux TCP peer with TX checksum offload
// left enabled. The existing controlled-peer test also verifies no extra Accept.
func TestLinuxHalfOpenWithChecksumOffload(t *testing.T) {
	if os.Getenv("NETSTACKVM_OFFLOAD_TEST") == "" {
		t.Skip("set NETSTACKVM_OFFLOAD_TEST and LIVE_DEVICE/SOURCE/TARGET")
	}
	iface, err := net.InterfaceByName(os.Getenv("NETSTACKVM_LIVE_DEVICE"))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.ParseIP(os.Getenv("NETSTACKVM_LIVE_SOURCE")), Gateway: net.ParseIP(os.Getenv("NETSTACKVM_LIVE_GATEWAY"))})
	require.NoError(t, err)
	defer h.Close()
	// Read a separate subscription so the observed proof is independent of the
	// probe receiver. Both subscriptions share the same native capture reader.
	observer, err := NewPCAPAdaptor(iface.Name, int32(iface.MTU+256), false)
	require.NoError(t, err)
	defer observer.Close()
	target := os.Getenv("NETSTACKVM_LIVE_TARGET")
	_, err = h.ProbeSYN(ctx, target)
	require.NoError(t, err)
	for {
		select {
		case <-ctx.Done():
			t.Fatal("no kernel-confirmed partial SYN-ACK captured")
		case packet := <-observer.PacketSource():
			ip, tcp, ok := ipv4TCP(packet)
			if !ok || !tcp.SYN || !tcp.ACK || net.JoinHostPort(ip.SrcIP.String(), fmt.Sprint(uint16(tcp.SrcPort))) != target {
				continue
			}
			for _, value := range packet.Metadata().AncillaryData {
				proof, ok := value.(captureChecksumEvidence)
				if !ok || !proof.partial {
					continue
				}
				raw := append(append([]byte(nil), ip.Contents...), ip.Payload...)
				hdr := header.IPv4(raw)
				require.False(t, header.TCP(hdr.Payload()).IsChecksumValid(hdr.SourceAddress(), hdr.DestinationAddress(), 0, 0), "test requires unfinished checksum bytes")
				require.True(t, capturedTCPChecksumValid(packet, hdr))
				t.Logf("verified native offload evidence on %s, checksum=%04x", iface.Name, tcp.Checksum)
				return
			}
		}
	}
}
