package pcaputil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLDAPSessionSummarySemanticContract(t *testing.T) {
	t.Run("search-request", func(t *testing.T) {
		session := map[string]any{
			"Message Name": "SearchRequest",
			"Message ID":   uint64(7),
		}
		fields := map[string]any{
			"LDAPMessage": map[string]any{
				"Base Object": []byte("dc=example,dc=com"),
				"Scope":       uint64(2),
				"Attributes":  []any{[]byte("cn"), []byte("mail")},
				"Filter":      []byte("SECRET-FILTER-VALUE"),
			},
		}
		require.Equal(t,
			"LDAP SearchRequest id=7 base=dc=example,dc=com scope=subtree attrs=2",
			ldapSessionSummary(session, fields),
		)
		require.NotContains(t, ldapSessionSummary(session, fields), "SECRET-FILTER-VALUE")
	})

	t.Run("completed-search", func(t *testing.T) {
		session := map[string]any{
			"Message Name":        "SearchResultDone",
			"Message ID":          uint64(7),
			"Result Code":         uint64(0),
			"Matched Request":     true,
			"Search Complete":     true,
			"Entries Returned":    uint32(3),
			"References Returned": uint32(1),
		}
		require.Equal(t,
			"LDAP SearchResultDone id=7 result=0 complete entries=3 refs=1 matched",
			ldapSessionSummary(session, nil),
		)
	})

	t.Run("bind-credentials-remain-opaque", func(t *testing.T) {
		session := map[string]any{
			"Message Name": "BindRequest",
			"Message ID":   uint64(9),
		}
		fields := map[string]any{
			"Choice":         "simple",
			"Directory Name": []byte("cn=admin,dc=example,dc=com"),
			"Simple Octets":  []byte("SUMMARY-MUST-NOT-LEAK"),
		}
		summary := ldapSessionSummary(session, fields)
		require.Equal(t,
			"LDAP BindRequest id=9 bind=simple name=cn=admin,dc=example,dc=com credentials=opaque",
			summary,
		)
		require.NotContains(t, summary, "SUMMARY-MUST-NOT-LEAK")
	})

	t.Run("anonymous-bind", func(t *testing.T) {
		summary := ldapSessionSummary(
			map[string]any{"Message Name": "BindRequest", "Message ID": uint64(10)},
			map[string]any{"Choice": "simple", "Directory Name": []byte{}},
		)
		require.Equal(t, "LDAP BindRequest id=10 bind=simple name=<anonymous> credentials=opaque", summary)
	})

	t.Run("bind-result-does-not-prove-identity", func(t *testing.T) {
		session := map[string]any{
			"Message Name":          "BindResponse",
			"Message ID":            uint64(9),
			"Result Code":           uint64(49),
			"Matched Request":       true,
			"Authentication Status": "failure-or-malformed-result-observed",
		}
		require.Equal(t,
			"LDAP BindResponse id=9 result=49 auth=failure-or-malformed-result-observed identity=unverified matched",
			ldapSessionSummary(session, nil),
		)
	})

	t.Run("protected-record", func(t *testing.T) {
		require.Equal(t,
			"LDAP SASLProtectedRecord payload=opaque authentication=unverified",
			ldapSessionSummary(map[string]any{"Message Name": "SASLProtectedRecord"}, nil),
		)
	})

	t.Run("abandon-target", func(t *testing.T) {
		require.Equal(t,
			"LDAP AbandonRequest id=10 abandoned_id=7",
			ldapSessionSummary(map[string]any{
				"Message Name":         "AbandonRequest",
				"Message ID":           uint64(10),
				"Abandoned Message ID": uint64(7),
			}, nil),
		)
	})
}

func TestKerberosSummarySemanticContract(t *testing.T) {
	facts := map[string]any{
		"Message Name":               "AS-REQ",
		"Realm":                      "EXAMPLE.COM",
		"Client Principal":           "alice",
		"Server Principal":           "krbtgt/EXAMPLE.COM",
		"Requested Encryption Types": []int64{18, 23, 17, 16, 20},
		"Observed Encryption Types":  []int64{18},
		"Ticket Count":               1,
	}
	summary := formatKerberosSummary(facts)
	require.Contains(t, summary, "Kerberos AS-REQ")
	require.Contains(t, summary, "realm=EXAMPLE.COM")
	require.Contains(t, summary, "client=alice")
	require.Contains(t, summary, "server=krbtgt/EXAMPLE.COM")
	require.Contains(t, summary, "etypes=18,23,17,16,+1")
	require.Contains(t, summary, "ticket=opaque")
	require.Contains(t, summary, "decrypt=no")
	require.Contains(t, summary, "identity=unverified")

	errorSummary := formatKerberosSummary(map[string]any{
		"Message Name":     "KRB-ERROR",
		"Realm":            "EXAMPLE.COM",
		"Server Principal": "krbtgt/EXAMPLE.COM",
		"Error Code":       int64(52),
	})
	require.Equal(t, "Kerberos KRB-ERROR error=52 realm=EXAMPLE.COM server=krbtgt/EXAMPLE.COM decrypt=no identity=unverified", errorSummary)
}

