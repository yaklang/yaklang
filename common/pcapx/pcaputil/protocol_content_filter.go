package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ContentMatcher uses the same linear scan for TUI ASCII-folded searches and
// exact display-filter text/hex searches. Advance carries only a prefix index.
type ContentMatcher struct {
	needle  []byte
	failure []int
	fold    bool
}

func NewContentMatcher(needle []byte, fold bool) *ContentMatcher {
	m := &ContentMatcher{needle: append([]byte(nil), needle...), failure: make([]int, len(needle)), fold: fold}
	for i, b := range m.needle {
		m.needle[i] = m.byte(b)
	}
	for i, j := 1, 0; i < len(m.needle); i++ {
		for j > 0 && m.needle[i] != m.needle[j] {
			j = m.failure[j-1]
		}
		if m.needle[i] == m.needle[j] {
			j++
		}
		m.failure[i] = j
	}
	return m
}
func (m *ContentMatcher) byte(b byte) byte {
	if m.fold && b >= 'A' && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}
func (m *ContentMatcher) Advance(data []byte, state int) (int, bool) {
	if len(m.needle) == 0 {
		return 0, true
	}
	found := false
	for _, b := range data {
		b = m.byte(b)
		for state > 0 && b != m.needle[state] {
			state = m.failure[state-1]
		}
		if b == m.needle[state] {
			state++
		}
		if state == len(m.needle) {
			found = true
			state = m.failure[state-1]
		}
	}
	return state, found
}
func (m *ContentMatcher) Contains(data []byte) bool { _, found := m.Advance(data, 0); return found }

type contentPredicate struct {
	key, scope string
	matcher    *ContentMatcher
}
type contentStreamKey struct {
	domain              CaptureDomain
	tcp, flow           uint64
	source, destination string
}
type contentStreamState struct {
	next     uint64
	states   []int
	found    []bool
	complete bool
}

// DisplayFilterObserver belongs to one serialized capture. It freezes scalar
// content decisions at publication, never payload copies in the output queue.
// Stream predicates concern the continuous observed prefix in one direction;
// gaps, encryption, eviction and resource limits cannot certify absence.
type DisplayFilterObserver struct {
	limited bool
	filter  *DisplayFilter
	streams map[contentStreamKey]*contentStreamState
	order   []contentStreamKey
}

func (f *DisplayFilter) NewObserver() *DisplayFilterObserver {
	return &DisplayFilterObserver{filter: f, streams: make(map[contentStreamKey]*contentStreamState)}
}
func (f *DisplayFilter) Observe(e *ProtocolEvent) DisplayFieldSnapshot {
	return f.NewObserver().Observe(e)
}
func (o *DisplayFilterObserver) Observe(e *ProtocolEvent) DisplayFieldSnapshot {
	snapshot := e.DisplayFieldValues()
	if o == nil || o.filter == nil || len(o.filter.contents) == 0 || e == nil {
		return snapshot
	}
	snapshot.content = make(map[string]fieldTruth, len(o.filter.contents))
	available := e.nativeDiagnosticError == "" && (e.Status == "decoded" || e.Status == "deferred") && len(e.Raw) <= 8<<20
	switch e.Completeness {
	case "encrypted", "incomplete", "headers", "evicted", "limited", "context-required", "unrecognized":
		available = false
	}
	if e.Session["Content Visibility"] == "encrypted" {
		available = false
	}
	if available && e.SourceBytes.SHA256 != "" {
		digest := sha256.Sum256(e.Raw)
		available = fmt.Sprintf("%x", digest) == e.SourceBytes.SHA256
	}
	var stream *contentStreamState
	for _, query := range o.filter.contents {
		if query.scope == "stream.content" && e.Transport == "tcp" && (e.TCPFlowID != 0 || e.FlowID != 0) {
			key := contentStreamKey{e.Domain, e.TCPFlowID, e.FlowID, e.Source, e.Destination}
			stream = o.streams[key]
			if stream == nil {
				if len(o.order) >= 1024 {
					o.limited = true
					delete(o.streams, o.order[0])
					o.order = o.order[1:]
				}
				stream = &contentStreamState{states: make([]int, len(o.filter.contents)), found: make([]bool, len(o.filter.contents)), complete: e.Offset == 0 && !o.limited}
				o.streams[key] = stream
				o.order = append(o.order, key)
			}
			if !available || e.Offset != stream.next {
				for i := range stream.states {
					stream.states[i] = 0
				}
				stream.complete = false
			}
			stream.next = e.Offset + uint64(len(e.Raw))
			break
		}
	}
	for i, query := range o.filter.contents {
		decision := fieldUnknown
		if query.scope == "pdu.content" {
			if available {
				decision = fieldFalse
				if query.matcher.Contains(e.Raw) {
					decision = fieldTrue
				}
			}
		} else if stream != nil {
			if available {
				var found bool
				stream.states[i], found = query.matcher.Advance(e.Raw, stream.states[i])
				stream.found[i] = stream.found[i] || found
			}
			if stream.found[i] {
				decision = fieldTrue
			} else if available && stream.complete {
				decision = fieldFalse
			}
		}
		snapshot.content[query.key] = decision
	}
	return snapshot
}

func (p *filterParser) content(name string) (fieldPredicate, error) {
	var needle []byte
	if p.pos == len(p.tokens) || p.tokens[p.pos].s == ")" || p.tokens[p.pos].s == "and" || p.tokens[p.pos].s == "or" {
		key := name + ":present"
		p.contents = append(p.contents, contentPredicate{key: key, scope: name, matcher: NewContentMatcher(nil, false)})
		return func(m fieldObservation) fieldTruth {
			if v, ok := m.content[key]; ok {
				return v
			}
			return fieldUnknown
		}, nil
	}
	if p.take("contains") {
		if p.pos >= len(p.tokens) || !p.tokens[p.pos].quoted {
			return nil, fmt.Errorf("content contains requires a quoted string")
		}
		needle = []byte(p.tokens[p.pos].s)
		p.pos++
	} else if p.pos < len(p.tokens) && !p.tokens[p.pos].quoted && strings.HasPrefix(p.tokens[p.pos].s, "hex:") {
		var err error
		needle, err = hex.DecodeString(strings.TrimPrefix(p.tokens[p.pos].s, "hex:"))
		p.pos++
		if err != nil {
			return nil, fmt.Errorf("invalid content hex bytes: %w", err)
		}
	} else {
		return nil, fmt.Errorf("%s requires contains or hex:BYTES", name)
	}
	if len(needle) == 0 || len(needle) > 4096 {
		return nil, fmt.Errorf("content search requires 1..4096 bytes")
	}
	key := name + ":" + hex.EncodeToString(needle)
	p.contents = append(p.contents, contentPredicate{key: key, scope: name, matcher: NewContentMatcher(needle, false)})
	return func(m fieldObservation) fieldTruth {
		if value, ok := m.content[key]; ok {
			return value
		}
		return fieldUnknown
	}, nil
}
