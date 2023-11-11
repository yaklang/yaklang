package lowhttp

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"io"
	"net"
	"sync"
	"testing"
)

type h2RequestWrapper struct {
	streamId       int
	headerHPackBuf *bytes.Buffer
	bodyBuf        *bytes.Buffer
	headerEnd      bool
	streamEnd      bool
}

func serveH2(r io.Reader, conn net.Conn) error {
	// handshake
	// 1. read preface
	var preface = make([]byte, len(http2.ClientPreface))
	n, err := io.ReadAtLeast(r, preface, len(preface))
	if err != nil {
		return utils.Errorf("h2 server read preface error: %v", err)
	}
	if n != len(preface) {
		return utils.Errorf("h2 server read preface error: read %d bytes, expected %d bytes", n, len(preface))
	}

	frame := http2.NewFramer(conn, r)

	// send settings
	err = frame.WriteSettings(
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: transportDefaultConnFlow},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: 1 << 24},
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1},
		http2.Setting{ID: http2.SettingMaxHeaderListSize, Val: 10 << 20},
		http2.Setting{ID: http2.SettingHeaderTableSize, Val: 4096},
	)
	if err != nil {
		return utils.Errorf("h2 server write settings error: %v", err)
	}

	// read settings

	streamToBuf := new(sync.Map)
	getReq := func(streamIdU21 uint32) *h2RequestWrapper {
		streamId := int(streamIdU21)
		var req *h2RequestWrapper
		raw, ok := streamToBuf.Load(streamId)
		if !ok {
			req = &h2RequestWrapper{
				streamId:       streamId,
				headerHPackBuf: new(bytes.Buffer),
				bodyBuf:        new(bytes.Buffer),
			}
			streamToBuf.Store(streamId, req)
			return req
		} else if req, ok = raw.(*h2RequestWrapper); !ok {
			req = &h2RequestWrapper{
				streamId:       streamId,
				headerHPackBuf: new(bytes.Buffer),
				bodyBuf:        new(bytes.Buffer),
			}
			streamToBuf.Store(streamId, req)
			return req
		} else {
			return req
		}
	}
	for {
		rawFrame, err := frame.ReadFrame()
		if err != nil {
			return utils.Errorf("h2 server read frame error: %v", err)
		}
		switch ret := rawFrame.(type) {
		case *http2.SettingsFrame:
			if ret.IsAck() {
				continue
			}
			// write settings ack
			err := frame.WriteSettingsAck()
			if err != nil {
				return utils.Errorf("h2 server write settings ack error: %v", err)
			}
		case *http2.HeadersFrame:
			// build request
			streamId := ret.StreamID
			req := getReq(streamId)
			if b := ret.HeaderBlockFragment(); len(b) > 0 {
				req.headerHPackBuf.Write(b)
			}
			if ret.HeadersEnded() {
				req.headerEnd = true
			}
			if ret.StreamEnded() {
				req.streamEnd = true
			}
		case *http2.ContinuationFrame:
			req := getReq(ret.StreamID)
			if ret.HeadersEnded() {
				req.headerEnd = true
			}
		case *http2.DataFrame:
			// update window
			err := frame.WriteWindowUpdate(0, transportDefaultConnFlow)
			if err != nil {
				return utils.Errorf("h2 server write window update error: %v", err)
			}
			err = frame.WriteWindowUpdate(ret.StreamID, transportDefaultConnFlow)
			if err != nil {
				return utils.Errorf("h2 server write window update error: %v", err)
			}

			req := getReq(ret.StreamID)
			if len(ret.Data()) > 0 {
				req.bodyBuf.Write(ret.Data())
			}
			if ret.StreamEnded() {
				req.streamEnd = true
			}
		case *http2.PingFrame:
			err := frame.WritePing(true, ret.Data)
			if err != nil {
				return utils.Errorf("h2 server write ping error: %v", err)
			}

		}
	}

}

func TestH2_Serve(t *testing.T) {

}
