package pcaputil

import "errors"

// A shared identity ledger describes the finite passive association policy.
// Command is an owner condition, not part of the PID32/MID/request-direction
// key: a cross-command reuse cannot make two outstanding owners unambiguous.
type smb1LegacyOwner struct {
	command     byte
	id, lineage uint64
	phase       byte
}

const (
	smb1LegacyPending byte = iota + 1
	smb1LegacyCompleted
	smb1LegacyAmbiguous
	smb1LegacyOrphan
)

type smb1OperationPending struct {
	id            uint64
	hash          string
	command       byte
	uid, tid      uint16
	path, service string
}

type smb1LegacyState struct {
	owners    map[smb1SetupKey]smb1LegacyOwner
	pending   map[smb1SetupKey]smb1OperationPending
	allocated [2]int
}

func smb1LegacyStateBound(owners, pending int) int64 {
	n := int64(256)
	for _, p := range [][2]int{{owners, 64}, {pending, 192}} {
		if p[0] > 0 {
			n += 256 + int64((p[0]+5)/6)*16*int64(p[1])
		}
	}
	return n
}

func (s *smb1LegacyState) stringBytes() int64 {
	var n int64
	if s != nil {
		for _, p := range s.pending {
			n += smb1OwnedStringBound(p.hash) + smb1OwnedStringBound(p.path) + smb1OwnedStringBound(p.service)
		}
	}
	return n
}

// Conservative retained backing allowance, including allocator rounding.
func smb1OwnedStringBound(value string) int64 {
	if len(value) == 0 {
		return 0
	}
	return int64(len(value))*2 + 16
}

func (s *smb1LegacyState) ownedBytes() int64 {
	if s == nil {
		return 0
	}
	return smb1LegacyStateBound(max(s.allocated[0], len(s.owners)), max(s.allocated[1], len(s.pending))) + s.stringBytes()
}

func (s *smb1LegacyState) clone() *smb1LegacyState {
	n := &smb1LegacyState{owners: map[smb1SetupKey]smb1LegacyOwner{}, pending: map[smb1SetupKey]smb1OperationPending{}}
	if s != nil {
		for k, v := range s.owners {
			n.owners[k] = v
		}
		for k, v := range s.pending {
			n.pending[k] = v
		}
		n.allocated = [2]int{len(n.owners), len(n.pending)}
	}
	return n
}

type smb1LegacyTxn struct {
	f                                                    *binFlow
	previous                                             *binSMB2
	key                                                  smb1SetupKey
	command                                              byte
	response, allowed, refused, tracked, refusalEligible bool
	reason                                               string
	e                                                    *ProtocolEvent
}

