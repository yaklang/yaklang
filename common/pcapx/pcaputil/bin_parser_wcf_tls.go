package pcaputil

// A private witness of an admitted MC-NMF ssl-tls request/response, confined to
// this TCP owner/domain. This is neither a TLS shutdown proof nor a generic
// TLS resynchronizer. Candidate bytes stay in the existing bounded buffers.
type wcfTLSCarrier struct {
	client        int
	observedIndex uint64
	application   [2]bool
	waiting       [2]bool
	returned      bool
}

const wcfTLSProbeLimit = 4096

func (s *wcfTLSCarrier) contextFields() map[string]any {
	context := map[string]any{"Observed Session Index": s.observedIndex, "Carrier": "MC-NMF application/ssl-tls upgrade", "Scope": "locally observed framing epoch; not an authentication or endpoint identity"}
	if s.returned {
		context["Previous Carrier Completion"] = "not verified; encrypted End and TLS close_notify were not inferred"
	}
	if s.waiting[0] || s.waiting[1] {
		context["Return Boundary Pending"] = "buffered bytes await complete initiator upgrade and receiver boundary; no new framing epoch admitted"
	}
	return context
}

func (f *binFlow) beginWCFTLS(s *binWCF, e *ProtocolEvent) error {
	if err := f.reserveSession(256 + s.storage()); err != nil {
		return err
	}
	f.wcfTLS = &wcfTLSCarrier{client: s.clientDir, observedIndex: s.observedIndex, returned: s.carrierReturn}
	// Acceptance is not evidence that a valid TLS record has arrived yet.
	// Preserve ordinary wire detection for the actual carrier bytes.
	f.protocol, f.wcf = "", nil
	e.Session["Carrier Protocol"] = "tls"
	e.Session["Carrier Epoch"] = s.observedIndex
	e.semanticFields = cloneSession(e.Session)
	return nil
}

// A coalesced/early ClientHello must await the receiver's actual acceptance.
// Conversely a captured receiver response can await a split initiator preamble.
func (f *binFlow) wcfAwaitUpgrade(dir int, w []byte) (bool, error) {
	s := f.wcf
	if len(w) == 0 || s == nil || !s.clientKnown || s.midstream {
		return false, nil
	}
	if dir != s.clientDir && w[0] == wcfRecordUpgradeResponse && s.upgradeRequest == "" && !s.preambleEnded && !s.faulted[dir] {
		s.awaitingPeer[dir] = true
		return true, nil
	}
	if dir == s.clientDir && s.upgradeRequest == "application/ssl-tls" && w[0] != wcfRecordUpgradeResponse {
		if w[0] != 22 || len(w) >= 2 && w[1] != 3 || len(w) >= 3 && w[2] > 4 {
			return false, protocolError(ErrMalformedMessage, "wcf: invalid TLS prefix before observed Upgrade Response")
		}
		if len(w) >= 5 && int(w[3])<<8|int(w[4]) > 18432 {
			return false, protocolError(ErrResourceExceeded, "wcf: TLS upgrade prefix exceeds record budget")
		}
		return true, nil
	}
	return false, nil
}

func (f *binFlow) observeWCFTLSRecord(dir int, raw []byte) {
	if s := f.wcfTLS; s != nil && len(raw) >= 5 && raw[0] == 23 {
		s.application[dir] = true // protected envelope observed; contents unverified
	}
}

// Require the entire upgrade grammar, rather than two magic prefix fields.
// This probe never scans for a start inside ciphertext, skips bytes or commits
// state. The finite prefix budget also bounds reparsing fragmented candidates.
func wcfTLSReturnPreamble(w []byte, limit int) (bool, error) {
	at := 0
	s := newBinWCF(0, true, false)
	for _, typ := range []byte{wcfRecordVersion, wcfRecordMode, wcfRecordVia, wcfRecordKnownEncoding, wcfRecordUpgradeRequest} {
		if at == len(w) {
			return false, nil
		}
		if at >= limit {
			return false, protocolError(ErrResourceExceeded, "wcf: TLS return preamble exceeds probe budget")
		}
		rec, complete, err := parseWCFRecord(w[at:min(len(w), limit)], limit-at)
		if err != nil {
			return false, err
		}
		if !complete {
			if len(w) >= limit {
				return false, protocolError(ErrResourceExceeded, "wcf: TLS return preamble exceeds probe budget")
			}
			return false, nil
		}
		if rec.typ != typ && !(typ == wcfRecordKnownEncoding && rec.typ == wcfRecordExtensible) {
			return false, protocolError(ErrMalformedMessage, "wcf: TLS return requires a complete ordered upgrade preamble")
		}
		if _, err := s.apply(0, rec); err != nil {
			return false, err
		}
		at += rec.total
	}
	if s.upgradeRequest != "application/ssl-tls" {
		return false, protocolError(ErrContextRequired, "wcf: TLS return upgrade protocol not supported")
	}
	return true, nil
}

