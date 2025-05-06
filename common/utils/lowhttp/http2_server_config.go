package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"io"
	"net"
	"strings"
	"sync"
)

type http2ConnectionConfig struct {
	handler      func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error)
	frame        *http2.Framer
	conn         net.Conn
	frWriteMutex *sync.Mutex

	// writer
	hencBuf   *bytes.Buffer
	henc      *hpack.Encoder
	hencMutex *sync.Mutex

	wg *sync.WaitGroup

	*windowSizeControl
}

func (c *http2ConnectionConfig) close() error {
	return c.conn.Close()
}

func (c *http2ConnectionConfig) writer(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c.frame == nil {
		return utils.Error("h2 server frame config is nil")
	}
	streamId := wrapper.streamId
	frame := c.frame
	henc := c.henc
	buf := c.hencBuf
	frWriteMutex := c.frWriteMutex

	c.hencMutex.Lock()
	buf.Reset()
	SplitHTTPPacket(header, nil, func(proto string, code int, codeMsg string) error {
		henc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprint(code)})
		return nil
	}, func(line string) string {
		k, v := SplitHTTPHeader(line)
		henc.WriteField(hpack.HeaderField{Name: strings.ToLower(k), Value: v})
		return line
	})
	var hpackHeaderBytes = buf.Bytes()
	buf.Reset()

	//defer func() {
	//	log.Infof("handle h2 stream(%v) done", streamId)
	//}()
	//log.Infof("start to write h2 response header stream-id: %v", streamId)
	ret := funk.Chunk(hpackHeaderBytes, defaultMaxFrameSize).([][]byte)
	first := true
	for index, item := range ret {
		if first {
			first = false
			frWriteMutex.Lock()
			err := frame.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      uint32(streamId),
				BlockFragment: item,
				EndStream:     false,
				EndHeaders:    index == len(ret)-1,
			})
			frWriteMutex.Unlock()
			if err != nil {
				return utils.Wrapf(err, "h2framer write header(%v) for stream:%v failed", len(hpackHeaderBytes), streamId)
			}
		} else {
			frWriteMutex.Lock()
			err := frame.WriteContinuation(uint32(streamId), index == len(ret)-1, item)
			frWriteMutex.Unlock()
			if err != nil {
				return utils.Wrapf(err, "h2framer write header(%v)-continuation for stream:%v failed", len(hpackHeaderBytes), streamId)
			}
		}
	}
	c.hencMutex.Unlock()

	results, err := io.ReadAll(body)
	//log.Infof("start to write data{%v} to stream-id: %v", len(results), streamId)
	if len(results) > 0 {
		chunks := funk.Chunk(results, defaultMaxFrameSize).([][]byte)
		first = true
		for index, dataFrameBytes := range chunks {
			dataLen := len(dataFrameBytes)

			// control by window size
			c.decreaseWindowSize(int64(dataLen))
			// log.Infof("window size decrease %v to %v", dataLen, c.windowSize)
			frWriteMutex.Lock()
			dataFrameErr := frame.WriteData(uint32(streamId), index == len(chunks)-1, dataFrameBytes)
			frWriteMutex.Unlock()
			if dataFrameErr != nil {
				return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", streamId)
			}
		}
	} else {
		frWriteMutex.Lock()
		dataFrameErr := frame.WriteData(uint32(streamId), true, nil)
		frWriteMutex.Unlock()
		if dataFrameErr != nil {
			return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", streamId)
		}
	}
	if err != nil {
		return utils.Wrapf(err, "read body for stream{%v} failed", streamId)
	}
	return nil
}

type h2Option func(*http2ConnectionConfig)

func withH2Handler(h func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error)) h2Option {
	return func(c *http2ConnectionConfig) {
		c.handler = h
	}
}

func (c *http2ConnectionConfig) handleRequest(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c == nil || c.handler == nil {
		return utils.Error("h2 server handler config is nil")
	}
	header, rc, err := c.handler(header, body)
	if err != nil {
		return utils.Errorf("waiting for userspace handling for h2 stream(%v) failed: %v", wrapper.streamId, err)
	}
	return c.writer(wrapper, header, rc)
}
