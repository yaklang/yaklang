package utils

import (
	"sync/atomic"

	"github.com/segmentio/ksuid"
)

type IDProvider[T any] interface {
	NewID() T
	CurrentID() T
}

// AtomicInt64IDProvider generates monotonically increasing int64 IDs safely under concurrency.
// The first returned ID is startAt, then startAt+1, ...
type AtomicInt64IDProvider struct {
	next atomic.Int64
}

func NewAtomicInt64IDProvider(startAt int64) *AtomicInt64IDProvider {
	p := &AtomicInt64IDProvider{}
	p.next.Store(startAt - 1)
	return p
}

func (p *AtomicInt64IDProvider) NewID() int64 {
	return p.next.Add(1)
}

func (p *AtomicInt64IDProvider) CurrentID() int64 {
	return p.next.Load()
}

// KSUIDProvider generates ksuid.KSUID values. It is safe for concurrent use.
type KSUIDProvider struct {
	last atomic.Pointer[ksuid.KSUID]
}

func NewKSUIDProvider() *KSUIDProvider { return &KSUIDProvider{} }

func (p *KSUIDProvider) NewID() ksuid.KSUID {
	id := ksuid.New()
	p.last.Store(&id)
	return id
}

func (p *KSUIDProvider) CurrentID() ksuid.KSUID {
	current := p.last.Load()
	if current == nil {
		return ksuid.Nil
	}
	return *current
}

// KSUIDStringProvider generates ksuid string values. It is safe for concurrent use.
type KSUIDStringProvider struct {
	last atomic.Pointer[string]
}

func NewKSUIDStringProvider() *KSUIDStringProvider { return &KSUIDStringProvider{} }

func (p *KSUIDStringProvider) NewID() string {
	id := ksuid.New().String()
	p.last.Store(&id)
	return id
}

func (p *KSUIDStringProvider) CurrentID() string {
	current := p.last.Load()
	if current == nil {
		return ""
	}
	return *current
}

var (
	_ IDProvider[int64]       = (*AtomicInt64IDProvider)(nil)
	_ IDProvider[ksuid.KSUID] = (*KSUIDProvider)(nil)
	_ IDProvider[string]      = (*KSUIDStringProvider)(nil)
)
