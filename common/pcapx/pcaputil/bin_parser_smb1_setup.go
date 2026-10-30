package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// Admission requires the complete fixed layout, explicit transport boundary,
// extended-security/Unicode role and terminal AndX, not a port or token prefix.
func probeSMB1SessionSetupExtended(w []byte, limit int) ProbeResult {
	reject := ProbeResult{Verdict: ProbeReject}
	if len(w) < 8 || w[0] != 0 || binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff {
		return reject
	}
	total := smb2DirectLength(w) + 4
	if total > min(limit, 65598) || total < 47 || len(w) > 8 && w[8] != 0x73 {
		return reject
	}
	if len(w) < 16 {
		return probeNeed("smb", "1-session-setup-extended", len(w), 16)
	}
	response := w[13]&0x80 != 0
	base, wc := 63, byte(12)
	if response {
		base, wc = 47, 4
		if total > 65582 {
			return reject
		}
	}
	if total < base {
		return reject
	}
	flags2 := binary.LittleEndian.Uint16(w[14:16])
	if flags2&0x8800 != 0x8800 {
		return reject
	}
	if response {
		status := binary.LittleEndian.Uint32(w[9:13])
		if status != 0 && (status != 0xc0000016 || flags2&0x4000 == 0) {
			return reject
		}
	}
	if len(w) > 36 && w[36] != wc || len(w) > 37 && w[37] != 0xff {
		return reject
	}
	if !response && len(w) >= 61 && binary.LittleEndian.Uint32(w[57:61])&0x80000000 == 0 {
		return reject
	}
	if response && len(w) >= 43 && binary.LittleEndian.Uint16(w[41:43])&2 != 0 {
		return reject
	}
	if len(w) < base {
		return probeNeed("smb", "1-session-setup-extended", len(w), base)
	}
	count := int(binary.LittleEndian.Uint16(w[base-2 : base]))
	minimum, blobAt := 4, 51
	if response {
		minimum, blobAt = 6, 43
	}
	if count < minimum || count != total-base || int(binary.LittleEndian.Uint16(w[blobAt:blobAt+2])) > count {
		return reject
	}
	if len(w) < total {
		return probeNeed("smb", "1-session-setup-extended", len(w), total)
	}
	return probeAccept("smb", "1-session-setup-extended", 99)
}

// The finite setup profile has its own PID32/MID namespace and UID lineage.
// Completed/duplicate keys are quarantined until the TCP generation ends: a
// reused numeric MID cannot silently bind a late response to a newer request.
type smb1SetupKey struct {
	pid uint32
	mid uint16
	dir int // direction of the request; a response must be opposite
}
type smb1SetupPending struct {
	id           uint64
	hash         string
	uid          uint16
	dir          int
	continuation uint64
	lineage      uint64
}
type smb1SetupContinuation struct {
	responseID uint64
	pid        uint32
	dir        int
	key        smb1SetupKey
	lineage    uint64
}
type smb1SetupState struct {
	pending       map[smb1SetupKey]smb1SetupPending
	seen          map[smb1SetupKey]uint64
	continuations map[uint16]smb1SetupContinuation
	usedUID       map[uint16]uint64 // root request ID; zero quarantines ambiguous reuse
	allocated     [4]int            // clone/insertion high-water counts; deletion retains buckets
}

func (s *smb1SetupState) ownedBytes() int64 {
	if s == nil {
		return 0
	}
	// Conservative map bucket/entry allowance, including tombstone keys and
	// string backing bytes. All entries also obey MaxCollectionElements.
	return smb1SetupStateBound(max(s.allocated[0], len(s.pending)), max(s.allocated[1], len(s.seen)), max(s.allocated[2], len(s.continuations)), max(s.allocated[3], len(s.usedUID)))
}

func smb1SetupStateBound(pending, seen, continuations, uids int) int64 {
	// Account for partly filled eight-slot Go1.22 buckets, backing strings,
	// growth/overflow allowance and map headers. Tombstones are never evicted
	// to admit a potentially ambiguous reused identifier.
	n := int64(256)
	for _, pair := range [][2]int{{pending, 160}, {seen, 48}, {continuations, 96}, {uids, 32}} {
		if pair[0] > 0 {
			n += 256 + int64((pair[0]+5)/6)*16*int64(pair[1])
		}
	}
	return n
}

