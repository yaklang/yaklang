package fp

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
)

type serviceRuleHasher struct {
	hash.Hash64
	buf [8]byte
}

func newServiceRuleHasher() *serviceRuleHasher {
	return &serviceRuleHasher{Hash64: fnv.New64a()}
}

func (h *serviceRuleHasher) number(value uint64) {
	binary.LittleEndian.PutUint64(h.buf[:], value)
	_, _ = h.Write(h.buf[:])
}

func (h *serviceRuleHasher) text(value string) {
	h.number(uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

func (h *serviceRuleHasher) matches(matches []*NmapMatch) {
	h.number(uint64(len(matches)))
	for _, match := range matches {
		if match == nil {
			h.number(0)
			continue
		}
		h.number(1)
		if re, ok := match.MatchRule.(*lazyRegexp); ok && re != nil {
			h.number(1)
			h.text(re.raw)
			h.number(uint64(re.options))
		} else {
			h.number(0)
			// Custom matchers need their implementation identity as well as their
			// expression. Do not eagerly compile lazyRegexp just to get String().
			h.text(fmt.Sprintf("%T:%p", match.MatchRule, match.MatchRule))
			if match.MatchRule != nil {
				h.text(match.MatchRule.String())
			}
		}
		h.text(match.ServiceName)
		h.text(match.ProductVerbose)
		h.text(match.Info)
		h.text(match.Version)
		h.text(match.Hostname)
		h.text(match.OperationVerbose)
		h.text(match.DeviceType)
		h.text(match.Raw)
		h.number(uint64(len(match.CPEs)))
		for _, cpe := range match.CPEs {
			h.text(cpe)
		}
	}
}

func nmapMatchesStamp(matches []*NmapMatch) uint64 {
	h := newServiceRuleHasher()
	h.matches(matches)
	return h.Sum64()
}

func rulesCacheStamp(rules map[*NmapProbe][]*NmapMatch) uint64 {
	entries := make([]uint64, 0, len(rules))
	for probe, matches := range rules {
		h := newServiceRuleHasher()
		if probe != nil {
			h.number(1)
			h.number(uint64(probe.Index))
			h.number(uint64(probe.Rarity))
			h.text(string(probe.Proto))
			h.text(probe.Name)
			h.text(probe.Payload)
			h.text(probe.Raw)
			h.number(uint64(len(probe.DefaultPorts)))
			for _, port := range probe.DefaultPorts {
				h.number(uint64(port))
			}
		} else {
			h.number(0)
		}
		h.matches(matches)
		entries = append(entries, h.Sum64())
	}
	// Sort entry digests so map iteration order does not affect the key. Hash
	// the sequence instead of XOR so duplicate probe contents do not cancel.
	sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
	h := newServiceRuleHasher()
	h.number(uint64(len(entries)))
	for _, entry := range entries {
		h.number(entry)
	}
	return h.Sum64()
}
