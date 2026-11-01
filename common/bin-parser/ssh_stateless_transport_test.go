package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func sshDirectString(b []byte) []byte {
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	return out
}

func sshDirectPacket(body []byte) []byte {
	pad := 8 - (5+len(body))%8
	if pad < 4 {
		pad += 8
	}
	out := make([]byte, 5+len(body)+pad)
	binary.BigEndian.PutUint32(out, uint32(len(out)-4))
	out[4] = byte(pad)
	copy(out[5:], body)
	return out
}

func sshDirectKEX() []byte {
	body := append([]byte{20}, bytes.Repeat([]byte{0x81}, 16)...)
	for _, text := range []string{"curve25519-sha256", "ssh-ed25519", "aes128-ctr", "aes128-ctr", "hmac-sha2-256", "hmac-sha2-256", "none", "none", "", ""} {
		body = append(body, sshDirectString([]byte(text))...)
	}
	body = append(body, 0, 0, 0, 0, 0)
	return sshDirectPacket(body)
}

func TestSSHStatelessTransportDoesNotSelectDHFromMessageNumber(t *testing.T) {
	key := bytes.Repeat([]byte{0xff}, 32)
	body := append([]byte{30}, sshDirectString(key)...)
	wire := sshDirectPacket(body)
	n := parseRule(t, wire, "application-layer.ssh", "SSHPacket")
	payload := mustChild(t, n, "Payload")
	require.Nil(t, payload.Child("SSHKexDHInit"))
	require.Equal(t, body[1:], bytesVal(t, payload.Child("Opaque Message Data")))
	require.Equal(t, uint64(30), uintVal(t, payload.Child("Message Number")))
	require.Equal(t, wire[len(wire)-int(wire[4]):], bytesVal(t, n.Child("Padding")))
	for _, generic := range [][]byte{wire, sshDirectPacket([]byte{21})} {
		parseMustFail(t, generic, "application-layer.ssh", "SSHKexInitPacket")
		frame := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, generic))
		require.Nil(t, mustChild(t, frame, "IP", "TCP").Child("SSHPacket"), "a service port and generic packet envelope do not establish a standalone SSH identity")
	}
}

func TestSSHStatelessKEXINITRequiresCompletePacket(t *testing.T) {
	wire := sshDirectKEX()
	for _, entry := range []string{"SSHPacket", "SSHKexInitPacket"} {
		n := parseRule(t, wire, "application-layer.ssh", entry)
		require.Equal(t, "curve25519-sha256", strVal(t, mustChild(t, n, "Payload", "Kex Algorithms")))
		require.Equal(t, bytes.Repeat([]byte{0x81}, 16), bytesVal(t, mustChild(t, n, "Payload", "Cookie")))
		for cut := range wire {
			parseMustFail(t, wire[:cut], "application-layer.ssh", entry)
		}
	}
	frame := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, wire))
	require.Equal(t, "curve25519-sha256", strVal(t, mustChild(t, frame, "IP", "TCP", "SSHPacket", "Payload", "Kex Algorithms")))
	for _, malformed := range [][]byte{mustHex(t, "0000000b041e000000010200000000"), sshDirectPacket(append([]byte{20}, make([]byte, 16)...)), append([]byte{0xff, 0xff, 0xff, 0xff}, wire[4:]...)} {
		parseMustFail(t, malformed, "application-layer.ssh", "SSHPacket")
		frame := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, malformed))
		require.Nil(t, mustChild(t, frame, "IP", "TCP").Child("SSHPacket"))
	}
}

type sshBoundedCandidate struct {
	*bytes.Reader
	bits uint64
}

func (r sshBoundedCandidate) InputBitLength() uint64 { return r.bits }

