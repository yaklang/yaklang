package bin_parser

import (
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This ledger binds the current six field-audit families to the retained
// manifest, including aliases, both generations, and negative captures.
// It does not replace the P0 delivery gate or claim full protocol support.
type currentFieldCapture struct {
	id, file, sha, evidence, roadmap string
	packets                          int
}
type currentFieldCheck struct {
	name string
	run  func(*testing.T)
}
type currentFieldAudit struct {
	name                     string
	roadmapNames, dissectors []string
	captures                 []currentFieldCapture
	checks                   []currentFieldCheck
}

var currentFieldAudits = []currentFieldAudit{
	{name: "LDAP", roadmapNames: []string{"LDAP", "CLDAP"}, dissectors: []string{"ldap", "cldap"}, captures: []currentFieldCapture{
		{"gen-ldap", "captures/generated-local/gen-ldap.pcap", "c17f0738743e3087d78a6c34fb89e231805046afa8ff7f47775ce287ee4e02da", "generated-positive", "LDAP", 4},
		{"gen-cldap", "captures/generated-local/gen-cldap.pcap", "d117870d10dd1769e57bdcad54d9aecf003aebd01bcdb6118e1d1e58f21dbacb", "generated-negative", "CLDAP", 1},
		{"gen-cldap-valid", "captures/generated-validated/gen-cldap-valid.pcap", "dd6e7f6f8ec0cf3c46311813def511d8950284e2b8ba88167bd4d10deecb1417", "generated-positive", "CLDAP", 1},
		{"pr5023-gen-ldap", "captures/generated-pr5023/pr5023-gen-ldap.pcap", "6475faf95acacc7beb2fb2fe7d6a0885fbd06119c807ccbf012abd0cdf0db8d1", "generated-positive", "LDAP", 4},
		{"pr5023-gen-cldap", "captures/generated-pr5023/pr5023-gen-cldap.pcap", "b209c594a317f3ff829d188b94d172009390ecc9491d71e4eef7969308feeea1", "generated-negative", "CLDAP", 1},
	}, checks: []currentFieldCheck{
		{"BindAndCLDAPOriginalRecords", TestProtocolCorpusLDAPFieldsOriginalRecords},
		{"CLDAPSearchEveryField", TestProtocolCorpusCLDAPSearchEveryField},
	}},
	{name: "MySQL", roadmapNames: []string{"MySQL", "MariaDB"}, dissectors: []string{"mysql"}, captures: []currentFieldCapture{
		{"ndpi-mysql", "captures/ndpi/ndpi-mysql.pcapng", "11e0988e75b471e25d4c1e3948b9357b77882e5710a492976f2c54d8cf2d98b7", "upstream-positive", "MySQL", 41},
		{"gen-mariadb", "captures/generated-local/gen-mariadb.pcap", "1d707377dffcda7fe2d1a5884f2b382780569f065a0727190e0865305ee7d1eb", "generated-positive", "MariaDB", 4},
		{"pr5023-gen-mariadb", "captures/generated-pr5023/pr5023-gen-mariadb.pcap", "1d1dacf36e35116d762ea93300f447b83f2d6f586f5ebfebbe87b683f73ab677", "generated-positive", "MariaDB", 4},
	}, checks: []currentFieldCheck{
		{"AllOriginalRecords", TestProtocolCorpusMySQLFieldsAllOriginalRecords},
	}},
	{name: "PostgreSQL", roadmapNames: []string{"PostgreSQL"}, dissectors: []string{"pgsql"}, captures: []currentFieldCapture{
		{"ndpi-postgresql", "captures/ndpi/ndpi-postgresql.pcap", "1bbd7586c3d43cb43cc19a0361cf3363cb4b3645df54431828f4c94120302163", "upstream-positive", "PostgreSQL", 88},
	}, checks: []currentFieldCheck{
		{"AllOriginalRecords", TestProtocolCorpusPostgreSQLFieldsAllOriginalRecords},
	}},
	{name: "SMB3", roadmapNames: []string{"SMB2", "SMB3"}, dissectors: []string{"smb2"}, captures: []currentFieldCapture{
		{"gen-smb2", "captures/generated-local/gen-smb2.pcap", "aff6cb5eada34206758cc3418478d510b6c287c13df1b3e086100fc4a4ac583f", "generated-positive", "SMB2", 4},
		{"pr5023-gen-smb2", "captures/generated-pr5023/pr5023-gen-smb2.pcap", "67bd5f1e96cccd4f3a0fdded0b749c5dbbff5599cdd1858e51bbaeb645bf41a1", "generated-positive", "SMB2", 4},
		{"pr5023-gen-smb3", "captures/generated-pr5023/pr5023-gen-smb3.pcap", "e11c4d69779ca9e3748284165ac2ec98064f3283b0ee460e9f73fde5f446d025", "generated-negative", "SMB3", 4},
		{"gen-smb3-valid", "captures/generated-validated/gen-smb3-valid.pcap", "937bf9022105992ced1a8192fa9f09975880ff2a6afe6cab0fbe9b28d3fbed1f", "generated-positive", "SMB3", 7},
	}, checks: []currentFieldCheck{
		{"OriginalEveryRecord", TestProtocolCorpusSMB3OriginalEveryRecord},
		{"CompanionEveryRecord", TestProtocolCorpusSMB3CompanionEveryRecord},
	}},
	{name: "Memcached", roadmapNames: []string{"Memcached", "Memcache binary"}, dissectors: []string{"memcache"}, captures: []currentFieldCapture{
		{"ndpi-memcached", "captures/ndpi/ndpi-memcached.cap", "3a74dd7c9f97e5a7ff75d201014accb76b673d5e13579ddf5faa8bf3844d17bd", "upstream-positive", "Memcached", 10},
		{"gen-memcache-bin", "captures/generated-local/gen-memcache-bin.pcap", "8ef5179f84123ec6a6fe435fd11a5b3291b5c6f60c8c441edf7980bead85e514", "generated-positive", "Memcache binary", 4},
		{"pr5023-gen-memcache-bin", "captures/generated-pr5023/pr5023-gen-memcache-bin.pcap", "a08ee0943de03aad2624f017cf876a2091ea07e776c67f90dba70184042479e2", "generated-positive", "Memcache binary", 4},
	}, checks: []currentFieldCheck{
		{"OriginalRecords", TestProtocolCorpusMemcachedFieldsOriginalRecords},
	}},
	{name: "Cassandra", roadmapNames: []string{"Cassandra CQL"}, dissectors: []string{"cql"}, captures: []currentFieldCapture{
		{"ndpi-cassandra", "captures/ndpi/ndpi-cassandra.pcap", "5992c6bbbd1f84fafb84052520e710adec4144fbf5f71b75a8a6d74bad57d155", "upstream-positive", "Cassandra CQL", 20},
	}, checks: []currentFieldCheck{
		{"OriginalRecords", TestProtocolCorpusCassandraFieldsOriginalRecords},
	}},
}

func currentFieldCaptureFromManifest(c protocolCorpusCapture) currentFieldCapture {
	name := ""
	if c.RoadmapName != nil {
		name = *c.RoadmapName
	}
	return currentFieldCapture{c.ID, c.CaptureFile, c.SHA256, c.EvidenceKind, name, c.PacketCount}
}

// Check both directions. An added matching capture cannot be hidden by a new
// name, a negative evidence kind, or an absent roadmap mapping when its
// recorded dissector identifies the family. Unidentified traffic still needs
// the independent corpus classification audit; this is not a packet detector.
func currentFieldInventoryError(audit currentFieldAudit, manifest []protocolCorpusCapture) error {
	if len(audit.captures) == 0 || len(audit.roadmapNames) == 0 || len(audit.dissectors) == 0 || len(audit.checks) == 0 {
		return fmt.Errorf("%s: incomplete field-audit definition", audit.name)
	}
	for _, check := range audit.checks {
		if check.name == "" || check.run == nil {
			return fmt.Errorf("%s: missing executable field check", audit.name)
		}
	}
	want := make(map[string]currentFieldCapture, len(audit.captures))
	files := make(map[string]bool, len(audit.captures))
	for _, pin := range audit.captures {
		digest, err := hex.DecodeString(pin.sha)
		if pin.id == "" || pin.file == "" || pin.roadmap == "" || pin.evidence == "" || pin.packets <= 0 || err != nil || len(digest) != 32 {
			return fmt.Errorf("%s: invalid capture pin %q", audit.name, pin.id)
		}
		if _, duplicate := want[pin.id]; duplicate || files[pin.file] {
			return fmt.Errorf("%s: duplicate capture pin %q", audit.name, pin.id)
		}
		want[pin.id], files[pin.file] = pin, true
	}
	seen := make(map[string]bool, len(want))
	for _, capture := range manifest {
		got := currentFieldCaptureFromManifest(capture)
		pin, pinned := want[capture.ID]
		matches := pinned || slices.Contains(audit.roadmapNames, got.roadmap)
		for _, protocol := range capture.FrameProtocols {
			matches = matches || slices.Contains(audit.dissectors, protocol)
		}
		if !matches {
			continue
		}
		if !pinned {
			return fmt.Errorf("%s: unreviewed capture %q; add an explicit whole-record field audit before updating this ledger", audit.name, capture.ID)
		}
		if seen[capture.ID] {
			return fmt.Errorf("%s: duplicate manifest capture %q", audit.name, capture.ID)
		}
		seen[capture.ID] = true
		if got != pin {
			return fmt.Errorf("%s: capture contract changed for %q: got %+v, want %+v", audit.name, capture.ID, got, pin)
		}
	}
	var missing []string
	for id := range want {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s: missing retained captures: %s", audit.name, strings.Join(missing, ", "))
	}
	return nil
}

func TestProtocolCorpusCurrentFieldEvidence(t *testing.T) {
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, "testdata/protocol-corpus/manifest.json", &manifest)
	totalCaptures, totalRecords := 0, 0
	for _, audit := range currentFieldAudits {
		t.Run(audit.name, func(t *testing.T) {
			require.NoError(t, currentFieldInventoryError(audit, manifest.Captures))
			records := 0
			for _, capture := range audit.captures {
				records += capture.packets
			}
			// Inventory agreement alone is not parsing evidence. Execute the
			// existing original-record field assertions, including malformed
			// originals, opaque protected bytes, and explicit control records.
			for _, check := range audit.checks {
				t.Run(check.name, check.run)
			}
			t.Logf("field audit: %d pinned captures, %d original records; not full protocol/session coverage", len(audit.captures), records)
			totalCaptures += len(audit.captures)
			totalRecords += records
		})
	}
	require.Equal(t, 17, totalCaptures)
	require.Equal(t, 205, totalRecords)
}

