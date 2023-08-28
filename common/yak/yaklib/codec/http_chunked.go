package codec

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"strconv"
	"unicode"
)

func bufioReadLine(reader *bufio.Reader) ([]byte, []byte, error) {
	if reader == nil {
		return nil, nil, errors.New("empty reader(bufio)")
	}

	var lineBuffer bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, nil, err
		}
		lineBuffer.WriteByte(b)
		if b == '\n' {
			break
		}
	}

	var lines = lineBuffer.Bytes()
	if bytes.HasSuffix(lines, []byte{'\r', '\n'}) {
		return lines[:len(lines)-2], []byte{'\r', '\n'}, nil
	}
	return lines[:len(lines)-1], []byte{'\n'}, nil
}

func ReadHTTPChunkedDataWithFixed(ret []byte) (data []byte, fixedChunked []byte, rest []byte) {
	blocks, fixed, reader, _ := readChunkedDataFromReader(bytes.NewReader(ret))
	rest, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, fixed, rest
}

func readHTTPChunkedData(ret []byte) (data []byte, rest []byte) {
	blocks, _, reader, _ := readChunkedDataFromReader(bytes.NewReader(ret))
	rest, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, rest
}

func readChunkedDataFromReader(r io.Reader) ([]byte, []byte, io.Reader, error) {
	var haveRead = new(bytes.Buffer)
	var reader *bufio.Reader
	switch r.(type) {
	case *bufio.Reader:
		reader = r.(*bufio.Reader)
	default:
		reader = bufio.NewReader(r)
	}
	// read until space
	for {
		spaceByte, err := reader.ReadByte()
		if err != nil {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked (strip left space) data error: %v", err)
		}
		haveRead.WriteByte(spaceByte)
		if unicode.IsSpace(rune(spaceByte)) {
			continue
		} else {
			err := reader.UnreadByte()
			if err != nil {
				return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked (strip left space) data error: %v", err)
			}
			break
		}
	}

	var results bytes.Buffer
	var fixedResults bytes.Buffer
	for {
		lineBytes, delim, err := bufioReadLine(reader)
		haveRead.Write(lineBytes)
		haveRead.Write(delim)

		if err != nil {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
		}

		var comment []byte
		var commentExisted bool
		lineBytes, comment, commentExisted = bytes.Cut(lineBytes, []byte{';'})
		lineBytes = bytes.TrimSpace(lineBytes)
		size, err := strconv.ParseInt(string(lineBytes), 16, 64)
		if err != nil {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
		}

		if size == 0 {
			lastLine, delim, err := bufioReadLine(reader)
			haveRead.Write(lastLine)
			haveRead.Write(delim)
			if err != nil {
				return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
			}

			if len(lastLine) == 0 {
				fixedResults.WriteString("0\r\n\r\n")
			} else {
				log.Warnf("last line of chunked data is not empty: %s", lastLine)
			}
			return results.Bytes(), fixedResults.Bytes(), reader, nil
		}

		var buf = make([]byte, size)
		blockN, err := io.ReadFull(reader, buf)
		results.Write(buf[:blockN])
		haveRead.Write(buf[:blockN])

		fixedResults.Write(lineBytes)
		if commentExisted {
			fixedResults.WriteByte(';')
			fixedResults.Write(comment)
		}
		fixedResults.WriteString("\r\n")
		fixedResults.Write(buf[:blockN])
		fixedResults.WriteString("\r\n")
		if err != nil {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked data error: %v", err)
		}

		endBlock, delim, _ := bufioReadLine(reader)
		haveRead.Write(endBlock)
		haveRead.Write(delim)
		if len(endBlock) != 0 {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked data error: %v", err)
		}
	}
}
