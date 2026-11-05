package state

import "net/netip"

// CaptureDomain identifies an independent capture source without depending on
// packet-capture or engine packages. All fields are comparable.
type CaptureDomain struct {
	Sensor    string
	Interface string
	ID        uint64
}

// FlowKey identifies one generation of a bidirectional transport tuple. Use
// NewFlowKey to canonicalize endpoints so both packet directions share state.
type FlowKey struct {
	Epoch      uint64
	Domain     CaptureDomain
	Generation uint64
	Protocol   uint8
	A          netip.AddrPort
	B          netip.AddrPort
}

func NewFlowKey(epoch uint64, domain CaptureDomain, generation uint64, protocol uint8, src, dst netip.AddrPort) FlowKey {
	src = normalizeAddrPort(src)
	dst = normalizeAddrPort(dst)
	if compareAddrPort(src, dst) > 0 {
		src, dst = dst, src
	}
	return FlowKey{Epoch: epoch, Domain: domain, Generation: generation, Protocol: protocol, A: src, B: dst}
}

// HostKey identifies one host in a capture domain and ruleset epoch.
type HostKey struct {
	Epoch  uint64
	Domain CaptureDomain
	Addr   netip.Addr
}

func NewHostKey(epoch uint64, domain CaptureDomain, addr netip.Addr) HostKey {
	return HostKey{Epoch: epoch, Domain: domain, Addr: addr.Unmap()}
}

// PairKey identifies an unordered host pair in a capture domain and epoch.
type PairKey struct {
	Epoch  uint64
	Domain CaptureDomain
	A      netip.Addr
	B      netip.Addr
}

func NewPairKey(epoch uint64, domain CaptureDomain, a, b netip.Addr) PairKey {
	a, b = a.Unmap(), b.Unmap()
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return PairKey{Epoch: epoch, Domain: domain, A: a, B: b}
}

func normalizeAddrPort(v netip.AddrPort) netip.AddrPort {
	if !v.IsValid() {
		return v
	}
	return netip.AddrPortFrom(v.Addr().Unmap(), v.Port())
}

func compareAddrPort(a, b netip.AddrPort) int {
	if c := a.Addr().Compare(b.Addr()); c != 0 {
		return c
	}
	switch {
	case a.Port() < b.Port():
		return -1
	case a.Port() > b.Port():
		return 1
	default:
		return 0
	}
}
