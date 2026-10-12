package pcaputil

import (
	"math"
)

type mmsListKey struct {
	tag          byte
	domain, item string
}
type mmsListEntry struct {
	names           []map[string]any
	revision, event uint64
}
type mmsListBinding struct {
	key             mmsListKey
	peer            int
	revision, event uint64
}
type mmsListTarget struct {
	revision uint64
	previous *mmsListEntry
}
type mmsListControl struct {
	ambiguous bool
	key       mmsListKey
	revision  uint64
	previous  *mmsListEntry
	targets   map[mmsListKey]mmsListTarget
}

func mmsKey(name map[string]any) mmsListKey {
	k := mmsListKey{tag: name["Choice Tag"].(byte)}
	if k.tag == 0xa0 || k.tag == 0xa2 {
		k.tag &^= 32
	}
	if k.tag == 0xa1 {
		k.domain = name["Domain ID"].(string)
		k.item = name["Item ID"].(string)
	} else {
		k.item = name["Identifier"].(string)
	}
	return k
}
func mmsNamesEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if mmsKey(a[i]) != mmsKey(b[i]) {
			return false
		}
	}
	return true
}
func (k mmsListKey) fields() map[string]any {
	f := map[string]any{"Choice Tag": k.tag}
	if k.tag == 0xa1 {
		f["Domain ID"], f["Item ID"] = k.domain, k.item
	} else {
		f["Identifier"] = k.item
	}
	return f
}
func mmsNamesStorage(names []map[string]any) int64 {
	var n int64
	for _, row := range names {
		n += 256
		for _, v := range row {
			if text, ok := v.(string); ok {
				n += int64(len(text))
			}
		}
	}
	return n
}
func mmsQueryStorage(q map[string]any) int64 {
	if q == nil {
		return 0
	}
	n := int64(512)
	if s, ok := q["Continue After"].(string); ok {
		n += int64(len(s))
	}
	if scope, ok := q["Object Scope"].(map[string]any); ok {
		if s, ok := scope["Identifier"].(string); ok {
			n += int64(len(s))
		}
	}
	if s, ok := q["Domain Name"].(string); ok {
		n += int64(len(s))
	}
	if names, ok := q["List Names"].([]map[string]any); ok {
		n += mmsNamesStorage(names)
	}
	return n
}
func (m *binMMS) namedStorage() int64 {
	var n int64
	if m.listObserved {
		n = 256
	}
	for dir := 0; dir < 2; dir++ {
		for k := range m.listVersions[dir] {
			n += 96 + int64(len(k.domain)+len(k.item))
		}
		for _, entry := range m.lists[dir] {
			n += 128 + mmsNamesStorage(entry.names)
		}
		for _, p := range m.pending[dir] {
			if p.control == nil {
				continue
			}
			n += 192
			if p.control.previous != nil {
				n += 128 + mmsNamesStorage(p.control.previous.names)
			}
			for k, t := range p.control.targets {
				n += 128 + int64(len(k.domain)+len(k.item))
				if t.previous != nil {
					n += 128 + mmsNamesStorage(t.previous.names)
				}
			}
		}
	}
	return n
}
func (m *binMMS) namedElements(skipDir int, skipInvoke uint32) int {
	n := len(m.listVersions[0]) + len(m.listVersions[1])
	for dir := 0; dir < 2; dir++ {
		for _, x := range m.lists[dir] {
			n += len(x.names)
		}
		for id, p := range m.pending[dir] {
			if dir == skipDir && id == skipInvoke || p.control == nil {
				continue
			}
			n += len(p.names)
			if p.control.previous != nil {
				n += len(p.control.previous.names)
			}
			n += len(p.control.targets)
			for _, t := range p.control.targets {
				if t.previous != nil {
					n += len(t.previous.names)
				}
			}
		}
	}
	return n
}
func (f *binFlow) ensureMMSListKey(peer int, key mmsListKey) error {
	m := f.mms
	if _, ok := m.listVersions[peer][key]; ok {
		return nil
	}
	if m.namedElements(-1, 0)+len(m.pending[0])+len(m.pending[1])+len(m.retired[0])+len(m.retired[1]) >= f.a.budget.MaxCollectionElements {
		return protocolError(ErrResourceExceeded, "MMS named-list identity/history budget")
	}
	if err := f.mmsReserve(f.sessionBytes + 512 + int64(len(key.domain)+len(key.item))); err != nil {
		return err
	}
	if m.listVersions[peer] == nil {
		m.listVersions[peer] = make(map[mmsListKey]uint64)
		m.lists[peer] = make(map[mmsListKey]mmsListEntry)
	}
	m.listVersions[peer][key] = 0
	return nil
}
func (m *binMMS) changeMMSList(peer int, key mmsListKey) (uint64, error) {
	if m.listClock == math.MaxUint64 {
		return 0, protocolError(ErrResourceExceeded, "MMS named-list revision space exhausted")
	}
	m.listClock++
	m.listVersions[peer][key] = m.listClock
	delete(m.lists[peer], key)
	return m.listClock, nil
}

