package pcaputil

import (
	"container/list"
	"crypto/sha256"
	"errors"
	"time"
	"unsafe"
)

const semtechCorrelationProfile = "semtech-v2-downlink-session"
const semtechPendingTTL = 30 * time.Second

// Charge the complete shared flow allocation plus bounded registry/address/map
// overhead, rather than treating this large structure as a512-byte handle.
const semtechConversationBytes = int64(unsafe.Sizeof(binFlow{})) + 1024

type semtechPending struct {
	id                  uint64
	source, destination string
	digest              [32]byte
	at                  time.Time
	pending             bool
}
type binSemtechDownlink struct {
	seen    map[uint16]*semtechPending
	blocked bool
}

func (s *binSemtechDownlink) storage() int64 {
	return semtechConversationBytes + 256*int64(len(s.seen))
}

// Explicit selection preserves the previous stateless observation profile.
// The two token bytes have no direction/epoch/authentication information. Seen
// tokens never become a fresh transaction within this capture, including after
// idle expiry, completion, collision or resource rejection. Exhausted history
// fails closed rather than evicting markers and falsely matching late feedback.
func (a *binParser) correlateSemtech(e *ProtocolEvent, w []byte, inputErr error) error {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	if a.semtechDisabled {
		return protocolError(ErrResourceExceeded, "Semtech association history exhausted")
	}
	if len(w) < 4 || w[0] != 2 || (w[3] != 3 && w[3] != 5) {
		return inputErr
	}
	key := binUDPKey{"semtech-session/" + e.Source, "semtech-session/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	store := a.semtechSessions
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.semtechSessions = store
	}
	if e.Timestamp.After(store.clock) {
		store.clock = e.Timestamp
	}
	el := store.entries[key]
	if el == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.semtechDisabled = true
			return protocolError(ErrResourceExceeded, "Semtech UDP conversation budget")
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: "semtech-udp", semtech: &binSemtechDownlink{seen: map[uint16]*semtechPending{}}, endpoints: [2]string{e.Source, e.Destination}}
		if err := f.reserveSession(semtechConversationBytes); err != nil {
			if len(store.entries) == 0 {
				a.semtechSessions = nil
			}
			a.semtechDisabled = true
			return err
		}
		el = store.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: store.clock})
		store.entries[key] = el
	}
	entry := el.Value.(*binUDPEntry)
	f := entry.flow
	s := f.semtech
	e.FlowID = f.id
	if e.Source != f.endpoints[0] {
		e.Direction = 1
	}
	// Expire pending messages without deleting bounded identity history.
	for _, v := range s.seen {
		if v.pending && store.clock.Sub(v.at) >= semtechPendingTTL {
			v.pending = false
		}
	}
	entry.touched = store.clock
	store.lru.MoveToBack(el)
	token := uint16(w[1])<<8 | uint16(w[2])
	prior := s.seen[token]
	if s.blocked {
		return protocolError(ErrResourceExceeded, "Semtech conversation history exhausted")
	}
	if inputErr != nil {
		if prior != nil {
			prior.pending = false
		} else {
			s.blocked = true
		}
		return inputErr
	}
	// A backdated packet cannot open or complete a newer transaction.
	if e.Timestamp.Before(store.clock) {
		if prior != nil {
			prior.pending = false
		}
		return protocolError(ErrContextRequired, "Semtech backdated datagram")
	}
	if prior == nil {
		if len(s.seen) >= min(128, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			s.blocked = true
			for _, v := range s.seen {
				v.pending = false
			}
			return protocolError(ErrResourceExceeded, "Semtech token history budget")
		}
		if err := f.reserveSession(s.storage() + 256); err != nil {
			s.blocked = true
			for _, v := range s.seen {
				v.pending = false
			}
			return err
		}
		prior = &semtechPending{id: e.ID, source: e.Source, destination: e.Destination, digest: sha256.Sum256(w), at: store.clock, pending: w[3] == 3}
		s.seen[token] = prior
		if w[3] == 5 {
			return protocolError(ErrContextRequired, "Semtech feedback lacks observed request")
		}
		e.TransactionID = e.ID
		e.Session = semtechAssociation("observed-request")
		return nil
	}
	if w[3] == 3 {
		if prior.pending && prior.source == e.Source && prior.destination == e.Destination && prior.digest == sha256.Sum256(w) {
			e.TransactionID = prior.id
			e.Session = semtechAssociation("observed-duplicate-request")
			return nil
		}
		prior.pending = false
		return protocolError(ErrContextRequired, "Semtech token reuse is ambiguous")
	}
	if !prior.pending || prior.source != e.Destination || prior.destination != e.Source {
		return protocolError(ErrContextRequired, "Semtech feedback direction or token context is missing")
	}
	prior.pending = false
	e.ResponseTo, e.TransactionID = prior.id, prior.id
	e.Session = semtechAssociation("observed-feedback")
	return nil
}
func semtechAssociation(kind string) map[string]any {
	return map[string]any{"Association": kind, "Authentication Verified": false, "RF Delivery Proven": false, "Observation Scope": "endpoint-and-capture-domain", "Token Reuse Policy": "quarantined for capture lifetime"}
}

func (a *binParser) decodeSemtechCorrelatedDatagram(e *ProtocolEvent, w []byte) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "semtech-udp", semtechCorrelationProfile, "explicit-decode-as", "message"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	temp := &binFlow{a: a}
	defer temp.closeSession()
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	var fields map[string]any
	var err error
	if len(w) > limit {
		err = protocolError(ErrResourceExceeded, "Semtech downlink exceeds message budget")
	} else if err = temp.reserveSession(semtechDownlinkProjectionBytes + 512*int64(len(w))); err == nil {
		fields, err = decodeSemtechDownlink(w, a.budget.MaxCollectionElements)
		if err == nil {
			err = semtechProjectionLimit(fields, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		}
	}
	err = a.correlateSemtech(e, w, err)
	if err == nil {
		e.semanticFields = cloneSession(fields)
	} else {
		e.Session = nil
		e.ResponseTo = 0
		e.TransactionID = 0
	}
	raw := w
	if len(w) > limit || temp.sessionBytes == 0 {
		raw = nil
	}
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
		err = typed
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil && len(w) > 0 {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}
