package pcaputil

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func cldapTestTLV(tag byte, value []byte) []byte {
	out := []byte{tag}
	switch {
	case len(value) < 0x80:
		out = append(out, byte(len(value)))
	case len(value) <= 0xff:
		out = append(out, 0x81, byte(len(value)))
	default:
		out = append(out, 0x84, byte(len(value)>>24), byte(len(value)>>16), byte(len(value)>>8), byte(len(value)))
	}
	return append(out, value...)
}

func cldapTestSearchRequest(messageID byte, base string, attributes ...string) []byte {
	search := cldapTestTLV(0x04, []byte(base))
	search = append(search, cldapTestTLV(0x0a, []byte{0})...)
	search = append(search, cldapTestTLV(0x0a, []byte{0})...)
	search = append(search, cldapTestTLV(0x02, []byte{0})...)
	search = append(search, cldapTestTLV(0x02, []byte{0})...)
	search = append(search, cldapTestTLV(0x01, []byte{0})...)
	search = append(search, cldapTestTLV(0x87, []byte("objectClass"))...)
	var attributeList []byte
	for _, attribute := range attributes {
		attributeList = append(attributeList, cldapTestTLV(0x04, []byte(attribute))...)
	}
	search = append(search, cldapTestTLV(0x30, attributeList)...)
	message := cldapTestTLV(0x02, []byte{messageID})
	message = append(message, cldapTestTLV(0x63, search)...)
	return cldapTestTLV(0x30, message)
}

func cldapTestSearchDone(messageID byte, resultCode byte) []byte {
	result := cldapTestTLV(0x0a, []byte{resultCode})
	result = append(result, cldapTestTLV(0x04, nil)...)
	result = append(result, cldapTestTLV(0x04, nil)...)
	message := cldapTestTLV(0x02, []byte{messageID})
	message = append(message, cldapTestTLV(0x65, result)...)
	return cldapTestTLV(0x30, message)
}

func TestCLDAPSearchRequestFieldsAndSummary(t *testing.T) {
	wire := cldapTestSearchRequest(7, "DC=example,DC=test", "dnsHostName", "netlogon")
	fields, err := decodeCLDAPDatagram(wire, 4096)
	require.NoError(t, err)
	require.Equal(t, uint64(7), fields["Message ID"])
	require.Equal(t, uint64(0x63), fields["Operation"])
	require.Equal(t, "SearchRequest", fields["Message Name"])
	require.Equal(t, "DC=example,DC=test", fields["Base Object"])
	require.Equal(t, uint64(0), fields["Scope"])
	require.Contains(t, cldapDatagramSummary(fields), "CLDAP SearchRequest id=7")
	require.Contains(t, cldapDatagramSummary(fields), "base=DC=example,DC=test")
	require.Contains(t, cldapDatagramSummary(fields), "scope=0")
	require.Contains(t, cldapDatagramSummary(fields), "attributes=2")
}

func TestCLDAPNativeProfileRegistered(t *testing.T) {
	for _, profile := range NativeProtocolProfiles() {
		if profile.Protocol == "cldap" {
			require.Equal(t, "udp", profile.Transport)
			require.Contains(t, profile.Framing, "UDP/389")
			require.Contains(t, profile.Context, "never borrow TCP LDAP")
			return
		}
	}
	t.Fatal("CLDAP native profile is not registered")
}

