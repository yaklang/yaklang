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
	blocks, fixed, reader, err := readChunkedDataFromReader(bytes.NewReader(ret))
	if err != nil {
		raw, _ := io.ReadAll(reader)
		return nil, nil, raw
	}
	rest, err = io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, fixed, rest
}

func ReadHTTPChunkedDataWithFixedError(ret []byte) (data []byte, fixedChunked []byte, rest []byte, _ error) {
	blocks, fixed, reader, err := readChunkedDataFromReader(bytes.NewReader(ret))
	if err != nil {
		raw, _ := io.ReadAll(reader)
		return nil, nil, raw, err
	}
	rest, err = io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, fixed, rest, nil
}

func readHTTPChunkedData(ret []byte) (data []byte, rest []byte) {
	blocks, fixed, reader, err := readChunkedDataFromReader(bytes.NewReader(ret))
	if err != nil {
		rest, err = io.ReadAll(reader)
		if err != nil {
			log.Errorf("read chunked data error: %v", err)
		}
		return rest, nil
	}
	_ = fixed
	rest, err = io.ReadAll(reader)
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

		if err != nil && len(lineBytes) > 0 {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
		}

		var comment []byte
		var commentExisted bool
		handledLineBytes, comment, commentExisted := bytes.Cut(lineBytes, []byte{';'})
		handledLineBytes = bytes.TrimSpace(handledLineBytes)
		size, err := strconv.ParseInt(string(handledLineBytes), 16, 64)
		if err != nil && len(handledLineBytes) > 0 {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
		}

		if size == 0 {
			lastLine, delim, err := bufioReadLine(reader)
			haveRead.Write(lastLine)
			haveRead.Write(delim)
			if len(lastLine) == 0 {
				fixedResults.WriteString("0\r\n\r\n")
			} else {
				return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("last line of chunked data is not empty: %s", lastLine)
			}

			if err != nil {
				if err == io.EOF {
					return results.Bytes(), fixedResults.Bytes(), reader, nil
				}
				return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), err
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