// The caller already decoded its complete selected canonical profile. Reserve
// old plus staged maps, possible setup-owner retirement and the following
// setup stage before any retained-state copy. A failed operation restores the
// old private state; session high-water accounting remains until flow close.
func (f *binFlow) beginSMB1Legacy(dir int, e *ProtocolEvent, fields map[string]any) (*smb1LegacyTxn, error) {
	if f.smb2 == nil {
		return nil, sessionContext("CIFS: carrier state unavailable")
	}
	h, ok := fields["Header"].(map[string]any)
	if !ok {
		return nil, protocolError(ErrMalformedMessage, "CIFS: missing owned identity header")
	}
	pid, mid, err := smb1NegotiateIdentity(fields)
	if err != nil {
		return nil, err
	}
	command, a := h["Command"].(byte)
	flags, b := h["Flags"].(byte)
	if !a || !b {
		return nil, protocolError(ErrMalformedMessage, "CIFS: missing typed command role")
	}
	uid, u := h["User ID"].(uint16)
	tid, v := h["Tree ID"].(uint16)
	if !u || !v {
		return nil, protocolError(ErrMalformedMessage, "CIFS: missing typed UID/TID")
	}
	response := flags&0x80 != 0
	requestDir := dir
	if response {
		requestDir = 1 - dir
	}
	tx := &smb1LegacyTxn{f: f, previous: f.smb2, key: smb1SetupKey{pid: pid, mid: mid, dir: requestDir}, command: command, response: response, e: e}
	if dir < 0 || dir > 1 || mid == 0xffff || f.smb2.smb1Modern {
		switch {
		case f.smb2.smb1Modern:
			tx.reason = "modern carrier observed; legacy association unavailable"
		case mid == 0xffff:
			tx.reason = "MID 0xffff is reserved for OpLock; no ordinary request association"
		default:
			tx.reason = "request direction unavailable; no passive legacy association"
		}
		working := *f.smb2
		f.smb2 = &working
		return tx, nil
	}
	s := f.smb2.smb1Legacy
	owners, pending := 0, 0
	if s != nil {
		owners, pending = len(s.owners), len(s.pending)
	}
	var exists bool
	var priorOwner smb1LegacyOwner
	if s != nil {
		priorOwner, exists = s.owners[tx.key]
	}
	if !exists && owners >= f.a.budget.MaxCollectionElements {
		return nil, protocolError(ErrResourceExceeded, "CIFS: shared operation identity collection exceeded")
	}
	setup := f.smb2.smb1Setup
	sp, ss, sc, su := 0, 0, 0, 0
	if setup != nil {
		sp, ss, sc, su = len(setup.pending), len(setup.seen), len(setup.continuations), len(setup.usedUID)
	}
	newStrings := int64(144) // 64-byte hexadecimal digest, with backing allowance.
	if body, ok := fields["Body"].(map[string]any); ok {
		for _, name := range []string{"Path", "File Name", "Service"} {
			if value, ok := body[name].(string); ok {
				newStrings += smb1OwnedStringBound(value)
			}
		}
	}
	stage := smb1LegacyStateBound(owners+1, pending+1) + s.stringBytes() + newStrings
	if command == 0x73 {
		stage += 2 * smb1SetupStateBound(sp+1, ss+1, sc+1, su+1)
	} else if !response && exists && priorOwner.command == 0x73 && priorOwner.lineage != 0 && setup != nil {
		stage += smb1SetupStateBound(sp, ss, sc, su)
	}
	stage += f.smb2.smb1.ownedBytes()
	if !response && command == 0x72 {
		rows, ok := fields["Dialects"].([]map[string]any)
		if !ok {
			return nil, protocolError(ErrMalformedMessage, "CIFS: missing owned dialect collection")
		}
		stage += 256 + int64(len(rows))*24 + 64
		for _, row := range rows {
			name, ok := row["Raw Name Bytes"].([]byte)
			if !ok {
				return nil, protocolError(ErrMalformedMessage, "CIFS: missing owned raw dialect name")
			}
			// bytes.Clone may return rounded backing capacity, not just len.
			stage += int64(len(name))*2 + 16
		}
	}
	if err = f.smb2.reserveState(stage); err != nil {
		return nil, smb1ResourceRefusal(err)
	}
	working := *f.smb2
	working.smb1Legacy = s.clone()
	f.smb2 = &working
	tx.tracked = true
	ledger := working.smb1Legacy
	owner, exists := ledger.owners[tx.key]
	if !response {
		if exists {
			tx.retireOwner(owner)
			owner.phase = smb1LegacyAmbiguous
			ledger.owners[tx.key] = owner
			tx.reason = "PID/MID operation group is ambiguous; passive association unavailable"
		} else {
			ledger.owners[tx.key] = smb1LegacyOwner{command: command, id: e.ID, phase: smb1LegacyPending}
			tx.allowed = true
		}
	} else if !exists {
		ledger.owners[tx.key] = smb1LegacyOwner{command: command, phase: smb1LegacyOrphan}
		// Existing command-specific code may publish an unmatched response, but
		// a future request cannot retroactively borrow this observed response.
		tx.allowed = true
	} else if (owner.phase == smb1LegacyPending || owner.phase == smb1LegacyCompleted) && owner.command == command {
		// A duplicate response cannot associate again. Command-specific rules
		// must still retire an actual old setup continuation on replay.
		tx.allowed = true
	} else {
		tx.reason = "no unique outstanding command owner for this PID/MID"
	}
	if !response {
		tx.refusalEligible = true
	} else if exists && tx.allowed && owner.command == command {
		switch command {
		case 0x73:
			if working.smb1Setup != nil {
				if p, ok := working.smb1Setup.pending[tx.key]; owner.phase == smb1LegacyPending && ok {
					tx.refusalEligible = p.id == owner.id && p.dir != dir && uid != 0 && (p.uid == 0 || p.uid == uid)
				} else if owner.phase == smb1LegacyCompleted && owner.lineage != 0 {
					// A refused duplicate can retire only its captured UID owner,
					// never an unrelated continuation claimed by an incoming UID.
					tx.refusalEligible = uid != 0 && working.smb1Setup.usedUID[uid] == owner.lineage
				}
			}
		case 0x75, 0x06:
			if p, ok := ledger.pending[tx.key]; owner.phase == smb1LegacyPending && ok {
				tx.refusalEligible = p.id == owner.id && p.command == command && p.uid == uid && (command == 0x75 || p.tid == tid)
			}
		}
	}
	ledger.allocated[0] = max(ledger.allocated[0], len(ledger.owners))
	return tx, nil
}

