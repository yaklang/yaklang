package base

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// An independent cursor/bit-string model exercises transactions interleaved
// with all three public read APIs. It does not use bitio or the replay journal.
func TestBitReaderBytePathAgainstBitCursor(t *testing.T) {
	for seed := int64(0); seed < 24; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			wire := make([]byte, 193)
			_, _ = rng.Read(wire)
			source := bytes.NewReader(wire)
			r := NewBitReader(struct{ io.Reader }{source})
			position, furthest := 0, 0
			var checkpoints []int
			pack := func(start, bits int) []byte {
				out := make([]byte, (bits+7)/8)
				for i := 0; i < bits; i++ {
					at := start + i
					out[i/8] = out[i/8]<<1 | (wire[at/8] >> (7 - at%8) & 1)
				}
				return out
			}
			for step := 0; step < 700; step++ {
				operation := rng.Intn(6)
				switch operation {
				case 0:
					if len(checkpoints) < 20 {
						require.NoError(t, r.Backup())
						checkpoints = append(checkpoints, position)
					}
				case 1, 2:
					if len(checkpoints) == 0 {
						if operation == 1 {
							require.Error(t, r.Recovery())
						} else {
							require.Error(t, r.PopBackup())
						}
					} else {
						if operation == 1 {
							require.NoError(t, r.Recovery())
							position = checkpoints[len(checkpoints)-1]
						} else {
							require.NoError(t, r.PopBackup())
						}
						checkpoints = checkpoints[:len(checkpoints)-1]
					}
				default:
					requested := rng.Intn(49)
					if operation == 4 {
						requested = 8
					}
					if operation == 5 {
						requested = requested / 8 * 8
					}
					available := min(requested, len(wire)*8-position)
					want := pack(position, available)
					var err error
					switch operation {
					case 3:
						var got []byte
						got, err = r.ReadBits(uint64(requested))
						if available == requested {
							require.Equal(t, want, got)
							for i := range got {
								got[i] ^= 0xff
							}
						} else {
							require.Nil(t, got)
						}
					case 4:
						var got byte
						got, err = r.ReadByte()
						if available == requested {
							require.Equal(t, want[0], got)
						}
					case 5:
						got := bytes.Repeat([]byte{0xcc}, requested/8)
						var n int
						n, err = r.Read(got)
						require.Equal(t, available/8, n)
						require.Equal(t, want[:n], got[:n])
						require.Equal(t, bytes.Repeat([]byte{0xcc}, len(got)-n), got[n:])
						for i := range got {
							got[i] ^= 0xff
						}
					}
					if available == requested {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, io.EOF)
					}
					position += available
					furthest = max(furthest, position)
				}
				require.Equal(t, len(wire)-(furthest+7)/8, source.Len(), "step %d: read-ahead/rollback moved the underlying boundary", step)
			}
		})
	}
}

type bytePathReader struct {
	bytes      []byte
	step       int
	finalError error
}

func (r *bytePathReader) Read(p []byte) (int, error) {
	if len(r.bytes) == 0 {
		return 0, r.finalError
	}
	n := copy(p, r.bytes[:min(len(r.bytes), r.step)])
	r.bytes = r.bytes[n:]
	if len(r.bytes) == 0 {
		return n, r.finalError
	}
	return n, nil
}

func TestBitReaderAlignedShortReadsAndErrorJournal(t *testing.T) {
	for _, width := range []int{1, 2, 7} {
		for _, final := range []error{io.EOF, io.ErrClosedPipe} {
			r := NewBitReader(&bytePathReader{bytes: []byte("abcdef"), step: width, finalError: final})
			require.NoError(t, r.Backup())
			out := make([]byte, 8)
			n, err := r.Read(out)
			require.Equal(t, 6, n)
			require.ErrorIs(t, err, final)
			require.Equal(t, "abcdef", string(out[:n]))
			for i := range out {
				out[i] = 0
			}
			require.NoError(t, r.Recovery())
			for _, want := range []byte("abcdef") {
				got, err := r.ReadByte()
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		}
	}
	r := NewBitReader(&bytePathReader{bytes: []byte{'x'}, step: 1, finalError: io.ErrClosedPipe})
	got, err := r.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte('x'), got)
	_, err = r.ReadByte()
	require.ErrorIs(t, err, io.ErrClosedPipe)
	r = NewBitReader(&bytePathReader{step: 1})
	_, err = r.ReadByte()
	require.ErrorIs(t, err, io.ErrNoProgress)
}
