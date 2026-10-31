package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

// This oracle reads raw wire positions, not parser Nodes, published ranges,
// native validators or the private legacy ledger. Zero-width raw fields and
// decoded text views have independent expected spans.
type smb1Fuzz102Literal struct {
	owner, name string
	lo, hi      int
	value       any
	view        bool
}

func smb1Fuzz102Hash(w []byte) string {
	h := sha256.Sum256(w)
	return hex.EncodeToString(h[:])
}

func smb1Fuzz102Expected(t *testing.T, w []byte) []smb1Fuzz102Literal {
	t.Helper()
	require.GreaterOrEqual(t, len(w), 39)
	require.Equal(t, byte(0), w[0])
	require.Equal(t, len(w)-4, int(w[1])<<16|int(w[2])<<8|int(w[3]))
	require.Equal(t, []byte{0xff, 'S', 'M', 'B'}, w[4:8])
	command, response := w[8], w[13]&0x80 != 0
	require.Contains(t, []byte{0x75, 0x06}, command)
	base, wc := 47, byte(4)
	if command == 0x75 && response {
		base, wc = 45, 3
	}
	if command == 0x06 {
		base, wc = 41, 1
		if response {
			base, wc = 39, 0
		}
	}
	require.GreaterOrEqual(t, len(w), base)
	require.Equal(t, wc, w[36])
	require.Equal(t, len(w)-base, int(binary.LittleEndian.Uint16(w[base-2:base])))
	if response {
		require.Zero(t, binary.LittleEndian.Uint32(w[9:13]))
	}
	if command == 0x75 || !response {
		require.NotZero(t, binary.LittleEndian.Uint16(w[14:16])&0x8000)
	}
	var out []smb1Fuzz102Literal
	add := func(owner, name string, lo, hi int, value any, view bool) {
		out = append(out, smb1Fuzz102Literal{owner, name, lo, hi, value, view})
	}
	u8 := func(owner, name string, at int) { add(owner, name, at, at+1, w[at], false) }
	u16 := func(owner, name string, at int) {
		add(owner, name, at, at+2, binary.LittleEndian.Uint16(w[at:at+2]), false)
	}
	u32 := func(owner, name string, at int) {
		add(owner, name, at, at+4, binary.LittleEndian.Uint32(w[at:at+4]), false)
	}
	raw := func(owner, name string, lo, hi int) {
		value := make([]byte, hi-lo)
		copy(value, w[lo:hi])
		add(owner, name, lo, hi, value, false)
	}
	u8("Direct TCP", "Message Type", 0)
	add("Direct TCP", "Payload Length", 1, 4, uint32(len(w)-4), false)
	u32("Header", "Protocol ID", 4)
	u8("Header", "Command", 8)
	u32("Header", "Status", 9)
	u8("Header", "Flags", 13)
	u16("Header", "Flags 2", 14)
	u16("Header", "PID High", 16)
	raw("Header", "Security Features", 18, 26)
	u16("Header", "Reserved", 26)
	u16("Header", "Tree ID", 28)
	u16("Header", "Process ID Low", 30)
	u16("Header", "User ID", 32)
	u16("Header", "Multiplex ID", 34)
	u8("Body", "Word Count", 36)
	if command == 0x75 {
		require.Equal(t, byte(0xff), w[37])
		u8("Body", "AndX Command", 37)
		u8("Body", "AndX Reserved", 38)
		u16("Body", "AndX Offset", 39)
		if response {
			u16("Body", "Optional Support", 41)
		} else {
			u16("Body", "Flags", 41)
			u16("Body", "Password Length", 43)
		}
	} else if !response {
		u16("Body", "Search Attributes", 37)
	}
	u16("Body", "Byte Count", base-2)
	at := base
	pad := func() {
		n := at % 2
		require.LessOrEqual(t, at+n, len(w))
		raw("Body", "Alignment Pad", at, at+n)
		at += n
	}
	unicode := func(name string) {
		start := at
		units := make([]uint16, 0)
		for {
			require.LessOrEqual(t, at+2, len(w), "unterminated %s", name)
			u := binary.LittleEndian.Uint16(w[at : at+2])
			at += 2
			if u == 0 {
				break
			}
			units = append(units, u)
		}
		decoded := utf16.Decode(units)
		require.Equal(t, units, utf16.Encode(decoded), "accepted UTF16 must not replace malformed surrogates")
		raw("Body", name+" Raw Bytes", start, at)
		add("Body", name, start, at-2, string(decoded), true)
	}
	service := func() {
		start := at
		for {
			require.Less(t, at, len(w), "unterminated OEM Service")
			b := w[at]
			at++
			if b == 0 {
				break
			}
			require.GreaterOrEqual(t, b, byte(0x20))
			require.LessOrEqual(t, b, byte(0x7e))
		}
		raw("Body", "Service Raw Bytes", start, at)
		add("Body", "Service", start, at-1, string(w[start:at-1]), true)
	}
	if command == 0x75 && !response {
		n := int(binary.LittleEndian.Uint16(w[43:45]))
		require.LessOrEqual(t, at+n, len(w))
		raw("Body", "Password", at, at+n)
		at += n
		pad()
		unicode("Path")
		service()
	} else if command == 0x75 {
		service()
		pad()
		unicode("Native File System")
	} else if !response {
		require.Less(t, at, len(w))
		require.Equal(t, byte(4), w[at])
		u8("Body", "Buffer Format", at)
		at++
		pad()
		unicode("File Name")
	}
	require.Equal(t, len(w), at, "no unclassified tail may be called a complete decoded finite profile")
	return out
}

