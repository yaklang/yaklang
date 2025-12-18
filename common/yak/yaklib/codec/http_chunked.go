package codec

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
)

var (
	// MaxHTTPChunkedHeaderLineBytes 限制单行 chunk-size（含 chunk-extension）的最大长度，用于避免超长行导致的内存/CPU DoS。
	// RFC 9112 并没有规定必须的上限，但实际场景中 chunk-size 行通常很短。
	MaxHTTPChunkedHeaderLineBytes = 8 * 1024

	// MaxHTTPChunkedBodyBytes 限制一次解码得到的总 body 大小（内存聚合模式），避免无限制增长导致 OOM/DoS。
	MaxHTTPChunkedBodyBytes int64 = 128 * 1024 * 1024

	// MaxHTTPChunkedChunkSize 限制单个 chunk 的大小，避免按 chunk-size 直接分配超大切片触发 OOM/DoS。
	// 注意：主要的 DoS 防护由 MaxHTTPChunkedBodyBytes 提供；这里建议保持 >= MaxHTTPChunkedBodyBytes，
	// 以免误伤“单次大 write -> 单 chunk”的正常大响应场景。
	MaxHTTPChunkedChunkSize int64 = 128 * 1024 * 1024
)

func readLineEx(reader io.Reader) (string, int64, error) {
	var count int64 = 0
	buf := make([]byte, 1)
	var res bytes.Buffer
	for {
		n, err := io.ReadFull(reader, buf)
		if err != nil && n <= 0 {
			return strings.TrimRightFunc(res.String(), unicode.IsSpace), count, err
		}
		count += int64(n)
		if buf[0] == '\n' {
			return strings.TrimRightFunc(res.String(), unicode.IsSpace), count, nil
		}
		if MaxHTTPChunkedHeaderLineBytes > 0 && res.Len() >= MaxHTTPChunkedHeaderLineBytes {
			return strings.TrimRightFunc(res.String(), unicode.IsSpace), count, fmt.Errorf("chunked line too long (>%d bytes)", MaxHTTPChunkedHeaderLineBytes)
		}
		res.WriteByte(buf[0])
	}
}

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
		if MaxHTTPChunkedHeaderLineBytes > 0 && lineBuffer.Len() > MaxHTTPChunkedHeaderLineBytes {
			return nil, nil, fmt.Errorf("chunked line too long (>%d bytes)", MaxHTTPChunkedHeaderLineBytes)
		}
		if b == '\n' {
			break
		}
	}

	lines := lineBuffer.Bytes()
	if bytes.HasSuffix(lines, []byte{'\r', '\n'}) {
		return lines[:len(lines)-2], []byte{'\r', '\n'}, nil
	}
	return lines[:len(lines)-1], []byte{'\n'}, nil
}

func ReadHTTPChunkedDataWithFixed(raw []byte) (data []byte, fixedChunked []byte, rest []byte) {
	blocks, fixed, rest, _ := ReadHTTPChunkedDataWithFixedError(raw)
	return blocks, fixed, rest
}

func ReadHTTPChunkedDataWithFixedError(raw []byte) (data []byte, fixedChunked []byte, rest []byte, _ error) {
	blocks, fixed, reader, err := readChunkedDataFromReader(bytes.NewReader(raw))
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, nil, rest, err
	}
	rest, err = io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}

	return blocks, fixed, rest, nil
}

func readHTTPChunkedData(ret []byte) (data []byte, rest []byte) {
	blocks, _, reader, err := readChunkedDataFromReader(bytes.NewReader(ret))
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		rest, err = io.ReadAll(reader)
		if err != nil {
			log.Errorf("read chunked data error: %v", err)
		}
		return rest, nil
	}
	rest, err = io.ReadAll(reader)
	if err != nil {
		log.Errorf("read chunked data error: %v", err)
	}
	return blocks, rest
}

func ReadChunkedStream(r io.Reader) (io.Reader, io.Reader, error) {
	reader, fixed, restReader, err := readChunkedDataFromReaderEx(r, func(err error) {
		log.Errorf("read chunked data error in (ReadChunkedStream): %v", err)
	})
	_ = fixed
	return reader, restReader, err
}

