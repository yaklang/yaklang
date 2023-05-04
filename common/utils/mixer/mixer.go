package mixer

import (
	"container/ring"
	"errors"
	"fmt"
	"io"
	"sync"
)

type Mixer struct {
	indexToRing, ringToIndex *sync.Map
	totalTable               *sync.Map
	ringTotal                int
	tickRing                 *StringRing
	total                    uint64

	ringCounter *sync.Map
}

func (m *Mixer) next(sr *StringRing) error {
	sr.Next()
	raw, _ := m.ringCounter.LoadOrStore(sr, 0)
	count := raw.(int) + 1
	next, last := count/sr.Len(), count%sr.Len()
	m.ringCounter.Store(sr, last)
	if next > 0 {
		indexRaw, _ := m.ringToIndex.Load(sr)
		index := indexRaw.(int)
		if index-1 < 0 {
			return io.EOF
		}
		nxtSr, _ := m.indexToRing.Load(index - 1)
		return m.next(nxtSr.(*StringRing))
	}
	return nil
}

func (m *Mixer) Next() error {
	return m.next(m.tickRing)
}

func (m *Mixer) Value() []string {
	results := make([]string, m.ringTotal)
	m.indexToRing.Range(func(key, value interface{}) bool {
		r := value.(*StringRing)
		index := key.(int)
		results[index] = r.Value()
		return true
	})
	return results
}

type StringRing struct {
	ring *ring.Ring
}

func (r *StringRing) Value() string {
	return r.ring.Value.(string)
}

func (r *StringRing) Next() {
	r.ring = r.ring.Next()
}

func (r *StringRing) Len() int {
	return r.ring.Len()
}

func NewStringRing(l ...string) *StringRing {
	length := len(l)

	r := ring.New(length)
	for _, value := range l {
		r.Value = value
		r = r.Next()
	}

	sr := &StringRing{
		ring: r,
	}
	return sr
}

func MixForEach(
	list [][]string,
	callback func(i ...string) error,
) error {
	if callback == nil {
		return errors.New("empty mixer callback")
	}

	m, err := NewMixer(list...)
	if err != nil {
		return err
	}

	for {
		if err := callback(m.Value()...); err != nil {
			return err
		}

		err := m.Next()
		if err != nil {
			return nil
		}
	}
}

func NewMixer(lists ...[]string) (*Mixer, error) {
	indexToRingMap := new(sync.Map)
	ringToIndexMap := new(sync.Map)
	totalTable := new(sync.Map)

	var total uint64 = 1
	for index, l := range lists {
		length := len(l)
		total *= uint64(length)
		if length <= 0 {
			return nil, errors.New(fmt.Sprintf("failed to create mixer for empty list: %d", index))
		}

		r := ring.New(length)
		for _, value := range l {
			r.Value = value
			r = r.Next()
		}

		sr := &StringRing{
			ring: r,
		}
		totalTable.Store(sr, length)
		indexToRingMap.Store(index, sr)
		ringToIndexMap.Store(sr, index)
	}

	if tickRing, loaded := indexToRingMap.Load(len(lists) - 1); loaded {
		return &Mixer{
			indexToRing: indexToRingMap,
			ringToIndex: ringToIndexMap,
			totalTable:  totalTable,
			ringTotal:   len(lists),
			tickRing:    tickRing.(*StringRing),
			total:       total,
			ringCounter: new(sync.Map),
		}, nil
	} else {
		return nil, errors.New("no selected tick ring")
	}
}

func (m *Mixer) Size() uint64 {
	return m.total
}