func TestSMB2SessionSummaryCarriesRoleResultAndTarget(t *testing.T) {
	request := map[string]any{
		"Packet Name": "CREATE",
		"Response":    false,
		"File Name":   "\\srv\\share\\report.txt",
		"Message ID":  uint64(11),
	}
	require.Equal(t,
		"SMB2 CREATE request target=\\srv\\share\\report.txt result=request-pending mid=11",
		smb2SessionSummary(request),
	)

	response := map[string]any{
		"Packet Name":        "READ",
		"Response":           true,
		"Status":             uint32(0),
		"Count":              uint32(4096),
		"Message ID":         uint64(12),
		"Association Status": "matched",
		"Request File Name":  "\\srv\\share\\report.txt",
	}
	require.Equal(t,
		"SMB2 READ response target=\\srv\\share\\report.txt count=4096 status=0x00000000 result=response-status-observed matched mid=12",
		smb2SessionSummary(response),
	)

	pending := map[string]any{
		"Packet Name": "IOCTL",
		"Response":    true,
		"Status":      uint32(0x00000103),
		"Message ID":  uint64(13),
	}
	require.Contains(t, smb2SessionSummary(pending), "result=pending")

	longTarget := smb2SessionSummary(map[string]any{
		"Packet Name": "CREATE",
		"Response":    true,
		"Status":      uint32(0),
		"File Name":   strings.Repeat("x", 512),
		"Message ID":  uint64(14),
	})
	require.LessOrEqual(t, len([]rune(longTarget)), protocolSummaryLimit)
	require.True(t, strings.HasSuffix(longTarget, "result=response-status-observed mid=14"))

	encrypted := smb2SessionSummary(map[string]any{
		"Packet Name":   "transform",
		"Encrypted":     true,
		"Original Size": uint32(1440),
	})
	require.Equal(t, "SMB2 transform ciphertext=opaque decrypt=no len=1440", encrypted)
}

func TestProtocolSemanticSummarySanitizesAndBoundsObservedText(t *testing.T) {
	facts := map[string]any{
		"Message Name":               "AS-REQ",
		"Realm":                      strings.Repeat("R", 512),
		"Client Principal":           "alice\x00\nadmin" + strings.Repeat("x", 512),
		"Server Principal":           strings.Repeat("s", 512),
		"Ticket Count":               1,
		"Requested Encryption Types": []int64{18, 19, 20, 21, 22, 23},
	}
	summary := formatKerberosSummary(facts)
	require.LessOrEqual(t, len([]rune(summary)), protocolSummaryLimit)
	require.NotContains(t, summary, "\x00")
	require.NotContains(t, summary, "\n")
	require.Contains(t, summary, "?")
	require.True(t, strings.HasSuffix(summary, " decrypt=no identity=unverified"))
}

func FuzzProtocolSemanticSummaryText(f *testing.F) {
	f.Add("EXAMPLE.COM", "alice", "krbtgt/EXAMPLE.COM", "dc=example,dc=com", `\srv\share\file.txt`)
	f.Fuzz(func(t *testing.T, realm, principal, server, base, target string) {
		kerberos := formatKerberosSummary(map[string]any{
			"Message Name":               "AS-REQ",
			"Realm":                      realm,
			"Client Principal":           principal,
			"Server Principal":           server,
			"Ticket Server":              server,
			"Ticket Count":               1,
			"Requested Encryption Types": []int64{18, 23},
		})
		ldap := ldapSessionSummary(
			map[string]any{"Message Name": "SearchRequest", "Message ID": uint64(1)},
			map[string]any{"Base Object": []byte(base), "Scope": uint64(2), "Attributes": []any{}},
		)
		smb := smb2SessionSummary(map[string]any{
			"Packet Name": "CREATE",
			"File Name":   target,
			"Message ID":  uint64(1),
		})
		for _, summary := range []string{kerberos, ldap, smb} {
			require.LessOrEqual(t, len([]rune(summary)), protocolSummaryLimit)
			for _, r := range summary {
				require.False(t, r < 0x20 || r == 0x7f, "control character leaked into summary: %q", summary)
			}
		}
	})
}
