package pcaputil

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
	"github.com/yaklang/yaklang/common/log"
)

// StreamFactory implements tcpassembly.StreamFactory
type StreamFactory struct{}

// Stream implements tcpassembly.Stream
type Stream struct {
	net, transport gopacket.Flow
	r              tcpreader.ReaderStream
}

// New creates a new stream
func (StreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	s := &Stream{
		net:       net,
		transport: transport,
		r:         tcpreader.NewReaderStream(),
	}
	go s.run() // start reading in the background
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
