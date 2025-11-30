package tcpmitm

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// FrameQueue manages a queue of frames for a single direction of a connection.
// It provides peek functionality to look ahead at buffered frames while maintaining
// sequential callback invocation within the same connection.
type FrameQueue struct {
	mu sync.RWMutex

	// frames is the queue of frames waiting to be processed
	frames []*Frame

	// peekBuffer holds frames that have been peeked but not yet processed
	peekBuffer []*Frame

	// closed indicates if the queue is closed
	closed bool

	// notifyNewFrame is used to signal when new frames are available
	notifyNewFrame chan struct{}

	// direction of this queue
	direction FrameDirection

	// parent context reference for the connection
	connCtx *ConnectionContext
}

// NewFrameQueue creates a new FrameQueue for the given direction.
func NewFrameQueue(direction FrameDirection, connCtx *ConnectionContext) *FrameQueue {
	return &FrameQueue{
		frames:         make([]*Frame, 0, 64),
		peekBuffer:     make([]*Frame, 0, 16),
		notifyNewFrame: make(chan struct{}, 1),
		direction:      direction,
		connCtx:        connCtx,
	}
}

// Enqueue adds a frame to the queue.
func (q *FrameQueue) Enqueue(frame *Frame) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	// Set the queue reference on the frame for peek functionality
	frame.setQueue(q)

	q.frames = append(q.frames, frame)

	// Notify waiting consumers
	select {
	case q.notifyNewFrame <- struct{}{}:
	default:
	}
}

// Dequeue removes and returns the next frame from the queue.
// Returns nil if the queue is empty.
func (q *FrameQueue) Dequeue() *Frame {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.frames) == 0 {
		return nil
	}

	frame := q.frames[0]
	q.frames = q.frames[1:]
	return frame
}

// DequeueWait waits for and returns the next frame from the queue.
// Returns nil if the context is cancelled or queue is closed.
func (q *FrameQueue) DequeueWait(ctx context.Context) *Frame {
	for {
		// Check if there's a frame available
		q.mu.Lock()
		if len(q.frames) > 0 {
			frame := q.frames[0]
			q.frames = q.frames[1:]
			q.mu.Unlock()
			return frame
		}
		closed := q.closed
		q.mu.Unlock()

		if closed {
			return nil
		}

		// Wait for new frame or context cancellation
		select {
		case <-ctx.Done():
			return nil
		case <-q.notifyNewFrame:
			continue
		}
	}
}

// PeekNext returns the next frame(s) in the queue without removing them.
// The offset parameter specifies how many frames ahead to look.
// offset=0 means the next frame, offset=1 means the one after that, etc.
func (q *FrameQueue) PeekNext(offset int) *Frame {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if offset < 0 || offset >= len(q.frames) {
		return nil
	}
	return q.frames[offset]
}

// PeekNextN returns up to n frames from the queue without removing them.
func (q *FrameQueue) PeekNextN(n int) []*Frame {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if n <= 0 {
		return nil
	}

	if n > len(q.frames) {
		n = len(q.frames)
	}

	result := make([]*Frame, n)
	copy(result, q.frames[:n])
	return result
}

// PeekNextRawBytes returns the raw bytes of the next frame without removing it.
// Returns nil if no more frames are available.
func (q *FrameQueue) PeekNextRawBytes() []byte {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.frames) == 0 {
		return nil
	}
	return q.frames[0].GetRawBytes()
}

// PeekAllRawBytes returns all buffered raw bytes concatenated.
func (q *FrameQueue) PeekAllRawBytes() []byte {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var totalLen int
	for _, f := range q.frames {
		totalLen += len(f.rawBytes)
	}

	if totalLen == 0 {
		return nil
	}

	result := make([]byte, 0, totalLen)
	for _, f := range q.frames {
		result = append(result, f.rawBytes...)
	}
	return result
}

// Size returns the number of frames in the queue.
func (q *FrameQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.frames)
}

// BufferedBytes returns the total bytes currently buffered in the queue.
func (q *FrameQueue) BufferedBytes() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	total := 0
	for _, f := range q.frames {
		total += len(f.rawBytes)
	}
	return total
}

// Close closes the queue and signals any waiting consumers.
func (q *FrameQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.closed = true
	close(q.notifyNewFrame)
}

