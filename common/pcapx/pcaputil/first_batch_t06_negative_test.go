package pcaputil

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

// The child deliberately deletes a real UDP response. Merely finding DNS still
// succeeds; the complete message/field oracle must fail. Originals stay intact.
func TestT06MissingResponseNegativeControl(t *testing.T) {
	const control = "YAK_T06_RESPONSE_NEGATIVE_CONTROL"
	if mode := os.Getenv(control); mode != "" {
		profile := t06Profiles(t)[0]
		if mode == "delete-response" {
			reader, err := NewCaptureReader(bytes.NewReader(profile.raw))
			require.NoError(t, err)
			var out bytes.Buffer
			writer := pcapgo.NewWriter(&out)
			require.NoError(t, writer.WriteFileHeader(65535, reader.LinkType()))
			removed := false
			for {
				wire, info, err := reader.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				packet := gopacket.NewPacket(wire, reader.LinkType(), gopacket.NoCopy)
				if udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP); ok && !removed && len(udp.Payload) >= 4 && udp.Payload[2]&0x80 != 0 {
					removed = true
					continue
				}
				require.NoError(t, writer.WritePacket(info, wire))
			}
			require.True(t, removed)
			profile.raw = out.Bytes()
		}
		found := false
		options := append([]CaptureOption{}, profile.options...)
		options = append(options, WithBinParser(func(event *ProtocolEvent) { found = found || event.Protocol == "dns" }))
		require.NoError(t, ReplayPcap(bytes.NewReader(profile.raw), options...))
		require.True(t, found, "legacy found-protocol assertion")
		t.Log("legacy found-protocol assertion passed; checking complete response oracle")
		t06Replay(t, profile, 1, "full")
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, mode := range []string{"original", "delete-response"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, executable, "-test.run=^TestT06MissingResponseNegativeControl$", "-test.v")
			child.Env = append(os.Environ(), control+"="+mode)
			output, err := child.CombinedOutput()
			require.NoError(t, ctx.Err(), string(output))
			require.Contains(t, string(output), "legacy found-protocol assertion passed")
			if mode == "original" {
				require.NoError(t, err, string(output))
			} else {
				var failed *exec.ExitError
				require.ErrorAs(t, err, &failed, string(output))
				require.Equal(t, 1, failed.ExitCode(), string(output))
				require.Contains(t, string(output), "Not equal", "must fail the complete oracle, not startup")
			}
		})
	}
}
