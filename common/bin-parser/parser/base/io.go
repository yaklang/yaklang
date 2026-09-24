package base

import (
	"errors"
	"fmt"
	"io"

	"github.com/icza/bitio"
)

type ConcatReader struct {
	readers []io.Reader
}

func NewConcatReader(readers ...io.Reader) *ConcatReader {
	return &ConcatReader{readers: readers}
}

func (cr *ConcatReader) Read(p []byte) (n int, err error) {
	current := 0
	allEOF := true
	for len(cr.readers) > 0 {
		if current >= len(p) {
			break
		}
		n, err = cr.readers[0].Read(p[current:])
		current += n
		if err == io.EOF {
			cr.readers = cr.readers[1:]
		} else {
			allEOF = false
		}
	}
	if allEOF {
		return current, io.EOF
	}
	return current, nil
}

type bitReaderChunk struct {
	data   []byte
	length uint64
	offset uint64
}

type bitReaderBackup struct {
	chunks []bitReaderChunk
}

func (b *bitReaderBackup) appendChunk(chunk bitReaderChunk) {
	if chunk.length == 0 {
		return
	}
	if chunk.offset == 0 && chunk.length%8 == 0 && len(b.chunks) > 0 {
		last := &b.chunks[len(b.chunks)-1]
		if last.offset == 0 && last.length%8 == 0 {
			last.data = append(last.data, chunk.data[:chunk.length/8]...)
			last.length += chunk.length
			return
		}
	}
	b.chunks = append(b.chunks, chunk)
}

type BitReader struct {
	Reader          *bitio.Reader
	backupList      []bitReaderBackup
	replay          []bitReaderChunk
	sourceBitOffset uint8
	single          [1]byte
}
type BitWriter struct {
	*bitio.Writer
	output     io.Writer
	PreIsBit   bool
	PreByte    uint8
	PreByteLen uint8
}

type BitWriterState struct {
	PreIsBit   bool
	PreByte    uint8
	PreByteLen uint8
}

func (r *BitReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.sourceBitOffset == 0 && len(r.replay) == 0 {
		n, err = r.readAligned(p)
		r.recordRead(p, uint64(n)*8)
		return n, err
	}

	data, bitsRead, err := r.readBits(uint64(len(p)) * 8)
	n = int(bitsRead / 8)
	copy(p, data[:n])
	return n, err
}

func (b *BitReader) Backup() error {
	b.backupList = append(b.backupList, bitReaderBackup{})
	return nil
}

func (b *BitReader) PopBackup() error {
	if len(b.backupList) == 0 {
		return errors.New("no backup")
	}

	topIndex := len(b.backupList) - 1
	top := b.backupList[topIndex]
	b.backupList[topIndex] = bitReaderBackup{}
	b.backupList = b.backupList[:len(b.backupList)-1]
	if len(b.backupList) > 0 {
		parent := &b.backupList[len(b.backupList)-1]
		for _, chunk := range top.chunks {
			parent.appendChunk(chunk)
		}
	}
	return nil
}

func (b *BitReader) Recovery() error {
	if len(b.backupList) == 0 {
		return errors.New("no backup")
	}

	topIndex := len(b.backupList) - 1
	top := b.backupList[topIndex]
	b.backupList[topIndex] = bitReaderBackup{}
	b.backupList = b.backupList[:topIndex]
	if len(top.chunks) == 0 {
		return nil
	}

	replay := make([]bitReaderChunk, len(top.chunks), len(top.chunks)+len(b.replay))
	copy(replay, top.chunks)
	b.replay = append(replay, b.replay...)
	return nil
}

type bitSequence struct {
	data   []byte
	length uint64
}

func (s *bitSequence) appendBit(bit byte) {
	byteIndex := s.length / 8
	if byteIndex == uint64(len(s.data)) {
		s.data = append(s.data, 0)
	}
	s.data[byteIndex] = s.data[byteIndex]<<1 | bit&1
	s.length++
}

func chunkBit(chunk *bitReaderChunk, index uint64) byte {
	fullBytes := chunk.length / 8
	if index < fullBytes*8 {
		return chunk.data[index/8] >> (7 - index%8) & 1
	}

	partialLength := chunk.length % 8
	partialIndex := index - fullBytes*8
	return chunk.data[fullBytes] >> (partialLength - partialIndex - 1) & 1
}

