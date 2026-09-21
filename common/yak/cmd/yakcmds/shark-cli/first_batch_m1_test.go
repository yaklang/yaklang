package sharkcli

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFirstBatchT05(t *testing.T) {
	base := "../../../../pcapx/pcaputil/testdata/protocol-sessions/first-batch-m1"
	key, err := os.ReadFile(filepath.Join(base, "tls-h2-bidi.keys"))
	require.NoError(t, err)
	keys, err := pcaputil.ParseTLSKeyLog(string(key))
	require.NoError(t, err)
	for _, name := range []string{"http-ws-native.pcap", "tls-h2-bidi.pcap", "dhcpv6-native.pcap"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(base, name)
			var expected []*pcaputil.ProtocolEvent
			require.NoError(t, pcaputil.ReplayPcapFile(path, pcaputil.WithTLSSecrets(keys), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { expected = append(expected, e) })))
			cfg := captureConfig{input: path, output: filepath.Join(t.TempDir(), "output.pcap"), snaplen: 65535, tlsSecrets: keys}
			src, err := openCapture(cfg)
			require.NoError(t, err)
			defer src.Close()
			s := startCapture(context.Background(), src, cfg, false)
			got := map[uint64]*pcaputil.ProtocolEvent{}
			var packets []*capturedPacket
			for p := range s.packets {
				packets = append(packets, p)
			}
			require.NoError(t, s.result())
			for _, p := range packets {
				for _, e := range p.protocolEvents() {
					got[e.ID] = e
				}
			}
			require.Equal(t, len(expected), len(got))
			for _, e := range expected {
				require.Contains(t, got, e.ID)
				require.Equal(t, e.Protocol, got[e.ID].Protocol)
				require.Equal(t, e.Completeness, got[e.ID].Completeness)
				require.Equal(t, e.DisplayFields(), got[e.ID].DisplayFields())
				require.Equal(t, e.ResponseTo, got[e.ID].ResponseTo)
			}
			// A display predicate consumes facts only, independently from saved records.
			f, err := pcaputil.CompileDisplayFilter(`protocol == "dns"`)
			require.NoError(t, err)
			for _, p := range packets {
				for _, e := range p.protocolEvents() {
					_ = f.Match(e)
				}
			}
			in, err := os.Open(path)
			require.NoError(t, err)
			defer in.Close()
			out, err := os.Open(cfg.output)
			require.NoError(t, err)
			defer out.Close()
			r, err := pcaputil.NewCaptureReader(in)
			require.NoError(t, err)
			w, err := pcaputil.NewCaptureReader(out)
			require.NoError(t, err)
			for {
				a, ci, err := r.ReadPacketData()
				b, co, err2 := w.ReadPacketData()
				if err == io.EOF {
					require.ErrorIs(t, err2, io.EOF)
					break
				}
				require.NoError(t, err)
				require.NoError(t, err2)
				require.Equal(t, a, b)
				require.Equal(t, ci.Length, co.Length)
			}
		})
	}
}
