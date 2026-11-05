package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// Only complete protected handshake records observed after CCS, before initial
// context/keys, can qualify. This is not ciphertext/header scanning, arbitrary
// application resynchronization or a queue spanning connections/key epochs.
type tlsPrecontextRecord struct {
	wire []byte
	id   uint64
}

func (t *binTLS) precontextStorage() int64 {
	var n int64
	for _, records := range t.precontext {
		for _, record := range records {
			n += int64(len(record.wire) + 128)
		}
	}
	return n
}

func (f *binFlow) discardPrecontextFinished(dir int) {
	if f.tls == nil || len(f.tls.precontext[dir]) == 0 && f.tls.precontextCharged[dir] == 0 {
		return
	}
	charge := f.tls.precontextCharged[dir]
	f.tls.precontext[dir] = nil
	f.tls.precontextCharged[dir] = 0
	// reserveSession retains historical peaks. Release only this queue's actual
	// growth, clamped to all still-live TLS/carrier state, never another owner.
	minimum := f.tls.storage()
	if f.wcfTLS != nil {
		minimum += 256
	}
	if release := min(charge, max(int64(0), f.sessionBytes-minimum)); release > 0 {
		f.sessionBytes -= release
		f.a.buffered.Add(-release)
	}
}

// Retiring an unavailable observation is not a failed authentication trial or
// a malformed record. Retain the conservative private continuity barrier, but
// do not publish the generic "one record rejected" diagnosis for absent keys.
func (f *binFlow) notePrecontextUnavailable(dir int, info map[string]any) {
	f.noteTLSRecordGap(dir, info, false)
	delete(info, "Record Failure Isolated")
	info["Recovery State"] = "earlier encrypted handshake unavailable; plaintext continuity unverified"
	info["Inner Application Status"] = "not dissected across an unavailable earlier encrypted handshake"
}

func (f *binFlow) retainPrecontextFinished(dir int, e *ProtocolEvent, info map[string]any) {
	t := f.tls
	if f.transportFlow == nil && !f.orderedTLSCarrier || t == nil {
		return
	}
	eligible := (t.version == 0 || t.version == 0x303) && t.ccs[dir] && len(e.Raw) >= 5 && e.Raw[0] == 22 &&
		t.keys[dir] == nil && t.seq[dir] == 0 && t.child == nil && !t.recordGap[dir] && !f.plaintextGap[dir]
	if !eligible {
		if len(t.precontext[dir]) > 0 {
			f.discardPrecontextFinished(dir)
			f.notePrecontextUnavailable(dir, info)
			info["Precontext Finished Recovery Status"] = "unavailable after another undecoded protected record"
		}
		return
	}
	var held int
	for _, r := range t.precontext[dir] {
		held += len(r.wire)
	}
	if len(t.precontext[dir]) >= 4 || len(t.precontext[dir]) >= f.a.budget.MaxCollectionElements || len(e.Raw) > 65536-held {
		f.discardPrecontextFinished(dir)
		f.notePrecontextUnavailable(dir, info)
		info["Precontext Finished Recovery Status"] = "not retained: bounded record/byte limit"
		return
	}
	before := f.sessionBytes
	if err := f.reserveSession(t.storage() + int64(len(e.Raw)+128)); err != nil {
		f.discardPrecontextFinished(dir)
		f.notePrecontextUnavailable(dir, info)
		info["Precontext Finished Recovery Status"] = "not retained: configured session resource limit"
		return
	}
	owned := make([]byte, len(e.Raw))
	copy(owned, e.Raw)
	// emit normally assigns the identity after consumeTLS. Reserve it before
	// retaining an association; copying zero or a later editable event cannot
	// identify the earlier source after the Hello context arrives.
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	t.precontext[dir] = append(t.precontext[dir], tlsPrecontextRecord{wire: owned, id: e.ID})
	t.precontextCharged[dir] += f.sessionBytes - before
	info["Precontext Finished Recovery Status"] = "complete protected handshake record retained; context and authentication unavailable"
}

// Authenticate every retained record at its exact sequence and require exactly
// one complete TLS 1.2 Finished before committing catch-up. A failed tag, body,
// missing record or another content type cannot authorize application delivery.
// Previously published events remain unchanged; the later event owns a clearly
// separate supplemental handshake observation and its original PDU identities.
func (f *binFlow) recoverPrecontextFinished(dir int, info map[string]any) {
	t := f.tls
	if t == nil || len(t.precontext[dir]) == 0 {
		return
	}
	if t.version != 0x303 {
		f.discardPrecontextFinished(dir)
		info["Precontext Finished Recovery Status"] = "unavailable: negotiated profile is not TLS 1.2"
		return
	}
	refuse := func(reason string) {
		f.discardPrecontextFinished(dir)
		f.noteTLSRecordGap(dir, info, false)
		info["Precontext Finished Recovery Status"] = reason
	}
	if t.seq[dir] != 0 || t.child != nil || t.recordGap[dir] || f.plaintextGap[dir] || len(t.handshake[dir]) != 0 {
		refuse("unavailable: prior sequence/application gap or changed context")
		return
	}
	trial := *t
	var encoded []byte
	ids := make([]uint64, 0, len(t.precontext[dir]))
	digests := make([]string, 0, len(t.precontext[dir]))
	for _, record := range t.precontext[dir] {
		plain, kind, err := trial.open(dir, record.wire)
		if err != nil || kind != 22 || len(plain) > 16-len(encoded) {
			refuse("unavailable: earlier record did not authenticate as a bounded handshake")
			return
		}
		encoded = append(encoded, plain...)
		ids = append(ids, record.id)
		sum := sha256.Sum256(record.wire)
		digests = append(digests, hex.EncodeToString(sum[:]))
	}
	if _, err := DecodeTLSFinished(encoded, trial.version, trial.suite); err != nil {
		refuse("unavailable: earlier authenticated plaintext is not one complete TLS 1.2 Finished")
		return
	}
	message, err := trial.handshakeMessage(dir, encoded, true, f.a.budget.MaxCollectionElements)
	if err != nil {
		refuse("unavailable: earlier Finished body was refused")
		return
	}
	trial.precontext[dir] = nil
	*t = trial
	f.discardPrecontextFinished(dir)
	info["Precontext Finished Recovery Verified"] = true
	info["Precontext Finished Original PDUs"] = ids
	info["Precontext Finished Original Record SHA256"] = digests
	info["Recovered Context Handshake Messages"] = []map[string]any{message}
	info["Precontext Finished Recovery Status"] = "earlier complete records AEAD authenticated; Finished body recovered; transcript MAC and peer trust not evaluated"
	// The plaintext belongs to the supplemental message, not the current record.
	message["Encoded Finished"] = bytes.Clone(encoded)
}