func (t *smb1LegacyTxn) retireOwner(owner smb1LegacyOwner) {
	s := t.f.smb2
	switch owner.command {
	case 0x72:
		if n := s.smb1; n != nil && n.offer != nil && n.offer.id == owner.id {
			copy := *n
			copy.ambiguous, copy.offer = true, nil
			s.smb1 = &copy
		}
	case 0x73:
		if s.smb1Setup != nil && owner.lineage != 0 {
			s.smb1Setup = s.smb1Setup.clone()
			s.smb1Setup.retireLineage(owner.lineage)
		}
	default:
		delete(s.smb1Legacy.pending, t.key)
	}
}

func (t *smb1LegacyTxn) finish(result *error) {
	var typed *ProtocolError
	diagnostic := t.refused && errors.As(*result, &typed) && (typed.Kind == ErrMalformedMessage || typed.Kind == ErrUnsupportedFeature) && (t.e.smb1SetupBodyRefusal && probeSMB1SessionSetupExtended(t.e.Raw, 65598).Verdict == ProbeAccept ||
		t.e.smb1FileBodyRefusal && boundedSMB1FileEnvelope(t.e.Raw))
	if *result != nil && !diagnostic {
		t.f.smb2 = t.previous
		t.e.ResponseTo = 0
		t.e.smb1SetupBodyRefusal, t.e.smb1FileBodyRefusal = false, false
		t.e.semanticFields, t.e.semanticSession, t.e.Session = nil, nil, nil
		return
	}
	if t.tracked {
		s := t.f.smb2
		owner := s.smb1Legacy.owners[t.key]
		if diagnostic && t.refusalEligible {
			if owner.phase == smb1LegacyPending || t.command == 0x73 && owner.phase == smb1LegacyCompleted {
				t.retireOwner(owner)
			}
			owner.phase = smb1LegacyAmbiguous
		} else if t.response {
			if owner.phase == smb1LegacyPending && owner.command == t.command && t.e.ResponseTo == owner.id && owner.id != 0 {
				owner.phase = smb1LegacyCompleted
			}
		} else if owner.phase == smb1LegacyPending && owner.id == t.e.ID {
			if t.command == 0x73 && s.smb1Setup != nil {
				owner.lineage = s.smb1Setup.seen[t.key]
			}
		}
		s.smb1Legacy.owners[t.key] = owner
		s.smb1Legacy.allocated[1] = max(s.smb1Legacy.allocated[1], len(s.smb1Legacy.pending))
	}
	if err := t.f.smb2.reserveState(0); err != nil {
		t.f.smb2 = t.previous
		t.e.ResponseTo = 0
		t.e.semanticFields, t.e.semanticSession, t.e.Session = nil, nil, nil
		t.e.smb1SetupBodyRefusal, t.e.smb1FileBodyRefusal = false, false
		*result = smb1ResourceRefusal(err)
		return
	}
	// Preserve the carrier object's identity for its flow and existing users.
	// Only the staged private namespace fields are published after success.
	*t.previous = *t.f.smb2
	t.f.smb2 = t.previous
}