func smb1Fuzz102Fields(t *testing.T, w []byte, fields map[string]any) {
	t.Helper()
	rows := smb1Fuzz102Expected(t, w)
	expected := map[string]map[string]smb1Fuzz102Literal{}
	var literal []smb1Fuzz102Literal
	for _, row := range rows {
		if expected[row.owner] == nil {
			expected[row.owner] = map[string]smb1Fuzz102Literal{}
		}
		expected[row.owner][row.name] = row
		if !row.view {
			literal = append(literal, row)
		}
	}
	for _, ownerName := range []string{"Direct TCP", "Header", "Body"} {
		owner, ok := fields[ownerName].(map[string]any)
		require.True(t, ok, ownerName)
		ranges, ok := owner["Wire Field Ranges"].(map[string]any)
		require.True(t, ok, ownerName)
		require.Len(t, ranges, len(expected[ownerName]))
		require.Len(t, owner, len(expected[ownerName])+1)
		for name, row := range expected[ownerName] {
			require.Equal(t, row.value, owner[name], "literal type/value %s.%s", ownerName, name)
			require.Equal(t, []any{uint64(row.lo), uint64(row.hi)}, ranges[name], "literal span %s.%s", ownerName, name)
		}
	}
	sort.SliceStable(literal, func(i, j int) bool {
		if literal[i].lo == literal[j].lo {
			return literal[i].hi < literal[j].hi
		}
		return literal[i].lo < literal[j].lo
	})
	at := 0
	for _, row := range literal {
		require.Equal(t, at, row.lo, "source gap/overlap %s.%s", row.owner, row.name)
		at = row.hi
	}
	require.Equal(t, len(w), at)
	for _, name := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
		require.Equal(t, false, fields[name], name)
	}
	if ranges, ok := fields["Wire Field Ranges"].(map[string]any); ok {
		for _, name := range []string{"Authentication Verified", "Signature Verified", "Session Established", "Request Event ID", "Request PDU SHA256", "Request Path", "Request File Name"} {
			require.NotContains(t, ranges, name)
		}
	}
}

func smb1Fuzz102RejectedAuthority(t *testing.T, e *ProtocolEvent) {
	t.Helper()
	got, err := e.GetFields()
	require.Error(t, err)
	require.Nil(t, got)
	var before *ProtocolError
	typed := errors.As(err, &before)
	message := err.Error()
	if !typed {
		// A native close diagnostic preserves the established non-message
		// refusal, rather than inventing a typed body-decoder failure.
		require.Equal(t, "incomplete", e.Status)
		require.EqualError(t, err, "protocol parser: event is incomplete, not an exact message")
	}
	poison := map[string]any{"accepted": true, "Authentication Verified": true, "Body": map[string]any{"forged": true}}
	e.Fields, e.Session, e.Structured = poison, poison, map[string]any{"fields": poison}
	e.Status, e.Error, e.Protocol, e.Profile, e.Completeness = "decoded", "", "tcp", "poisoned", "message"
	e.Rule, e.Entry = "application-layer.cifs", "CIFSDirectTCPDeleteResponse"
	got, err = e.GetFields()
	require.Error(t, err)
	require.Nil(t, got)
	var after *ProtocolError
	if typed {
		require.ErrorAs(t, err, &after)
		require.Equal(t, before.Kind, after.Kind)
	} else {
		require.EqualError(t, err, message)
	}
	_, err = e.Decode()
	require.Error(t, err)
	var decoded *ProtocolError
	if typed {
		require.ErrorAs(t, err, &decoded)
		require.Equal(t, before.Kind, decoded.Kind)
	} else {
		require.EqualError(t, err, message)
	}
}