func TestCLDAPNativeUDPDispatchAndResponseBoundary(t *testing.T) {
	request := cldapTestSearchRequest(1, "", "Netlogon")
	response := cldapTestSearchDone(1, 0)
	raw := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: request}, {dir: 1, wire: response}}, 389)

	var events []*ProtocolEvent
	stats := ProtocolStats{}
	require.NoError(t, ReplayPcap(bytes.NewReader(raw),
		WithOnProtocolMessage(func(event *ProtocolEvent) {
			if event.Transport == "udp" {
				events = append(events, event)
			}
		}),
		WithOnProtocolStats(func(got ProtocolStats) { stats = got }),
	))
	require.Len(t, events, 2)
	require.Equal(t, "cldap", events[0].Protocol)
	require.Equal(t, "cldap-native", events[0].Profile)
	require.Equal(t, "decoded", events[0].Status, events[0].Error)
	require.Equal(t, "SearchRequest", events[0].Session["Message Name"])
	require.Equal(t, "", events[0].Session["Base Object"])
	require.Equal(t, uint64(0), events[0].Session["Scope"])
	require.Contains(t, events[0].Summary, "CLDAP SearchRequest id=1")
	require.Contains(t, events[0].Summary, "attributes=1")
	display := events[0].DisplayFields()
	require.Equal(t, []any{uint64(1)}, display["cldap.message_id"])
	require.Equal(t, []any{uint64(0x63)}, display["cldap.operation"])
	require.Equal(t, []any{""}, display["cldap.base_object"])
	require.Equal(t, []any{uint64(0)}, display["cldap.scope"])
	require.Equal(t, []any{"objectClass"}, display["cldap.filter_present_attribute"])
	require.Equal(t, []any{"Netlogon"}, display["cldap.attribute"])

	require.Equal(t, "cldap", events[1].Protocol)
	require.Equal(t, "context-required", events[1].Status)
	require.Contains(t, events[1].Error, "SearchResultDone response operation")
	require.Equal(t, uint64(1), events[1].Session["Message ID"])
	require.Equal(t, uint64(0x65), events[1].Session["Operation"])
	require.Equal(t, uint64(0), events[1].Session["Result Code"])
	require.Equal(t, true, events[1].Session["Response Body Opaque"])
	require.Equal(t, uint64(2), stats.Messages)
	require.Equal(t, uint64(1), stats.Decoded)
	require.Equal(t, uint64(1), stats.ContextRequired)
}

func TestCLDAPCombinedResponseMessagesStayBounded(t *testing.T) {
	entry := []byte{0x02, 0x01, 0x09, 0x64, 0x00}
	entry = append([]byte{0x30, byte(len(entry))}, entry...)
	done := cldapTestSearchDone(9, 0)
	combined := append(bytes.Clone(entry), done...)
	fields, err := decodeCLDAPDatagram(combined, 4096)
	require.Error(t, err)
	require.Equal(t, ErrUnsupportedFeature, err.(*ProtocolError).Kind)
	require.Equal(t, 2, fields["Message Count"])
	messages := fields["Messages"].([]map[string]any)
	require.Equal(t, "SearchResultEntry", messages[0]["Message Name"])
	require.Equal(t, "SearchResultDone", messages[1]["Message Name"])
	require.Equal(t, uint64(0), fields["Result Code"])
	require.Contains(t, cldapDatagramSummary(fields), "messages=2")
}

func TestCLDAPExplicitDecodeAsNonstandardPort(t *testing.T) {
	request := cldapTestSearchRequest(3, "DC=explicit", "name")
	raw := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: request}}, 4389)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw),
		WithProtocolDecodeAs("udp", 4389, "cldap"),
		WithOnProtocolMessage(func(event *ProtocolEvent) {
			if event.Transport == "udp" {
				events = append(events, event)
			}
		}),
	))
	require.Len(t, events, 1)
	require.Equal(t, "cldap", events[0].Protocol)
	require.Equal(t, "decoded", events[0].Status, events[0].Error)
	require.Equal(t, "explicit-decode-as", events[0].Admission)
	require.Equal(t, "DC=explicit", events[0].Session["Base Object"])
}

func TestCLDAPMalformedUDP389RemainsCLDAP(t *testing.T) {
	raw := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: []byte{0x30, 0x05, 0x02, 0x01, 0x01, 0x63}}}, 389)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(event *ProtocolEvent) {
		if event.Transport == "udp" {
			events = append(events, event)
		}
	})))
	require.Len(t, events, 1)
	require.Equal(t, "cldap", events[0].Protocol)
	require.Equal(t, "malformed", events[0].Status)
	require.Contains(t, events[0].Error, "cldap")
	require.Equal(t, "UDP", events[0].Session["Transport"])
}

func TestCLDAPPortHintDoesNotOverrideWireEvidence(t *testing.T) {
	raw := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: []byte{0x31, 0x00}}}, 389)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	})))
	require.NotEmpty(t, events)
	for _, event := range events {
		require.NotEqual(t, "cldap", event.Protocol)
	}
}

func TestCLDAPResponseOperationMatrix(t *testing.T) {
	for _, tag := range []byte{0x64, 0x73} {
		message := []byte{0x02, 0x01, 0x04, tag, 0x00}
		wire := append([]byte{0x30, byte(len(message))}, message...)
		fields, err := decodeCLDAPDatagram(wire, 4096)
		require.Error(t, err)
		require.Equal(t, ErrUnsupportedFeature, err.(*ProtocolError).Kind)
		require.Equal(t, uint64(tag), fields["Operation"])
		require.Equal(t, true, fields["Response Body Opaque"])
	}
}

