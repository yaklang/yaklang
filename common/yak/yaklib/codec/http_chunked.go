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

func ReadHTTPChunkedDataWithFixed(ret []byte) (data []byte, fixedChunked []byte, rest []byte) {
	blocks, fixed, reader := readChunkedDataFromReader(bytes.NewReader(ret))
	rest, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, fixed, rest
}

func readHTTPChunkedData(ret []byte) (data []byte, rest []byte) {
	blocks, _, reader := readChunkedDataFromReader(bytes.NewReader(ret))
	rest, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, rest
}

func readChunkedDataFromReader(r io.Reader) ([]byte, []byte, io.Reader) {
	var haveRead bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(r, &haveRead))
	// read until space
	for {
		spaceRune, size, err := reader.ReadRune()
		if err != nil {
			return nil, nil, io.MultiReader(&haveRead, reader)
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
	var fixedResults bytes.Buffer
	for {
		lineBytes, err := bufioReadLine(reader)
		if err != nil {
			return nil, nil, io.MultiReader(&haveRead, reader)
		}
		var comment []byte
		var commentExisted bool
		lineBytes, comment, commentExisted = bytes.Cut(lineBytes, []byte{';'})
		lineBytes = bytes.TrimSpace(lineBytes)
		size, err := strconv.ParseInt(string(lineBytes), 16, 64)
		if err != nil {
			return nil, nil, io.MultiReader(&haveRead, reader)
		}

		if size == 0 {
			lastLine, err := bufioReadLine(reader)
			if err != nil {
				return nil, nil, io.MultiReader(&haveRead, reader)
			}
			if len(lastLine) == 0 {
				fixedResults.WriteString("0\r\n\r\n")
				return results.Bytes(), fixedResults.Bytes(), reader
			} else {
				return nil, nil, io.MultiReader(&haveRead, reader)
			}
		}

		var buf = make([]byte, size)
		blockN, err := io.ReadFull(reader, buf)
		results.Write(buf[:blockN])
		fixedResults.Write(lineBytes)
		if commentExisted {
			fixedResults.WriteByte(';')
			fixedResults.Write(comment)
		}
		fixedResults.WriteString("\r\n")
		fixedResults.Write(buf[:blockN])
		fixedResults.WriteString("\r\n")
		if err != nil {
			return nil, nil, io.MultiReader(&haveRead, reader)
		}
		var endBlock, _ = bufioReadLine(reader)
		if len(endBlock) != 0 {
			return nil, nil, io.MultiReader(&haveRead, reader)
		}
	}
}