func (f *binFlow) frameWCFTLS(dir int, w []byte) (int, *binSpec, error) {
	s := f.wcfTLS
	if len(w) == 0 || s == nil {
		return f.frame(w)
	}
	clientCandidate := dir == s.client && w[0] == wcfRecordVersion
	serverCandidate := dir != s.client && w[0] == wcfRecordUpgradeResponse
	if !clientCandidate && !serverCandidate {
		return f.frame(w)
	}
	if !s.application[dir] || f.tls == nil || f.tls.recordGap[0] || f.tls.recordGap[1] || f.tls.handshakeGap[0] || f.tls.handshakeGap[1] {
		return 0, nil, protocolError(ErrContextRequired, "wcf: return boundary lacks contiguous prior TLS envelope context")
	}
	if clientCandidate {
		complete, err := wcfTLSReturnPreamble(w, min(wcfTLSProbeLimit, f.a.config.MaxMessageBytes, f.a.budget.MaxFrameBytes))
		if err != nil {
			return 0, nil, err
		}
		s.waiting[dir] = complete
		return 0, nil, nil
	}
	s.waiting[dir] = true
	if !s.waiting[s.client] || !s.application[s.client] {
		return 0, nil, nil
	}
	if f.tls.client != s.client || len(f.tls.random) != 32 || f.tls.version == 0 || f.tls.suite == 0 {
		return 0, nil, protocolError(ErrContextRequired, "wcf: return boundary lacks observed TLS hello negotiation")
	}
	client := &f.directions[s.client]
	if client.stopped || len(client.buffer) == 0 {
		return 0, nil, protocolError(ErrContextRequired, "wcf: return initiator bytes no longer available")
	}
	if s.observedIndex == ^uint64(0) {
		return 0, nil, protocolError(ErrResourceExceeded, "wcf: observed carrier counter exhausted")
	}
	next := newBinWCF(s.client, true, false)
	next.observedIndex, next.carrierReturn = s.observedIndex+1, true
	next.carrierPeerOffset = f.directions[dir].offset
	if err := f.reserveSession(f.tls.storage() + next.storage()); err != nil {
		return 0, nil, err
	}
	if f.tls.child != nil {
		f.tls.child.close(TrafficFlowCloseReason("WCF framing boundary observed; previous TLS completion not verified"))
	}
	// Fresh carrier state cannot inherit secrets, sequence numbers, randoms,
	// plaintext child dictionaries or prior authentication/request evidence.
	f.tls = nil
	f.plaintextGap = [2]bool{}
	f.protocol, f.wcf = "wcf", next
	f.feed(s.client, nil, client.ts)
	if f.protocol != "wcf" || f.wcf != next || client.stopped {
		return 0, nil, protocolError(ErrContextRequired, "wcf: return preamble refused before receiver acceptance")
	}
	return f.frameWCF(dir, w)
}

func (f *binFlow) replayWCFTLSPeer(dir int) {
	if f.wcfTLSReplaying || f.protocol != "tls" || f.wcfTLS == nil {
		return
	}
	s := f.wcfTLS
	server := 1 - s.client
	peer := &f.directions[server]
	if dir != s.client || !s.waiting[s.client] || !s.waiting[server] || peer.stopped || len(peer.buffer) == 0 {
		return
	}
	f.wcfTLSReplaying = true
	defer func() { f.wcfTLSReplaying = false }()
	f.feed(server, nil, peer.ts)
}