func TestCurrentFieldInventoryRejectsDrift(t *testing.T) {
	for _, audit := range currentFieldAudits {
		t.Run(audit.name, func(t *testing.T) {
			baseline := func() []protocolCorpusCapture {
				var captures []protocolCorpusCapture
				for _, pin := range audit.captures {
					name := pin.roadmap
					captures = append(captures, protocolCorpusCapture{ID: pin.id, CaptureFile: pin.file, SHA256: pin.sha,
						EvidenceKind: pin.evidence, RoadmapName: &name, PacketCount: pin.packets})
				}
				return captures
			}
			require.NoError(t, currentFieldInventoryError(audit, baseline()))
			tests := []struct {
				name string
				edit func([]protocolCorpusCapture) []protocolCorpusCapture
			}{
				{"added-named-capture", func(c []protocolCorpusCapture) []protocolCorpusCapture {
					n := c[0]
					n.ID = "new-generation"
					n.CaptureFile = "captures/new.pcap"
					return append(c, n)
				}},
				{"added-unmapped-dissector", func(c []protocolCorpusCapture) []protocolCorpusCapture {
					return append(c, protocolCorpusCapture{ID: "unmapped", FrameProtocols: []string{audit.dissectors[0]}})
				}},
				{"added-negative-capture", func(c []protocolCorpusCapture) []protocolCorpusCapture {
					n := c[0]
					n.ID = "negative-generation"
					n.EvidenceKind = "generated-negative"
					return append(c, n)
				}},
				{"missing-capture", func(c []protocolCorpusCapture) []protocolCorpusCapture { return c[1:] }},
				{"renamed-capture", func(c []protocolCorpusCapture) []protocolCorpusCapture { c[0].ID += "-renamed"; return c }},
				{"duplicate-capture", func(c []protocolCorpusCapture) []protocolCorpusCapture { return append(c, c[0]) }},
				{"changed-path", func(c []protocolCorpusCapture) []protocolCorpusCapture { c[0].CaptureFile += ".new"; return c }},
				{"changed-bytes", func(c []protocolCorpusCapture) []protocolCorpusCapture {
					c[0].SHA256 = strings.Repeat("0", 64)
					return c
				}},
				{"changed-record-count", func(c []protocolCorpusCapture) []protocolCorpusCapture { c[0].PacketCount++; return c }},
				{"changed-disposition", func(c []protocolCorpusCapture) []protocolCorpusCapture {
					c[0].EvidenceKind = "structural-only"
					return c
				}},
				{"lost-mapping", func(c []protocolCorpusCapture) []protocolCorpusCapture { c[0].RoadmapName = nil; return c }},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) { require.Error(t, currentFieldInventoryError(audit, tt.edit(baseline()))) })
			}
			unrelated := append(baseline(), protocolCorpusCapture{ID: "unrelated-family", FrameProtocols: []string{"other"}})
			require.NoError(t, currentFieldInventoryError(audit, unrelated))
			missingCheck := audit
			missingCheck.checks = nil
			require.Error(t, currentFieldInventoryError(missingCheck, baseline()))
			incomplete := audit
			incomplete.captures = slices.Clone(audit.captures[1:])
			require.Error(t, currentFieldInventoryError(incomplete, baseline()))
			duplicate := audit
			duplicate.captures = append(slices.Clone(audit.captures), audit.captures[0])
			require.Error(t, currentFieldInventoryError(duplicate, baseline()))
		})
	}
}
