package yakit

import (
	"context"
	"io"
	"io/fs"
	"os"
	"runtime"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/mmap"
)

func ReaderAtIndexByte(r io.ReaderAt, c byte, start, end int64) (int64, error) {
	if start < 0 {
		return -1, nil
	}
	if start >= end {
		return -1, nil
	}

	// Use a reasonable buffer size for reading chunks
	const bufSize = 8192
	buf := make([]byte, bufSize)

	// Read and search through the content in chunks
	offset := start
	for offset < end {
		// Calculate how many bytes to read in this chunk
		n := bufSize
		remaining := end - offset
		if remaining < int64(bufSize) {
			n = int(remaining)
		}

		// Read the chunk at current offset
		readN, err := r.ReadAt(buf[:n], offset)
		if err != nil && err != io.EOF {
			return -1, err
		}

		// Search for the byte in current chunk
		for i := 0; i < readN; i++ {
			if buf[i] == c {
				return offset + int64(i), nil
			}
		}

		// Move to next chunk
		offset += int64(readN)

		// Check if we've reached EOF
		if err == io.EOF || readN < n {
			break
		}
	}

	return -1, nil
}

func ReadLargeFileLineWithCallBack(ctx context.Context, filename string, preHandler func(fs.FileInfo), handler func(string) error) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if preHandler != nil {
		preHandler(fi)
	}

	size := fi.Size()
	if size <= 0 || size != int64(int(size)) {
		return utils.Errorf("invalid file size: %d", size)
	}
	reader, err := mmap.Open(filename)
	if err != nil {
		return utils.Wrapf(err, "mmap open file %s failed", filename)
	}
	defer reader.Close()

	return processMMapReader(ctx, reader, handler)
}

func processMMapReader(ctx context.Context, reader *mmap.ReaderAt, handler func(string) error) error {
	lenOfData := int64(reader.Len())
	nChunks := int64(runtime.NumCPU())

	chunkSize := lenOfData / nChunks
	if chunkSize == 0 {
		chunkSize = lenOfData
	}

	chunks := make([]int64, 0, nChunks)
	var offset int64
	for offset < lenOfData {
		offset += chunkSize
		if offset >= lenOfData {
			chunks = append(chunks, lenOfData)
			break
		}

		nlPos, err := ReaderAtIndexByte(reader, '\n', offset, lenOfData)
		if err != nil {
			return err
		}
		if nlPos == -1 {
			chunks = append(chunks, lenOfData)
			break
		} else {
			offset = nlPos + 1
			chunks = append(chunks, offset)
		}
	}

	var start int64
	var wg sync.WaitGroup
	wg.Add(len(chunks))

	for _, chunk := range chunks {
		go func(reader io.ReaderAt, start, offset int64) {
			processMMapChunk(ctx, reader, start, chunk, handler)
			wg.Done()
		}(reader, start, chunk)
		start = chunk
	}
	wg.Wait()
	return nil
}

func processMMapChunk(ctx context.Context, reader io.ReaderAt, start, end int64, handler func(string) error) {
	for start < end {
		select {
		case <-ctx.Done():
			return
		default:
			nlPos, err := ReaderAtIndexByte(reader, '\n', start, end)
			if err != nil {
				log.Warnf("failed to read line: %v", err)
				return
			}
			if nlPos == -1 {
				// not found \n, but we still need to process the rest of the data
				nlPos = end
			}

			line := make([]byte, nlPos-start)
			_, err = reader.ReadAt(line, start)
			if err != nil {
				return
			}

			err = handler(string(line))
			if err != nil {
				return
			}

			start = nlPos + 1
		}
	}
}