func (s *bitSequence) appendChunk(chunk *bitReaderChunk, length uint64) {
	start := chunk.offset
	if s.length%8 == 0 && start%8 == 0 {
		fullBitLimit := chunk.length / 8 * 8
		fullBits := length
		if start+fullBits > fullBitLimit {
			fullBits = fullBitLimit - start
		}
		fullBits = fullBits / 8 * 8
		if fullBits > 0 {
			byteStart := start / 8
			byteEnd := byteStart + fullBits/8
			s.data = append(s.data, chunk.data[byteStart:byteEnd]...)
			s.length += fullBits
			start += fullBits
			length -= fullBits
		}
	}

	for i := uint64(0); i < length; i++ {
		s.appendBit(chunkBit(chunk, start+i))
	}
}

func (s *bitSequence) appendData(data []byte, length uint64) {
	chunk := bitReaderChunk{data: data, length: length}
	s.appendChunk(&chunk, length)
}

func (b *BitReader) recordRead(data []byte, length uint64) {
	if length == 0 || len(b.backupList) == 0 {
		return
	}

	top := &b.backupList[len(b.backupList)-1]
	// The journal owns this storage. Merge a full-byte read directly into it,
	// rather than allocating a temporary copy only to copy it a second time.
	if length%8 == 0 && len(top.chunks) > 0 {
		last := &top.chunks[len(top.chunks)-1]
		if last.offset == 0 && last.length%8 == 0 {
			last.data = append(last.data, data[:length/8]...)
			last.length += length
			return
		}
	}
	byteLength := (length + 7) / 8
	dataCopy := append([]byte(nil), data[:byteLength]...)
	top.appendChunk(bitReaderChunk{data: dataCopy, length: length})
}

// ReadByte has the same exact-consumption and transaction semantics as
// ReadBits(8). Unaligned/partial replay still uses the general bit reader so
// an EOF after consuming a partial octet remains recoverable.
func (b *BitReader) ReadByte() (byte, error) {
	var value byte
	if len(b.replay) > 0 {
		chunk := &b.replay[0]
		if chunk.offset%8 != 0 || chunk.offset+8 > chunk.length/8*8 {
			data, err := b.ReadBits(8)
			if err != nil {
				return 0, err
			}
			return data[0], nil
		}
		value = chunk.data[chunk.offset/8]
		chunk.offset += 8
		if chunk.offset == chunk.length {
			b.replay = b.replay[1:]
		}
	} else if b.sourceBitOffset == 0 {
		// Keep using io.Reader.Read, as ReadBits does, even if a caller supplies
		// a ByteReader with different buffering/error behavior. Scratch belongs
		// to this serial reader, never to a caller or a retained result.
		n, err := b.Reader.Read(b.single[:])
		if n == 0 {
			if err == nil {
				err = io.ErrNoProgress
			}
			return 0, err
		}
		value = b.single[0]
	} else {
		data, err := b.ReadBits(8)
		if err != nil {
			return 0, err
		}
		return data[0], nil
	}
	if len(b.backupList) > 0 {
		top := &b.backupList[len(b.backupList)-1]
		if len(top.chunks) > 0 {
			last := &top.chunks[len(top.chunks)-1]
			if last.offset == 0 && last.length%8 == 0 {
				last.data = append(last.data, value)
				last.length += 8
				return value, nil
			}
		}
		top.chunks = append(top.chunks, bitReaderChunk{data: []byte{value}, length: 8})
	}
	return value, nil
}

func (b *BitReader) readAligned(buf []byte) (int, error) {
	read := 0
	for read < len(buf) {
		n, err := b.Reader.Read(buf[read:])
		read += n
		// A reader may return the final requested bytes together with EOF.
		if read == len(buf) {
			return read, nil
		}
		if err != nil {
			return read, err
		}
		if n == 0 {
			return read, io.ErrNoProgress
		}
	}
	return read, nil
}

func (b *BitReader) readReplay(sequence *bitSequence, length uint64) uint64 {
	var read uint64
	for read < length && len(b.replay) > 0 {
		chunk := &b.replay[0]
		remaining := chunk.length - chunk.offset
		consume := length - read
		if consume > remaining {
			consume = remaining
		}
		sequence.appendChunk(chunk, consume)
		chunk.offset += consume
		read += consume
		if chunk.offset == chunk.length {
			b.replay = b.replay[1:]
		}
	}
	return read
}