func TestSSHStatelessIdentificationRequiresWholeBoundedLine(t *testing.T) {
	for _, line := range []string{
		"SSH-2.0-OpenSSH_9.5\r\n", "SSH-1.99-example\n", "SSH-2.0-example\n",
		"SSH-2.0-example UTF-8 comment 中文\r\n",
		"SSH-2.0-" + strings.Repeat("a", 245) + "\r\n",
		"SSH-2.0-" + strings.Repeat("a", 246) + "\n",
	} {
		t.Run(fmt.Sprintf("%d-%x", len(line), []byte(line[:min(len(line), 18)])), func(t *testing.T) {
			// A later CRLF must not make an LF-only identification consume a
			// following binary packet. The enclosing boundary includes both.
			tail := append(sshDirectKEX(), '\r', '\n')
			wire := append([]byte(line), tail...)
			for _, bounded := range []bool{false, true} {
				var r io.Reader = bytes.NewReader(wire)
				if bounded {
					r = sshBoundedCandidate{bytes.NewReader(wire), uint64(len(wire) * 8)}
				}
				n, err := parser.ParseBinary(r, "application-layer.ssh", "SSH")
				require.NoError(t, err)
				require.Equal(t, uint64(len(line)*8), n.Ctx.GetUint64("pointer"))
				v, err := n.Result()
				require.NoError(t, err)
				require.Equal(t, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), strVal(t, v.Child("Identification")))
			}
			for cut := 0; cut < len(line); cut++ {
				_, err := parser.ParseBinary(sshBoundedCandidate{bytes.NewReader(wire), uint64(cut * 8)}, "application-layer.ssh", "SSH")
				require.Error(t, err, "must not read past a caller's %d-byte boundary", cut)
			}
		})
	}
	for _, line := range []string{
		"SSH-\r\n", "SSH-1.5-example\r\n", "SSH-3.0-example\r\n", "SSH-2.0-\r\n",
		"SSH-2.0- comment\r\n", "SSH-2.0-Cisco-1.25\r\n", "SSH-2.0-a\tb\r\n",
		"SSH-2.0-中文\r\n", "SSH-2.0-a \xff\r\n", "SSH-2.0-a \x00\r\n", "SSH-2.0-a \x7f\r\n",
		"SSH-2.0-" + strings.Repeat("a", 246) + "\r\n",
		"SSH-2.0-" + strings.Repeat("a", 247) + "\n",
	} {
		parseMustFail(t, []byte(line), "application-layer.ssh", "SSH")
		for _, port := range []layers.TCPPort{22, 2222} {
			frame := parseEthernet(t, ipv4TCPFrame(t, 50000, port, []byte(line)))
			tcp := mustChild(t, frame, "IP", "TCP")
			require.Nil(t, tcp.Child("SSH"), "invalid header must not establish identity on port %d", port)
		}
	}
}

func FuzzSSHStatelessCandidateBoundaries(f *testing.F) {
	for _, raw := range [][]byte{[]byte("SSH-2.0-OpenSSH_9.5\r\n"), []byte("SSH-1.99-example\n"), []byte("SSH-2.0-Cisco-1.25\r\n"), sshDirectKEX(), sshDirectPacket(append([]byte{30}, sshDirectString(bytes.Repeat([]byte{0xff}, 32))...)), sshDirectPacket([]byte{21})} {
		for _, binaryPacket := range []bool{false, true} {
			f.Add(raw, uint32(len(raw)*8), binaryPacket)
		}
	}
	f.Fuzz(func(t *testing.T, raw []byte, bits uint32, binaryPacket bool) {
		if len(raw) > 35008 {
			return
		}
		entry := "SSH"
		if binaryPacket {
			entry = "SSHPacket"
		}
		n, err := parser.ParseBinary(sshBoundedCandidate{bytes.NewReader(raw), uint64(bits)}, "application-layer.ssh", entry)
		if err != nil {
			return
		}
		used := n.Ctx.GetUint64("pointer")
		require.Zero(t, used%8)
		require.LessOrEqual(t, used, uint64(bits), "a successful candidate cannot cross the caller boundary")
		require.LessOrEqual(t, used, uint64(len(raw))*8)
		if binaryPacket {
			require.GreaterOrEqual(t, used, uint64(16*8))
			require.Equal(t, uint64(binary.BigEndian.Uint32(raw[:4]))+4, used/8)
			require.Equal(t, raw[:used/8], NodeToBytes(n), "fields must retain the complete declared packet, including padding")
			if raw[5] >= 30 && raw[5] <= 49 {
				v, err := n.Result()
				require.NoError(t, err)
				require.Equal(t, raw[6:used/8-uint64(raw[4])], bytesVal(t, mustChild(t, v, "Payload", "Opaque Message Data")), "a message number alone does not select an exchange")
			}
		} else {
			require.LessOrEqual(t, used, uint64(255*8))
			require.Equal(t, bytes.IndexByte(raw, '\n')+1, int(used/8), "an LF in following data cannot extend the first identification line")
			require.Equal(t, raw[:used/8], NodeToBytes(n), "line terminators and literal identification bytes must round-trip")
		}
	})
}
