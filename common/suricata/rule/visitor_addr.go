package rule

import (
	"encoding/binary"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/config"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"math/big"
	"math/rand"
	"net/netip"
	"sort"
	"strings"
)

type ipv4Range struct {
	start uint32
	end   uint32
}

type AddressRule struct {
	// 这两个是修饰词
	Any      bool
	Negative bool

	// 这几个是终结词
	hostFilter *utils.HostsFilter

	IPv4CIDR string
	IPv6CIDR string

	negativeRules []*AddressRule
	positiveRules []*AddressRule
	SubRules      []*AddressRule

	// Flattened fast lookup structures populated by Prepare() to avoid
	// allocating per-CIDR HostsFilter maps and repeated string IP parsing.
	ipv4Ranges   []ipv4Range
	ipv6Prefixes []netip.Prefix
	positiveSlow []*AddressRule
	envMatcher   *config.VarMatcher
	prepared     bool

	// envtable
	Config *config.Config
	Env    string
}

func parseAddrOrPrefix(raw string) (v4 ipv4Range, isV4 bool, v6 netip.Prefix, isV6 bool, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ipv4Range{}, false, netip.Prefix{}, false, false
	}
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return ipv4Range{}, false, netip.Prefix{}, false, false
		}
		p = p.Masked()
		if p.Addr().Is4() {
			b := p.Addr().As4()
			base := binary.BigEndian.Uint32(b[:])
			bits := p.Bits()
			var mask uint32
			if bits > 0 {
				mask = ^uint32(0) << uint(32-bits)
			}
			start := base & mask
			end := start | ^mask
			return ipv4Range{start: start, end: end}, true, netip.Prefix{}, false, true
		}
		return ipv4Range{}, false, p, true, true
	}
	a, err := netip.ParseAddr(raw)
	if err != nil {
		return ipv4Range{}, false, netip.Prefix{}, false, false
	}
	a = a.Unmap()
	if a.Is4() {
		b := a.As4()
		v := binary.BigEndian.Uint32(b[:])
		return ipv4Range{start: v, end: v}, true, netip.Prefix{}, false, true
	}
	return ipv4Range{}, false, netip.PrefixFrom(a, 128), true, true
}

