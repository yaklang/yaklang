// Package newcrawlerx
package newcrawlerx

import (
	"errors"
	"fmt"
	"sync/atomic"
)

type T interface{}

var ErrIsEmpty = errors.New("ring buffer is empty")

// cell
//
//	@Description: https://github.com/zngw/zchan
type cell struct {
	Data     []T
	fullFlag bool
	next     *cell
	pre      *cell

	r int
	w int
}

// RingBuffer
//
//	@Description: https://github.com/zngw/zchan
type RingBuffer struct {
	cellSize  int
	cellCount int
	count     int32

	readCell  *cell
	writeCell *cell
}

func NewRingBuffer(cellSize int) (buf *RingBuffer, err error) {
	if cellSize <= 0 || cellSize&(cellSize-1) != 0 {
		err = fmt.Errorf("init size must be power of 2")
		return
	}
	rootCell := &cell{
		Data: make([]T, cellSize),
	}
	lastCell := &cell{
		Data: make([]T, cellSize),
	}
	rootCell.pre = lastCell
	lastCell.pre = rootCell
	rootCell.next = lastCell
	lastCell.next = rootCell

	buf = &RingBuffer{
		cellSize:  cellSize,
		cellCount: 2,
		count:     0,
		readCell:  rootCell,
		writeCell: rootCell,
	}
	return
}

func (ringBuffer *RingBuffer) Read() (data T, err error) {
	if ringBuffer.IsEmpty() {
		err = ErrIsEmpty
		return
	}
	data = ringBuffer.readCell.Data[ringBuffer.readCell.r]
	ringBuffer.readCell.r++
	atomic.AddInt32(&ringBuffer.count, -1)

	if ringBuffer.readCell.r == ringBuffer.cellSize {
		ringBuffer.readCell.r = 0
		ringBuffer.readCell.fullFlag = false
		ringBuffer.readCell = ringBuffer.readCell.next
	}
	return
}

func (ringBuffer *RingBuffer) Pop() (data T) {
	data, err := ringBuffer.Read()
	if errors.Is(err, ErrIsEmpty) {
		panic(ErrIsEmpty.Error())
	}
	return
}

func (ringBuffer *RingBuffer) Peek() (data T) {
	if ringBuffer.IsEmpty() {
		panic(ErrIsEmpty.Error())
	}
	data = ringBuffer.readCell.Data[ringBuffer.readCell.r]
	return
}

func (ringBuffer *RingBuffer) Write(value T) {
	ringBuffer.writeCell.Data[ringBuffer.writeCell.w] = value
	ringBuffer.writeCell.w++
	atomic.AddInt32(&ringBuffer.count, 1)

	if ringBuffer.writeCell.w == ringBuffer.cellSize {
		ringBuffer.writeCell.w = 0
		ringBuffer.writeCell.fullFlag = true
		ringBuffer.writeCell = ringBuffer.writeCell.next
	}

	if ringBuffer.writeCell.fullFlag == true {
		ringBuffer.grow()
	}
}

func (ringBuffer *RingBuffer) grow() {
	newCell := &cell{
		Data: make([]T, ringBuffer.cellSize),
	}
	preCell := ringBuffer.writeCell.pre
	preCell.next = newCell
	newCell.pre = preCell
	newCell.next = ringBuffer.writeCell
	ringBuffer.writeCell.pre = newCell

	ringBuffer.writeCell = ringBuffer.writeCell.pre
	ringBuffer.cellCount++
}

func (ringBuffer *RingBuffer) IsEmpty() bool {
	return ringBuffer.Len() == 0
}

func (ringBuffer *RingBuffer) Capacity() int {
	return ringBuffer.cellCount * ringBuffer.cellSize
}

func (ringBuffer *RingBuffer) Len() (count int) {
	count = int(ringBuffer.count)
	return
}

func (ringBuffer *RingBuffer) Reset() {
	if ringBuffer.count == 0 && ringBuffer.cellCount == 2 {
		return
	}

	lastCell := ringBuffer.readCell.next

	lastCell.w = 0
	lastCell.r = 0
	ringBuffer.readCell.r = 0
	ringBuffer.readCell.w = 0
	ringBuffer.cellCount = 2
	ringBuffer.count = 0

	lastCell.next = ringBuffer.readCell
}

type UChan struct {
	In     chan<- T
	Out    <-chan T
	buffer *RingBuffer
}

func (uChan *UChan) Len() int {
	return len(uChan.In) + uChan.BufLen() + len(uChan.Out)
}

func (uChan *UChan) BufLen() int {
	return uChan.buffer.Len()
}

func NewUChan(initCapacity int) (ch *UChan, err error) {
	rb, err := NewRingBuffer(512)
	if err != nil {
		return
	}
	in := make(chan T, initCapacity)
	out := make(chan T, initCapacity)
	ch = &UChan{In: in, Out: out, buffer: rb}
	go process(in, out, ch)
	return
}

func process(in, out chan T, ch *UChan) {
	defer close(out)
loop:
	for {
		value, ok := <-in
		if !ok {
			break loop
		}

		if ch.buffer.Len() > 0 {
			ch.buffer.Write(value)
		} else {
			select {
			case out <- value:
				continue
			default:

			}
			ch.buffer.Write(value)
		}
		for ch.buffer.Len() > 0 {
			select {
			case val, ok := <-in:
				if !ok {
					break loop
				}
				ch.buffer.Write(val)
			case out <- ch.buffer.Peek():
				ch.buffer.Pop()
				if ch.buffer.IsEmpty() {
					ch.buffer.Reset()
				}
			}
		}
	}
	for ch.buffer.Len() > 0 {
		out <- ch.buffer.Pop()
	}
}
