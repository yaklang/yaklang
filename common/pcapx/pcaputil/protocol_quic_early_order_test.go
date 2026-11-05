package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// RFC9001 4.9.3/5.6 and RFC9002 4.2: early and application packets share
// monotonically increasing numbers. Capture arrival order is independent.
func earlyOrderContext(t *testing.T, version uint32, algorithm string) (*binQUIC, *quicTrafficKeys, *quicTrafficKeys) {
	t.Helper()
	q, app := quicConformanceContext(t, version, algorithm)
	early, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{9}, len(app.secret)), algorithm, version)
	require.NoError(t, err)
	q.keys.early[0], q.keys.early[1] = early, nil
	q.cids = map[string]int{"seedcid8": 2}
	return q, early, app
}

func TestQUICEarlyAndApplicationPacketNumberOrder(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, c := range []struct {
				name                string
				firstKind, nextKind string
				firstPN, nextPN     uint64
				valid               bool
			}{
				{"delayed-early", "1-RTT", "0-RTT", 20, 19, true},
				{"equal-after-app", "1-RTT", "0-RTT", 20, 20, false},
				{"higher-early", "1-RTT", "0-RTT", 20, 21, false},
				{"lower-app", "0-RTT", "1-RTT", 20, 19, false},
				{"equal-after-early", "0-RTT", "1-RTT", 20, 20, false},
				{"normal-transition", "0-RTT", "1-RTT", 20, 21, true},
				{"first-app-zero", "1-RTT", "0-RTT", 0, 1, false},
				{"early-zero", "0-RTT", "1-RTT", 0, 1, true},
			} {
				t.Run(fmt.Sprintf("v%x/%s/%s", version, algorithm, c.name), func(t *testing.T) {
					q, early, app := earlyOrderContext(t, version, algorithm)
					key := app
					if c.firstKind == "0-RTT" {
						key = early
					}
					_, err := q.consume(0, quicConformancePacket(t, key, c.firstKind, c.firstPN, 0, []byte{1}), 128)
					require.NoError(t, err)
					key = app
					if c.nextKind == "0-RTT" {
						key = early
					}
					wire := quicConformancePacket(t, key, c.nextKind, c.nextPN, 0, []byte{1})
					order := q.earlyOrder
					assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
					require.Equal(t, order, q.earlyOrder, "ownership checks cannot establish an encryption-level boundary")
					keys := cloneQUICKeyring(q.keys)
					largest := q.spaces[0][quicSpaceApplication].largest
					info, err := q.consume(0, wire, 128)
					require.Equal(t, true, info["Authentication Verified"])
					if c.valid {
						require.NoError(t, err)
						return
					}
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Contains(t, err.Error(), "0-RTT")
					require.NotContains(t, info, "Frames")
					require.Equal(t, keys, q.keys)
					require.Equal(t, order, q.earlyOrder)
					require.Equal(t, largest, q.spaces[0][quicSpaceApplication].largest, "contradiction cannot change packet-number reconstruction")
				})
			}
		}
	}
}

func TestQUICEarlyOrderRefusalsAndLateTightening(t *testing.T) {
	for _, appFirst := range []bool{false, true} {
		for _, fault := range []string{"bad-tag", "reserved", "invalid-frame", "frame-budget"} {
			t.Run(fmt.Sprintf("app%v/%s", appFirst, fault), func(t *testing.T) {
				q, early, app := earlyOrderContext(t, quicVersion1, quicAEADAES128GCM)
				key, kind := early, "0-RTT"
				if appFirst {
					key, kind = app, "1-RTT"
				}
				reserved, plain, limit := byte(0), []byte{1}, 128
				if fault == "reserved" {
					reserved = 8
				}
				if fault == "invalid-frame" {
					plain = []byte{0xff}
				}
				if fault == "frame-budget" {
					plain, limit = []byte{1, 1}, 1
				}
				wire := quicConformancePacket(t, key, kind, 100, reserved, plain)
				if fault == "bad-tag" {
					wire[len(wire)-1] ^= 1
				}
				_, err := q.consume(0, wire, limit)
				require.Error(t, err)
				if fault == "frame-budget" {
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrResourceExceeded, typed.Kind)
				}
				require.Equal(t, quicEarlyOrder{}, q.earlyOrder)
				key, kind, pn := app, "1-RTT", uint64(99)
				if appFirst {
					key, kind, pn = early, "0-RTT", 101
				}
				_, err = q.consume(0, quicConformancePacket(t, key, kind, pn, 0, []byte{1}), 128)
				require.NoError(t, err, "a refused earlier packet is not an admitted boundary witness")
			})
		}
	}
	q, early, app := earlyOrderContext(t, quicVersion1, quicAEADAES128GCM)
	info, err := q.consume(0, quicConformancePacket(t, app, "1-RTT", 100, 0, []byte{1}), 128)
	require.NoError(t, err)
	info["Packet Number"], info["Packet Name"] = uint64(0), "0-RTT"
	_, err = q.consume(0, quicConformancePacket(t, app, "1-RTT", 90, 0, []byte{1}), 128)
	require.NoError(t, err)
	_, err = q.consume(0, quicConformancePacket(t, early, "0-RTT", 95, 0, []byte{1}), 128)
	require.ErrorContains(t, err, "0-RTT")
	_, err = q.consume(0, quicConformancePacket(t, early, "0-RTT", 85, 0, []byte{1}), 128)
	require.NoError(t, err)
	before := q.earlyOrder
	_, err = q.consume(0, quicConformancePacket(t, app, "1-RTT", 80, 0, []byte{1}), 128)
	require.ErrorContains(t, err, "0-RTT")
	require.Equal(t, before, q.earlyOrder)
}

