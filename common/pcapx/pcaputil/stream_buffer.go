package pcaputil

import (
	"io"
	"sync"
)

const streamBlockLimit = 64 << 10

type streamBlock struct {
	data []byte
	read int
	next *streamBlock
}

// tcpStreamBuffer preserves the late-reader contract without growing and
// recopying a contiguous buffer containing the entire unread TCP stream.
// Blocks are private to the reader: callback buffers never alias these bytes.
type tcpStreamBuffer struct {
	mu         sync.Mutex
	ready      *sync.Cond
	head, tail *streamBlock
	readCount  int
	waiters    int
	closed     bool
}

func newTCPStreamBuffer() *tcpStreamBuffer {
	s := &tcpStreamBuffer{}
	s.ready = sync.NewCond(&s.mu)
	return s
}

func (s *tcpStreamBuffer) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(data)
	for len(data) > 0 {
		if s.tail == nil || len(s.tail.data) == cap(s.tail.data) {
			size := len(data)
			if s.tail != nil && size < cap(s.tail.data)*2 {
				size = cap(s.tail.data) * 2
			}
			if size > streamBlockLimit {
				size = streamBlockLimit
			}
			block := &streamBlock{data: make([]byte, 0, size)}
			if s.tail == nil {
				s.head = block
			} else {
				s.tail.next = block
			}
			s.tail = block
		}
		take := cap(s.tail.data) - len(s.tail.data)
		if take > len(data) {
			take = len(data)
		}
		s.tail.data = append(s.tail.data, data[:take]...)
		data = data[take:]
	}
	if s.waiters > 0 {
		s.ready.Broadcast()
	}
	return n, nil
}

func (s *tcpStreamBuffer) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.head == nil || s.head.read == len(s.head.data) {
		if s.closed {
			s.head, s.tail = nil, nil
			return 0, io.EOF
		}
		s.waiters++
		s.ready.Wait()
		s.waiters--
	}
	n := copy(data, s.head.data[s.head.read:])
	s.head.read += n
	s.readCount += n
	if s.head.read == len(s.head.data) {
		if s.head.next != nil {
			s.head = s.head.next
		} else {
			// Reuse the final block when a live reader keeps up with capture.
			s.head.data = s.head.data[:0]
			s.head.read = 0
		}
	}
	return n, nil
}

func (s *tcpStreamBuffer) Count() int { s.mu.Lock(); defer s.mu.Unlock(); return s.readCount }

func (s *tcpStreamBuffer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.ready.Broadcast()
	s.mu.Unlock()
	return nil
}
