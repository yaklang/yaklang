package utils

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"strings"
)

const (
	__FRAME_PARSE_HEADER_STATUS_INIT = iota
	__FRAME_PARSE_HEADER_STATUS_FAKE_HEADER_KEY
	__FRAME_PARSE_HEADER_STATUS_HEADER_DIVIDER
	__FRAME_PARSE_HEADER_STATUS_HEADER_VALUE
	__FRAME_PARSE_HEADER_STATUS_HEADER_VALUE_HEREDOC
	__FRAME_PARSE_HEADER_STATUS_HEADER_END

	__FRAME_PARSE_HEADER_STATUS_HEADER_START
	__FRAME_PARSE_HEADER_STATUS_HEADER_KEY
)

func HTTPFrameParser(raw io.Reader) ([][2]string, [][2]string, io.Reader, error) {

	pda := NewStack[int]()

	br := bufio.NewReader(raw)
	var last1byte string
	var last2bytes string
	var last3bytes string
	var _last4bytes string
	// var lastLine string

	next := func() (byte, error) {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		last1byte = string([]byte{b})
		last2bytes += last1byte
		if len(last2bytes) > 2 {
			last2bytes = last2bytes[1:]
		}
		last3bytes += last1byte
		if len(last3bytes) > 3 {
			last3bytes = last3bytes[1:]
		}

		_last4bytes += last1byte
		if len(_last4bytes) > 4 {
			_last4bytes = _last4bytes[1:]
		}
		return b, nil
	}

	unread1byte := func() {
		br.UnreadByte()
		if len(last1byte) > 0 {
			last1byte = last1byte[:len(last1byte)-1]
		}
		if len(last2bytes) > 0 {
			last2bytes = last2bytes[:len(last2bytes)-1]
		}
		if len(last3bytes) > 0 {
			last3bytes = last3bytes[:len(last3bytes)-1]
		}
		if len(_last4bytes) > 0 {
			_last4bytes = _last4bytes[:len(_last4bytes)-1]
		}
	}

	nextUntilLF := func() (string, error) {
		var buf bytes.Buffer
		for {
			b, err := next()
			if err != nil {
				return "", err
			}
			buf.WriteByte(b)
			if b == '\n' {
				break
			}
		}
		return buf.String(), nil
	}

INIT:
	switch pda.Peek() {
	case __FRAME_PARSE_HEADER_STATUS_INIT:
		for {
			firstByte, err := next()
			if err != nil {
				return nil, nil, nil, err
			}
			// skip space
			if firstByte == ' ' || firstByte == '\n' || firstByte == '\r' || firstByte == '\t' {
				continue
			} else {
				unread1byte()
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_START)
				break INIT
			}
		}
	default:
		return nil, nil, nil, Error("BUG: invalid status")
	}

	// pr, pw := NewBufPipe(nil)
	currentHeaderKey := bytes.NewBuffer(nil)
	currentHeaderValue := bytes.NewBuffer(nil)

	fakeHeader := [][2]string{}
	headers := [][2]string{}

