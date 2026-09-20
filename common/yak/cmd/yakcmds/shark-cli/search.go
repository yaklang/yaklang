package sharkcli

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"
)

type packetSearch struct {
	groups   [][]searchTerm // OR of AND groups.
	dynamic  bool
	metadata bool
}
type searchTerm struct {
	kind, value   string
	needle        []byte
	matcher       *foldMatcher
	exact, negate bool
	network       *net.IPNet
	ports         [][2]uint16
}
type searchToken struct {
	value       string
	alternative bool
}

func searchTokens(input string) ([]searchToken, error) {
	var tokens []searchToken
	var b strings.Builder
	var quote rune
	escaped, quoted := false, false
	flush := func() {
		if b.Len() > 0 {
			value := b.String()
			if quoted || value != "AND" {
				tokens = append(tokens, searchToken{value, !quoted && value == "OR"})
			}
		}
		b.Reset()
		quoted = false
	}
	for _, r := range input {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			quoted = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		if r == '|' {
			flush()
			tokens = append(tokens, searchToken{alternative: true})
			continue
		}
		b.WriteRune(r)
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unclosed quote or trailing escape")
	}
	flush()
	if len(tokens) > 64 {
		return nil, fmt.Errorf("use at most 64 search terms")
	}
	return tokens, nil
}

func compileSearch(input string) (*packetSearch, error) {
	if len(input) > 4096 {
		return nil, fmt.Errorf("search exceeds 4096 bytes")
	}
	tokens, err := searchTokens(input)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	q := &packetSearch{groups: [][]searchTerm{{}}}
	for _, token := range tokens {
		last := len(q.groups) - 1
		if token.alternative {
			if len(q.groups[last]) == 0 {
				return nil, fmt.Errorf("OR needs a condition on both sides")
			}
			q.groups = append(q.groups, nil)
			continue
		}
		t, err := compileSearchTerm(token.value)
		if err != nil {
			return nil, err
		}
		q.dynamic = q.dynamic || t.kind == "any" || t.kind == "proto" || t.kind == "stream"
		q.metadata = q.metadata || t.kind != "content" && t.kind != "hex" && t.kind != "text" && t.kind != "stream"
		q.groups[last] = append(q.groups[last], t)
	}
	if len(q.groups[len(q.groups)-1]) == 0 {
		return nil, fmt.Errorf("OR needs a condition on both sides")
	}
	return q, nil
}

func compileSearchTerm(input string) (searchTerm, error) {
	t := searchTerm{kind: "any"}
	if strings.HasPrefix(input, "!") || strings.HasPrefix(input, "-") {
		t.negate = true
		input = input[1:]
	}
	value := input
	if at := strings.IndexAny(input, ":="); at > 0 {
		key := strings.ToLower(input[:at])
		negative := strings.HasSuffix(key, "!")
		key = strings.TrimSuffix(key, "!")
		aliases := map[string]string{"proto": "proto", "protocol": "proto", "ip": "ip", "ip.str": "ip", "src": "src", "dst": "dst", "port": "port", "sport": "sport", "dport": "dport", "content": "content", "text": "text", "hex": "hex", "info": "info", "stream": "stream"}
		if kind := aliases[key]; kind != "" {
			t.kind, t.exact = kind, input[at] == '='
			if negative {
				t.negate = !t.negate
			}
			value = input[at+1:]
		}
	}
	if value == "" {
		return t, fmt.Errorf("empty %s condition", t.kind)
	}
	t.value, t.needle = strings.ToLower(value), []byte(value)
	if t.exact && (t.kind == "content" || t.kind == "hex" || t.kind == "text" || t.kind == "stream") {
		return t, fmt.Errorf("use %s:VALUE for byte containment; = compares protocol/address/info fields", t.kind)
	}
	switch t.kind {
	case "ip", "src", "dst":
		if strings.Contains(value, "/") {
			_, network, err := net.ParseCIDR(value)
			if err != nil {
				return t, fmt.Errorf("invalid CIDR %q", value)
			}
			t.network = network
		}
	case "port", "sport", "dport":
		for _, part := range strings.Split(value, ",") {
			ends := strings.Split(part, "-")
			if len(ends) > 2 {
				return t, fmt.Errorf("invalid port range %q", part)
			}
			low, err := strconv.ParseUint(ends[0], 10, 16)
			if err != nil {
				return t, fmt.Errorf("invalid port %q", part)
			}
			high := low
			if len(ends) == 2 {
				high, err = strconv.ParseUint(ends[1], 10, 16)
			}
			if err != nil || high < low {
				return t, fmt.Errorf("invalid port range %q", part)
			}
			t.ports = append(t.ports, [2]uint16{uint16(low), uint16(high)})
		}
	case "hex":
		clean := strings.NewReplacer(" ", "", ":", "", "-", "", "\t", "").Replace(value)
		clean = strings.TrimPrefix(clean, "0x")
		var err error
		t.needle, err = hex.DecodeString(clean)
		if err != nil || len(t.needle) == 0 {
			return t, fmt.Errorf("hex needs complete byte pairs, e.g. hex:00ff or hex:\"00 ff\"")
		}
	}
	if t.kind == "any" || t.kind == "text" || t.kind == "stream" {
		t.matcher = newFoldMatcher(t.needle)
	}
	return t, nil
}

