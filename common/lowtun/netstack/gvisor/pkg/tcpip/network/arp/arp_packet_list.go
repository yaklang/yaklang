package arp

import (
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"time"
)

// ElementMapper provides an identity mapping by default.
//
// This can be replaced to provide a struct that maps elements to linker
// objects, if they are not the same. An ElementMapper is not typically
// required if: Linker is left as is, Element is left as is, or Linker and
// Element are the same type.
type arpPacketElementMapper struct{}

// linkerFor maps an Element to a Linker.
//
// This default implementation should be inlined.
//
//go:nosplit
func (arpPacketElementMapper) linkerFor(elem *arpPacket) *arpPacket { return elem }

// List is an intrusive list. Entries can be added to or removed from the list
// in O(1) time and with no additional memory allocations.
//
// The zero value for List is an empty list ready to use.
//
// To iterate over a list (where l is a List):
//
//	for e := l.Front(); e != nil; e = e.Next() {
//		// do something with e.
//	}
//
// +stateify savable
type arpPacketList struct {
	head *arpPacket
	tail *arpPacket
}

// Reset resets list l to the empty state.
func (l *arpPacketList) Reset() {
	l.head = nil
	l.tail = nil
}

// Empty returns true iff the list is empty.
//
//go:nosplit
func (l *arpPacketList) Empty() bool {
	return l.head == nil
}

// Front returns the first element of list l or nil.
//
//go:nosplit
func (l *arpPacketList) Front() *arpPacket {
	return l.head
}

// Back returns the last element of list l or nil.
//
//go:nosplit
func (l *arpPacketList) Back() *arpPacket {
	return l.tail
}

// Len returns the number of elements in the list.
//
// NOTE: This is an O(n) operation.
//
//go:nosplit
func (l *arpPacketList) Len() (count int) {
	for e := l.Front(); e != nil; e = (arpPacketElementMapper{}.linkerFor(e)).Next() {
		count++
	}
	return count
}

// PushFront inserts the element e at the front of list l.
//
//go:nosplit
func (l *arpPacketList) PushFront(e *arpPacket) {
	linker := arpPacketElementMapper{}.linkerFor(e)
	linker.SetNext(l.head)
	linker.SetPrev(nil)
	if l.head != nil {
		arpPacketElementMapper{}.linkerFor(l.head).SetPrev(e)
	} else {
		l.tail = e
	}

	l.head = e
}

// PushFrontList inserts list m at the start of list l, emptying m.
//
//go:nosplit
func (l *arpPacketList) PushFrontList(m *arpPacketList) {
	if l.head == nil {
		l.head = m.head
		l.tail = m.tail
	} else if m.head != nil {
		arpPacketElementMapper{}.linkerFor(l.head).SetPrev(m.tail)
		arpPacketElementMapper{}.linkerFor(m.tail).SetNext(l.head)

		l.head = m.head
	}
	m.head = nil
	m.tail = nil
}

// PushBack inserts the element e at the back of list l.
//
//go:nosplit
func (l *arpPacketList) PushBack(e *arpPacket) {
	linker := arpPacketElementMapper{}.linkerFor(e)
	linker.SetNext(nil)
	linker.SetPrev(l.tail)
	if l.tail != nil {
		arpPacketElementMapper{}.linkerFor(l.tail).SetNext(e)
	} else {
		l.head = e
	}

	l.tail = e
}

// PushBackList inserts list m at the end of list l, emptying m.
//
//go:nosplit
func (l *arpPacketList) PushBackList(m *arpPacketList) {
	if l.head == nil {
		l.head = m.head
		l.tail = m.tail
	} else if m.head != nil {
		arpPacketElementMapper{}.linkerFor(l.tail).SetNext(m.head)
		arpPacketElementMapper{}.linkerFor(m.head).SetPrev(l.tail)

		l.tail = m.tail
	}
	m.head = nil
	m.tail = nil
}

// InsertAfter inserts e after b.
//
//go:nosplit
func (l *arpPacketList) InsertAfter(b, e *arpPacket) {
	bLinker := arpPacketElementMapper{}.linkerFor(b)
	eLinker := arpPacketElementMapper{}.linkerFor(e)

	a := bLinker.Next()

	eLinker.SetNext(a)
	eLinker.SetPrev(b)
	bLinker.SetNext(e)

	if a != nil {
		arpPacketElementMapper{}.linkerFor(a).SetPrev(e)
	} else {
		l.tail = e
	}
}

// InsertBefore inserts e before a.
//
//go:nosplit
func (l *arpPacketList) InsertBefore(a, e *arpPacket) {
	aLinker := arpPacketElementMapper{}.linkerFor(a)
	eLinker := arpPacketElementMapper{}.linkerFor(e)

	b := aLinker.Prev()
	eLinker.SetNext(a)
	eLinker.SetPrev(b)
	aLinker.SetPrev(e)

	if b != nil {
		arpPacketElementMapper{}.linkerFor(b).SetNext(e)
	} else {
		l.head = e
	}
}

// Remove removes e from l.
//
//go:nosplit
func (l *arpPacketList) Remove(e *arpPacket) {
	linker := arpPacketElementMapper{}.linkerFor(e)
	prev := linker.Prev()
	next := linker.Next()

	if prev != nil {
		arpPacketElementMapper{}.linkerFor(prev).SetNext(next)
	} else if l.head == e {
		l.head = next
	}

	if next != nil {
		arpPacketElementMapper{}.linkerFor(next).SetPrev(prev)
	} else if l.tail == e {
		l.tail = prev
	}

	linker.SetNext(nil)
	linker.SetPrev(nil)
}

// Entry is a default implementation of Linker. Users can add anonymous fields
// of this type to their structs to make them automatically implement the
// methods needed by List.
//
// +stateify savable
type arpPacketEntry struct {
	next *arpPacket
	prev *arpPacket
}

// Next returns the entry that follows e in the list.
//
//go:nosplit
func (e *arpPacketEntry) Next() *arpPacket {
	return e.next
}

// Prev returns the entry that precedes e in the list.
//
//go:nosplit
func (e *arpPacketEntry) Prev() *arpPacket {
	return e.prev
}

// SetNext assigns 'entry' as the entry that follows e in the list.
//
//go:nosplit
func (e *arpPacketEntry) SetNext(elem *arpPacket) {
	e.next = elem
}

// SetPrev assigns 'entry' as the entry that precedes e in the list.
//
//go:nosplit
func (e *arpPacketEntry) SetPrev(elem *arpPacket) {
	e.prev = elem
}

type arpPacket struct {
	arpPacketEntry
	senderMac  tcpip.LinkAddress
	data       *stack.PacketBuffer
	receivedAt time.Time `state:".(int64)"`
}