func TestCLDAPDatagramBudgets(t *testing.T) {
	wire := cldapTestSearchRequest(5, "DC=budget", "name")
	_, err := decodeCLDAPDatagram(wire, 1)
	require.Error(t, err)
	require.Equal(t, ErrResourceExceeded, err.(*ProtocolError).Kind)

	budget := DefaultParserBudget()
	_, err = decodeCLDAPDatagramBudget(wire, budget, len(wire)-1)
	require.Error(t, err)
	require.Equal(t, ErrResourceExceeded, err.(*ProtocolError).Kind)
}

func TestCLDAPCanonicalCorpusReplay(t *testing.T) {
	raw, err := trafficfixture.ReadFile(filepath.Join("..", "..", "bin-parser", "testdata", "protocol-corpus", "captures", "generated-validated", "gen-cldap-valid.pcap"))
	require.NoError(t, err)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(event *ProtocolEvent) {
		if event.Protocol == "cldap" {
			events = append(events, event)
		}
	})))
	require.NotEmpty(t, events)
	for _, event := range events {
		require.Equal(t, "decoded", event.Status, event.Error)
		require.Equal(t, "SearchRequest", event.Session["Message Name"])
		require.NotNil(t, event.Session["Base Object"])
		require.NotNil(t, event.Session["Scope"])
		require.NotNil(t, event.Session["Message ID"])
	}
}

func TestCLDAPRealCaptureIfAvailable(t *testing.T) {
	root := os.Getenv("YAKLANG_ENTERPRISE_FIXTURE_ROOT")
	if root == "" {
		t.Skip("set YAKLANG_ENTERPRISE_FIXTURE_ROOT to replay the external enterprise capture")
	}
	path := filepath.Join(root, "host-user-identification", "Wireshark-tutorial-identifying-hosts-and-users-3-of-5.pcap")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("optional enterprise fixture unavailable: %v", err)
	}
	var cldapEvents, cldapDecoded, cldapContext, cldapMalformed, tcpLDAP, unrecognized389 int
	require.NoError(t, ReplayPcapFile(path,
		WithTCPReassemblyWorkers(2),
		WithOnProtocolMessage(func(event *ProtocolEvent) {
			_, sourcePort, sourceErr := net.SplitHostPort(event.Source)
			_, destinationPort, destinationErr := net.SplitHostPort(event.Destination)
			udp389 := event.Transport == "udp" && (sourceErr == nil && sourcePort == "389" || destinationErr == nil && destinationPort == "389")
			if udp389 && event.Protocol == "" && event.Status == "unrecognized" {
				unrecognized389++
			}
			if event.Protocol == "cldap" {
				cldapEvents++
				require.Equal(t, "cldap-native", event.Profile)
				require.Equal(t, "UDP", event.Session["Transport"])
				switch event.Status {
				case "decoded":
					cldapDecoded++
					require.Equal(t, "SearchRequest", event.Session["Message Name"])
					require.NotNil(t, event.Session["Message ID"])
					require.NotNil(t, event.Session["Base Object"])
					require.NotNil(t, event.Session["Scope"])
				case "context-required":
					cldapContext++
					require.Contains(t, event.Error, "outside the SearchRequest profile")
				case "malformed":
					cldapMalformed++
					require.Contains(t, event.Error, "cldap:")
				}
			}
			if event.Transport == "tcp" && event.Protocol == "ldap" {
				tcpLDAP++
			}
		}),
	))
	require.Equal(t, 34, cldapEvents)
	t.Logf("CLDAP real capture: decoded=%d context=%d malformed=%d tcp_ldap=%d", cldapDecoded, cldapContext, cldapMalformed, tcpLDAP)
	require.NotZero(t, cldapDecoded)
	require.NotZero(t, cldapContext)
	require.Zero(t, unrecognized389)
	require.NotZero(t, tcpLDAP)
}

func FuzzCLDAPDatagram(f *testing.F) {
	f.Add(cldapTestSearchRequest(1, "DC=example", "name"))
	f.Add(cldapTestSearchDone(1, 0))
	f.Add([]byte{0x30, 0x80})
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 1<<16 {
			wire = wire[:1<<16]
		}
		if validCLDAPDatagram(wire) {
			_, _ = decodeCLDAPDatagram(wire, 1024)
		}
	})
}