func FuzzSMB1FilePublicWireSources(f *testing.F) {
	for _, command := range []byte{0x75, 0x06} {
		for _, response := range []bool{false, true} {
			w := smb1FileWire(command, response, 4, 2048, 2048)
			f.Add(w, uint16(len(w)/2))
		}
	}
	w := smb1FileWire(0x75, false, 4, 2048, 0)
	f.Add(bytes.Clone(w[:47]), uint16(9))
	bad := bytes.Clone(w)
	binary.LittleEndian.PutUint16(bad[48:50], 0xd800)
	f.Add(bad, uint16(48))
	f.Add(append(bytes.Clone(w), smb1FileWire(0x06, true, 5, 2048, 2048)...), uint16(len(w)+3))
	f.Add([]byte("\x00\x00\x00J\xffSMBu000000\xc000000000000000000000\x04\xff0000000\x1f\x0000000000000000000000000000000000"), uint16(106))
	f.Fuzz(func(t *testing.T, input []byte, cut uint16) {
		if len(input) > 4096 {
			return
		} // Explicit bounded fuzz domain; full65535B cases remain separate unit tests.
		wire, original := bytes.Clone(input), bytes.Clone(input)
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer func() { s.Close("FIN"); require.Zero(t, s.Stats().BufferedBytes) }()
		split := int(cut) % (len(wire) + 1)
		var events []*ProtocolEvent
		for i, piece := range [][]byte{wire[:split], wire[split:]} {
			r := s.Feed(0, time.Unix(int64(100+i), 0), piece)
			require.Equal(t, len(piece), r.Consumed)
			if r.Err != nil && r.Err.Kind == ErrNeedMore {
				require.True(t, r.NeedMore)
			}
			available := len(original)
			if i == 0 {
				available = split
			}
			for _, e := range r.Events {
				if e.Protocol == "smb" && e.Error == "" && (e.Status == "decoded" || e.Status == "deferred") {
					require.LessOrEqual(t, e.Offset+uint64(e.Length), uint64(available), "event preceded its actual bytes")
				}
			}
			events = append(events, r.Events...)
		}
		events = append(events, s.Close("FIN")...)
		require.Zero(t, s.Stats().BufferedBytes)
		clear(wire)
		for _, e := range events {
			if e.Protocol != "smb" {
				continue
			} // Mutated input may be another protocol; no all-protocol claim.
			require.LessOrEqual(t, e.Offset+uint64(len(e.Raw)), uint64(len(original)))
			require.Equal(t, original[e.Offset:e.Offset+uint64(len(e.Raw))], e.Raw, "events own caller feed bytes")
			if len(e.Raw) > 0 {
				require.Equal(t, smb1Fuzz102Hash(e.Raw), e.SourceBytes.SHA256)
			}
			if e.Error != "" || (e.Status != "decoded" && e.Status != "deferred") {
				smb1Fuzz102RejectedAuthority(t, e)
				continue
			}
			if len(e.Raw) < 9 || (e.Raw[8] != 0x75 && e.Raw[8] != 0x06) {
				continue
			}
			require.Equal(t, len(e.Raw), e.Length)
			require.Equal(t, "message", e.Completeness)
			require.Equal(t, "reassembled", e.SourceBytes.Kind)
			require.Empty(t, e.SourceBytes.PacketRefs)
			require.Nil(t, e.SourceBytes.CompletedIn)
			fields, err := e.GetFields()
			require.NoError(t, err)
			smb1Fuzz102Fields(t, e.Raw, fields)
			before, err := json.Marshal(fields)
			require.NoError(t, err)
			fields["Header"].(map[string]any)["Security Features"].([]byte)[0] ^= 1
			fresh, err := e.GetFields()
			require.NoError(t, err)
			after, err := json.Marshal(fresh)
			require.NoError(t, err)
			require.Equal(t, before, after, "returned fields are owned; edits cannot mutate private source")
			if e.Fields != nil {
				e.Fields["Header"].(map[string]any)["Command"] = byte(0xff)
			}
			e.Fields, e.Session, e.Structured = map[string]any{"forged": true}, map[string]any{"Authentication Verified": true}, map[string]any{"fields": map[string]any{"forged": true}}
			e.Protocol, e.Profile, e.Rule, e.Entry = "tcp", "poisoned", "application-layer.smb2", "SMB2"
			e.Raw = nil
			fresh, err = e.GetFields()
			require.NoError(t, err)
			after, err = json.Marshal(fresh)
			require.NoError(t, err)
			require.Equal(t, before, after, "public projections do not own accepted native fields")
			decoded, err := e.Decode()
			require.NoError(t, err)
			after, err = json.Marshal(decoded["fields"])
			require.NoError(t, err)
			require.Equal(t, before, after)
		}
	})
}