HEADER_READER:
	for {
	RETRY:
		switch pda.Peek() {
		case __FRAME_PARSE_HEADER_STATUS_HEADER_END:
			headerKey := currentHeaderKey.String()
			if headerKey == "" {
				break HEADER_READER
			}
			headerValue := currentHeaderValue.String()
			if strings.HasPrefix(headerKey, ":") {
				fakeHeader = append(fakeHeader, [2]string{headerKey, headerValue})
			} else {
				headers = append(headers, [2]string{headerKey, headerValue})
			}
			currentHeaderKey.Reset()
			currentHeaderValue.Reset()
			pda.Pop()
			pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_START)
			goto RETRY
		case __FRAME_PARSE_HEADER_STATUS_HEADER_VALUE_HEREDOC:
			line, err := nextUntilLF()
			if err != nil {
				pda.Pop()
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
				goto RETRY
			}
			headerKey := currentHeaderKey.String()
			if headerKey == "" {
				log.Errorf("invalid header name when heredoc start parsing: %s", line)
				pda.Pop()
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
				goto RETRY
			}
			flag := strings.TrimRight(line, "\r\n")
			if ":"+strings.ToLower(flag) == (strings.ToLower(headerKey)) {
				results := currentHeaderValue.String()
				currentHeaderValue.Reset()
				results = results[:len(results)-3]
				currentHeaderValue.WriteString(results)
				for {
					line, err := nextUntilLF()
					if err != nil {
						pda.Pop()
						pda.Pop()
						pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
						goto RETRY
					}
					if strings.TrimRight(line, "\r\n") == flag {
						break
					}
					currentHeaderValue.WriteString(line)
				}
				pda.Pop()
				pda.Pop()
				results = currentHeaderValue.String()
				if strings.HasSuffix(results, "\r\n") {
					results = results[:len(results)-2]
					currentHeaderValue.Reset()
					currentHeaderValue.WriteString(results)
				} else if strings.HasSuffix(results, "\n") {
					results = results[:len(results)-1]
					currentHeaderValue.Reset()
					currentHeaderValue.WriteString(results)
				}
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
				goto RETRY
			} else {
				currentHeaderValue.WriteString(line)
				pda.Pop()
			}
		case __FRAME_PARSE_HEADER_STATUS_HEADER_VALUE:
			/*
				handle <<<TEXT
			*/
			for {
				b, err := next()
				if err != nil {
					break
				}
				if b == '\n' {
					results := currentHeaderValue.String()
					if strings.HasSuffix(results, "\r") {
						results = results[:len(results)-1]
						currentHeaderValue.Reset()
						currentHeaderValue.WriteString(results)
					}
					break
				}

				currentHeaderValue.WriteByte(b)
				if b == '<' && last3bytes == "<<<" {
					pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_VALUE_HEREDOC)
					goto RETRY
				}
			}
			pda.Pop()
			pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
			goto RETRY
		case __FRAME_PARSE_HEADER_STATUS_HEADER_DIVIDER:
			firstByte, err := next()
			if err != nil {
				break HEADER_READER
			}
			if firstByte == ':' {
				nb, err := next()
				if err != nil {
					break HEADER_READER
				}
				if nb != ' ' {
					unread1byte()
				}
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_VALUE)
				goto RETRY
			} else {
				unread1byte()
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_VALUE)
				goto RETRY
			}
		case __FRAME_PARSE_HEADER_STATUS_FAKE_HEADER_KEY:
			currentHeaderKey.WriteByte(':')
			for {
				b, err := next()
				if err != nil {
					break HEADER_READER
				}
				if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' {
					currentHeaderKey.WriteByte(b)
				} else if b == ':' {
					unread1byte()
					pda.Pop()
					pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_DIVIDER)
					goto RETRY
				} else {
					pda.Pop()
					pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
					goto RETRY
				}
			}
		case __FRAME_PARSE_HEADER_STATUS_HEADER_KEY:
			for {
				b, err := next()
				if err != nil {
					break HEADER_READER
				}
				if b == ':' {
					unread1byte()
					pda.Pop()
					pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_DIVIDER)
					goto RETRY
				} else if b == '\n' {
					pda.Pop()
					pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
					goto RETRY
				}
				currentHeaderKey.WriteString(string(b))
			}
		case __FRAME_PARSE_HEADER_STATUS_HEADER_START:
			firstByte, err := next()
			if err != nil {
				break HEADER_READER
			}
			if firstByte == '\r' {
				continue
			}

			if firstByte == '\n' {
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_END)
				goto RETRY
			}

			if firstByte == ':' && currentHeaderKey.Len() == 0 {
				pda.Push(__FRAME_PARSE_HEADER_STATUS_FAKE_HEADER_KEY)
				goto RETRY
			} else {
				unread1byte()
				pda.Pop()
				pda.Push(__FRAME_PARSE_HEADER_STATUS_HEADER_KEY)
				goto RETRY
			}
		default:
			return nil, nil, nil, Error("BUG: invalid status")
		}
	}

	if headerKey := currentHeaderKey.String(); len(headerKey) > 0 {
		headerValue := currentHeaderValue.String()
		if strings.HasPrefix(headerKey, ":") {
			fakeHeader = append(fakeHeader, [2]string{headerKey, headerValue})
		} else {
			headers = append(headers, [2]string{headerKey, headerValue})
		}
	}

	buf, err := io.ReadAll(br)
	if err != nil {
		return nil, nil, nil, err
	}

	return fakeHeader, headers, bytes.NewReader(buf), nil
}
