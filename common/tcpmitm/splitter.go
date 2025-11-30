package tcpmitm

import (
	"bytes"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// SplitStrategy defines how to segment raw TCP streams into frames.
type SplitStrategy int

const (
	// SplitByTimeGap segments based on silence intervals.
	// If no data arrives within a threshold, the current buffer is treated as a complete frame.
	SplitByTimeGap SplitStrategy = iota

	// SplitByDirection segments when data direction changes.
	// Useful for request-response protocols.
	SplitByDirection

	// SplitBySize segments when buffer reaches a fixed size.
	// Used for logging/forensics, not protocol-aware.
	SplitBySize

	// SplitNone performs no segmentation - raw transparent forwarding.
	SplitNone
)

// Default configuration constants
const (
	// DefaultTimeGapThreshold is the default silence duration to trigger a split.
	// Can be configured to 100ms, 200ms, 300ms, etc.
	DefaultTimeGapThreshold = 100 * time.Millisecond

	// DefaultMaxBufferSize is the default maximum buffer size (8KB).
	// When buffer exceeds this size, a frame is automatically emitted.
	DefaultMaxBufferSize = 8 * 1024

	// DefaultReadBufferSize is the default read buffer size for I/O operations.
	DefaultReadBufferSize = 4 * 1024
)

// Common time gap thresholds for convenience
var (
	TimeGap50ms  = 50 * time.Millisecond
	TimeGap100ms = 100 * time.Millisecond
	TimeGap200ms = 200 * time.Millisecond
	TimeGap300ms = 300 * time.Millisecond
)

// SplitterConfig holds configuration for frame splitting.
type SplitterConfig struct {
	// Strategy to use for splitting
	Strategy SplitStrategy

	// TimeGapThreshold is the silence duration to trigger a split (for SplitByTimeGap).
	// Default: 100ms. Common values: 50ms, 100ms, 200ms, 300ms
	TimeGapThreshold time.Duration

	// MaxBufferSize is the maximum buffer size before forcing a frame split.
	// Default: 8KB. When buffer exceeds this, a frame is automatically emitted.
	MaxBufferSize int

	// MaxFrameSize is the target frame size for SplitBySize strategy.
	// Default: 4096 bytes
	MaxFrameSize int

	// ReadBufferSize is the I/O read buffer size.
	// Default: 4KB
	ReadBufferSize int

	// EnableProtocolAwareSplit enables protocol-aware frame splitting.
	// When true, the splitter will try to respect protocol boundaries.
	EnableProtocolAwareSplit bool
}

// DefaultSplitterConfig returns a default splitter configuration.
// Uses 100ms time gap threshold and 8KB max buffer size.
func DefaultSplitterConfig() *SplitterConfig {
	return &SplitterConfig{
		Strategy:                 SplitByTimeGap,
		TimeGapThreshold:         DefaultTimeGapThreshold,
		MaxBufferSize:            DefaultMaxBufferSize,
		MaxFrameSize:             DefaultMaxBufferSize,
		ReadBufferSize:           DefaultReadBufferSize,
		EnableProtocolAwareSplit: false,
	}
}

// NewSplitterConfig creates a splitter config with custom time gap threshold.
// Common values: 50ms, 100ms, 200ms, 300ms
func NewSplitterConfig(timeGap time.Duration) *SplitterConfig {
	config := DefaultSplitterConfig()
	config.TimeGapThreshold = timeGap
	return config
}

// StreamSplitter handles reading from a stream and splitting into frames.
type StreamSplitter struct {
	config    *SplitterConfig
	direction FrameDirection
	reader    io.Reader
	writer    io.Writer

	frameChan chan *Frame

	// injector for immediate writes
	injector func([]byte) error

	// close signal
	done chan struct{}
}

// NewStreamSplitter creates a new stream splitter.
func NewStreamSplitter(reader io.Reader, writer io.Writer, direction FrameDirection, config *SplitterConfig) *StreamSplitter {
	if config == nil {
		config = DefaultSplitterConfig()
	}

	ss := &StreamSplitter{
		config:    config,
		direction: direction,
		reader:    reader,
		writer:    writer,
		frameChan: make(chan *Frame, 100),
		done:      make(chan struct{}),
	}

	ss.injector = func(data []byte) error {
		_, err := writer.Write(data)
		return err
	}

	return ss
}

// Frames returns the channel of parsed frames.
func (ss *StreamSplitter) Frames() <-chan *Frame {
	return ss.frameChan
}

// Start begins reading from the stream and splitting into frames.
func (ss *StreamSplitter) Start() {
	go ss.readLoop()
}

// Stop stops the splitter.
func (ss *StreamSplitter) Stop() {
	close(ss.done)
}

// readLoop continuously reads from the stream.
func (ss *StreamSplitter) readLoop() {
	defer close(ss.frameChan)

	buf := make([]byte, ss.config.ReadBufferSize)

	switch ss.config.Strategy {
	case SplitNone:
		ss.readLoopTransparent(buf)
	case SplitByTimeGap:
		ss.readLoopTimeGapWithMaxBuffer(buf)
	case SplitByDirection:
		// Direction-based splitting is handled at a higher level
		ss.readLoopTimeGapWithMaxBuffer(buf)
	case SplitBySize:
		ss.readLoopBySize(buf)
	default:
		ss.readLoopTransparent(buf)
	}
}

// readLoopTransparent performs raw forwarding without frame splitting.
func (ss *StreamSplitter) readLoopTransparent(buf []byte) {
	for {
		select {
		case <-ss.done:
			return
		default:
		}

		n, err := ss.reader.Read(buf)
		if n > 0 {
			frame := NewFrame(buf[:n], ss.direction, ss.injector)
			select {
			case ss.frameChan <- frame:
			case <-ss.done:
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Debugf("tcpmitm: read error: %v", err)
			}
			return
		}
	}
}

// readLoopTimeGapWithMaxBuffer segments based on time gaps and enforces max buffer size.
// When buffer exceeds MaxBufferSize (default 8KB), a frame is automatically emitted.
func (ss *StreamSplitter) readLoopTimeGapWithMaxBuffer(buf []byte) {
	var frameBuffer bytes.Buffer
	timer := time.NewTimer(ss.config.TimeGapThreshold)
	defer timer.Stop()

	// Channel to receive read results
	readChan := make(chan struct {
		n   int
		err error
	})

	// Start background reader
	go func() {
		for {
			n, err := ss.reader.Read(buf)
			select {
			case readChan <- struct {
				n   int
				err error
			}{n, err}:
			case <-ss.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// emitFrame emits the current buffer as a frame and resets the buffer
	emitFrame := func() {
		if frameBuffer.Len() > 0 {
			frame := NewFrame(frameBuffer.Bytes(), ss.direction, ss.injector)
			select {
			case ss.frameChan <- frame:
			case <-ss.done:
				return
			}
			frameBuffer.Reset()
		}
	}

	for {
		select {
		case <-ss.done:
			// Flush remaining buffer
			emitFrame()
			return

		case result := <-readChan:
			if result.n > 0 {
				frameBuffer.Write(buf[:result.n])

				// Check if buffer exceeds max size - auto split
				for frameBuffer.Len() >= ss.config.MaxBufferSize {
					// Emit a full frame
					data := make([]byte, ss.config.MaxBufferSize)
					frameBuffer.Read(data)
					frame := NewFrame(data, ss.direction, ss.injector)
					frame.SetUnknownProtocol(true) // Mark as unknown since we're force-splitting
					select {
					case ss.frameChan <- frame:
					case <-ss.done:
						return
					}
				}

				// Reset timer
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(ss.config.TimeGapThreshold)
			}
			if result.err != nil {
				// Flush remaining buffer
				emitFrame()
				if result.err != io.EOF {
					log.Debugf("tcpmitm: read error: %v", result.err)
				}
				return
			}

		case <-timer.C:
			// Time gap exceeded, emit frame
			emitFrame()
			timer.Reset(ss.config.TimeGapThreshold)
		}
	}
}

// readLoopBySize segments when buffer reaches a fixed size.
func (ss *StreamSplitter) readLoopBySize(buf []byte) {
	var frameBuffer bytes.Buffer

	for {
		select {
		case <-ss.done:
			// Flush remaining buffer
			if frameBuffer.Len() > 0 {
				frame := NewFrame(frameBuffer.Bytes(), ss.direction, ss.injector)
				ss.frameChan <- frame
			}
			return
		default:
		}

		n, err := ss.reader.Read(buf)
		if n > 0 {
			frameBuffer.Write(buf[:n])

			// Check if buffer exceeds max size
			for frameBuffer.Len() >= ss.config.MaxFrameSize {
				data := make([]byte, ss.config.MaxFrameSize)
				frameBuffer.Read(data)
				frame := NewFrame(data, ss.direction, ss.injector)
				select {
				case ss.frameChan <- frame:
				case <-ss.done:
					return
				}
			}
		}
		if err != nil {
			// Flush remaining buffer
			if frameBuffer.Len() > 0 {
				frame := NewFrame(frameBuffer.Bytes(), ss.direction, ss.injector)
				ss.frameChan <- frame
			}
			if err != io.EOF {
				log.Debugf("tcpmitm: read error: %v", err)
			}
			return
		}
	}
}

// WriteFrame writes a frame to the output writer.
func (ss *StreamSplitter) WriteFrame(frame *Frame) error {
	if frame.IsDropped() {
		return nil
	}

	// Process inject queue first
	for _, data := range frame.getInjectQueue() {
		if _, err := ss.writer.Write(data); err != nil {
			return err
		}
	}

	// Write the frame data
	if frame.shouldForward() {
		_, err := ss.writer.Write(frame.getFinalBytes())
		return err
	}

	return nil
}
