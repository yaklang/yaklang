package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMB1FileCommandsAssociationIsSourceBound(t *testing.T) {
	for _, cmd := range []byte{0x75, 0x06} {
		for _, kind := range []string{"ordinary", "orphan-first", "duplicate-request", "duplicate-response", "completed-key-reuse", "foreign-UID", "foreign-PID-high", "foreign-direction", "foreign-TID", "MID-zero", "MID-reserved"} {
			if kind == "foreign-TID" && cmd != 0x06 {
				continue
			}
			t.Run(fmt.Sprintf("command%02x/%s", cmd, kind), func(t *testing.T) {
				s := smb1SetupSession(t)
				mid := uint16(4)
				if kind == "MID-zero" {
					mid = 0
				}
				if kind == "MID-reserved" {
					mid = 0xffff
				}
				tid := uint16(2048)
				if cmd == 0x75 {
					tid = 0
				}
				request, response := smb1FileWire(cmd, false, mid, 2048, tid), smb1FileWire(cmd, true, mid, 2048, 2048)
				if kind == "orphan-first" {
					require.Zero(t, smb1FeedComplete(t, s, 1, response).ResponseTo)
				}
				r := smb1FeedComplete(t, s, 0, request)
				if kind == "duplicate-request" {
					smb1FeedComplete(t, s, 0, request)
				}
				if kind == "completed-key-reuse" {
					require.Equal(t, r.ID, smb1FeedComplete(t, s, 1, response).ResponseTo)
					smb1FeedComplete(t, s, 0, request)
				}
				if kind == "foreign-UID" || kind == "foreign-PID-high" || kind == "foreign-direction" || kind == "foreign-TID" {
					bad := bytes.Clone(response)
					dir := 1
					switch kind {
					case "foreign-UID":
						bad[32] ^= 1
					case "foreign-PID-high":
						bad[16] ^= 1
					case "foreign-TID":
						bad[28] ^= 1
					case "foreign-direction":
						dir = 0
					}
					e := smb1FeedComplete(t, s, dir, bad)
					require.Zero(t, e.ResponseTo)
					require.NotContains(t, e.Session, "Request Event ID")
				}
				e := smb1FeedComplete(t, s, 1, response)
				unresolved := kind == "orphan-first" || kind == "duplicate-request" || kind == "completed-key-reuse" || kind == "MID-reserved"
				if unresolved {
					require.Zero(t, e.ResponseTo)
					require.NotContains(t, e.Session, "Request Event ID")
				} else {
					require.Equal(t, r.ID, e.ResponseTo)
					require.Equal(t, r.ID, e.Session["Request Event ID"])
					require.Equal(t, r.SourceBytes.SHA256, e.Session["Request PDU SHA256"])
					key := "Request File Name"
					value := "\\dir\\文件.txt"
					if cmd == 0x75 {
						key, value = "Request Path", "\\\\server\\共享"
					}
					require.Equal(t, value, e.Session[key])
					require.NotContains(t, e.Session["Wire Field Ranges"], key)
					require.NotContains(t, e.Session["Body"], key)
				}
				if kind == "duplicate-response" {
					require.Zero(t, smb1FeedComplete(t, s, 1, response).ResponseTo)
				}
			})
		}
	}
}

func smb1ChangeIdentity(w []byte, pid uint32, mid, uid, tid uint16) []byte {
	w = bytes.Clone(w)
	binary.LittleEndian.PutUint16(w[16:18], uint16(pid>>16))
	binary.LittleEndian.PutUint16(w[30:32], uint16(pid))
	binary.LittleEndian.PutUint16(w[34:36], mid)
	binary.LittleEndian.PutUint16(w[32:34], uid)
	binary.LittleEndian.PutUint16(w[28:30], tid)
	return w
}

func TestSMB1FileCommandsDoNotSeparateCollidingCommandNamespaces(t *testing.T) {
	for _, first := range []byte{0x73, 0x75, 0x06, 0x72} {
		for _, second := range []byte{0x73, 0x75, 0x06, 0x72} {
			if first == second {
				continue
			}
			t.Run(fmt.Sprintf("%02x-to-%02x", first, second), func(t *testing.T) {
				s := smb1SetupSession(t)
				wire := func(command byte, response bool) []byte {
					var w []byte
					switch command {
					case 0x73:
						w = smb1SetupWire(response, 4, 0, 0)
					case 0x72:
						if response {
							w = smb1ExtendedResponse(0)
						} else {
							w = smb1ExtendedOffer()
						}
					default:
						w = smb1FileWire(command, response, 4, 2048, 2048)
					}
					uid := uint16(2048)
					if !response && (command == 0x73 || command == 0x72) {
						uid = 0
					}
					return smb1ChangeIdentity(w, 0x12345678, 4, uid, 2048)
				}
				smb1FeedComplete(t, s, 0, wire(first, false))
				collision := smb1FeedComplete(t, s, 0, wire(second, false))
				require.Equal(t, "decoded", collision.Status, "finite ambiguity is not a malformed wire verdict")
				for _, command := range []byte{first, second} {
					e := smb1FeedComplete(t, s, 1, wire(command, true))
					require.Zero(t, e.ResponseTo)
					require.NotContains(t, e.Session, "Request Event ID")
					require.NotContains(t, e.Session, "Selected Dialect Name")
				}
			})
		}
	}
}

