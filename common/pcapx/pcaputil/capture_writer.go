package pcaputil

import (
	"fmt"
	"io"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

func (p *TrafficPool) observeCapture(size int) {
	p.capturedPackets.Add(1)
	p.capturedBytes.Add(uint64(size))
}

func (c *CaptureConfig) requiresExclusiveHandle() bool {
	return c.reassemblyOptions.Workers > 1 || c.binParser != nil || c.recorder != nil || c.captureBuffer > 0
}

// WithCaptureBufferSize requests a native buffer per live device before opening
// it. Zero keeps the backend default; otherwise use 64 KiB through 256 MiB.
// A larger buffer absorbs bursts, but cannot fix sustained analysis overload.
func WithCaptureBufferSize(size int) CaptureOption {
	return func(c *CaptureConfig) error {
		if size != 0 && (size < 64<<10 || size > 256<<20) {
			return fmt.Errorf("capture buffer must be zero or 64 KiB through 256 MiB")
		}
		if size > 0 && c.EnableCache {
			return fmt.Errorf("capture buffer requires an exclusive capture handle")
		}
		c.captureBuffer = size
		return nil
	}
}

// WithCaptureWriter records packets before analysis without constructing public
// packet objects. It writes classic nanosecond pcap with one link type. Multiple
// devices with different link types must use separate captures. Writes provide
// backpressure; errors stop capture instead of silently discarding recordings.
// The caller owns the writer and must flush/close it after Start/ReplayPcap.
func WithCaptureWriter(w io.Writer) CaptureOption {
	return func(c *CaptureConfig) error {
		if w == nil {
			return fmt.Errorf("capture writer is nil")
		}
		if c.EnableCache {
			return fmt.Errorf("capture writer requires an exclusive capture handle")
		}
		c.recorder = &captureWriter{output: w}
		return nil
	}
}

type captureWriter struct {
	mu     sync.Mutex
	output io.Writer
	writer *pcapgo.Writer
	link   layers.LinkType
	err    error
}

func (r *captureWriter) initLocked(link layers.LinkType) error {
	if r.err != nil {
		return r.err
	}
	if r.writer == nil {
		r.writer, r.link = pcapgo.NewWriterNanos(r.output), link
		r.err = r.writer.WriteFileHeader(16<<20, link)
	} else if link != r.link {
		r.err = fmt.Errorf("capture writer: link type changed from %v to %v; record interfaces separately", r.link, link)
	}
	return r.err
}

func (r *captureWriter) init(link layers.LinkType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initLocked(link)
}

func (r *captureWriter) write(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.initLocked(link); err != nil {
		return err
	}
	r.err = r.writer.WritePacket(ci, raw)
	return r.err
}
