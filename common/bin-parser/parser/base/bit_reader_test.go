package base

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitReader(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("hello world")))
	err := reader.Backup()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reader.ReadBits(40)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello" {
		t.Fatal("read bits error")
	}
	err = reader.Recovery()
	if err != nil {
		t.Fatal(err)
	}
	res, err = reader.ReadBits(88)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello world" {
		t.Fatal("read bits error")
	}
}
func TestMultiReader(t *testing.T) {
	reader1 := bytes.NewReader([]byte("hello"))
	reader2 := bytes.NewReader([]byte(" world"))
	multiReader := NewConcatReader(reader1, reader2)

	buf := make([]byte, 20)
	n, err := multiReader.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 11, n)
	assert.Equal(t, "hello world", string(buf[:n]))
	n, err = multiReader.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}
func TestBackup(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("hello world")))
	err := reader.Backup()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reader.ReadBits(40)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello" {
		t.Fatal("read bits error")
	}
	err = reader.Recovery()
	if err != nil {
		t.Fatal(err)
	}
	res, err = reader.ReadBits(88)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "hello world" {
		t.Fatal("read bits error")
	}
}

func TestNestedBackupCommitThenOuterRollback(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("abcdef")))
	assert.NoError(t, reader.Backup())
	assert.NoError(t, reader.Backup())
	got, err := reader.ReadBits(32)
	assert.NoError(t, err)
	assert.Equal(t, "abcd", string(got))
	assert.NoError(t, reader.PopBackup())
	assert.NoError(t, reader.Recovery())

	got, err = reader.ReadBits(48)
	assert.NoError(t, err)
	assert.Equal(t, "abcdef", string(got))
}

func TestNestedBackupShortReadCommitThenOuterRollback(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte("abcdef")))
	assert.NoError(t, reader.Backup())
	assert.NoError(t, reader.Backup())
	_, err := reader.ReadBits(64)
	assert.Error(t, err)
	assert.NoError(t, reader.PopBackup())
	assert.NoError(t, reader.Recovery())

	got, err := reader.ReadBits(48)
	assert.NoError(t, err)
	assert.Equal(t, "abcdef", string(got))
}

func TestNestedBitBackupCommitThenOuterRollback(t *testing.T) {
	input := []byte{0xb6, 0x69}
	reader := NewBitReader(bytes.NewReader(input))
	assert.NoError(t, reader.Backup())

	got, err := reader.ReadBits(3)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b101}, got)

	assert.NoError(t, reader.Backup())
	got, err = reader.ReadBits(5)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b10110}, got)
	assert.NoError(t, reader.PopBackup())

	got, err = reader.ReadBits(4)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b0110}, got)
	assert.NoError(t, reader.Recovery())
	assert.Empty(t, reader.backupList)

	got, err = reader.ReadBits(16)
	assert.NoError(t, err)
	assert.Equal(t, input, got)
}

func TestNestedBitRecoveryDoesNotDuplicateParentReads(t *testing.T) {
	input := []byte{0xb6, 0x69}
	reader := NewBitReader(bytes.NewReader(input))
	assert.NoError(t, reader.Backup())

	got, err := reader.ReadBits(3)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b101}, got)

	assert.NoError(t, reader.Backup())
	got, err = reader.ReadBits(4)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b1011}, got)
	assert.NoError(t, reader.Recovery())

	got, err = reader.ReadBits(6)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0b101100}, got)
	assert.NoError(t, reader.Recovery())
	assert.Empty(t, reader.backupList)

	got, err = reader.ReadBits(16)
	assert.NoError(t, err)
	assert.Equal(t, input, got)
}

func TestBitRecoveryThenByteAlignedReads(t *testing.T) {
	input := []byte{0x00, 0x00, 0x29, 0x10, 0x00}
	reader := NewBitReader(bytes.NewReader(input))
	assert.NoError(t, reader.Backup())
	got, err := reader.ReadBits(2)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0}, got)
	assert.NoError(t, reader.Recovery())

	got, err = reader.ReadBits(8)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x00}, got)
	got, err = reader.ReadBits(16)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x29}, got)
	got, err = reader.ReadBits(16)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x10, 0x00}, got)
}

func TestUnalignedShortReadRecoveryPreservesRemainingBits(t *testing.T) {
	reader := NewBitReader(bytes.NewReader([]byte{0xb6}))

	got, err := reader.ReadBits(3)
	require.NoError(t, err)
	require.Equal(t, []byte{0b101}, got)
	require.NoError(t, reader.Backup())

	_, err = reader.ReadBits(8)
	require.Error(t, err)
	require.NoError(t, reader.Recovery())

	got, err = reader.ReadBits(5)
	require.NoError(t, err)
	require.Equal(t, []byte{0b10110}, got)
	require.Empty(t, reader.backupList)
}

type readerWithoutByteReader struct {
	reader *bytes.Reader
}

