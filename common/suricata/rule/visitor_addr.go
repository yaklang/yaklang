package rule

import (
	"encoding/binary"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"math/big"
	"math/rand"
	"net/netip"
	"strings"
)

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

	// envtable
	envtable map[string]string
	Env      string
}

func (a *AddressRule) _matchWithoutNegative(i string) bool {
	if a == nil {
		return false
	}
	if a.Any {
		return true
	}

	if a.Env != "" && a.envtable != nil {
		raw, existed := a.envtable[a.Env]
		if !existed {
			log.Warnf("suricata env %s not found, fallback to any", a.Env)
			return true
		}
		return raw == i
	}

	for _, n := range a.negativeRules {
		if !n.Match(i) {
			return false
		}
	}

	for _, n := range a.positiveRules {
		if n.Match(i) {
			return true
		}
	}

	if a.IPv4CIDR != "" {
		if a.hostFilter == nil {
			a.hostFilter = utils.NewHostsFilter()
		}
		a.hostFilter.Add(a.IPv4CIDR)
	}

	if a.IPv6CIDR != "" {
		if a.hostFilter == nil {
			a.hostFilter = utils.NewHostsFilter()
		}
		a.hostFilter.Add(a.IPv6CIDR)
	}

	if a.hostFilter != nil {
		if a.hostFilter.Contains(i) {
			return true
		}
	}
	return false
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

func (a *AddressRule) Match(i string) bool {
	if a == nil {
		return false
	}

	if a.Negative {
		return !a._matchWithoutNegative(i)
	}
	return a._matchWithoutNegative(i)
}

// Generate is not uniform distribution
// also, linklocal addr, multicast addr, loopback addr, etc. are not escaped
func (a *AddressRule) Generate() string {
	if a == nil || a.Any {
		return utils.GetRandomIPAddress()
	}

	if a.Env != "" && a.envtable != nil {
		raw, existed := a.envtable[a.Env]
		if !existed {
			log.Warnf("suricata env %s not found, fallback to any", a.Env)
			//return utils.GetRandomLocalAddr()
			return "127.0.0.1"
		}
		return raw
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
	addr = &AddressRule{envtable: v.Environment}
	defer func() {
		if addr != nil {
			addr.parseSubRules()
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
		if addr.Env == "EXTERNAL_NET" {
			addr.Any = true
		}
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
