package pcaputil

// The first admitted client 1-RTT packet bounds every admitted 0-RTT packet,
// including reordered captures and later application-key epochs. These are
// private wire observations, never supplied by a public session DTO.
type quicEarlyOrder struct {
	lastEarly, firstApplication        uint64
	earlyObserved, applicationObserved bool
}

func (q *binQUIC) checkEarlyPacketOrder(dir int, h quicHdr, pn uint64) error {
	if !q.native || dir != 0 || h.space != quicSpaceApplication {
		return nil
	}
	order := q.earlyOrder
	if h.typeName == "0-RTT" && order.applicationObserved && pn >= order.firstApplication ||
		!h.long && order.earlyObserved && pn <= order.lastEarly {
		return protocolError(ErrMalformedMessage, "QUIC 0-RTT/1-RTT packet number order contradicts admitted encryption levels")
	}
	return nil
}

// Record only an admitted packet, after body/resource processing. A rejected
// frame or budget refusal must not constrain a later, otherwise valid packet.
// Application observations explicitly deferred for missing context are allowed.
func (q *binQUIC) observeEarlyPacketOrder(dir int, h quicHdr, pn uint64) {
	if !q.native || dir != 0 || h.space != quicSpaceApplication {
		return
	}
	order := &q.earlyOrder
	if h.typeName == "0-RTT" {
		if !order.earlyObserved || pn > order.lastEarly {
			order.lastEarly = pn
		}
		order.earlyObserved = true
	} else if !h.long {
		if !order.applicationObserved || pn < order.firstApplication {
			order.firstApplication = pn
		}
		order.applicationObserved = true
	}
}
