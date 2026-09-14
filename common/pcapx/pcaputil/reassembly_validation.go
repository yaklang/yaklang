package pcaputil

import "errors"

type reassemblyError struct{ err error }

// An invalid segment must not poison the cursor or silently disappear. Keep
// processing subsequent packets, but return the first diagnostic at capture end.
func (p *TrafficPool) invalidSegment(reason string) {
	if p.counters != nil {
		p.counters.invalid.Add(1)
	} else if p.parallel != nil {
		p.parallel.stats.invalid.Add(1)
	}
	p.reassemblyFailure(reason)
}

func (p *TrafficPool) malformedPacket(reason string) {
	if p.counters != nil {
		p.counters.decodeErrors.Add(1)
	} else if p.parallel != nil {
		p.parallel.stats.decodeErrors.Add(1)
	}
	p.reassemblyFailure(reason)
}

func (p *TrafficPool) reassemblyFailure(reason string) {
	err := errors.New(reason)
	if p.parallel != nil {
		p.parallel.fail(err)
	} else {
		p.firstError.CompareAndSwap(nil, &reassemblyError{err: err})
	}
}