func FuzzQUICEarlyAndApplicationOrder(f *testing.F) {
	for _, s := range []struct {
		app         bool
		first, next uint32
		profile     byte
	}{{true, 20, 21, 0}, {true, 20, 19, 1}, {false, 20, 21, 2}, {false, 20, 19, 3}, {true, 0, 0, 4}, {false, 0, 1, 5}} {
		f.Add(s.app, s.first, s.next, s.profile)
	}
	f.Fuzz(func(t *testing.T, appFirst bool, first, next uint32, profile byte) {
		version := quicVersion1
		if profile&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(profile>>1)%3]
		q, early, app := earlyOrderContext(t, version, algorithm)
		a, b := uint64(first&((1<<30)-1)), uint64(next&((1<<30)-1))
		key, kind := early, "0-RTT"
		if appFirst {
			key, kind = app, "1-RTT"
		}
		info, err := q.consume(0, quicConformancePacket(t, key, kind, a, 0, []byte{1}), 128)
		require.NoError(t, err)
		clear(info) // Display metadata is not an order witness.
		key, kind = app, "1-RTT"
		if appFirst {
			key, kind = early, "0-RTT"
		}
		wire := quicConformancePacket(t, key, kind, b, 0, []byte{1})
		before := q.earlyOrder
		assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
		require.Equal(t, before, q.earlyOrder)
		info, err = q.consume(0, wire, 128)
		require.Equal(t, true, info["Authentication Verified"])
		invalid := appFirst && b >= a || !appFirst && b <= a
		if !invalid {
			require.NoError(t, err)
			return
		}
		require.ErrorContains(t, err, "0-RTT")
		require.NotContains(t, info, "Frames")
		require.Equal(t, before, q.earlyOrder)
		require.Equal(t, a, q.spaces[0][quicSpaceApplication].largest)
	})
}

func TestQUICEarlyOrderSurvivesKeyUpdatesAndDirection(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			t.Run(fmt.Sprintf("v%x/%s", version, algorithm), func(t *testing.T) {
				q, early, app := earlyOrderContext(t, version, algorithm)
				_, err := q.consume(1, quicConformancePacket(t, app, "1-RTT", 1, 0, []byte{1}), 128)
				require.NoError(t, err)
				_, err = q.consume(0, quicConformancePacket(t, early, "0-RTT", 50, 0, []byte{1}), 128)
				require.NoError(t, err, "the server's independent PN space does not close client early data")
				_, err = q.consume(0, quicConformancePacket(t, app, "1-RTT", 100, 0, []byte{1}), 128)
				require.NoError(t, err)
				epochs := missedQUICEpochKeys(t, app, 2)
				_, err = q.consume(0, epochQUICPacket(t, epochs[2], nil, 200, 0, 2), 128)
				require.NoError(t, err)
				info, err := q.consume(0, quicConformancePacket(t, early, "0-RTT", 150, 0, []byte{1}), 128)
				require.ErrorContains(t, err, "0-RTT", "a new key epoch cannot move the lifetime first-application boundary")
				require.Equal(t, true, info["Authentication Verified"])
				require.NotContains(t, info, "Frames")
				_, err = q.consume(0, quicConformancePacket(t, early, "0-RTT", 70, 0, []byte{1}), 128)
				require.NoError(t, err, "old early traffic reordered below the first application PN remains observable")
			})
		}
	}
}