func (s *smb1SetupState) retireLineage(lineage uint64) {
	if lineage == 0 {
		return
	}
	for k, v := range s.pending {
		if v.lineage == lineage {
			delete(s.pending, k)
		}
	}
	for k, v := range s.continuations {
		if v.lineage == lineage {
			delete(s.continuations, k)
		}
	}
	for k, v := range s.usedUID {
		if v == lineage {
			s.usedUID[k] = 0
		}
	}
}

func (s *smb1SetupState) clone() *smb1SetupState {
	n := &smb1SetupState{pending: map[smb1SetupKey]smb1SetupPending{}, seen: map[smb1SetupKey]uint64{}, continuations: map[uint16]smb1SetupContinuation{}, usedUID: map[uint16]uint64{}}
	if s != nil {
		n.allocated = [4]int{len(s.pending), len(s.seen), len(s.continuations), len(s.usedUID)}
		for k, v := range s.pending {
			n.pending[k] = v
		}
		for k, v := range s.seen {
			n.seen[k] = v
		}
		for k, v := range s.continuations {
			n.continuations[k] = v
		}
		for k, v := range s.usedUID {
			n.usedUID[k] = v
		}
	}
	return n
}

func (f *binFlow) consumeSMB1Setup(dir int, e *ProtocolEvent) error {
	response := e.Entry == "CIFSDirectTCPSessionSetupExtendedResponse"
	e.nativeSessionMessage = true
	e.Profile, e.Completeness = "cifs-session-setup-extended-request", "message"
	if response {
		e.Profile = "cifs-session-setup-extended-response"
	}
	fields, err := decodeSMB1SessionSetupFields(e.Raw, f.a.budget, response)
	if err != nil {
		var typed *ProtocolError
		if !errors.As(err, &typed) {
			err = protocolError(ErrMalformedMessage, "%s", err)
		}
		if f.smb2 != nil && probeSMB1SessionSetupExtended(e.Raw, 65598).Verdict == ProbeAccept {
			var refused *ProtocolError
			if errors.As(err, &refused) && (refused.Kind == ErrMalformedMessage || refused.Kind == ErrUnsupportedFeature) {
				return f.refuseSMB1SetupBody(dir, e, err)
			}
		}
		return smb1ResourceRefusal(err)
	}
	if f.smb2 == nil {
		return sessionContext("CIFS setup: carrier unavailable")
	}
	pid, mid, err := smb1NegotiateIdentity(fields)
	if err != nil {
		return err
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	h := fields["Header"].(map[string]any)
	uid := h["User ID"].(uint16)
	requestDir := dir
	if response {
		requestDir = 1 - dir
	}
	key := smb1SetupKey{pid: pid, mid: mid, dir: requestDir}
	next, err := f.stageSMB1SetupState()
	if err != nil {
		return err
	}
	fields["Setup Context Status"] = "request context unavailable"
	if f.smb2.smb1Modern {
		fields["Setup Context Status"] = "modern carrier observed; legacy setup context unavailable"
		next = &smb1SetupState{}
	} else if !response {
		if lineage, seen := next.seen[key]; seen {
			delete(next.pending, key)
			// Retire only the old key's actual lineage. An untrusted duplicate's
			// claimed UID cannot erase an independent PID's valid continuation.
			next.retireLineage(lineage)
			fields["Setup Context Status"] = "duplicate or reused PID/MID; association unavailable"
		} else {
			digest := sha256.Sum256(e.Raw)
			pending := smb1SetupPending{id: e.ID, hash: hex.EncodeToString(digest[:]), uid: uid, dir: dir, lineage: e.ID}
			eligible := uid == 0
			if uid != 0 {
				if continuation, ok := next.continuations[uid]; ok && continuation.pid == pid && continuation.dir == dir {
					pending.continuation = continuation.responseID
					pending.lineage = continuation.lineage
					fields["Continuation Response Event ID"] = continuation.responseID
					fields["Setup Phase"] = "continuation request"
					eligible = true
					delete(next.continuations, uid)
				}
			}
			next.seen[key] = 0
			if eligible {
				next.seen[key] = pending.lineage
				next.pending[key] = pending
				fields["Setup Context Status"] = "request observed; response required"
			} else {
				fields["Setup Context Status"] = "assigned UID has no unambiguous observed continuation"
			}
		}
	} else {
		pending, ok := next.pending[key]
		if ok && pending.dir != dir && uid != 0 && (pending.uid == 0 || pending.uid == uid) {
			delete(next.pending, key)
			e.ResponseTo = pending.id
			fields["Matched Request"] = "SMB_COM_SESSION_SETUP_ANDX"
			fields["Request Event ID"], fields["Request PDU SHA256"] = pending.id, pending.hash
			fields["Setup Context Status"] = "matched observed request; authentication unverified"
			status := h["Status"].(uint32)
			owner, allocated := next.usedUID[uid]
			if allocated && owner != pending.lineage {
				next.retireLineage(owner)
				next.retireLineage(pending.lineage)
				next.usedUID[uid] = 0
				delete(next.continuations, uid)
				fields["UID Context Status"] = "allocated UID reuse is ambiguous; continuation unavailable"
			} else {
				next.usedUID[uid] = pending.lineage
				fields["UID Context Status"] = "observed setup lineage; authentication unverified"
				if status == 0xc0000016 {
					next.continuations[uid] = smb1SetupContinuation{responseID: e.ID, pid: pid, dir: pending.dir, key: key, lineage: pending.lineage}
				} else {
					delete(next.continuations, uid)
				}
			}
		} else {
			// An unmatched response never allocates a UID lineage. Its completed
			// key cannot later be reused to borrow an already seen response.
			if !ok {
				next.retireLineage(next.seen[key])
				if _, seen := next.seen[key]; !seen {
					next.seen[key] = 0
				}
			}
			fields["Setup Context Status"] = "response direction, UID or PID/MID has no matching request"
		}
	}
	for i, count := range []int{len(next.pending), len(next.seen), len(next.continuations), len(next.usedUID)} {
		next.allocated[i] = max(next.allocated[i], count)
	}
	if len(next.seen) > f.a.budget.MaxCollectionElements || len(next.pending) > f.a.budget.MaxCollectionElements || len(next.continuations) > f.a.budget.MaxCollectionElements || len(next.usedUID) > f.a.budget.MaxCollectionElements {
		e.ResponseTo = 0
		return protocolError(ErrResourceExceeded, "CIFS setup: bounded association collection exceeded")
	}
	f.smb2.smb1Setup = next
	if err = f.smb2.reserveState(0); err != nil {
		e.ResponseTo = 0
		return smb1ResourceRefusal(err)
	}
	e.Session = fields
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	return nil
}

func (f *binFlow) stageSMB1SetupState() (*smb1SetupState, error) {
	previous := f.smb2.smb1Setup
	pending, seen, continuations, uids := 0, 0, 0, 0
	if previous != nil {
		pending, seen, continuations, uids = len(previous.pending), len(previous.seen), len(previous.continuations), len(previous.usedUID)
	}
	// Reserve the temporary copy before allocation, in addition to the current
	// owner's retained buckets. Accounting keeps this high-water until close.
	if err := f.smb2.reserveState(smb1SetupStateBound(pending+1, seen+1, continuations+1, uids+1)); err != nil {
		return nil, smb1ResourceRefusal(err)
	}
	return previous.clone(), nil
}

// Only a complete, independently bounded selected fixed layout reaches this
// path. Keep literal headers and the exact refused PDU, retire that setup's
// eligibility, then continue at the next declared Direct TCP record. There is
// no byte scan, accepted body publication, token inference or authenticated UID.
func (f *binFlow) refuseSMB1SetupBody(dir int, e *ProtocolEvent, cause error) error {
	w := e.Raw
	response := w[13]&0x80 != 0
	pid := uint32(binary.LittleEndian.Uint16(w[16:18]))<<16 | uint32(binary.LittleEndian.Uint16(w[30:32]))
	requestDir := dir
	if response {
		requestDir = 1 - dir
	}
	key := smb1SetupKey{pid: pid, mid: binary.LittleEndian.Uint16(w[34:36]), dir: requestDir}
	next, err := f.stageSMB1SetupState()
	if err != nil {
		return err
	}
	lineage := next.seen[key]
	if !response && lineage == 0 {
		uid := binary.LittleEndian.Uint16(w[32:34])
		if c, ok := next.continuations[uid]; ok && c.pid == pid && c.dir == dir {
			lineage = c.lineage
		}
	}
	next.retireLineage(lineage)
	if _, seen := next.seen[key]; !seen {
		next.seen[key] = 0
	}
	if len(next.seen) > f.a.budget.MaxCollectionElements {
		return protocolError(ErrResourceExceeded, "CIFS setup: refused-record identity collection exceeded")
	}
	for i, count := range []int{len(next.pending), len(next.seen), len(next.continuations), len(next.usedUID)} {
		next.allocated[i] = max(next.allocated[i], count)
	}
	f.smb2.smb1Setup = next
	if err = f.smb2.reserveState(0); err != nil {
		return smb1ResourceRefusal(err)
	}
	header := map[string]any{
		"Protocol ID": binary.LittleEndian.Uint32(w[4:8]), "Command": w[8], "Status": binary.LittleEndian.Uint32(w[9:13]), "Flags": w[13],
		"Flags 2": binary.LittleEndian.Uint16(w[14:16]), "PID High": binary.LittleEndian.Uint16(w[16:18]), "Security Features": bytes.Clone(w[18:26]),
		"Reserved": binary.LittleEndian.Uint16(w[26:28]), "Tree ID": binary.LittleEndian.Uint16(w[28:30]), "Process ID Low": binary.LittleEndian.Uint16(w[30:32]),
		"User ID": binary.LittleEndian.Uint16(w[32:34]), "Multiplex ID": binary.LittleEndian.Uint16(w[34:36]),
	}
	ranges := map[string]any{}
	for _, field := range []struct {
		name       string
		start, end uint64
	}{
		{"Protocol ID", 4, 8}, {"Command", 8, 9}, {"Status", 9, 13}, {"Flags", 13, 14}, {"Flags 2", 14, 16}, {"PID High", 16, 18}, {"Security Features", 18, 26},
		{"Reserved", 26, 28}, {"Tree ID", 28, 30}, {"Process ID Low", 30, 32}, {"User ID", 32, 34}, {"Multiplex ID", 34, 36},
	} {
		ranges[field.name] = []any{field.start, field.end}
	}
	header["Wire Field Ranges"] = ranges
	fields := map[string]any{
		"Header": header, "Packet Name": "SMB_COM_SESSION_SETUP_ANDX", "Response": response,
		"Body Decode Error": cause.Error(), "Decoded Body Published": false, "Error Scope": "message",
		"Context Eligibility": "refused setup lineage retired; no accepted body committed",
		"Direct TCP":          map[string]any{"Message Type": w[0], "Payload Length": uint32(len(w) - 4), "Wire Field Ranges": map[string]any{"Message Type": []any{uint64(0), uint64(1)}, "Payload Length": []any{uint64(1), uint64(4)}}},
		"Wire Field Ranges":   map[string]any{},
	}
	e.ResponseTo = 0
	e.Profile, e.Completeness = "smb1-session-setup-body-diagnostic", "headers"
	e.Summary = "SMB1 SESSION_SETUP_ANDX · refused body · exact record boundary retained"
	e.Session = fields
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	e.smb1SetupBodyRefusal = true
	return cause
}

func isSMB1SetupEntry(entry string) bool {
	return entry == "CIFSDirectTCPSessionSetupExtendedRequest" || entry == "CIFSDirectTCPSessionSetupExtendedResponse"
}

func smb1SetupUnsupported(w []byte) error {
	if len(w) < 39 {
		return fmt.Errorf("CIFS setup: truncated word/byte envelope")
	}
	fixed := 39 + 2*int(w[36])
	if fixed > len(w) || int(binary.LittleEndian.Uint16(w[fixed-2:fixed])) != len(w)-fixed {
		return fmt.Errorf("CIFS setup: WordCount/ByteCount exceed complete record")
	}
	return protocolError(ErrUnsupportedFeature, "SMB1 SESSION_SETUP_ANDX form outside Unicode extended-security terminal profile")
}