// IsClosed returns whether the queue is closed.
func (q *FrameQueue) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// WaitForFrameOrTimeout waits for a new frame with timeout.
// Returns true if a frame arrived, false on timeout.
func (q *FrameQueue) WaitForFrameOrTimeout(timeout time.Duration) bool {
	select {
	case <-q.notifyNewFrame:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ConnectionContext holds the context for a single hijacked connection.
// It manages frame queues for both directions and provides methods for
// accessing buffered data across the connection.
type ConnectionContext struct {
	mu sync.RWMutex

	// Flow information
	flow *ConnectionFlow

	// Frame queues for each direction
	clientToServerQueue *FrameQueue
	serverToClientQueue *FrameQueue

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Metadata storage for user-defined data
	metadata map[string]interface{}

	// State tracking
	closed bool

	// Timestamp when connection was established
	createdAt time.Time

	// Frame counter for this connection
	frameCounter uint64
}

// NewConnectionContext creates a new ConnectionContext for the given flow.
func NewConnectionContext(parentCtx context.Context, flow *ConnectionFlow) *ConnectionContext {
	ctx, cancel := context.WithCancel(parentCtx)

	cc := &ConnectionContext{
		flow:      flow,
		ctx:       ctx,
		cancel:    cancel,
		metadata:  make(map[string]interface{}),
		createdAt: time.Now(),
	}

	cc.clientToServerQueue = NewFrameQueue(DirectionClientToServer, cc)
	cc.serverToClientQueue = NewFrameQueue(DirectionServerToClient, cc)

	return cc
}

// GetFlow returns the connection flow information.
func (cc *ConnectionContext) GetFlow() *ConnectionFlow {
	return cc.flow
}

// GetQueue returns the frame queue for the specified direction.
func (cc *ConnectionContext) GetQueue(direction FrameDirection) *FrameQueue {
	if direction == DirectionClientToServer {
		return cc.clientToServerQueue
	}
	return cc.serverToClientQueue
}

// GetClientToServerQueue returns the C->S frame queue.
func (cc *ConnectionContext) GetClientToServerQueue() *FrameQueue {
	return cc.clientToServerQueue
}

// GetServerToClientQueue returns the S->C frame queue.
func (cc *ConnectionContext) GetServerToClientQueue() *FrameQueue {
	return cc.serverToClientQueue
}

// Context returns the connection's context.
func (cc *ConnectionContext) Context() context.Context {
	return cc.ctx
}

// Close closes the connection context and all queues.
func (cc *ConnectionContext) Close() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.closed {
		return
	}

	cc.closed = true
	cc.cancel()
	cc.clientToServerQueue.Close()
	cc.serverToClientQueue.Close()

	log.Debugf("tcpmitm: connection context closed for %s", cc.flow.String())
}

// IsClosed returns whether the connection context is closed.
func (cc *ConnectionContext) IsClosed() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.closed
}

// SetMetadata stores user-defined metadata.
func (cc *ConnectionContext) SetMetadata(key string, value interface{}) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.metadata[key] = value
}

// GetMetadata retrieves user-defined metadata.
func (cc *ConnectionContext) GetMetadata(key string) (interface{}, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	v, ok := cc.metadata[key]
	return v, ok
}

// GetCreatedAt returns when the connection was established.
func (cc *ConnectionContext) GetCreatedAt() time.Time {
	return cc.createdAt
}

// GetDuration returns how long the connection has been active.
func (cc *ConnectionContext) GetDuration() time.Duration {
	return time.Since(cc.createdAt)
}

// IncrementFrameCounter atomically increments and returns the frame counter.
func (cc *ConnectionContext) IncrementFrameCounter() uint64 {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.frameCounter++
	return cc.frameCounter
}

// GetFrameCount returns the total number of frames processed.
func (cc *ConnectionContext) GetFrameCount() uint64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.frameCounter
}

// TotalBufferedBytes returns the total bytes buffered in both queues.
func (cc *ConnectionContext) TotalBufferedBytes() int {
	return cc.clientToServerQueue.BufferedBytes() + cc.serverToClientQueue.BufferedBytes()
}

// TotalBufferedFrames returns the total frames buffered in both queues.
func (cc *ConnectionContext) TotalBufferedFrames() int {
	return cc.clientToServerQueue.Size() + cc.serverToClientQueue.Size()
}
