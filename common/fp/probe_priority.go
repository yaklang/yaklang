package fp

import "sort"

const (
	// defaultProbesMax is the Config.ProbesMax default: the generic
	// active-probe budget. Port-matched probes are not charged against it.
	defaultProbesMax = 10
	// defaultGenericProbeBudget is the generic-sweep floor for the default and
	// any raised ProbesMax. It fits the lowered common protocols, the rarity-1
	// foundation and the rarity-0 RDP probe. An explicitly smaller ProbesMax
	// is still honored.
	defaultGenericProbeBudget = 14
	// defaultPortProbeBudget is how many default-port probes one target may
	// receive even when their embedded rarity is above RarityMax.
	defaultPortProbeBudget = 12
	// defaultProbeTotalBudget stops a silent port from turning into an
	// unbounded sweep. 26 probes is still about three rounds at the default
	// concurrency of 10.
	defaultProbeTotalBudget = 26
)

// probeRarityOverride replaces embedded nmap rarity without editing the
// fingerprint file. A lower value is tried sooner on ports that do not list
// the probe. Port matches ignore rarity and are still sent.
//
// Demotions move probes that currently occupy the front of every port out of
// the default generic sweep. Their own default ports still select them.
var probeRarityOverride = map[string]int{
	"mqtt":          2,
	"mongodb":       2,
	"ms-sql-s":      2,
	"Memcache":      2,
	"redis-server":  2,
	"oracle-tns":    3,
	"LDAPBindReq":   3,
	"LDAPSearchReq": 3,
	"memcached":     3,

	// RdpStandard stays at its embedded rarity. A five-probe scan is expected
	// to identify RDP, and that probe is the one that elicits the banner.
	// The TLS hybrid variant, the legacy ORACLE probe and SMB1 negotiate are
	// not useful on unrelated ports.
	"RdpSSLHybridAndHybridEx": 7,
	"ORACLE":                  7,
	"SMBProgNeg":              7,
}

// protectedProbes stay in the generic sweep when a lowered protocol needs a slot.
var protectedProbes = map[string]struct{}{
	"GetRequest":           {},
	"HTTPOptions":          {},
	"GenericLines":         {},
	"TLSSessionReq":        {},
	"SSLSessionReq":        {},
	"DNSVersionBindReq":    {},
	"DNSVersionBindReqTCP": {},
	"NTPRequest":           {},
}

func effectiveProbeRarity(probe *NmapProbe) int {
	if probe == nil {
		return 9
	}
	if rarity, ok := probeRarityOverride[probe.Name]; ok {
		return rarity
	}
	return probe.Rarity
}

func loweredProtocolProbe(probe *NmapProbe) bool {
	if probe == nil {
		return false
	}
	rarity, ok := probeRarityOverride[probe.Name]
	return ok && rarity < probe.Rarity
}

func selectionLimits(config *Config) (portLimit, genericLimit int, limited bool) {
	if config == nil || config.ProbesMax <= 0 {
		return 0, 0, false
	}
	if config.ProbesMax > defaultProbesMax {
		// Raising the budget must never send fewer probes than the default
		// sweep, so both default floors stay in effect as minimums.
		return max(config.ProbesMax, defaultPortProbeBudget),
			max(config.ProbesMax, defaultGenericProbeBudget), true
	}
	portLimit = defaultPortProbeBudget
	genericLimit = config.ProbesMax
	if config.ProbesMax >= defaultProbesMax && genericLimit < defaultGenericProbeBudget {
		genericLimit = defaultGenericProbeBudget
	}
	if portLimit+genericLimit > defaultProbeTotalBudget {
		genericLimit = defaultProbeTotalBudget - portLimit
		if genericLimit < 1 {
			genericLimit = 1
		}
	}
	return portLimit, genericLimit, true
}

func selectionCap(config *Config) int {
	portLimit, genericLimit, limited := selectionLimits(config)
	if !limited {
		return int(^uint(0) >> 1)
	}
	return portLimit + genericLimit
}

func sortProbeBlocks(blocks []*RuleBlock) {
	sortProbeBlocksBy(blocks, effectiveProbeRarity)
}

// Port matches keep the fingerprint file's rarity. A demotion only pushes a
// probe out of the generic sweep; on its own port it still goes out first.
func sortPortProbeBlocks(blocks []*RuleBlock) {
	sortProbeBlocksBy(blocks, func(probe *NmapProbe) int {
		if probe == nil {
			return 9
		}
		return probe.Rarity
	})
}

func sortProbeBlocksBy(blocks []*RuleBlock, rarityOf func(*NmapProbe) int) {
	sort.SliceStable(blocks, func(i, j int) bool {
		left, right := blocks[i], blocks[j]
		if left == nil || left.Probe == nil {
			return false
		}
		if right == nil || right.Probe == nil {
			return true
		}
		if rarity := rarityOf(left.Probe) - rarityOf(right.Probe); rarity != 0 {
			return rarity < 0
		}
		if left.Probe.Index != right.Probe.Index {
			return left.Probe.Index < right.Probe.Index
		}
		return left.Probe.Name < right.Probe.Name
	})
}

// preferLoweredProbes keeps protocols whose embedded rarity was too high to
// survive the generic budget. It evicts the least useful generic probe instead
// of growing the sweep.
func preferLoweredProbes(candidates, kept, portMatched []*RuleBlock, limit, rarityMax int) []*RuleBlock {
	if limit <= 0 {
		return nil
	}
	if len(kept) > limit {
		kept = append([]*RuleBlock(nil), kept[:limit]...)
	} else {
		kept = append([]*RuleBlock(nil), kept...)
	}
	seen := map[string]bool{}
	mark := func(blocks []*RuleBlock) {
		for _, block := range blocks {
			if block != nil && block.Probe != nil {
				seen[block.Probe.Name] = true
			}
		}
	}
	mark(portMatched)
	mark(kept)
	for _, block := range candidates {
		probe := block.Probe
		if probe == nil || seen[probe.Name] || !loweredProtocolProbe(probe) || effectiveProbeRarity(probe) > rarityMax {
			continue
		}
		if len(kept) < limit {
			kept = append(kept, block)
			seen[probe.Name] = true
			continue
		}
		idx := evictableGeneric(kept, effectiveProbeRarity(probe))
		if idx < 0 {
			continue
		}
		if kept[idx] != nil && kept[idx].Probe != nil {
			delete(seen, kept[idx].Probe.Name)
		}
		kept[idx] = block
		seen[probe.Name] = true
	}
	sortProbeBlocks(kept)
	return kept
}

func evictableGeneric(blocks []*RuleBlock, incoming int) int {
	chosen := -1
	rarity := -1
	index := -1
	for i, block := range blocks {
		if block == nil || block.Probe == nil {
			continue
		}
		if _, protected := protectedProbes[block.Probe.Name]; protected || loweredProtocolProbe(block.Probe) {
			continue
		}
		got := effectiveProbeRarity(block.Probe)
		// A lowered protocol may replace a less common generic probe. It must
		// not displace a probe the fingerprint file already ranks first, such
		// as the RDP negotiate that a small scan is expected to send.
		if got < incoming {
			continue
		}
		if chosen < 0 || got > rarity || (got == rarity && block.Probe.Index > index) {
			chosen = i
			rarity = got
			index = block.Probe.Index
		}
	}
	return chosen
}