func TestSMB1FileCommandsDisallowedNegotiateStillRetiresGlobalOffer(t *testing.T) {
	for _, prior := range []bool{false, true} {
		t.Run(fmt.Sprint(prior), func(t *testing.T) {
			s := smb1SetupSession(t)
			nego := func(response bool, mid uint16) []byte {
				w := smb1ExtendedOffer()
				if response {
					w = smb1ExtendedResponse(0)
				}
				return smb1ChangeIdentity(w, 0x12345678, mid, 0, 0)
			}
			if prior {
				smb1FeedComplete(t, s, 0, nego(false, 1))
			}
			smb1FeedComplete(t, s, 0, smb1FileWire(0x75, false, 2, 2048, 0))
			smb1FeedComplete(t, s, 0, nego(false, 2))
			smb1FeedComplete(t, s, 0, nego(false, 3))
			for _, mid := range []uint16{1, 2, 3} {
				e := smb1FeedComplete(t, s, 1, nego(true, mid))
				require.Zero(t, e.ResponseTo)
				require.NotContains(t, e.Session, "Selected Dialect Name")
			}
		})
	}
}

func TestSMB1FileCommandsWrongRefusedResponseKeepsExistingOwner(t *testing.T) {
	s := smb1SetupSession(t)
	request := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
	bad := smb1FileWire(0x75, true, 2, 2048, 2048)
	// A complete fixed window with a malformed Unicode NativeFS body.
	binary.LittleEndian.PutUint16(bad[len(bad)-4:len(bad)-2], 0xd800)
	r := s.Feed(1, time.Unix(101, 0), bad)
	require.NotNil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, "headers", e.Completeness)
	require.Zero(t, e.ResponseTo)
	require.NotContains(t, e.Session, "Body")
	require.NotContains(t, e.Session["Context Eligibility"], "retired")
	good := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	require.Equal(t, request.ID, good.ResponseTo)
}

func TestSMB1FileCommandsRefusedDuplicateSetupCannotKeepContinuation(t *testing.T) {
	s := smb1SetupSession(t)
	smb1FeedComplete(t, s, 0, smb1SetupWire(false, 2, 0, 0))
	challenge := smb1FeedComplete(t, s, 1, smb1SetupWire(true, 2, 2048, 0xc0000016))
	require.NotZero(t, challenge.ResponseTo)
	bad := smb1SetupWire(true, 2, 2048, 0xc0000016)
	binary.LittleEndian.PutUint16(bad[50:52], 0xd800)
	r := s.Feed(1, time.Unix(102, 0), bad)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	require.Len(t, r.Events, 1)
	require.Zero(t, r.Events[0].ResponseTo)
	next := smb1FeedComplete(t, s, 0, smb1SetupWire(false, 3, 2048, 0))
	require.NotContains(t, next.Session, "Continuation Response Event ID", "a fully bounded refused replay from the actual UID owner must not keep the older setup continuation eligible")
}

func TestSMB1FileCommandsCollisionRetiresOnlyTheStoredSetupOwner(t *testing.T) {
	s := smb1SetupSession(t)
	setup := func(response bool, pid uint32, mid, uid uint16, status uint32) []byte {
		return smb1ChangeIdentity(smb1SetupWire(response, mid, uid, status), pid, mid, uid, 0)
	}
	const a, b uint32 = 0x12345678, 0x92345678
	smb1FeedComplete(t, s, 0, setup(false, a, 2, 0, 0))
	smb1FeedComplete(t, s, 1, setup(true, a, 2, 2048, 0xc0000016))
	smb1FeedComplete(t, s, 0, setup(false, b, 2, 0, 0))
	challengeB := smb1FeedComplete(t, s, 1, setup(true, b, 2, 4096, 0xc0000016))
	smb1FeedComplete(t, s, 0, setup(false, a, 3, 2048, 0))
	// The colliding command claims B's UID, but only the actual stored A owner
	// may be retired. Equal low PID halves must not merge the namespaces.
	attack := smb1ChangeIdentity(smb1FileWire(0x75, false, 3, 4096, 0), a, 3, 4096, 0)
	smb1FeedComplete(t, s, 0, attack)
	contB := smb1FeedComplete(t, s, 0, setup(false, b, 3, 4096, 0))
	require.Equal(t, challengeB.ID, contB.Session["Continuation Response Event ID"])
	require.Zero(t, smb1FeedComplete(t, s, 1, setup(true, a, 3, 2048, 0)).ResponseTo)
	require.Equal(t, contB.ID, smb1FeedComplete(t, s, 1, setup(true, b, 3, 4096, 0)).ResponseTo)
}