func (t searchTerm) stringMatch(value string) bool {
	if t.network != nil {
		return t.network.Contains(net.ParseIP(value))
	}
	if t.exact {
		return strings.EqualFold(value, t.value)
	}
	return strings.Contains(strings.ToLower(value), t.value)
}

// Packet data is binary, so ASCII case folding avoids converting whole retained
// frames to strings or allocating lowercase copies on every refresh.
type foldMatcher struct {
	needle  []byte
	failure []int
}

func foldByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}
func newFoldMatcher(needle []byte) *foldMatcher {
	m := &foldMatcher{needle: append([]byte(nil), needle...), failure: make([]int, len(needle))}
	for i, b := range m.needle {
		m.needle[i] = foldByte(b)
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
func containsFold(data, needle []byte) bool { return newFoldMatcher(needle).contains(data) }
func (m *foldMatcher) contains(data []byte) bool {
	if len(m.needle) == 0 {
		return true
	}
	matched := 0
	for _, b := range data {
		b = foldByte(b)
		for matched > 0 && b != m.needle[matched] {
			matched = m.failure[matched-1]
		}
		if b == m.needle[matched] {
			matched++
		}
		if matched == len(m.needle) {
			return true
		}
	}
	return false
}

type searchStream struct {
	version            uint64
	protocol, evidence string
	snapshot           *streamSnapshot
	loaded             bool
	matches            map[string]bool
}
type packetSearchContext struct {
	store   *streamStore
	streams map[uint64]*searchStream
}

func newSearchContext(store *streamStore) *packetSearchContext {
	return &packetSearchContext{store: store, streams: make(map[uint64]*searchStream)}
}
func (c *packetSearchContext) stream(id uint64) *searchStream {
	if cached := c.streams[id]; cached != nil {
		return cached
	}
	v := &searchStream{}
	if c.store != nil && id != 0 {
		c.store.mu.RLock()
		if r := c.store.records[id]; r != nil {
			v.version, v.protocol, v.evidence = r.version, r.protocol, r.evidence
		}
		c.store.mu.RUnlock()
	}
	c.streams[id] = v
	return v
}
func (c *packetSearchContext) streamContains(id uint64, matcher *foldMatcher) bool {
	v := c.stream(id)
	if !v.loaded {
		v.snapshot = c.store.snapshot(id)
		v.loaded = true
		v.matches = make(map[string]bool)
	}
	key := string(matcher.needle)
	if result, ok := v.matches[key]; ok {
		return result
	}
	result := streamContainsMatch(v.snapshot, matcher)
	v.matches[key] = result
	return result
}

func streamContains(v *streamSnapshot, needle []byte) bool {
	return streamContainsMatch(v, newFoldMatcher(needle))
}
func streamContainsMatch(v *streamSnapshot, matcher *foldMatcher) bool {
	needle := matcher.needle
	if v == nil || len(needle) == 0 {
		return false
	}
	for _, prefix := range v.prefix {
		if matcher.contains(prefix) {
			return true
		}
	}
	var tail [2][]byte
	var next [2]uint64
	for _, chunk := range v.chunks {
		d := chunk.direction
		if matcher.contains(chunk.data) {
			return true
		}
		if chunk.gap != 0 || chunk.offset != next[d] {
			tail[d] = nil
		}
		join := append(append([]byte(nil), tail[d]...), chunk.data[:min(len(chunk.data), len(needle)-1)]...)
		if matcher.contains(join) {
			return true
		}
		if len(chunk.data) >= len(needle)-1 {
			tail[d] = chunk.data[max(0, len(chunk.data)-len(needle)+1):]
		} else {
			joined := append(append([]byte(nil), tail[d]...), chunk.data...)
			tail[d] = joined[max(0, len(joined)-len(needle)+1):]
		}
		next[d] = chunk.offset + uint64(len(chunk.data))
	}
	return false
}

func (t searchTerm) matches(e *packetEntry, row packetSummary, c *packetSearchContext) bool {
	switch t.kind {
	case "proto":
		for _, p := range row.Layers {
			if t.stringMatch(p) {
				return true
			}
		}
		for _, p := range []string{row.Transport, row.packetProtocol, strings.TrimPrefix(row.Protocol, "TCP/")} {
			if p != "" && t.stringMatch(p) {
				return true
			}
		}
	case "ip":
		return t.stringMatch(row.Source) || t.stringMatch(row.Destination)
	case "src":
		return t.stringMatch(row.Source)
	case "dst":
		return t.stringMatch(row.Destination)
	case "info":
		return t.stringMatch(row.Info)
	case "port", "sport", "dport":
		if row.Transport == "" {
			return false
		}
		for _, ports := range t.ports {
			if t.kind != "dport" && row.SourcePort >= ports[0] && row.SourcePort <= ports[1] {
				return true
			}
			if t.kind != "sport" && row.DestinationPort >= ports[0] && row.DestinationPort <= ports[1] {
				return true
			}
		}
	case "content", "hex":
		return bytes.Contains(e.packet.data, t.needle)
	case "text":
		return t.matcher.contains(e.packet.data)
	case "stream":
		return c.streamContains(e.packet.streamID, t.matcher)
	case "any":
		return t.stringMatch(row.Source) || t.stringMatch(row.Destination) || t.stringMatch(row.Protocol) || t.stringMatch(row.Info) || t.matcher.contains(e.packet.data)
	}
	return false
}

func (u *tui) matchesSearch(e *packetEntry, c *packetSearchContext) bool {
	if u.search == nil {
		return true
	}
	version := uint64(0)
	var stream *searchStream
	if u.search.dynamic {
		stream = c.stream(e.packet.streamID)
		version = stream.version
	}
	if e.searchGeneration != 0 && e.searchGeneration == u.searchGeneration && e.searchVersion == version {
		return e.searchMatched
	}
	var row packetSummary
	if u.search.metadata {
		row = e.row()
	}
	if stream != nil && stream.protocol != "" {
		row = applyApplication(row, stream.protocol, stream.evidence, e.packet.streamID)
	}
	matched := false
	for _, group := range u.search.groups {
		all := true
		for _, t := range group {
			if t.matches(e, row, c) == t.negate {
				all = false
				break
			}
		}
		if all {
			matched = true
			break
		}
	}
	e.searchGeneration, e.searchVersion, e.searchMatched = u.searchGeneration, version, matched
	return matched
}
