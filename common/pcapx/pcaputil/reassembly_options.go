package pcaputil

import (
	"fmt"
	"time"
)

// TCPReassemblyOptions controls passive TCP reassembly. Zero limits select the
// defaults. Limits close the affected flow with reason "resource-limit"; gaps
// are never silently concatenated. Limits apply to captured TCP payload bytes.
type TCPReassemblyOptions struct {
	// Stream emits ordered data through the arrived callback without retaining
	// a readable copy of the stream. Reassembled callbacks receive chunks of at
	// most MaxFrameBytes. Read/GetBuffer return EOF in this mode. Consumers may
	// retain callback frames; their memory is then owned by the consumer.
	Stream                  bool
	MaxFrameBytes           int
	MaxPendingBytes         int
	MaxPendingSegments      int
	MaxTotalPendingBytes    int
	MaxTotalPendingSegments int
	MaxFlows                int
	IdleTimeout             time.Duration
	// Maximum forward sequence distance, including payload. Keeping every
	// pending interval within half the sequence space makes wrap comparisons
	// transitive. Far-future segments are rejected without advancing the flow.
	MaxSequenceGap int
	// Workers > 1 enables asynchronous ingestion with concurrent callbacks
	// across flows. Each bidirectional flow stays ordered on one worker.
	Workers             int
	WorkerQueueDepth    int
	WorkerBatchPackets  int
	WorkerBatchBytes    int
	WorkerFlushInterval time.Duration
}

func (o TCPReassemblyOptions) normalized() (TCPReassemblyOptions, error) {
	if o.MaxSequenceGap < 0 || uint64(o.MaxSequenceGap) >= 1<<31 {
		return o, fmt.Errorf("TCP sequence gap must be positive and less than 2^31")
	}
	if o.MaxSequenceGap == 0 {
		o.MaxSequenceGap = 64 << 20
	}
	if o.Workers < 0 || o.Workers > 64 || o.WorkerQueueDepth < 0 || o.WorkerQueueDepth > 64 || o.WorkerBatchPackets < 0 || o.WorkerBatchPackets > 4096 || o.WorkerBatchBytes < 0 || o.WorkerBatchBytes > 16<<20 || o.WorkerFlushInterval < 0 {
		return o, fmt.Errorf("invalid TCP worker limits (workers <= 64, queue depth <= 64, batch packets <= 4096, batch bytes <= 16 MiB)")
	}
	if o.Workers == 0 {
		o.Workers = 1
	}
	if o.WorkerQueueDepth == 0 {
		o.WorkerQueueDepth = 2
	}
	if o.WorkerBatchPackets == 0 {
		o.WorkerBatchPackets = 256
	}
	if o.WorkerBatchBytes == 0 {
		o.WorkerBatchBytes = 256 << 10
	}
	if o.WorkerFlushInterval == 0 {
		o.WorkerFlushInterval = time.Millisecond
	}
	if o.MaxFrameBytes < 0 || o.MaxPendingBytes < 0 || o.MaxPendingSegments < 0 || o.MaxTotalPendingBytes < 0 || o.MaxTotalPendingSegments < 0 || o.MaxFlows < 0 || o.IdleTimeout < 0 {
		return o, fmt.Errorf("TCP reassembly limits must not be negative")
	}
	if o.MaxFrameBytes == 0 {
		o.MaxFrameBytes = 64 << 10
	}
	if o.MaxPendingBytes == 0 {
		o.MaxPendingBytes = 8 << 20
	}
	if o.MaxPendingSegments == 0 {
		o.MaxPendingSegments = 8192
	}
	if o.MaxTotalPendingBytes == 0 {
		o.MaxTotalPendingBytes = 64 << 20
	}
	if o.MaxTotalPendingSegments == 0 {
		o.MaxTotalPendingSegments = 65536
	}
	if o.MaxFlows == 0 {
		o.MaxFlows = 65536
	}
	if o.IdleTimeout == 0 {
		o.IdleTimeout = 30 * time.Second
	}
	return o, nil
}

// WithTCPReassemblyWorkers opts into concurrent callbacks across flows. Data
// accepted before EOF/cancellation is drained before Start returns. A full
// bounded queue blocks capture; kernel drops still require device statistics.
func WithTCPReassemblyWorkers(workers int) CaptureOption {
	return func(c *CaptureConfig) error {
		if workers < 1 || workers > 64 {
			return fmt.Errorf("TCP workers must be between 1 and 64")
		}
		c.reassemblyOptions.Workers = workers
		return nil
	}
}

// WithTCPReassemblyOptions configures resource limits and optional streaming.
// Stream is intended for large transfers handled by data callbacks; it cannot
// be combined with the built-in HTTP/TLS parsers, which require whole messages.
func WithTCPReassemblyOptions(options TCPReassemblyOptions) CaptureOption {
	return func(c *CaptureConfig) error {
		var err error
		c.reassemblyOptions, err = options.normalized()
		return err
	}
}

// WithTCPReassemblyStream enables bounded data callbacks for very large flows.
// The chunk size must be positive. Use arrived callbacks for immediate delivery.
func WithTCPReassemblyStream(chunkBytes int) CaptureOption {
	return func(c *CaptureConfig) error {
		if chunkBytes <= 0 {
			return fmt.Errorf("TCP stream chunk size must be positive")
		}
		c.reassemblyOptions.Stream = true
		c.reassemblyOptions.MaxFrameBytes = chunkBytes
		return nil
	}
}
