package pcaputil

import (
	"context"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil/tcpassembly"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"time"
)

// StreamFactory implements tcpassembly.StreamFactory
type StreamFactory struct {
	ctx context.Context
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
	go func() {
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

		switch rets[0] {
		case 0x16: // TLS Client Hello
			hello, _ := tlsutils.ParseClientHello(utils.StableReader(peekable, 5*time.Second, 4096))
			if hello != nil && hello.SNI() != "" {
				log.Infof("tls client hello to sni: %v", hello.SNI())
			}
		default:

		}
	}()
	return s
}

// run reads packets from the stream
func (s *Stream) run() {
	buf := make([]byte, 1024)
	for {
		n, err := s.r.Read(buf)
		if err != nil {
			// end of stream
			return
		}
		// print data from the stream
		log.Printf("%v: received %d bytes", s.net, n)
	}
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
