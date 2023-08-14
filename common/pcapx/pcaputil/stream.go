package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil/tcpassembly"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"sync/atomic"
	"time"
)

// StreamFactory implements tcpassembly.StreamFactory
type StreamFactory struct {
	ctx         context.Context
	activeCount int64
}

func NewStreamFactory(ctx context.Context) *StreamFactory {
	return &StreamFactory{ctx: ctx}
}

// Stream implements tcpassembly.Stream
type Stream struct {
	net, transport gopacket.Flow
	r              tcpassembly.ReaderStream
}

// New creates a new stream
func (f *StreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	s := &Stream{
		net:       net,
		transport: transport,
		r:         tcpassembly.NewReaderStream(),
	}
	s.r.ReaderStreamOptions.LossErrors = true
	atomic.AddInt64(&f.activeCount, 1)
	netEpSrc, netEpDst := net.Endpoints()
	vector := fmt.Sprintf("%v:%v -> %v:%v", netEpSrc.String(), transport.Src(), netEpDst.String(), transport.Dst())
	go func() {
		defer func() {
			count := atomic.AddInt64(&f.activeCount, -1)
			log.Infof("active stream %v closed, current count: %d", vector, count)
		}()

		reAssemblyReader := &(s.r)
		var ctxReader = ctxio.NewReader(f.ctx, reAssemblyReader)
		peekable := utils.NewPeekableReader(ctxReader)
		rets, err := peekable.Peek(1)
		if err != nil {
			if err != io.EOF {
				log.Errorf("peek error: %v", err)
			}
			return
		}
		defer func() {
			io.Copy(io.Discard, peekable)
		}()

		if len(rets) == 0 {
			return
		}

		// tls client hello
		data := utils.StableReader(peekable, 5*time.Second, 4096)
		if rets[0] == 0x16 {
			hello, err := tlsutils.ParseClientHello(data)
			if err != nil {
				return
			}
			if hello.SNI() != "" {
				log.Infof("tls client hello: %s", hello.SNI())
			}
		} else if (rets[0] >= 'A' && rets[0] <= 'Z') || (rets[0] >= 'a' && rets[0] <= 'z') {
			uIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(data, false)
			if err != nil {
				return
			}
			method, _, _ := lowhttp.GetHTTPPacketFirstLine(data)
			if method != "" {
				log.Infof("http [%v]: %s", method, uIns.String())
			}
		} else {

		}
	}()
	return s
}

// Reassembled handles reassembled packets
func (s *Stream) Reassembled(rs []tcpassembly.Reassembly) {
	for _, r := range rs {
		s.r.Reassembled([]tcpassembly.Reassembly{r})
	}
}

// ReassemblyComplete handles end of stream
func (s *Stream) ReassemblyComplete() {
	s.r.ReassemblyComplete()
}
