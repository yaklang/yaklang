package pcaputil

// This bounded, allocation-free tag walk selects a conservative compact pool
// before semantic maps are built. Compact rows reuse their type description;
// expanded maps can outnumber tag bytes. Scalar payload bytes are never tags.
type wrapperCompactProbe struct {
	wire      []byte
	at, nodes int
}

func (p *wrapperCompactProbe) skip(n int) bool {
	if n < 0 || n > len(p.wire)-p.at {
		return false
	}
	p.at += n
	return true
}
func (p *wrapperCompactProbe) count() (uint64, bool) {
	if p.at >= len(p.wire) {
		return 0, false
	}
	b := p.wire[p.at]
	p.at++
	if b < 128 {
		return uint64(b), true
	}
	width := int(b & 127)
	if width < 1 || width > 4 || width > len(p.wire)-p.at {
		return 0, false
	}
	n := uint64(0)
	for i := 0; i < width; i++ {
		n = n<<8 | uint64(p.wire[p.at])
		p.at++
	}
	return n, true
}
func (p *wrapperCompactProbe) data(level int) (compact, complete bool) {
	if level >= wrapperDataLevels || p.nodes >= wrapperDataNodes || p.at >= len(p.wire) {
		return false, false
	}
	p.nodes++
	tag := p.wire[p.at]
	p.at++
	if tag == 19 {
		return true, true
	}
	if tag == 1 || tag == 2 {
		n, ok := p.count()
		if !ok || n > uint64(wrapperDataNodes-p.nodes) {
			return false, false
		}
		for i := uint64(0); i < n; i++ {
			found, ok := p.data(level + 1)
			if found {
				return true, true
			}
			if !ok {
				return false, false
			}
		}
		return false, true
	}
	if n := wrapperScalarSize(tag); n >= 0 {
		return false, p.skip(n)
	}
	if tag != 4 && tag != 9 && tag != 10 && tag != 12 {
		return false, false
	}
	n, ok := p.count()
	if !ok {
		return false, false
	}
	if tag == 4 {
		if n > uint64(wrapperListOctets)*8 {
			return false, false
		}
		n = (n + 7) / 8
	} else if n > wrapperListOctets {
		return false, false
	}
	return false, p.skip(int(n))
}
func wrapperContainsCompactData(w []byte) bool {
	if len(w) < 12 {
		return false
	}
	p := wrapperCompactProbe{wire: w}
	if w[9] == 1 {
		if w[8] == 0xc4 && w[11] == 0 {
			p.at = 12
			yes, _ := p.data(0)
			return yes
		}
		if wrapperNormalAccess(w) {
			p.at = 22
			yes, _ := p.data(0)
			return yes
		}
		return false
	}
	if !wrapperIsList(w) {
		return false
	}
	p.at = 11
	n, ok := p.count()
	if !ok || n > wrapperListItems {
		return false
	}
	for i := uint64(0); i < n; i++ {
		if w[8] == 0xc0 {
			if !p.skip(10) {
				return false
			}
			selection := w[p.at-1]
			if selection == 0 {
				continue
			}
			if selection != 1 || !p.skip(1) {
				return false
			}
		} else if w[8] == 0xc4 {
			if !p.skip(1) {
				return false
			}
			choice := w[p.at-1]
			if choice == 1 {
				if !p.skip(1) {
					return false
				}
				continue
			}
			if choice != 0 {
				return false
			}
		} else {
			return false
		}
		found, complete := p.data(0)
		if found {
			return true
		}
		if !complete {
			return false
		}
	}
	return false
}