func (b *BitReader) readSourceBits(length uint64) ([]byte, uint64, error) {
	if b.sourceBitOffset == 0 && length%8 == 0 {
		buf := make([]byte, length/8)
		n, err := b.readAligned(buf)
		return buf[:n], uint64(n) * 8, err
	}
	sequence := &bitSequence{data: make([]byte, 0, (length+7)/8)}
	remaining := length

	// bitio.Reader cannot report a partial byte when an unaligned byte read
	// reaches EOF. Reading up to the next byte boundary one bit at a time makes
	// every successfully consumed bit visible to the transaction recorder.
	if b.sourceBitOffset != 0 {
		toBoundary := uint64(8 - b.sourceBitOffset)
		if toBoundary > remaining {
			toBoundary = remaining
		}
		for i := uint64(0); i < toBoundary; i++ {
			bit, err := b.Reader.ReadBits(1)
			if err != nil {
				return sequence.data, sequence.length, err
			}
			sequence.appendBit(byte(bit))
			b.sourceBitOffset = (b.sourceBitOffset + 1) % 8
			remaining--
		}
	}

	bytesLength := remaining / 8
	if bytesLength > 0 {
		buf := make([]byte, bytesLength)
		bytesRead := 0
		for bytesRead < len(buf) {
			n, readErr := b.Reader.Read(buf[bytesRead:])
			if n > 0 {
				sequence.appendData(buf[bytesRead:bytesRead+n], uint64(n)*8)
				bytesRead += n
			}
			if bytesRead == len(buf) {
				break
			}
			if readErr != nil {
				return sequence.data, sequence.length, readErr
			}
			if n == 0 {
				return sequence.data, sequence.length, io.ErrNoProgress
			}
		}
		remaining -= bytesLength * 8
	}

	if remaining == 0 {
		return sequence.data, sequence.length, nil
	}

	bit, err := b.Reader.ReadBits(uint8(remaining))
	if err != nil {
		return sequence.data, sequence.length, err
	}
	sequence.appendData([]byte{byte(bit)}, remaining)
	b.sourceBitOffset = uint8(remaining)
	return sequence.data, sequence.length, nil
}

func (b *BitReader) readBits(length uint64) ([]byte, uint64, error) {
	if length == 0 {
		return []byte{}, 0, nil
	}
	if len(b.replay) == 0 {
		data, bitsRead, err := b.readSourceBits(length)
		b.recordRead(data, bitsRead)
		return data, bitsRead, err
	}

	sequence := &bitSequence{data: make([]byte, 0, (length+7)/8)}
	replayRead := b.readReplay(sequence, length)
	var readErr error
	if replayRead < length {
		data, sourceRead, err := b.readSourceBits(length - replayRead)
		sequence.appendData(data, sourceRead)
		readErr = err
	}

	b.recordRead(sequence.data, sequence.length)
	if sequence.length != length && readErr == nil {
		readErr = io.ErrUnexpectedEOF
	}
	return sequence.data, sequence.length, readErr
}

func (b *BitReader) ReadBits(n uint64) ([]byte, error) {
	buf, bitsRead, err := b.readBits(n)
	if err != nil {
		return nil, err
	}
	if bitsRead != n {
		return nil, io.ErrUnexpectedEOF
	}
	return buf, nil
}

// Preserve the caller's message boundary when it supplies only io.Reader.
// bitio otherwise inserts a private buffered reader and can consume the next
// message's bytes, which become inaccessible when this parser is discarded.
// Callers wanting buffering can supply their own reusable bufio.Reader.
type exactByteReader struct {
	io.Reader
	single [1]byte
}

func (r *exactByteReader) ReadByte() (byte, error) {
	_, err := io.ReadFull(r.Reader, r.single[:])
	if err != nil {
		return 0, err
	}
	return r.single[0], err
}

func NewBitReader(reader io.Reader) *BitReader {
	if _, ok := reader.(io.ByteReader); !ok {
		reader = &exactByteReader{Reader: reader}
	}
	return &BitReader{
		Reader: bitio.NewReader(reader),
	}
}
func NewBitWriter(writer io.Writer) *BitWriter {
	return &BitWriter{Writer: bitio.NewWriter(writer), output: writer}
}

