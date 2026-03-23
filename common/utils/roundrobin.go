package utils

import (
	"container/ring"
	"fmt"
	"sync"
)

type StringRoundRobinSelector struct {
	mu sync.Mutex
	l  []string
	r  *ring.Ring
}

func NewStringRoundRobinSelector(l ...string) *StringRoundRobinSelector {
	r := ring.New(len(l))
	for _, v := range l {
		r.Value = v
		r = r.Next()
	}
	return &StringRoundRobinSelector{
		l: l,
		r: r,
	}
}

func (s *StringRoundRobinSelector) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.l...)
}

func (s *StringRoundRobinSelector) Add(raw ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.l = append(s.l, raw...)
	r := ring.New(len(s.l))
	for _, v := range s.l {
		r.Value = v
		r = r.Next()
	}
	s.r = r
}

func (s *StringRoundRobinSelector) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.r == nil {
		return ""
	}
	result := s.r.Value
	s.r = s.r.Next()
	return fmt.Sprintf("%v", result)
}

func (s *StringRoundRobinSelector) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	if s.r != nil {
		n = 1
		for p := s.r.Next(); p != s.r; p = p.Next() {
			n++
		}
	}
	return n
}