func (r *readerWithoutByteReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func newLargeBufferedReadFixture() ([]byte, *readerWithoutByteReader) {
	const payloadLength = 8192
	input := make([]byte, payloadLength+1)
	for i := range input {
		input[i] = byte(i*31 + 7)
	}
	return input, &readerWithoutByteReader{reader: bytes.NewReader(input)}
}

func TestLargeReadContinuesAfterBitIOShortRead(t *testing.T) {
	input, source := newLargeBufferedReadFixture()
	reader := NewBitReader(source)

	prefix, err := reader.ReadBits(8)
	require.NoError(t, err)
	require.Equal(t, input[:1], prefix)

	payload, err := reader.ReadBits(uint64(len(input)-1) * 8)
	require.NoError(t, err)
	require.Equal(t, input[1:], payload)
	require.Zero(t, source.reader.Len())
}

func TestLargeReadRecoveryReplaysAllShortReadChunks(t *testing.T) {
	input, source := newLargeBufferedReadFixture()
	reader := NewBitReader(source)

	_, err := reader.ReadBits(8)
	require.NoError(t, err)
	require.NoError(t, reader.Backup())

	payload, err := reader.ReadBits(uint64(len(input)-1) * 8)
	require.NoError(t, err)
	require.Equal(t, input[1:], payload)
	require.NoError(t, reader.Recovery())

	replayed, err := reader.ReadBits(uint64(len(input)-1) * 8)
	require.NoError(t, err)
	require.Equal(t, input[1:], replayed)
	require.Zero(t, source.reader.Len())
	require.ErrorContains(t, reader.Recovery(), "no backup")
}

func TestBitWriterRestorePreservesUnalignedPrefix(t *testing.T) {
	var output bytes.Buffer
	writer := NewBitWriter(&output)
	require.NoError(t, writer.WriteBits([]byte{0b101}, 3))
	checkpoint := writer.Snapshot()

	require.NoError(t, writer.WriteBits([]byte{0x69}, 8))
	require.Equal(t, []byte{0xad}, output.Bytes())
	require.Equal(t, uint8(3), writer.PreByteLen)
	require.Equal(t, uint8(0b001), writer.PreByte)

	output.Truncate(0)
	require.NoError(t, writer.Restore(checkpoint))
	require.NoError(t, writer.WriteBits([]byte{0b10010}, 5))
	require.Equal(t, []byte{0xb2}, output.Bytes())
	require.False(t, writer.PreIsBit)
}

func TestNestedRecoveryAcrossAllBitAlignments(t *testing.T) {
	input := []byte{0xb6, 0x69, 0xd3}
	for prefixLength := uint64(0); prefixLength < 16; prefixLength++ {
		for probeLength := uint64(1); probeLength <= 24-prefixLength; probeLength++ {
			reader := NewBitReader(bytes.NewReader(input))
			require.NoError(t, reader.Backup())
			_, err := reader.ReadBits(prefixLength)
			require.NoErrorf(t, err, "prefix=%d probe=%d", prefixLength, probeLength)

			require.NoError(t, reader.Backup())
			probe, err := reader.ReadBits(probeLength)
			require.NoErrorf(t, err, "prefix=%d probe=%d", prefixLength, probeLength)
			require.NoError(t, reader.Recovery())
			replayed, err := reader.ReadBits(probeLength)
			require.NoErrorf(t, err, "prefix=%d probe=%d", prefixLength, probeLength)
			require.Equalf(t, probe, replayed, "prefix=%d probe=%d", prefixLength, probeLength)

			require.NoError(t, reader.Recovery())
			all, err := reader.ReadBits(24)
			require.NoErrorf(t, err, "prefix=%d probe=%d", prefixLength, probeLength)
			require.Equalf(t, input, all, "prefix=%d probe=%d", prefixLength, probeLength)
		}
	}
}

func TestBitWriterRestoreAcrossAllBitAlignments(t *testing.T) {
	input := []byte{0xb6, 0x69, 0xd3}
	readRange := func(start, length uint64) []byte {
		reader := NewBitReader(bytes.NewReader(input))
		_, err := reader.ReadBits(start)
		require.NoError(t, err)
		result, err := reader.ReadBits(length)
		require.NoError(t, err)
		return result
	}

	for prefixLength := uint64(0); prefixLength <= 24; prefixLength++ {
		for speculativeLength := uint64(1); speculativeLength <= 17; speculativeLength++ {
			var output bytes.Buffer
			writer := NewBitWriter(&output)
			require.NoError(t, writer.WriteBits(readRange(0, prefixLength), prefixLength))
			checkpoint := writer.Snapshot()
			require.NoError(t, writer.WriteBits([]byte{0x55, 0xaa, 0x01}, speculativeLength))

			output.Truncate(int(prefixLength / 8))
			require.NoError(t, writer.Restore(checkpoint))
			remainingLength := uint64(24) - prefixLength
			require.NoError(t, writer.WriteBits(readRange(prefixLength, remainingLength), remainingLength))
			require.Equalf(t, input, output.Bytes(), "prefix=%d speculative=%d", prefixLength, speculativeLength)
			require.Falsef(t, writer.PreIsBit, "prefix=%d speculative=%d", prefixLength, speculativeLength)
		}
	}
}
