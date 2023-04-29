package utils

import (
	"container/ring"
	"fmt"
)

type StringRoundRobinSelector struct {
	l []string
	r *ring.Ring
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
	return s.l[:]
}

func (s *StringRoundRobinSelector) Add(raw ...string) {
	s.l = append(s.l, raw...)
	r := ring.New(len(s.l))
	for _, v := range s.l {
		r.Value = v
		r = r.Next()
	}
	s.r = r
}

func (s *StringRoundRobinSelector) Next() string {
	result := s.r.Value
	s.r = s.r.Next()
	return fmt.Sprintf("%v", result)
}

func (s *StringRoundRobinSelector) Len() int {
	n := 0
	if s.r != nil {
		n = 1
		for p := s.r.Next(); p != s.r; p = p.Next() {
			n++
		}
	}
	return n
}
