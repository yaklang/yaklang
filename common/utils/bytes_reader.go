package utils

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func IOCopy(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	if buf == nil {
		size := 32 * 1024
		if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = Errorf("short write")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func ReadWithContextTickCallback(ctx context.Context, rc io.Reader, callback func([]byte) bool, interval time.Duration) {
	scanner := bufio.NewScanner(rc)
	scanner.Split(bufio.ScanBytes)
	ticker := time.Tick(interval)

	// one go routine to read
	var (
		mux = new(sync.Mutex)
		buf []byte
	)
	go func() {
		for scanner.Scan() {
			// 根据上下文退出
			if ctx.Err() != nil {
				break
			}

			// 临时读一下现有指纹信息
			mux.Lock()
			buf = append(buf, scanner.Bytes()...)
			mux.Unlock()
		}
	}()

	defer callback(buf)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			mux.Lock()
			flag := callback(buf)
			mux.Unlock()
			if flag {
				continue
			} else {
				return
			}
		}
	}
}

func IsErrorNetOpTimeout(err error) bool {
	if err == nil {
		return false
	}

	var netOpError interface{ Timeout() bool }
	result := errors.As(err, &netOpError) && netOpError != nil && netOpError.Timeout()
	if !result {
		// check context exceeded
		if errors.As(err, &context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return true
		}
	}
	ret := err.Error()
	if strings.Contains(ret, "i/o timeout") || strings.Contains(ret, "context deadline exceeded") {
		return true
	}

	return false
}

func ReadConnUntil(conn net.Conn, timeout time.Duration, sep ...byte) ([]byte, error) {
	if conn == nil {
		return nil, Error("empty(nil) conn")
	}

	buf := make([]byte, 1)
	var result bytes.Buffer
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer func() {
		conn.SetReadDeadline(time.Time{})
	}()

	stopWord := make(map[byte]struct{})
	for _, stop := range sep {
		stopWord[stop] = struct{}{}
	}

	for {
		n, err := io.ReadFull(conn, buf)
		if err != nil {
			var netOpError interface{ Timeout() bool }
			if errors.As(err, &netOpError) && netOpError != nil && netOpError.Timeout() {
				if result.Len() > 0 {
					return result.Bytes(), nil
				} else {
					return nil, err
				}
			}
			return result.Bytes(), err
		}
		result.Write(buf[:n])
		if n == 1 {
			_, isStop := stopWord[buf[0]]
			if isStop {
				return result.Bytes(), nil
			}
		}
	}
}

// ReadUntilStable is a stable reader check interval(stableTimeout)
// safe for conn is empty
func ReadUntilStable(reader io.Reader, conn net.Conn, timeout time.Duration, stableTimeout time.Duration, sep ...byte) ([]byte, error) {
	return ReadUntilStableEx(reader, false, conn, timeout, stableTimeout, sep...)
}

// ReadUntilStableEx allow skip timeout, read until stop word or timeout
func ReadUntilStableEx(reader io.Reader, noTimeout bool, conn net.Conn, timeout time.Duration, stableTimeout time.Duration, sep ...byte) ([]byte, error) {
	buf := make([]byte, 1)
	var result bytes.Buffer

	var ctx context.Context
	var cancel context.CancelFunc
	if noTimeout {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}
	defer cancel()
	stopStep := 0

	wrapperTimeout := func(originReader io.Reader) io.Reader {
		if noTimeout {
			return originReader
		}

		if conn != nil {
			_ = conn.SetReadDeadline(time.Now().Add(stableTimeout))
			return originReader
		} else {
			return ctxio.NewReader(TimeoutContext(stableTimeout), originReader)
		}
	}
	recoverTimeout := func() {
		if noTimeout {
			return
		}

		if conn != nil {
			_ = conn.SetReadDeadline(time.Time{})
		}
	}

	for {
		n, err := io.ReadFull(wrapperTimeout(reader), buf)
		recoverTimeout()

		if err != nil {
			var netOpError interface{ Timeout() bool }
			if errors.As(err, &netOpError) && netOpError != nil && netOpError.Timeout() {
				if result.Len() > 0 {
					return result.Bytes(), nil
				} else {
					return nil, err
				}
			}
			return result.Bytes(), err
		}
		if n > 0 {
			result.Write(buf[:n])
		}
		select {
		case <-ctx.Done():
			if result.Len() > 0 {
				return result.Bytes(), nil
			}
			return nil, Error("i/o timeout")
		default:
		}
		if n == 1 && stopStep < len(sep) {
			if buf[0] == sep[stopStep] {
				stopStep++
				if stopStep == len(sep) {
					return result.Bytes(), nil
				}
			} else {
				stopStep = 0
			}
		}
	}
}

func StableReaderEx(conn net.Conn, timeout time.Duration, maxSize int) []byte {
	var mu sync.Mutex
	buffer := bytes.NewBuffer(nil)
	readTimeout := 1000 * time.Millisecond
	readAsyncTimeout := 250 * time.Millisecond
	readGapTimeout := 350 * time.Millisecond

	defer conn.SetDeadline(time.Now().Add(3 * time.Minute))

	ddlCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool)
	go func() {
		defer close(done)
		ch := make([]byte, 1)
		for {
			if err := conn.SetDeadline(time.Now().Add(readAsyncTimeout)); err != nil {
				log.Debugf("SetDeadline failed: %v", err)
				return
			}

			n, err := conn.Read(ch)
			if n > 0 {
				mu.Lock()
				buffer.Write(ch)
				currentLen := buffer.Len()
				mu.Unlock()

				if currentLen >= maxSize {
					done <- true
					return
				}
			}

			if err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					done <- true
					return
				}
				if conn.RemoteAddr() != nil {
					log.Debugf("conn[%s] met error: %v", conn.RemoteAddr().String(), err)
				}
			}

			select {
			case <-ddlCtx.Done():
				done <- false
				return
			default:
			}
		}
	}()

	var lastLen int
	timer := time.NewTimer(readTimeout)
	defer timer.Stop()

	for {
		<-timer.C
		timer.Reset(readTimeout)

		mu.Lock()
		currentLen := buffer.Len()
		mu.Unlock()

		if currentLen == 0 || currentLen == lastLen {
			break
		}
		lastLen = currentLen
	}

	if success := <-done; success {
		time.Sleep(readGapTimeout)
	}

	mu.Lock()
	defer mu.Unlock()
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

