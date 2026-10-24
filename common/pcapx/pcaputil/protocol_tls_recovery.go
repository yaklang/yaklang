package pcaputil

import "errors"

func tlsAuthenticationError(err error) bool {
	var p *ProtocolError
	return errors.As(err, &p) && p.Kind == ErrAuthenticationFailed
}

// Authenticate each candidate before committing sequence or key-epoch state.
// This only recovers at an already framed record boundary: never scan random
// TCP bytes for a header, and never deliver unauthenticated candidate plaintext.
func (f *binFlow) openTLSRecord(dir int, w []byte, info map[string]any) ([]byte, byte, error) {
	t := f.tls
	wasGap := t.recordGap[dir]
	plain, typ, err := t.open(dir, w)
	if err == nil {
		if wasGap {
			info["Record Recovery Verified"] = true
			info["Recovery State"] = "record authentication resumed; earlier plaintext remains unavailable"
		}
		t.recordGap[dir] = false
		t.recoveryFailures[dir] = 0
		return plain, typ, nil
	}
	if !tlsAuthenticationError(err) || f.transportFlow == nil {
		return nil, 0, err
	}
	commit := func(trial binTLS, p []byte, kind byte, missing bool) ([]byte, byte, error) {
		*t = trial
		t.recordGap[dir] = false
		t.recoveryFailures[dir] = 0
		info["Record Recovery Verified"], info["Recovered Record Sequence"] = true, t.seq[dir]-1
		info["Missing Earlier Plaintext"] = missing
		info["Recovery State"] = "record sequence/epoch authenticated; missing plaintext is not reconstructed"
		return p, kind, nil
	}
	// Missing complete records can change the implicit sequence counter. The
	// finite window bounds CPU; a failed trial cannot mutate the live state.
	if wasGap {
		for skip := uint64(1); skip <= 16 && t.seq[dir] <= ^uint64(0)-skip; skip++ {
			trial := *t
			trial.seq[dir] += skip
			p, k, e := trial.open(dir, w)
			if e == nil {
				return commit(trial, p, k, true)
			}
		}
	}
	if t.version == 0x304 {
		trial := *t
		if !t.app[dir] {
			trial.app[dir] = true
			trial.keys[dir] = nil
			trial.seq[dir] = 0
			if ok, e := trial.loadKey(dir, f.a.tlsSecrets); e == nil && ok {
				for n := uint64(0); n <= 16; n++ {
					candidate := trial
					candidate.seq[dir] = n
					p, k, e := candidate.open(dir, w)
					if e == nil {
						info["Application Epoch Inferred From Authenticated Record"] = true
						candidate.handshakeGap[dir] = false
						return commit(candidate, p, k, n > 0 || t.child != nil)
					}
				}
			}
		} else if len(t.secret[dir]) > 0 {
			if next, e := quicHKDFExpandLabel(t.secret[dir], "traffic upd", len(t.secret[dir])); e == nil && trial.install13(dir, next) == nil {
				for n := uint64(0); n <= 16; n++ {
					candidate := trial
					candidate.seq[dir] = n
					p, k, e := candidate.open(dir, w)
					if e == nil {
						info["Unobserved KeyUpdate Inferred From Authentication"] = true
						candidate.handshakeGap[dir] = false
						return commit(candidate, p, k, true)
					}
				}
			}
		}
	}
	return nil, 0, err
}

func (f *binFlow) markTLSInnerGap(dir int) {
	f.plaintextGap[dir] = true
	if t := f.tls; t != nil && t.child != nil {
		child := t.child
		child.plaintextGap[dir] = true
		d := &child.directions[dir]
		d.stopped, d.tlsPlaintextGap = true, true
		child.release(d)
		d.http = nil
	}
}
func (f *binFlow) noteTLSRecordFailure(dir int, info map[string]any) {
	if f.transportFlow == nil || f.tls == nil {
		return
	}
	t := f.tls
	t.handshake[dir] = nil
	t.handshakeGap[dir] = true
	t.recordGap[dir] = true
	if t.recoveryFailures[dir] < 4 {
		t.recoveryFailures[dir]++
	}
	if t.seq[dir] != ^uint64(0) {
		t.seq[dir]++
	}
	f.markTLSInnerGap(dir)
	info["Record Failure Isolated"] = true
	info["Recovery State"] = "one record rejected; exact-boundary authentication recovery allowed"
	info["Inner Application Status"] = "missing plaintext; this direction is not dissected across the gap"
}
