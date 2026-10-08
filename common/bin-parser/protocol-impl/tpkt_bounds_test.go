package protocol_impl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"io"

	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestTPKTEnvelopeBounds(t *testing.T) {
	dir := filepath.Join("..", "testdata", "protocol-recognition-regressions", "tpkt")
	data, err := trafficfixture.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Files []struct {
			File, SHA256, Expected string
			Bytes                  int
		}
	}
	require.NoError(t, json.Unmarshal(data, &manifest))
	require.Len(t, manifest.Files, 5)
	for _, sample := range manifest.Files {
		t.Run(sample.File, func(t *testing.T) {
			wire, err := trafficfixture.ReadFile(filepath.Join(dir, sample.File))
			require.NoError(t, err)
			require.Len(t, wire, sample.Bytes)
			hash := sha256.Sum256(wire)
			require.Equal(t, sample.SHA256, hex.EncodeToString(hash[:]))
			packet, err := ParseTpkt(bytes.NewReader(wire))
			if sample.Expected == "error" {
				require.Error(t, err)
			} else {
				require.Equal(t, "decoded", sample.Expected)
				require.NoError(t, err)
				require.Equal(t, wire[4:], packet.TPDU)
			}
		})
	}
	for _, wire := range [][]byte{{3}, {3, 0, 0}, {3, 0, 0, 7, 2, 0xf0}} {
		_, err := ParseTpkt(bytes.NewReader(wire))
		require.Error(t, err, "truncated TPKT %x", wire)
	}
	valid := []byte{3, 0, 0, 7, 2, 0xf0, 0x80}
	input := bytes.NewReader(append(bytes.Clone(valid), valid...))
	for i := 0; i < 2; i++ {
		packet, err := ParseTpkt(input)
		require.NoError(t, err)
		require.Equal(t, valid[4:], packet.TPDU)
		require.Equal(t, (1-i)*len(valid), input.Len(), "must consume one envelope only")
	}
}

func TestTPKTMarshalBounds(t *testing.T) {
	for _, size := range []int{0, 1, 2, 65532} {
		_, err := NewTpktPacket(make([]byte, size)).Marshal()
		require.Error(t, err, "TPDU length %d", size)
	}
	wrongVersion := NewTpktPacket([]byte{2, 0xf0, 0x80})
	wrongVersion.Version = 2
	_, err := wrongVersion.Marshal()
	require.Error(t, err)
	packet := NewTpktPacket(make([]byte, 65531))
	wire, err := packet.Marshal()
	require.NoError(t, err)
	require.Len(t, wire, 65535)
	require.Equal(t, []byte{3, 0, 255, 255}, wire[:4])
	decoded, err := ParseTpkt(bytes.NewReader(wire))
	require.NoError(t, err)
	require.Equal(t, packet.TPDU, decoded.TPDU)
}

type tpktPartialWriter struct {
	n   int
	err error
}

func (w tpktPartialWriter) Write(p []byte) (int, error) { return w.n, w.err }
func TestTPKTWriteReportsPartialOutput(t *testing.T) {
	packet := NewTpktPacket([]byte{2, 0xf0, 0x80})
	n, err := packet.WriteTo(tpktPartialWriter{n: 3})
	require.Equal(t, 3, n)
	require.ErrorIs(t, err, io.ErrShortWrite)
	sentinel := errors.New("writer failed")
	n, err = packet.WriteTo(tpktPartialWriter{n: 2, err: sentinel})
	require.Equal(t, 2, n)
	require.ErrorIs(t, err, sentinel)
	var buffer bytes.Buffer
	n, err = packet.WriteTo(&buffer)
	require.NoError(t, err)
	require.Equal(t, 7, n)
	require.Equal(t, []byte{3, 0, 0, 7, 2, 0xf0, 0x80}, buffer.Bytes())
}
