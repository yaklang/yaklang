package pcaputil

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Independent TCP framing cannot depend on a lucky packet split. Use the
// existing versioned message fixtures, not a preselected decoder or port guess.
func TestTop30StreamSegmentationAndStickyAdmission(t *testing.T) {
	var cases []m1Sample
	for _, c := range m1SessionSamples(t) {
		if c.name == "ssh-kex-newkeys" { // Valid companion; the sealed original remains a one-byte Curve25519 negative.
			c.name = "ssh-valid-curve-newkeys"
			c.steps = append([]sessionStep(nil), c.steps...)
			c.steps[4].wire = sshValidCurveInit()
		}
		if c.name == "snmpv3-get-response" {
			// The historical mixed sample deliberately changes contextEngineID
			// on its replies. The strengthened correlator correctly leaves those
			// requests unmatched. This positive segmentation companion uses one
			// context consistently; the original negative remains unchanged.
			c.steps = append([]sessionStep(nil), c.steps...)
			engine := []byte{0x80, 0, 0, 0, 1}
			for _, i := range []int{2, 4, 7} {
				wire := c.steps[i].wire
				_, msg, _, err := snmpReadTLV(wire, 0)
				require.NoError(t, err)
				_, _, at, err := snmpReadTLV(msg, 0)
				require.NoError(t, err)
				_, _, at, err = snmpReadTLV(msg, at)
				require.NoError(t, err)
				_, _, at, err = snmpReadTLV(msg, at)
				require.NoError(t, err)
				_, scoped, _, err := snmpReadTLV(msg, at)
				require.NoError(t, err)
				_, _, at, err = snmpReadTLV(scoped, 0)
				require.NoError(t, err)
				_, _, at, err = snmpReadTLV(scoped, at)
				require.NoError(t, err)
				id := int64(2)
				if i == 4 {
					id = 3
				}
				if i == 7 {
					id = 4
				}
				c.steps[i].wire = snmpV3(id, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped(engine, nil, scoped[at:]))
			}
		}
		switch c.protocol {
		case "postgresql", "ldap", "redis", "smb2", "ssh", "mqtt", "smtp", "imap", "pop3", "ftp", "rdp", "snmp":
			cases = append(cases, c)
		}
	}
	cases = append(cases, m1Sample{name: "http2", protocol: "http2", steps: h2TestFixtures(t)}, m1Sample{name: "mysql", protocol: "mysql", steps: mysqlTestFixtures(false)})
	cases = append(cases, m1Sample{name: "http-body-foreign-signature", protocol: "http", steps: []sessionStep{
		{0, []byte("POST /keep HTTP/1.1\r\nHost: example.test\r\nContent-Length: 22\r\n\r\nSSH-2.0-foreign-body\r\n")},
		{1, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")},
		{0, []byte("GET /next HTTP/1.1\r\nHost: example.test\r\n\r\n")},
		{1, []byte("HTTP/1.1 204 No Content\r\n\r\n")},
	}})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var baseline []string
			for _, chunk := range []int{0, 1, 2, 3, 7, 31} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(41000, 41001), WithSessionClientDirection(0))
					require.NoError(t, err)
					var events []*ProtocolEvent
					var admittedProbes uint64
					for i, step := range tc.steps {
						for at := 0; at < len(step.wire); {
							n := len(step.wire) - at
							if chunk > 0 {
								n = min(n, chunk)
							}
							r := s.Feed(step.dir, time.Unix(1, 0), step.wire[at:at+n])
							events = append(events, r.Events...)
							at += n
						}
						if i == 0 {
							admittedProbes = s.Stats().ProbeCalls
						}
					}
					events = append(events, s.Close("EOF")...)
					require.Zero(t, s.Stats().BufferedBytes)
					require.LessOrEqual(t, s.Stats().ProbeCalls, admittedProbes+2, "recognized connections do not rescan every later message")
					var signatures []string
					valid := 0
					for _, e := range events {
						require.NotContains(t, []string{"malformed", "unrecognized", "incomplete", "limited"}, e.Status, "positive fixture must not hide a failed decoder: %s %s", e.Error, e.Summary)
						signatures = append(signatures, fmt.Sprintf("%s/%s/%x/%v/%v", e.Protocol, e.Status, sha256.Sum256(e.Raw), e.Session["Packet Name"], e.Session["Message Name"]))
						if e.Status == "decoded" || e.Status == "deferred" {
							valid++
							require.Equal(t, tc.protocol, e.Protocol)
						}
					}
					require.Positive(t, valid, "fixture must establish the decoder, not just compare two rejection results")
					if chunk == 0 {
						baseline = signatures
					} else {
						require.Equal(t, baseline, signatures, "semantic PDU boundaries are invariant under TCP segmentation")
					}
				})
			}
		})
	}
}
