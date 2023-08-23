package codec

import (
	"bufio"
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"strconv"
	"unicode"
)

func bufioReadLine(reader *bufio.Reader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("empty reader(bufio)")
	}

	var buf bytes.Buffer
	for {
		tmp, isPrefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		buf.Write(tmp)
		if !isPrefix {
			return buf.Bytes(), nil
		}
	}
}

func readHTTPChunkedData(ret []byte) ([]byte, []byte) {
	blocks, reader := readChunkedDataFromReader(bytes.NewReader(ret))
	rest, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, rest
}

func readChunkedDataFromReader(r io.Reader) ([]byte, io.Reader) {
	var haveRead bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(r, &haveRead))
	// read until space
	for {
		spaceRune, size, err := reader.ReadRune()
		if err != nil {
			return nil, io.MultiReader(&haveRead, reader)
		}
		if unicode.IsSpace(spaceRune) {
			_ = size
			continue
		} else {
			err := reader.UnreadRune()
			if err != nil {
				reader = bufio.NewReader(io.MultiReader(bytes.NewBufferString(string([]rune{spaceRune})), reader))
			}
			break
		}
	}

	var results bytes.Buffer
	for {
		lineBytes, err := bufioReadLine(reader)
		if err != nil {
			return nil, io.MultiReader(&haveRead, reader)
		}
		lineBytes, _, _ = bytes.Cut(lineBytes, []byte{';'})
		lineBytes = bytes.TrimSpace(lineBytes)
		size, err := strconv.ParseInt(string(lineBytes), 16, 64)
		if err != nil {
			return nil, io.MultiReader(&haveRead, reader)
		}

		if size == 0 {
			lastLine, err := bufioReadLine(reader)
			if err != nil {
				return nil, io.MultiReader(&haveRead, reader)
			}
			if len(lastLine) == 0 {
				return results.Bytes(), reader
			} else {
				return nil, io.MultiReader(&haveRead, reader)
			}
		}

		var buf = make([]byte, size)
		blockN, err := io.ReadFull(reader, buf)
		results.Write(buf[:blockN])
		if err != nil {
			return nil, io.MultiReader(&haveRead, reader)
		}
		var endBlock, _ = bufioReadLine(reader)
		if len(endBlock) != 0 {
			return nil, io.MultiReader(&haveRead, reader)
		}
	}
}
