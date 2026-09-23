package pcaputil

import (
	"bytes"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
)

// BenchmarkFirstBatchT20T21Replay records reproducible capture-to-event costs
// for representative media and enterprise captures. The deferred-capture arm
// measures bounded event capture without projecting fields; deferred-on-demand
// explicitly calls GetFields so it measures the same semantics as the full arm.
// It is a mode comparison on the current implementation, not a before/after
// comparison against a parser that did not support these profiles.
func BenchmarkFirstBatchT20T21Replay(b *testing.B) {
	profiles := []struct {
		name       string
		path       string
		corpusPath string
		protocols  []string
		decodeAs   bool
	}{
		{name: "sip-rtp-duplex", path: "testdata/protocol-sessions/first-batch-oracles/sip-sipp-rtp-duplex-loopback.pcap", protocols: []string{"sip", "rtp"}},
		{name: "stun-turn", corpusPath: "ndpi/ndpi-stun.pcap", protocols: []string{"stun"}},
		{name: "snmp-v1-live", path: "testdata/protocol-sessions/first-batch-oracles/net-snmp-v1-live.pcap", protocols: []string{"snmp"}},
		{name: "syslog-snmp-rsyslog-v2c", path: "testdata/protocol-sessions/first-batch-oracles/net-snmp-rsyslog-interop-v2c.pcap", protocols: []string{"snmp", "syslog"}, decodeAs: true},
		{name: "smb2-samba", path: "testdata/protocol-sessions/first-batch-oracles/smb2-samba-live.pcap", protocols: []string{"smb2"}},
		{name: "ldap-openldap-starttls", path: "testdata/protocol-sessions/first-batch-oracles/ldap-openldap-starttls-live.pcap", protocols: []string{"ldap", "tls"}},
		{name: "kerberos-ndpi", corpusPath: "ndpi/ndpi-kerberos-login.pcap", protocols: []string{"kerberos"}},
		{name: "dcerpc-generated-full", path: "testdata/protocol-sessions/first-batch-oracles/dcerpc-generated-full-session.pcap", protocols: []string{"dcerpc"}},
	}
	modes := []struct {
		name     string
		deferred bool
		decode   bool
	}{
		{name: "full"},
		{name: "deferred-capture", deferred: true},
		{name: "deferred-on-demand", deferred: true, decode: true},
	}
	for _, profile := range profiles {
		var wire []byte
		var err error
		if profile.corpusPath != "" {
			wire = binCorpusBytes(b, profile.corpusPath)
		} else {
			wire, err = os.ReadFile(profile.path)
			if err != nil {
				b.Fatal(err)
			}
		}
		for _, mode := range modes {
			for _, workers := range []int{1, 2, 4} {
				name := fmt.Sprintf("%s/%s/workers-%d", profile.name, mode.name, workers)
				b.Run(name, func(b *testing.B) {
					b.SetBytes(int64(len(wire)))
					b.ReportAllocs()
					b.ResetTimer()
					var total int64
					for i := 0; i < b.N; i++ {
						var observed atomic.Int64
						var bad atomic.Bool
						options := []CaptureOption{
							WithTCPReassemblyWorkers(workers),
							WithBinParserConfig(BinParserConfig{
								Deferred: mode.deferred,
								OnEvent: func(event *ProtocolEvent) {
									matches := false
									for _, protocol := range profile.protocols {
										if event.Protocol == protocol {
											matches = true
											break
										}
									}
									if !matches {
										return
									}
									observed.Add(1)
									if mode.decode {
										if _, err := event.GetFields(); err != nil {
											bad.Store(true)
										}
									}
								},
							}),
						}
						if profile.decodeAs {
							options = append(options, WithProtocolDecodeAs("udp", 1514, "syslog"))
						}
						if err := ReplayPcap(bytes.NewReader(wire), options...); err != nil {
							b.Fatal(err)
						}
						count := observed.Load()
						if count == 0 || bad.Load() {
							b.Fatalf("invalid replay: matching-events=%d field-decode-error=%v", count, bad.Load())
						}
						total += count
					}
					b.ReportMetric(float64(total)/float64(b.N), "events/op")
				})
			}
		}
	}
}