func readChunkedDataFromReaderEx(r io.Reader, onError func(error)) (io.Reader, io.Reader, io.Reader, error) {
	var resultReader, resultWriter = bufpipe.NewPipe()
	var fixedReader, fixedWriter = bufpipe.NewPipe()
	var originMirror, originMirrorWriter = bufpipe.NewPipe()

	var mirror bytes.Buffer
	r = io.TeeReader(r, &mirror)

	start := time.Now()
	go func() {
		defer func() {
			resultWriter.Close()
			fixedWriter.Close()
			originMirrorWriter.Close()
		}()

		haveRead := new(bytes.Buffer)
		// read until space
		var trimbuf = make([]byte, 1)
		for {
			n, err := io.ReadFull(r, trimbuf)
			if err != nil && n <= 0 {
				if onError != nil {
					onError(err)
				}
				return
			}
			if n == 0 {
				continue
			}
			log.Infof("readChunkedDataFromReaderEx first byte: %v", time.Since(start))
			spaceByte := trimbuf[0]
			if unicode.IsSpace(rune(spaceByte)) {
				continue
			} else {
				r = io.MultiReader(bytes.NewReader(trimbuf[:n]), r)
				break
			}
		}

		handler := func() (io.Reader, io.Reader, io.Reader, error) {
			var totalDecoded int64
			for {
				fmt.Println("---------------------------------------------------------------")
				waitForParseInt, n, err := readLineEx(r)
				fmt.Println(waitForParseInt, "since", time.Since(start))
				//lineBytes, delim, err := bufioReadLine(reader)
				_ = n
				haveRead.WriteString(waitForParseInt)
				haveRead.WriteString("\r\n")

				lineBytes := []byte(waitForParseInt)

				//fmt.Println(string(lineBytes))

				getRestReader := func() io.Reader {
					io.Copy(originMirrorWriter, io.MultiReader(bytes.NewReader(haveRead.Bytes()), r))
					return originMirror
				}

				if err != nil && len(lineBytes) > 0 {
					return nil, nil, getRestReader(), err
				}

				var comment []byte
				var commentExisted bool
				handledLineBytes, comment, commentExisted := bytes.Cut(lineBytes, []byte{';'})
				handledLineBytes = bytes.TrimSpace(handledLineBytes)
				size, err := strconv.ParseInt(string(handledLineBytes), 16, 64)
				if err != nil && len(handledLineBytes) > 0 {
					fmt.Println("====================================================================================")
					fmt.Println("====================================================================================")
					fmt.Println(mirror.String())
					fmt.Println("====================================================================================")
					fmt.Println("====================================================================================")
					return nil, nil, getRestReader(), err
				}

				if size < 0 {
					return nil, nil, getRestReader(), fmt.Errorf("invalid negative chunk size: %d", size)
				}
				if MaxHTTPChunkedChunkSize > 0 && size > MaxHTTPChunkedChunkSize {
					return nil, nil, getRestReader(), fmt.Errorf("chunk size too large: %d (max %d)", size, MaxHTTPChunkedChunkSize)
				}
				if MaxHTTPChunkedBodyBytes > 0 && totalDecoded+size > MaxHTTPChunkedBodyBytes {
					return nil, nil, getRestReader(), fmt.Errorf("chunked body too large: %d (max %d)", totalDecoded+size, MaxHTTPChunkedBodyBytes)
				}

				if size == 0 {
					lastLine, _, err := readLineEx(r)
					haveRead.WriteString(lastLine)
					haveRead.WriteString("\r\n")
					if len(lastLine) == 0 {
						fixedWriter.WriteString("0\r\n\r\n")
					} else {
						return nil, nil, getRestReader(), fmt.Errorf("last line of chunked data is not empty: %s", lastLine)
					}

					if err != nil {
						if err == io.EOF {
							return resultReader, fixedReader, r, nil
						}
						return nil, nil, getRestReader(), err
					}
					return resultReader, fixedReader, r, nil
				}

				totalDecoded += size
				buf := make([]byte, size)
				blockN, err := io.ReadFull(r, buf)
				fmt.Printf("%#v\n", string(buf[:blockN]))
				resultWriter.Write(buf[:blockN])
				haveRead.Write(buf[:blockN])

				fixedWriter.Write(lineBytes)
				if commentExisted {
					fixedWriter.WriteString(";")
					fixedWriter.Write(comment)
				}
				fixedWriter.WriteString("\r\n")
				fixedWriter.Write(buf[:blockN])
				fixedWriter.WriteString("\r\n")
				if err != nil {
					if errors.Is(err, io.ErrUnexpectedEOF) {
						return resultReader, fixedReader, r, err
					} else {
						return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), r), fmt.Errorf("read chunked data error: %v", err)
					}
				}

				endBlock, delim, err := readLineEx(r)
				_ = delim
				haveRead.WriteString(endBlock)
				haveRead.WriteString("\r\n")
				if len(endBlock) != 0 {
					fmt.Println("====================================================================================")
					fmt.Println("====================================================================================")
					fmt.Println(mirror.String())
					fmt.Println("====================================================================================")
					fmt.Println("====================================================================================")
					return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), r), fmt.Errorf("read chunked data error: %v, endblock: %#v", err, string(endBlock))
				}
			}
		}
		_, _, _, err := handler()
		if err != nil {
			if onError != nil {
				onError(err)
			}
		}
	}()
	return resultReader, fixedReader, originMirror, nil
}

func readChunkedDataFromReader(r io.Reader) ([]byte, []byte, io.Reader, error) {
	haveRead := new(bytes.Buffer)
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
	var totalDecoded int64
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

		if size < 0 {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("invalid negative chunk size: %d", size)
		}
		if MaxHTTPChunkedChunkSize > 0 && size > MaxHTTPChunkedChunkSize {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("chunk size too large: %d (max %d)", size, MaxHTTPChunkedChunkSize)
		}
		if MaxHTTPChunkedBodyBytes > 0 && totalDecoded+size > MaxHTTPChunkedBodyBytes {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("chunked body too large: %d (max %d)", totalDecoded+size, MaxHTTPChunkedBodyBytes)
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

		totalDecoded += size
		buf := make([]byte, size)
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
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return results.Bytes(), bytes.TrimSpace(fixedResults.Bytes()), reader, err
			} else {
				return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked data error: %v", err)
			}
		}

		endBlock, delim, _ := bufioReadLine(reader)
		haveRead.Write(endBlock)
		haveRead.Write(delim)
		if len(endBlock) != 0 {
			return nil, nil, io.MultiReader(bytes.NewReader(haveRead.Bytes()), reader), fmt.Errorf("read chunked data error: %v", err)
		}
	}
}