type smb1Fuzz102Key struct {
	pid        uint32
	mid        uint16
	requestDir int
}
type smb1Fuzz102Observed struct {
	id            uint64
	wire          []byte
	dir           int
	key           smb1Fuzz102Key
	command       byte
	uid, tid      uint16
	response, bad bool
	root          uint64
	setupEligible bool
}
type smb1Fuzz102Challenge struct {
	responseID, root uint64
	pid              uint32
	uid              uint16
	requestDir       int
	used             bool
}

func smb1Fuzz102Identity(w []byte, dir int) smb1Fuzz102Key {
	requestDir := dir
	if w[13]&0x80 != 0 {
		requestDir = 1 - dir
	}
	return smb1Fuzz102Key{uint32(binary.LittleEndian.Uint16(w[16:18]))<<16 | uint32(binary.LittleEndian.Uint16(w[30:32])), binary.LittleEndian.Uint16(w[34:36]), requestDir}
}

// Four-byte instruction: kind/flags, PID+MID selector, UID+TID selector,
// extra MID bits/string variant. Constructors contain only synthetic tokens.
func smb1Fuzz102Operation(op [4]byte) (w []byte, dir int, bad bool) {
	kind := int(op[0]&15) % 10
	response := kind%2 == 1
	command := byte(0x72)
	switch {
	case kind >= 8:
		command = 0x06
	case kind >= 6:
		command = 0x75
	case kind >= 2:
		command = 0x73
	}
	pid := []uint32{0x12345678, 0x87655678, 0x12340001, 0}[int(op[1]>>4)&3]
	mid := uint16(op[1]&7) | uint16(op[3]&3)<<3
	if op[1]&8 != 0 {
		mid = 0xffff
	}
	uid := []uint16{0, 1, 2048, 4096}[int(op[2]&3)]
	tid := []uint16{0, 1, 2048, 4096}[int(op[2]>>2)&3]
	if op[3]&0x80 != 0 {
		uid = 0xffff
	}
	switch command {
	case 0x72:
		uid, tid = 0, 0
		w = smb1ExtendedOffer()
		if response {
			w = smb1ExtendedResponse(0)
		}
	case 0x73:
		if kind == 2 {
			uid = 0
		}
		status := uint32(0)
		if kind == 3 {
			status = 0xc0000016
		}
		w = smb1SetupWire(response, mid, uid, status)
		at := 66
		if response {
			at = 50
		}
		w[at] = byte('A') + (op[3]>>2)%26
		bad = op[0]&0x10 != 0
		if bad {
			binary.LittleEndian.PutUint16(w[at:at+2], 0xd800)
		}
	default:
		w = smb1FileWire(command, response, mid, uid, tid)
	}
	w = smb1ChangeIdentity(w, pid, mid, uid, tid)
	if response {
		dir = 1
	}
	if op[0]&0x20 != 0 {
		dir = 1 - dir
	}
	return
}

