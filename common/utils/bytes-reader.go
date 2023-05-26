package utils

import (
	"bufio"
	"bytes"
	"context"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"time"
	"unicode"
)

func CopyReader(r io.ReadCloser) (io.ReadCloser, io.ReadCloser, error) {
	var buf = *bytes.NewBufferString("")
	if r == nil {
		return ioutil.NopCloser(bytes.NewBuffer(nil)), ioutil.NopCloser(bytes.NewBuffer(nil)), Errorf("empty input reader")
	}

	if _, err := buf.ReadFrom(r); err != nil {
		return ioutil.NopCloser(bytes.NewBuffer(nil)), r, err
	}

	if err := r.Close(); err != nil {
		return ioutil.NopCloser(bytes.NewBuffer(nil)), r, err
	}

	return ioutil.NopCloser(&buf), ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

func ReaderToReaderCloser(body io.Reader) io.ReadCloser {
	if body == nil {
		return nil
	}

	rc, ok := body.(io.ReadCloser)
	if !ok {
		rc = ioutil.NopCloser(body)
	}
	return rc
}

func ReadWithChunkLen(raw []byte, length int) chan []byte {
	outC := make(chan []byte)
	go func() {
		defer close(outC)

		scanner := bufio.NewScanner(bytes.NewBuffer(raw))
		scanner.Split(bufio.ScanBytes)

		buffer := []byte{}
		n := 0
		for scanner.Scan() {
			buff := scanner.Bytes()
			buffSize := len(buff)
			n += buffSize
			buffer = append(buffer, buff...)

			if n >= length {
				outC <- buffer
				n = 0
				buffer = []byte{}
			}
		}

		if len(buffer) > 0 {
			outC <- buffer
		}
	}()
	return outC
}

func BufReadLen(r io.Reader, length uint64) ([]byte, error) {
	return nil, nil
}

func StableReaderEx(conn net.Conn, timeout time.Duration, maxSize int) []byte {
	ch := make([]byte, 1)
	var n int
	var err error
	l := 0
	var buffer = bytes.NewBuffer(nil)
	readTimeout := 1000 * time.Millisecond
	readAsyncTimeout := 250 * time.Millisecond
	readGapTimeout := 600 * time.Millisecond
	defer conn.SetDeadline(time.Now().Add(3 * time.Minute))
	ddlCtx, originCancel := context.WithTimeout(context.Background(), timeout)
	var cancel = func() {
		originCancel()
	}
	go func() {
		for {
			conn.SetDeadline(time.Now().Add(readAsyncTimeout))
			n, err = conn.Read(ch)
			if n > 0 {
				buffer.Write(ch)
				if buffer.Len() == maxSize {
					//cancel()
					return
				}
			}

			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			}
			if err != nil {
				msg := err.Error()
				switch true {
				case MatchAllOfGlob(msg, "*i/o timeout"):
				default:
					time.Sleep(time.Second)
					log.Errorf("conn[%s] met error: %v", conn.RemoteAddr().String(), err)
				}
			}

			// bufio.Scanner/Buffer scanner.Split(bufio.ScanByte)

			select {
			case <-ddlCtx.Done():
				cancel()
				return
			default:
				continue
			}
		}
	}()
	//wait := make(chan int)
	for {
		time.Sleep(readTimeout)
		if buffer.Len() == 0 || buffer.Len() == l {
			break
		}
		l = buffer.Len()
	}
	cancel()
	time.Sleep(readGapTimeout)
	return buffer.Bytes()
}

func StableReader(conn io.Reader, timeout time.Duration, maxSize int) []byte {
	var buffer bytes.Buffer
	ddlCtx, cancel := context.WithTimeout(context.Background(), timeout)
	// read first connection
	go func() {
		_, err := io.Copy(&buffer, ctxio.NewReader(ddlCtx, conn))
		if err != nil {
			log.Debugf("copy end: %v", err)
		}
	}()
	defer cancel()

	var banner []byte
	var bannerHash string
TOKEN:
	for {
		// check for every 0.5 seconds
		select {
		case <-ddlCtx.Done():
			break TOKEN
		default:
			time.Sleep(500 * time.Millisecond)
		}

		if len(buffer.Bytes()) <= 0 {
			continue
		}

		if len(buffer.Bytes()) > maxSize {
			banner = buffer.Bytes()
			break
		}

		currentHash := codec.Sha1(buffer.Bytes())
		if currentHash == bannerHash {
			break
		}
		banner = buffer.Bytes()
		bannerHash = currentHash
	}
	return banner
}

func ReadWithContext(ctx context.Context, reader io.Reader) []byte {
	outc := make(chan []byte)
	go func() {
		defer close(outc)

		scanner := bufio.NewScanner(reader)
		scanner.Split(bufio.ScanBytes)
		for scanner.Scan() {

			if ctx.Err() != nil {
				return
			}

			outc <- scanner.Bytes()
		}
	}()

	var raw []byte
	for {
		select {
		case data, ok := <-outc:
			if !ok {
				return raw
			}
			raw = append(raw, data...)
		case <-ctx.Done():
			return raw
		}
	}
}

func ReadN(reader io.Reader, n int) ([]byte, error) {
	var buf = make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	_, err := io.ReadFull(reader, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func ReadConnWithTimeout(r net.Conn, timeout time.Duration) ([]byte, error) {
	err := r.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, errors.Errorf("set read timeout failed: %s", err)
	}

	raw, err := ioutil.ReadAll(ioutil.NopCloser(r))
	if len(raw) > 0 {
		return raw, nil
	}

	return nil, errors.Errorf("read empty: %s", err)
}

func WriteConnWithTimeout(w net.Conn, timeout time.Duration, data []byte) error {
	err := w.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		return errors.Errorf("write failed: %s", err)
	}

	_, err = w.Write(data)
	if err != nil {
		return errors.Errorf("write failed: %s", err)
	}

	return nil
}

func ConnExpect(c net.Conn, timeout time.Duration, callback func([]byte) bool) (bool, error) {
	err := c.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return false, errors.Errorf("set timeout for reading conn failed: %s", err)
	}

	scanner := bufio.NewScanner(c)
	scanner.Split(bufio.ScanBytes)

	var buf []byte
	for scanner.Scan() {
		buf = append(buf, scanner.Bytes()...)
		if callback(buf) {
			return true, nil
		}
	}
	return false, nil
}

func BufioReadLine(reader *bufio.Reader) ([]byte, error) {
	if reader == nil {
		return nil, Error("empty reader(bufio)")
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

func ReadLineEx(reader io.Reader) (string, int64, error) {
	var count int64 = 0
	var buf = make([]byte, 1)
	var res bytes.Buffer
	for {
		n, err := reader.Read(buf)
		if err != nil {
			return strings.TrimRightFunc(res.String(), unicode.IsSpace), count, err
		}
		count += int64(n)
		if buf[0] == '\n' {
			return strings.TrimRightFunc(res.String(), unicode.IsSpace), count, nil
		}
		res.WriteByte(buf[0])
	}
}