func ReadN(reader io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
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
		return nil, errors.Wrap(err, "set read timeout failed")
	}

	raw, err := ioutil.ReadAll(ioutil.NopCloser(r))
	if len(raw) > 0 {
		return raw, nil
	}

	return nil, errors.Wrap(err, "read empty")
}

func ReadTimeout(r io.Reader, timeout time.Duration) ([]byte, error) {
	if IsNil(r) {
		return nil, Error("nil reader")
	}
	if timeout <= 0 {
		return io.ReadAll(r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	r = ctxio.NewReader(ctx, r)
	return io.ReadAll(r)
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

func BufioReadLineString(reader *bufio.Reader) (string, error) {
	if reader == nil {
		return "", Error("empty reader(bufio)")
	}

	var buf bytes.Buffer
	for {
		tmp, isPrefix, err := reader.ReadLine()
		if err != nil {
			return "", err
		}
		buf.Write(tmp)
		if !isPrefix {
			return buf.String(), nil
		}
	}
}

func ReadLine(reader io.Reader) ([]byte, error) {
	lineRaw, err := ReadUntilStableEx(reader, true, nil, 0, 0, '\n')
	if err != nil {
		return lineRaw, err
	}
	return bytes.TrimRight(lineRaw, "\r\n"), nil
}

func ReadLineEx(reader io.Reader) (string, int64, error) {
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
		res.WriteByte(buf[0])
	}
}

type TriggerWriter struct {
	sizeTrigger uint64
	bytesCount  uint64

	timeTriggerDuration time.Duration
	timeTrigger         *time.Timer
	writeFirstOnce      *sync.Once

	r    io.ReadCloser
	w    io.WriteCloser
	once *sync.Once
	h    func(buffer io.ReadCloser, triggerEvent string)
}

func NewTriggerWriter(trigger uint64, h func(buffer io.ReadCloser, triggerEvent string)) *TriggerWriter {
	r, w := NewBufPipe(nil)
	return &TriggerWriter{
		sizeTrigger: trigger,
		w:           w, r: r,
		once:           new(sync.Once),
		writeFirstOnce: new(sync.Once),
		h:              h,
	}
}

type wrapperForFirstWrite struct {
	once         *sync.Once
	onFirstWrite func([]byte)
}

func (w *wrapperForFirstWrite) Write(p []byte) (n int, err error) {
	if len(p) > 0 {
		w.once.Do(func() {
			if w.onFirstWrite != nil {
				w.onFirstWrite(p)
			}
		})
	}
	return io.Discard.Write(p)
}

func FirstWriter(onFirstWrite func([]byte)) io.Writer {
	return &wrapperForFirstWrite{
		once:         new(sync.Once),
		onFirstWrite: onFirstWrite,
	}
}

func ReaderOnFirstByte(origin io.Reader, onFirstByte func()) io.Reader {
	var buf = make([]byte, 1)
	n, _ := io.ReadFull(origin, buf)
	if n > 0 {
		onFirstByte()
		return io.MultiReader(bytes.NewReader(buf[:n]), origin)
	}
	return origin
}

func NewTriggerWriterEx(sizeTrigger uint64, timeTrigger time.Duration, h func(buffer io.ReadCloser, triggerEvent string)) *TriggerWriter {
	r, w := NewBufPipe(nil)
	return &TriggerWriter{
		sizeTrigger:         sizeTrigger,
		timeTriggerDuration: timeTrigger,
		w:                   w, r: r,
		once:           new(sync.Once),
		writeFirstOnce: new(sync.Once),
		h:              h,
	}
}

func (f *TriggerWriter) GetCount() int64 {
	return int64(atomic.LoadUint64(&f.bytesCount))
}

func (f *TriggerWriter) initTimeTrigger() {
	f.timeTrigger = time.NewTimer(f.timeTriggerDuration)
	if f.timeTriggerDuration <= 0 {
		f.timeTrigger.Stop()
	}
}

func (f *TriggerWriter) Write(p []byte) (n int, err error) {
	byteCount := atomic.AddUint64(&f.bytesCount, uint64(len(p)))
	f.writeFirstOnce.Do(func() {
		f.initTimeTrigger()
	})
	select {
	case <-f.timeTrigger.C:
		f.once.Do(func() {
			f.h(f.r, httpctx.REQUEST_CONTEXT_KEY_ResponseTooSlow)
		})
	default:
		if f.sizeTrigger > 0 && byteCount > f.sizeTrigger {
			f.once.Do(func() {
				f.h(f.r, httpctx.REQUEST_CONTEXT_KEY_ResponseTooLarge)
			})
		}
	}
	n, err = f.w.Write(p)
	return
}

func (f *TriggerWriter) Close() error {
	return f.w.Close()
}