func FuzzSMB1LegacyMixedAssociationSafety(f *testing.F) {
	f.Add([]byte{6, 4, 2, 0, 7, 4, 10, 0, 8, 5, 10, 0, 9, 5, 10, 0})
	f.Add([]byte{0, 1, 0, 0, 1, 1, 0, 0, 2, 2, 0, 0, 3, 2, 2, 0, 4, 3, 2, 0, 5, 3, 2, 0})
	f.Add([]byte{2, 4, 0, 0, 6, 4, 2, 0, 5, 4, 2, 0, 7, 4, 10, 0})
	f.Add([]byte{7, 4, 10, 0, 6, 4, 2, 0, 7, 4, 10, 0})
	f.Add([]byte{2, 2, 0, 0, 3, 2, 2, 0, 3, 2, 2, 0, 4, 3, 2, 0, 5, 3, 2, 0})
	f.Add([]byte{2, 2, 0, 0, 3, 2, 2, 0, 0x13, 2, 2, 0, 4, 3, 2, 0, 5, 3, 2, 0})
	f.Fuzz(func(t *testing.T, program []byte) {
		if len(program) > 24*4 {
			return
		} // At most24 independently authored complete observations.
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer func() { s.Close("FIN"); require.Zero(t, s.Stats().BufferedBytes) }()
		byID := map[uint64]*smb1Fuzz102Observed{}
		first := map[smb1Fuzz102Key]*smb1Fuzz102Observed{}
		requestCount := map[smb1Fuzz102Key]int{}
		completed := map[uint64]bool{}
		linked := map[uint64]bool{}
		completedUID := map[uint64]uint16{}
		retired := map[uint64]bool{}
		uidOwner := map[uint16]uint64{}
		challenges := map[uint16]*smb1Fuzz102Challenge{}
		negRequests := 0
		negOrphan := false
		var offsets [2]uint64
		for at := 0; at < len(program); at += 4 {
			var op [4]byte
			copy(op[:], program[at:min(at+4, len(program))])
			wire, dir, bad := smb1Fuzz102Operation(op)
			original := bytes.Clone(wire)
			r := s.Feed(dir, time.Unix(int64(100+at/4), 0), wire)
			require.Equal(t, len(wire), r.Consumed)
			require.Len(t, r.Events, 1, "authored complete profile or isolated body refusal")
			e := r.Events[0]
			require.NotZero(t, e.ID)
			require.NotContains(t, byID, e.ID)
			require.Equal(t, "smb", e.Protocol)
			require.Equal(t, original, e.Raw)
			require.Equal(t, offsets[dir], e.Offset)
			require.Equal(t, smb1Fuzz102Hash(original), e.SourceBytes.SHA256)
			offsets[dir] += uint64(len(original))
			clear(wire)
			if bad {
				require.NotNil(t, r.Err)
				require.Contains(t, []ProtocolErrorKind{ErrMalformedMessage, ErrUnsupportedFeature}, r.Err.Kind)
			} else {
				require.Nil(t, r.Err)
				require.Equal(t, "decoded", e.Status)
			}
			obs := &smb1Fuzz102Observed{id: e.ID, wire: original, dir: dir, key: smb1Fuzz102Identity(original, dir), command: original[8],
				uid: binary.LittleEndian.Uint16(original[32:34]), tid: binary.LittleEndian.Uint16(original[28:30]), response: original[13]&0x80 != 0, bad: bad}
			prior := first[obs.key]
			var fields map[string]any
			if !bad {
				fields, err = e.GetFields()
				require.NoError(t, err)
				if obs.command == 0x75 || obs.command == 0x06 {
					smb1Fuzz102Fields(t, original, fields)
				}
				for _, name := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
					if value, present := fields[name]; present {
						require.Equal(t, false, value, name)
					}
				}
			}
			if !obs.response {
				requestCount[obs.key]++
				if obs.command == 0x72 {
					negRequests++
				}
				if prior != nil && !prior.response && prior.command == 0x73 && prior.root != 0 {
					retired[prior.root] = true
				}
				var expected *smb1Fuzz102Challenge
				if obs.command == 0x73 && prior == nil && obs.key.mid != 0xffff && obs.uid != 0 {
					if c := challenges[obs.uid]; c != nil && !c.used && !retired[c.root] && c.pid == obs.key.pid && c.requestDir == dir && uidOwner[obs.uid] == c.root {
						expected = c
					}
				}
				if obs.command == 0x73 && !bad {
					if obs.uid == 0 {
						obs.root, obs.setupEligible = obs.id, true
					} else if expected != nil {
						obs.root, obs.setupEligible = expected.root, true
					}
					if link, present := fields["Continuation Response Event ID"]; present {
						require.NotNil(t, expected, "no source-observed eligible challenge")
						require.Equal(t, expected.responseID, link)
						challenge := byID[expected.responseID]
						require.NotNil(t, challenge)
						require.True(t, challenge.response)
						require.Equal(t, byte(0x73), challenge.command)
						require.Equal(t, uint32(0xc0000016), binary.LittleEndian.Uint32(challenge.wire[9:13]))
						require.Equal(t, obs.uid, challenge.uid)
						require.Equal(t, obs.key.pid, challenge.key.pid)
						require.NotEqual(t, dir, challenge.dir)
					}
				}
				if expected != nil {
					expected.used = true
					if bad {
						retired[expected.root] = true
					}
				}
				require.Zero(t, e.ResponseTo)
			} else {
				if prior == nil && obs.command == 0x72 && negRequests == 0 && obs.key.mid != 0xffff {
					negOrphan = true
				}
				eligible := prior != nil && !prior.response && requestCount[obs.key] == 1 && !completed[prior.id] && prior.command == obs.command && obs.key.mid != 0xffff && !prior.bad
				if eligible {
					switch obs.command {
					case 0x72:
						eligible = negRequests == 1 && !negOrphan
					case 0x73:
						eligible = prior.setupEligible && !retired[prior.root] && obs.uid != 0 && (prior.uid == 0 || prior.uid == obs.uid)
					case 0x75:
						eligible = prior.uid == obs.uid
					case 0x06:
						eligible = prior.uid == obs.uid && prior.tid == obs.tid
					}
				}
				// A duplicate completed SETUP response, including a bad body,
				// may retire only its own source-observed allocated UID lineage.
				if prior != nil && !prior.response && prior.command == 0x73 && obs.command == 0x73 && completed[prior.id] && completedUID[prior.id] == obs.uid && uidOwner[obs.uid] == prior.root {
					retired[prior.root] = true
				}
				if bad && eligible && obs.command == 0x73 {
					retired[prior.root] = true
				}
				if e.ResponseTo != 0 {
					require.False(t, bad)
					require.True(t, eligible, "response borrowed absent/reused/retired/wrong-command identity")
					request := byID[e.ResponseTo]
					require.NotNil(t, request)
					require.Equal(t, prior.id, request.id)
					require.False(t, request.response)
					require.Equal(t, obs.key, request.key)
					require.NotEqual(t, obs.dir, request.dir)
					require.Equal(t, request.command, obs.command)
					require.False(t, linked[request.id], "one request linked twice")
					linked[request.id] = true
					for _, view := range []map[string]any{fields, e.Session} {
						require.Equal(t, request.id, view["Request Event ID"])
						require.Equal(t, smb1Fuzz102Hash(request.wire), view["Request PDU SHA256"])
						if ranges, ok := view["Wire Field Ranges"].(map[string]any); ok {
							require.NotContains(t, ranges, "Request Event ID")
							require.NotContains(t, ranges, "Request PDU SHA256")
						}
					}
				}
				// Source completion is recorded even if the product omits a link:
				// a later duplicate cannot become the first completion retroactively.
				if eligible && !bad {
					completed[prior.id] = true
					completedUID[prior.id] = obs.uid
					if obs.command == 0x73 {
						root := prior.root
						if owner, seen := uidOwner[obs.uid]; seen && owner != root {
							retired[owner], retired[root] = true, true
							uidOwner[obs.uid] = 0
							delete(challenges, obs.uid)
						} else {
							uidOwner[obs.uid] = root
							if binary.LittleEndian.Uint32(original[9:13]) == 0xc0000016 {
								challenges[obs.uid] = &smb1Fuzz102Challenge{responseID: obs.id, root: root, pid: obs.key.pid, uid: obs.uid, requestDir: obs.key.requestDir}
							} else {
								delete(challenges, obs.uid)
							}
						}
					}
				}
			}
			if !bad && e.ResponseTo == 0 {
				require.NotContains(t, fields, "Request Event ID")
				require.NotContains(t, fields, "Request PDU SHA256")
			}
			if prior == nil {
				first[obs.key] = obs
			}
			byID[obs.id] = obs
			if bad {
				smb1Fuzz102RejectedAuthority(t, e)
			} else if !obs.response {
				// Mutating returned public projections must not alter a future
				// association; the oracle above retains raw source observations.
				poison := map[string]any{"Header": map[string]any{"Multiplex ID": uint16(999), "User ID": uint16(999)}}
				e.Session, e.Fields, e.Structured = poison, poison, map[string]any{"fields": poison}
				e.Protocol, e.Profile = "tcp", "poisoned"
			}
		}
		s.Close("FIN")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
