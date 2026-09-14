package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// P0 completion refers to these executable message contracts, not to all
// versions, session semantics, directory operations or encrypted plaintext.
// Run unconditionally: changing a catalog status must not bypass the proof.
func requireP0MessageScopes(t *testing.T) map[string]bool {
	checks := map[string][]currentFieldCheck{
		"LDAP": {
			{"BindOriginal", TestProtocolCorpusLDAPFieldsOriginalRecords},
			{"BindBoundaries", TestProtocolCorpusLDAPFieldsBoundariesAndIsolation},
			{"Operations", TestP0LDAPOperationEntries},
		},
		"MySQL": {
			{"AllOriginalRecords", TestProtocolCorpusMySQLFieldsAllOriginalRecords},
			{"Boundaries", TestProtocolCorpusMySQLFieldsBoundariesAndIsolation},
			{"Fallback", TestProtocolCorpusMySQLFieldsNestedFallback},
			{"WideIntegers", TestProtocolCorpusMySQLFieldsWideIntegers},
		},
		"PostgreSQL": {
			{"AllOriginalRecords", TestProtocolCorpusPostgreSQLFieldsAllOriginalRecords},
			{"Boundaries", TestProtocolCorpusPostgreSQLFieldsBoundariesAndIsolation},
		},
		"SMB3": {
			{"Negotiate", TestProtocolCorpusSMB3Fields},
			{"OriginalRecords", TestProtocolCorpusSMB3OriginalEveryRecord},
			{"CompanionRecords", TestProtocolCorpusSMB3CompanionEveryRecord},
			{"Boundaries", TestProtocolCorpusSMB3PrefixesAndBoundaries},
			{"ReceiveContexts", TestProtocolCorpusSMB3ContextReceiveRules},
			{"Transform", TestP0SMB3TransformEntry},
		},
	}
	covered := map[string]bool{}
	for _, name := range []string{"LDAP", "MySQL", "PostgreSQL", "SMB3"} {
		require.NotEmpty(t, checks[name])
		foundCatalog, foundRoadmap := false, false
		for _, c := range ProtocolCatalog {
			if c.Name == name {
				foundCatalog = true
				require.Equal(t, statusPartial, c.Status, "retain overall support boundary")
				require.NotEmpty(t, c.Notes)
			}
		}
		for _, r := range ProtocolRoadmap {
			if r.Name == name {
				foundRoadmap = true
				require.Equal(t, priP0, r.Priority)
				require.Equal(t, stDone, r.Status)
				require.NotEmpty(t, r.Notes)
			}
		}
		require.True(t, foundCatalog)
		require.True(t, foundRoadmap)
		passed := t.Run(name+"Scope", func(t *testing.T) {
			for _, check := range checks[name] {
				require.NotNil(t, check.run)
				t.Run(check.name, check.run)
			}
		})
		covered[name] = passed
	}
	return covered
}

func TestP0LDAPOperationEntries(t *testing.T) {
	// Independent wire literals, derived from RFC 4511 section 4 ASN.1.
	for _, tc := range []struct {
		entry, wire, field string
		value              any
	}{
		{"LDAPBindResponseFields", "300c02010161070a010004000400", "Result Code", uint64(0)},
		{"LDAPUnbindRequestFields", "30050201014200", "Message ID", uint64(1)},
		{"LDAPSearchRequestFields", "301c020101631704000a01020a01000201000201000101008702636e3000", "Present Attribute", "cn"},
		{"LDAPSearchEntryFields", "301502010164100400300c300a0402636e310404000400", "Attribute Description", "cn"},
		{"LDAPSearchDoneFields", "300c02010165070a010004000400", "Result Code", uint64(0)},
		{"LDAPSearchReferenceFields", "300f020101730a04086c6461703a2f2f78", "URI", "ldap://x"},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			wire := mustHex(t, tc.wire)
			for _, entry := range []string{tc.entry, tc.entry + "Carrier"} {
				n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ldap_fields", entry)
				protocolCorpusRequireValue(t, n, tc.field, tc.value)
				require.NotNil(t, NodeToMap(n))
				require.Equal(t, wire, NodeToBytes(n))
				info := n.Cfg.GetItem("additionInfo")
				if info == nil {
					info = protocolCorpusFindNode(n, tc.entry).Cfg.GetItem("additionInfo")
				}
				require.Equal(t, false, info.(map[string]any)["Session State Validated"])
				for cut := 0; cut < len(wire); cut++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.ldap_fields", tc.entry)
					require.Error(t, err, "prefix %d", cut)
				}
				bad := bytes.Clone(wire)
				bad[0] = 0x31
				fallback := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.ldap_fields", tc.entry+"Carrier")
				protocolCorpusRequireValue(t, fallback, "Unparsed LDAP Wire", bad)
				protocolCorpusRequireValue(t, n, tc.field, tc.value) // retained result
			}
			_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.ldap_fields", tc.entry)
			require.Error(t, err, "new strict entry requires a bound")
		})
	}
}

func TestP0SMB3TransformEntry(t *testing.T) {
	w := make([]byte, 116)
	copy(w, []byte{0xfd, 'S', 'M', 'B'})
	binary.LittleEndian.PutUint32(w[36:], 64)
	w[42] = 1
	binary.LittleEndian.PutUint64(w[44:], 0x1122334455667788)
	for i := 52; i < len(w); i++ {
		w[i] = byte(i)
	}
	for _, entry := range []string{"SMB3TransformFields", "SMB3TransformFieldsCarrier"} {
		n := protocolCorpusRequireBoundedRuleParse(t, w, "application-layer.smb3_transform_fields", entry)
		for name, value := range map[string]any{"ProtocolId": uint64(0x424d53fd), "OriginalMessageSize": uint64(64), "Flags": uint64(1), "SessionId": uint64(0x1122334455667788), "Encrypted Payload": w[52:]} {
			protocolCorpusRequireValue(t, n, name, value)
		}
		require.Equal(t, w, NodeToBytes(n))
		require.NotNil(t, NodeToMap(n))
	}
	for cut := 0; cut < len(w); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), "application-layer.smb3_transform_fields", "SMB3TransformFields")
		require.Error(t, err)
	}
	bad := append(bytes.Clone(w), 0)
	n := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.smb3_transform_fields", "SMB3TransformFieldsCarrier")
	protocolCorpusRequireValue(t, n, "Unparsed SMB3 Transform", bad)
	// Keep the historical header-only entry and its result shape unchanged.
	root, err := base.ParseRule("application-layer/smb3.yaml")
	require.NoError(t, err)
	require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(w[:52])), "SMB3Transform"))
}