func (b *BitWriter) Snapshot() BitWriterState {
	return BitWriterState{
		PreIsBit:   b.PreIsBit,
		PreByte:    b.PreByte,
		PreByteLen: b.PreByteLen,
	}
}

func (b *BitWriter) Restore(state BitWriterState) error {
	if state.PreByteLen > 7 {
		return fmt.Errorf("invalid pending bit length %d", state.PreByteLen)
	}
	b.Writer = bitio.NewWriter(b.output)
	if state.PreByteLen > 0 {
		if err := b.Writer.WriteBits(uint64(state.PreByte), state.PreByteLen); err != nil {
			return err
		}
	}
	b.PreIsBit = state.PreByteLen > 0
	b.PreByteLen = state.PreByteLen
	b.PreByte = state.PreByte & lowBitMask(state.PreByteLen)
	return nil
}

func lowBitMask(length uint8) byte {
	if length == 0 {
		return 0
	}
	return byte(1<<length) - 1
}

func (b *BitWriter) recordPendingBits(bs []byte, length uint64) {
	byteLength := length / 8
	bitLength := uint8(length % 8)
	pendingLength := b.PreByteLen
	pending := b.PreByte & lowBitMask(pendingLength)

	// Appending a complete byte preserves the number of pending bits. If the
	// stream is unaligned, those pending bits become the low bits of the last
	// appended byte.
	if byteLength > 0 {
		if pendingLength == 0 {
			pending = 0
		} else {
			pending = bs[byteLength-1] & lowBitMask(pendingLength)
		}
	}
	if bitLength > 0 {
		partial := bs[byteLength] & lowBitMask(bitLength)
		total := pendingLength + bitLength
		switch {
		case total < 8:
			pending = pending<<bitLength | partial
			pendingLength = total
		case total == 8:
			pending = 0
			pendingLength = 0
		default:
			pendingLength = total - 8
			pending = partial & lowBitMask(pendingLength)
		}
	}

	b.PreByte = pending
	b.PreByteLen = pendingLength
	b.PreIsBit = pendingLength > 0
}

func (b *BitWriter) WriteBits(bs []byte, length uint64) (err error) {
	if length == 0 {
		return nil
	}
	bytesLen := length / 8
	bitLen := length % 8
	expectBsLength := bytesLen
	if bitLen != 0 {
		expectBsLength++
	}
	if len(bs) < int(expectBsLength) {
		bs = append(bs, make([]byte, int(expectBsLength)-len(bs))...)
	}
	n, err := b.Writer.Write(bs[:bytesLen])
	if err != nil {
		return err
	}
	if n != int(bytesLen) {
		return io.ErrShortWrite
	}
	if bitLen != 0 {
		err := b.Writer.WriteBits(uint64(bs[bytesLen]), uint8(bitLen))
		if err != nil {
			return err
		}
	}
	b.recordPendingBits(bs, length)
	//if b.PreIsBit {
	//	if b.PreByteLen+uint8(bitLen) != 8 {
	//		return errors.New("pre byte len not equal 8")
	//	}
	//	defer func() {
	//		b.PreIsBit = false
	//		b.PreByteLen = 0
	//	}()
	//	if len(bs) == 0 {
	//		return errors.New("empty bytes")
	//	}
	//	err = b.Writer.WriteBits(uint64(bs[0]), uint8(bitLen))
	//	if err != nil {
	//		return err
	//	}
	//	bs = bs[1:]
	//	if bytesLen > 0 {
	//		_, err = b.Writer.Write(bs[:bytesLen])
	//		if err != nil {
	//			return err
	//		}
	//	}
	//} else {
	//	if bitLen != 0 {
	//		b.PreIsBit = true
	//		b.PreByte = bs[bytesLen]
	//		b.PreByteLen = uint8(bitLen)
	//	}
	//	if bytesLen > 0 {
	//		_, err = b.Writer.Write(bs[:bytesLen])
	//		if err != nil {
	//			return err
	//		}
	//	}
	//	if bitLen != 0 {
	//		err = b.Writer.WriteBits(uint64(bs[bytesLen]), uint8(bitLen))
	//		if err != nil {
	//			return err
	//		}
	//	}
	//}
	return nil
}