func mergeIPv4Ranges(ranges []ipv4Range) []ipv4Range {
	if len(ranges) <= 1 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start != ranges[j].start {
			return ranges[i].start < ranges[j].start
		}
		return ranges[i].end < ranges[j].end
	})
	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if uint64(r.start) <= uint64(last.end)+1 {
			if r.end > last.end {
				last.end = r.end
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

func containsIPv4Range(ranges []ipv4Range, v uint32) bool {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if ranges[mid].end < v {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(ranges) && ranges[lo].start <= v && v <= ranges[lo].end
}

func (a *AddressRule) isPureFastRangeLeaf() bool {
	return a != nil && !a.Any && !a.Negative && a.Env == "" &&
		len(a.negativeRules) == 0 && len(a.positiveSlow) == 0 && a.hostFilter == nil &&
		(len(a.ipv4Ranges) > 0 || len(a.ipv6Prefixes) > 0)
}

func (a *AddressRule) matchAddrWithoutNegative(addr netip.Addr, raw string) bool {
	if a == nil {
		return false
	}
	if a.Any {
		return true
	}

	if a.Env != "" && a.Config != nil {
		if a.envMatcher != nil {
			if addr.IsValid() {
				return a.envMatcher.MatchAddr(addr)
			}
			return a.Config.MatchVar(a.Env, raw)
		}
		if !a.Config.HasVar(a.Env) {
			log.Warnf("suricata env %s not found", a.Env)
			return false
		}
		if addr.IsValid() {
			return a.Config.MatchAddr(a.Env, addr)
		}
		return a.Config.MatchVar(a.Env, raw)
	}

	for _, n := range a.negativeRules {
		if !n.matchAddrWithRaw(addr, raw) {
			return false
		}
	}

	if addr.IsValid() {
		if addr.Is4() && len(a.ipv4Ranges) > 0 {
			b := addr.As4()
			if containsIPv4Range(a.ipv4Ranges, binary.BigEndian.Uint32(b[:])) {
				return true
			}
		} else if addr.Is6() && len(a.ipv6Prefixes) > 0 {
			for _, p := range a.ipv6Prefixes {
				if p.Contains(addr) {
					return true
				}
			}
		}
	}

	if a.prepared {
		for _, n := range a.positiveSlow {
			if n.matchAddrWithRaw(addr, raw) {
				return true
			}
		}
	} else {
		for _, n := range a.positiveRules {
			if n.matchAddrWithRaw(addr, raw) {
				return true
			}
		}
	}

	if a.hostFilter != nil {
		if raw == "" && addr.IsValid() {
			raw = addr.String()
		}
		if raw != "" && a.hostFilter.Contains(raw) {
			return true
		}
	}
	return false
}

func (a *AddressRule) matchAddrWithRaw(addr netip.Addr, raw string) bool {
	if a == nil {
		return false
	}
	if a.Negative {
		return !a.matchAddrWithoutNegative(addr, raw)
	}
	return a.matchAddrWithoutNegative(addr, raw)
}

func (a *AddressRule) parseSubRules() {
	if len(a.SubRules) <= 0 || len(a.negativeRules) > 0 || len(a.positiveRules) > 0 {
		return
	}
	for _, n := range a.SubRules {
		if n.Negative {
			a.negativeRules = append(a.negativeRules, n)
		} else {
			a.positiveRules = append(a.positiveRules, n)
		}
	}
}

// Prepare materializes all matcher state before a Rule is shared by matchers.
func (a *AddressRule) Prepare() {
	if a == nil || a.prepared {
		return
	}
	a.parseSubRules()
	for _, sub := range a.SubRules {
		sub.Prepare()
	}

	addLiteral := func(raw string) {
		if raw == "" {
			return
		}
		if v4, isV4, v6, isV6, ok := parseAddrOrPrefix(raw); ok {
			if isV4 {
				a.ipv4Ranges = append(a.ipv4Ranges, v4)
			} else if isV6 {
				a.ipv6Prefixes = append(a.ipv6Prefixes, v6)
			}
			return
		}
		if a.hostFilter == nil {
			a.hostFilter = utils.NewHostsFilter()
		}
		a.hostFilter.AddCIDR(raw)
	}
	addLiteral(a.IPv4CIDR)
	addLiteral(a.IPv6CIDR)

	for _, pos := range a.positiveRules {
		if pos.isPureFastRangeLeaf() {
			a.ipv4Ranges = append(a.ipv4Ranges, pos.ipv4Ranges...)
			a.ipv6Prefixes = append(a.ipv6Prefixes, pos.ipv6Prefixes...)
		} else {
			a.positiveSlow = append(a.positiveSlow, pos)
		}
	}
	if len(a.ipv4Ranges) > 1 {
		a.ipv4Ranges = mergeIPv4Ranges(a.ipv4Ranges)
	}
	if a.Env != "" && a.Config != nil {
		a.envMatcher = a.Config.LookupVar(a.Env)
	}
	a.prepared = true
}

// IPv4SupersetRanges returns a sound superset of sorted, non-overlapping IPv4
// [start, end] ranges that can match this AddressRule when the rule is
// bounded solely by concrete IPv4 CIDRs/IPs (with optional negative exceptions).
// Returns ok=false for Any, negated roots, env vars, IPv6 rules, or dynamic subrules.
func (a *AddressRule) IPv4SupersetRanges() ([][2]uint32, bool) {
	if a == nil || a.Any || a.Negative || a.Env != "" || len(a.positiveSlow) > 0 || a.hostFilter != nil || len(a.ipv6Prefixes) > 0 || len(a.ipv4Ranges) == 0 {
		return nil, false
	}
	out := make([][2]uint32, len(a.ipv4Ranges))
	for i, r := range a.ipv4Ranges {
		out[i] = [2]uint32{r.start, r.end}
	}
	return out, true
}

// MatchAddr matches a pre-parsed netip.Addr with zero string parsing or heap
// allocations on IPv4 ranges and address-group variables.
func (a *AddressRule) MatchAddr(addr netip.Addr) bool {
	if a == nil {
		return false
	}
	if addr.IsValid() {
		addr = addr.Unmap()
	}
	return a.matchAddrWithRaw(addr, "")
}

func (a *AddressRule) Match(i string) bool {
	if a == nil {
		return false
	}
	addr, err := netip.ParseAddr(i)
	if err == nil {
		addr = addr.Unmap()
	}
	return a.matchAddrWithRaw(addr, i)
}

// Generate is not uniform distribution
// also, linklocal addr, multicast addr, loopback addr, etc. are not escaped
func (a *AddressRule) Generate() string {
	if a == nil || a.Any {
		return utils.GetRandomIPAddress()
	}

	if a.Env != "" && a.Config != nil {
		existed := a.Config.HasVar(a.Env)
		if !existed {
			log.Warnf("suricata env %s not found, fallback to any", a.Env)
			//return utils.GetRandomLocalAddr()
			return a.Config.RandIpVar("ANY_IP")
		}
		return a.Config.RandIpVar(a.Env)
	}

	if a.Negative {
		rdip := utils.GetRandomIPAddress()
		for count := 10; a.Match(rdip) && count > 0; count-- {
			rdip = utils.GetRandomIPAddress()
		}
		return rdip
	}

	if a.IPv4CIDR != "" {
		cidr, err := netip.ParsePrefix(a.IPv4CIDR)
		if err != nil {
			log.Warnf("parse cidr (%s) failed: %v", a.IPv4CIDR, err)
			return utils.GetRandomLocalAddr()
		}
		cidr = cidr.Masked()
		var genip [4]byte
		binary.BigEndian.PutUint32(genip[:], binary.BigEndian.Uint32(cidr.Addr().AsSlice())+uint32(rand.Intn(1<<(32-cidr.Bits()))))
		return netip.AddrFrom4(genip).String()
	}

	if a.IPv6CIDR != "" {
		cidr, err := netip.ParsePrefix(a.IPv6CIDR)
		if err != nil {
			log.Warnf("parse cidr (%s) failed: %v", a.IPv6CIDR, err)
			return utils.GetRandomLocalAddr()
		}
		cidr = cidr.Masked()
		var genip [16]byte
		bn := big.NewInt(0).SetBytes(genip[:])
		add := big.NewInt(0).Rand(rand.New(rand.NewSource(rand.Int63())), big.NewInt(1).Lsh(big.NewInt(1), 128-uint(cidr.Bits())))
		bn.Add(bn, add)
		ip, ok := netip.AddrFromSlice(bn.Bytes())
		if !ok {
			log.Warnf("generate ipv6 failed: %v", err)
			return utils.GetRandomLocalAddr()
		}
		return ip.String()
	}

	if len(a.positiveRules) == 0 && len(a.negativeRules) == 0 {
		return utils.GetRandomLocalAddr()
	}

	var genip string
	retry := 10
retry:
	retry--
	if len(a.positiveRules) != 0 {
		genip = a.positiveRules[rand.Intn(len(a.positiveRules))].Generate()
	} else {
		genip = utils.GetRandomIPAddress()
	}
	if retry <= 0 {
		return genip
	}

	for _, v := range a.negativeRules {
		if v.Match(genip) {
			goto retry
		}
	}
	return genip
}

func (v *RuleSyntaxVisitor) VisitSrcAddress(i *parser.Src_addressContext) *AddressRule {
	return v.VisitAddress(i.Address().(*parser.AddressContext))
}

func (v *RuleSyntaxVisitor) VisitDstAddress(i *parser.Dest_addressContext) *AddressRule {
	return v.VisitAddress(i.Address().(*parser.AddressContext))
}

func (v *RuleSyntaxVisitor) VisitAddress(i *parser.AddressContext) (addr *AddressRule) {
	if i == nil {
		return nil
	}
	addr = &AddressRule{Config: v.Config}
	defer func() {
		if addr != nil {
			addr.Prepare()
		}
	}()
	if i.Any() != nil {
		addr.Any = true
		return addr
	}
	if i.Negative() != nil {
		addr.Negative = true
		addr.SubRules = append(addr.SubRules, v.VisitAddress(i.Address(0).(*parser.AddressContext)))
		return addr
	}
	switch true {
	case i.Ipv4() != nil:
		addr.IPv4CIDR = trim(i.Ipv4().GetText())
		return addr
	case i.Ipv6() != nil:
		addr.IPv6CIDR = trim(i.Ipv6().GetText())
		return addr
	case i.Environment_var() != nil:
		addr.Env = strings.Trim(trim(i.Environment_var().GetText()), "${}")
		return addr
	case len(i.AllAddress()) > 0:
		var subs []*AddressRule
		for _, r := range i.AllAddress() {
			if r == nil {
				continue
			}
			subs = append(subs, v.VisitAddress(r.(*parser.AddressContext)))
		}
		addr.SubRules = subs
		return addr
	default:
		log.Debugf("unhandled unit: %v", i.GetText())
		addr.Any = true
		return addr
	}
}