// An ambiguous auxiliary selector is kept literally but invalidates all local
// observations for that peer. It cannot manufacture a target namespace.
func mmsDeleteSelect(q map[string]any, key mmsListKey) bool {
	scope := q["Scope Of Delete"].(uint32)
	names, hasNames := q["List Names"].([]map[string]any)
	domain, hasDomain := q["Domain Name"].(string)
	switch scope {
	case 0:
		if !hasNames || hasDomain {
			return true
		}
		for _, name := range names {
			if mmsKey(name) == key {
				return true
			}
		}
		return false
	case 1:
		if hasNames || hasDomain {
			return true
		}
		return key.tag == 0x82
	case 2:
		if !hasDomain || hasNames {
			return true
		}
		return key.tag == 0xa1 && key.domain == domain
	case 3:
		if hasNames || hasDomain {
			return true
		}
		return key.tag == 0x80
	}
	return true
}
func (f *binFlow) namedMMSRequest(d mmsDecoded, p *mmsPending, dir int) error {
	m := f.mms
	if d.binding != nil {
		binding := *d.binding
		p.binding = &binding
		p.names = cloneSessionValue(d.fields["Observed Resolved Variables"]).([]map[string]any)
	}
	if d.service < 11 || d.service > 13 {
		return nil
	}
	peer := 1 - dir
	m.listObserved = true
	p.control = &mmsListControl{}
	if d.service == 11 || d.service == 12 {
		key := mmsKey(d.fields["Variable List"].(map[string]any))
		if err := f.ensureMMSListKey(peer, key); err != nil {
			return err
		}
		p.control.key = key
		p.control.revision = m.listVersions[peer][key]
		if d.service == 12 {
			for _, other := range m.pending[dir] {
				if other.control != nil && (other.service == 11 && other.control.key == key || other.service == 13 && mmsDeleteSelect(other.query, key)) {
					p.control.ambiguous = true
				}
			}
		}
		if d.service == 11 {
			names := d.fields["Variables"].([]map[string]any)
			if m.namedElements(-1, 0)+len(names)+len(m.pending[0])+len(m.pending[1])+len(m.retired[0])+len(m.retired[1]) > f.a.budget.MaxCollectionElements {
				return protocolError(ErrResourceExceeded, "MMS named-list member context budget")
			}
			if err := f.mmsReserve(f.sessionBytes + 512 + 2*mmsNamesStorage(names)); err != nil {
				return err
			}
			p.names = cloneSessionValue(names).([]map[string]any)
			if old, ok := m.lists[peer][key]; ok {
				p.control.previous = &old
			}
			version, err := m.changeMMSList(peer, key)
			if err != nil {
				return err
			}
			p.control.revision = version
		}
		return nil
	}
	q := map[string]any{"Scope Of Delete": d.fields["Scope Of Delete"]}
	if v, ok := d.fields["List Names"]; ok {
		q["List Names"] = v
	}
	if v, ok := d.fields["Domain Name"]; ok {
		q["Domain Name"] = v
	}
	if err := f.mmsReserve(f.sessionBytes + 2*mmsQueryStorage(q)); err != nil {
		return err
	}
	p.query = cloneSession(q)
	if names, ok := q["List Names"].([]map[string]any); ok {
		for _, name := range names {
			if err := f.ensureMMSListKey(peer, mmsKey(name)); err != nil {
				return err
			}
		}
	}
	targets := make([]mmsListKey, 0)
	for key := range m.listVersions[peer] {
		if mmsDeleteSelect(q, key) {
			targets = append(targets, key)
		}
	}
	if uint64(len(targets)) > math.MaxUint64-m.listClock {
		return protocolError(ErrResourceExceeded, "MMS named-list revision space exhausted")
	}
	if m.namedElements(-1, 0)+len(targets)+len(m.pending[0])+len(m.pending[1])+len(m.retired[0])+len(m.retired[1]) > f.a.budget.MaxCollectionElements {
		return protocolError(ErrResourceExceeded, "MMS named-list delete context budget")
	}
	if err := f.mmsReserve(f.sessionBytes + int64(len(targets))*256); err != nil {
		return err
	}
	p.control.targets = make(map[mmsListKey]mmsListTarget, len(targets))
	for _, key := range targets {
		target := mmsListTarget{}
		if old, ok := m.lists[peer][key]; ok {
			target.previous = &old
		}
		version, err := m.changeMMSList(peer, key)
		if err != nil {
			return err
		}
		target.revision = version
		p.control.targets[key] = target
	}
	return nil
}
func (f *binFlow) installMMSList(peer int, key mmsListKey, names []map[string]any, event uint64, dir int, invoke uint32, transfer bool) error {
	m := f.mms
	old := m.lists[peer][key]
	if m.namedElements(dir, invoke)-len(old.names)+len(names)+len(m.pending[0])+len(m.pending[1])+len(m.retired[0])+len(m.retired[1]) > f.a.budget.MaxCollectionElements {
		return protocolError(ErrResourceExceeded, "MMS retained list members budget")
	}
	if !transfer {
		if err := f.mmsReserve(f.sessionBytes + 2*mmsNamesStorage(names)); err != nil {
			return err
		}
		names = cloneSessionValue(names).([]map[string]any)
	}
	revision := m.listVersions[peer][key]
	if !transfer && old.names == nil || old.names != nil && !mmsNamesEqual(old.names, names) {
		if m.listClock == math.MaxUint64 {
			return protocolError(ErrResourceExceeded, "MMS named-list revision space exhausted")
		}
		m.listClock++
		revision = m.listClock
		m.listVersions[peer][key] = revision
	}
	m.lists[peer][key] = mmsListEntry{names: names, revision: revision, event: event}
	return nil
}
func (f *binFlow) namedMMSResponse(d mmsDecoded, p mmsPending, dir int, e *ProtocolEvent) error {
	m := f.mms
	if d.deleteSpecific && p.service != 13 {
		return protocolError(ErrMalformedMessage, "MMS delete service-specific error mismatches request")
	}
	if p.binding != nil {
		b := p.binding
		if d.tag == 0xa1 {
			entry, ok := m.lists[b.peer][b.key]
			if !ok || entry.revision != b.revision || m.listVersions[b.peer][b.key] != b.revision {
				return protocolError(ErrContextRequired, "MMS named-list schema changed during operation")
			}
		}
		if err := f.mmsReserve(f.sessionBytes + 512 + 8*mmsNamesStorage(p.names)); err != nil {
			return err
		}
		d.fields["Observed Variable List"], d.fields["Observed Resolved Variables"], d.fields["Observed List Event"] = b.key.fields(), cloneSessionValue(p.names), b.event
	}
	if p.control == nil {
		return nil
	}
	peer := dir
	c := p.control
	if p.service == 11 || p.service == 12 {
		if m.listVersions[peer][c.key] != c.revision || c.ambiguous {
			consistentLookup := false
			if p.service == 12 && d.tag == 0xa1 {
				entry, ok := m.lists[peer][c.key]
				consistentLookup = ok && mmsNamesEqual(entry.names, d.fields["Variables"].([]map[string]any))
			}
			if !consistentLookup {
				return protocolError(ErrContextRequired, "MMS named-list observation overlaps schema mutation")
			}
		}
		d.fields["Observed Variable List"] = c.key.fields()
		if p.service == 11 {
			d.fields["Observed Definition Request"], d.fields["Observed Variable Count"] = p.event, len(p.names)
			if d.tag == 0xa1 {
				return f.installMMSList(peer, c.key, p.names, e.ID, 1-dir, d.invoke, true)
			}
			errorFields := d.fields["Service Error"].(map[string]any)
			invalidated := errorFields["Class"] == uint32(2) && errorFields["Code"] == uint32(1)
			if code, ok := errorFields["Access Error"].(uint32); ok && (code == 2 || code == 4) {
				invalidated = true
			}
			if c.previous != nil && !invalidated {
				m.lists[peer][c.key] = *c.previous
				m.listVersions[peer][c.key] = c.previous.revision
			}
		} else {
			if d.tag == 0xa1 {
				return f.installMMSList(peer, c.key, d.fields["Variables"].([]map[string]any), e.ID, 1-dir, d.invoke, false)
			}
			errorFields := d.fields["Service Error"].(map[string]any)
			if code, ok := errorFields["Access Error"].(uint32); ok && (code == 2 || code == 4) {
				_, err := m.changeMMSList(peer, c.key)
				return err
			}
			if errorFields["Class"] == uint32(2) && errorFields["Code"] == uint32(1) {
				_, err := m.changeMMSList(peer, c.key)
				return err
			}
		}
		return nil
	}
	if err := f.mmsReserve(f.sessionBytes + 8*mmsQueryStorage(p.query)); err != nil {
		return err
	}
	d.fields["Observed Delete Request"] = cloneSession(p.query)
	deleted := uint32(0)
	knownCount := d.tag == 0xa1 || d.deleteSpecific
	if d.tag == 0xa1 {
		deleted = d.fields["Number Deleted"].(uint32)
		if _, hasDomain := p.query["Domain Name"]; p.query["Scope Of Delete"] == uint32(0) && !hasDomain {
			if names, ok := p.query["List Names"].([]map[string]any); ok && d.fields["Number Matched"].(uint32) > uint32(len(names)) {
				return protocolError(ErrMalformedMessage, "MMS delete matched count exceeds requested list count")
			}
		}
	} else if d.deleteSpecific {
		deleted = d.fields["Service Error"].(map[string]any)["Delete Named List Deleted"].(uint32)
	}
	if _, hasDomain := p.query["Domain Name"]; p.query["Scope Of Delete"] == uint32(0) && !hasDomain {
		if names, ok := p.query["List Names"].([]map[string]any); ok && deleted > uint32(len(names)) {
			return protocolError(ErrMalformedMessage, "MMS partial deleted count exceeds requested list count")
		}
	}
	if knownCount && deleted == 0 {
		for key, target := range c.targets {
			if target.previous != nil && m.listVersions[peer][key] == target.revision {
				m.lists[peer][key] = *target.previous
				m.listVersions[peer][key] = target.previous.revision
			}
		}
		return nil
	}
	// A delayed positive/partial deletion, or an error with unspecified effects,
	// can invalidate observations made since its request. Revisit matching keys.
	affected := make([]mmsListKey, 0)
	for key := range m.listVersions[peer] {
		if mmsDeleteSelect(p.query, key) {
			affected = append(affected, key)
		}
	}
	if uint64(len(affected)) > math.MaxUint64-m.listClock {
		return protocolError(ErrResourceExceeded, "MMS named-list revision space exhausted")
	}
	for _, key := range affected {
		if _, err := m.changeMMSList(peer, key); err != nil {
			return err
		}
	}
	return nil
}
