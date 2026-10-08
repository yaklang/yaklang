package pcaputil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Prefixes below are from the ident MVP real OpenSSH, pynetdicom, Dovecot,
// and chrony conversations. Test admission at the default bounded preview.
func TestProtocolIdentSignatureForeignPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		wire  []byte
		probe func([]byte, int) ProbeResult
	}{
		{"ssh-preamble", []byte("hello-not-ssh\r\nSSH-2.0-CorpusRaw_1.0\r\n"), probeIEC104},
		{"dicom-associate-ac", []byte{2, 0, 0, 0, 0, 188, 0, 1, 0, 0, 'I', 'D', 'E', 'N', 'T', '-', 'S', 'C', 'P'}, probeModbus},
		{"ftp-auth-tls", []byte("AUTH TLS\r\n"), probeSMTP},
		{"ftp-auth-ssl", []byte("AUTH SSL\r\n"), probeSMTP},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, ProbeReject, tt.probe(tt.wire, 64).Verdict)
		})
	}
}

func TestProtocolIdentSignatureLongIMAPGreeting(t *testing.T) {
	wire := []byte("* OK [CAPABILITY IMAP4rev1 LOGIN-REFERRALS ID ENABLE IDLE SASL-IR LITERAL+ STARTTLS AUTH=PLAIN AUTH=LOGIN] Dovecot ready.\r\n")
	for _, chunk := range []int{0, 1, 7, 64} {
		t.Run(fmt.Sprintf("chunk%d", chunk), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("EOF")
			p := s.Probe(wire)
			require.Equal(t, ProbeAccept, p.Verdict)
			require.Equal(t, "imap", p.Protocol)
			var events []*ProtocolEvent
			for off := 0; off < len(wire); {
				n := len(wire) - off
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(1, time.Unix(1, 0), wire[off:off+n])
				require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
				events = append(events, r.Events...)
				off += n
			}
			require.Len(t, events, 1)
			require.Equal(t, "decoded", events[0].Status)
			require.Equal(t, "imap", events[0].Protocol)
			_, err = events[0].GetFields()
			require.NoError(t, err)
			fields := events[0].Session
			require.Equal(t, "Untagged OK", fields["Packet Name"])
			require.Equal(t, string(wire[:len(wire)-2]), fields["Line"])
		})
	}
}

func TestProtocolIdentSignatureAdjacentProfilesAndBounds(t *testing.T) {
	// Admission must support an incomplete first frame and multiple coalesced
	// frames, without treating the whole preview as one declared message.
	request := []byte{0x80, 1, 0, 0, 0, 6, 1, 3, 0, 0, 0, 10}
	response := append([]byte{0x80, 1, 0, 0, 0, 23, 1, 3, 20}, make([]byte, 20)...)
	write := append([]byte{0x80, 2, 0, 0, 0, 27, 1, 16, 0, 0, 0, 10, 20}, make([]byte, 20)...)
	for _, wire := range [][]byte{request, response[:9], write[:13], append(append([]byte(nil), request...), request...)} {
		require.Equal(t, ProbeAccept, probeModbus(wire, 64).Verdict)
	}
	for n := 8; n < len(request); n++ {
		require.Equal(t, ProbeNeedMore, probeModbus(request[:n], 64).Verdict)
	}
	for _, wire := range [][]byte{
		{0, 1, 0, 0, 0, 2, 1, 3},       // missing address/quantity or response byte count
		{0, 1, 0, 0, 0, 7, 1, 3, 3},    // odd register response byte count
		{0, 1, 0, 0, 0, 4, 1, 5, 0, 0}, // wrong single-write length
		{0, 1, 0, 0, 0, 3, 1, 0x83},    // truncated exception remains pending
	} {
		require.NotEqual(t, ProbeAccept, probeModbus(wire, 64).Verdict)
	}
	badWrite := append([]byte(nil), write[:13]...)
	badWrite[12] = 19
	require.Equal(t, ProbeReject, probeModbus(badWrite, 64).Verdict)
	for _, wire := range [][]byte{{0x68, 4, 7, 0, 0, 0}, {0x68, 4, 1, 0, 2, 0}, {0x68, 10, 0, 0, 0, 0, 200, 1, 3, 0, 1, 0}} {
		require.Equal(t, ProbeAccept, probeIEC104(wire, 64).Verdict, "private ASDU types remain admissible")
	}
	for _, wire := range [][]byte{{0x68, 3, 7, 0, 0, 0}, {0x68, 4, 7, 1, 0, 0}, {0x68, 10, 0, 0, 1, 0, 1, 1, 3, 0, 1, 0}} {
		require.Equal(t, ProbeReject, probeIEC104(wire, 64).Verdict)
	}
	for _, command := range []string{"AUTH PLAIN AGFi", "AUTH LOGIN", "EHLO client.example", "STARTTLS"} {
		require.Equal(t, ProbeAccept, probeSMTP([]byte(command+"\r\n"), 64).Verdict)
	}
	for _, wire := range []string{"* OKAY ", "* CAPABILITYX ", "a1 LOGINNING ", "* OK bad\x00text"} {
		require.NotEqual(t, ProbeAccept, probeIMAP([]byte(wire), 64).Verdict)
	}
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("EOF")
	r := s.Feed(1, time.Unix(1, 0), []byte("* OK "+strings.Repeat("x", imapLineMax)+"\r\n"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
}

func TestProtocolIdentSignatureNTPProfileBounds(t *testing.T) {
	for _, version := range []byte{3, 4} {
		wire := ntpPkt(3, 0, nil, nil, nil)
		wire[0] = version<<3 | 3
		require.Equal(t, ProbeAccept, probeNTP(wire, 64).Verdict)
		for n := 0; n < 48; n++ {
			require.Equal(t, ProbeReject, probeNTP(wire[:n], 64).Verdict)
			_, err := (&binNTP{}).consume(wire[:n])
			require.Error(t, err)
		}
		for _, mode := range []byte{0, 1, 2, 5, 6, 7} {
			wire[0] = version<<3 | mode
			require.Equal(t, ProbeReject, probeNTP(wire, 64).Verdict)
			_, err := (&binNTP{}).consume(wire)
			require.Error(t, err)
		}
		wire[0], wire[1] = version<<3|4, 17
		require.Equal(t, ProbeReject, probeNTP(wire, 64).Verdict)
		_, err := (&binNTP{}).consume(wire)
		require.Error(t, err)
	}
}

func TestProtocolIdentSignatureNTPv3RealProfile(t *testing.T) {
	request, err := hex.DecodeString("1b000000000000000000000000000000000000000000000000000000000000000000000000000000ee724725603a9000")
	require.NoError(t, err)
	response, err := hex.DecodeString("1c0200e80000097d000011614559cfc7ee72471c04299d86ee724725603a9000ee7247256045417eee724725604959a5")
	require.NoError(t, err)
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("EOF")
	p := s.Probe(request)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "ntp", p.Protocol)
	require.Equal(t, "3", p.Version)
	for dir, wire := range [][]byte{request, response} {
		r := s.Feed(dir, time.Unix(1, 0), wire)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "decoded", r.Events[0].Status)
		_, err = r.Events[0].GetFields()
		require.NoError(t, err)
		fields := r.Events[0].Session
		require.Equal(t, 3, fields["Version"])
		require.Equal(t, 3+dir, fields["Mode"])
		require.Equal(t, wire[24:32], fields["Origin Timestamp"])
		require.Equal(t, wire[32:40], fields["Receive Timestamp"])
		require.Equal(t, wire[40:48], fields["Transmit Timestamp"])
		if dir == 1 {
			require.Equal(t, "Client", fields["In Reply To"])
		}
	}
}

// assertSignatureMVPEvents consumes each sealed signature case's expected.json.
func assertSignatureMVPEvents(t testing.TB, expected json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Protocol  string          `json:"expected_protocol"`
		Forbidden []string        `json:"forbidden_protocols"`
		Fields    json.RawMessage `json:"exact_session_fields"`
	}
	require.NoError(t, json.Unmarshal(expected, &a))
	var decoded []*ProtocolEvent
	for _, e := range events {
		for _, forbidden := range a.Forbidden {
			require.NotEqual(t, forbidden, e.Protocol, "forbidden profile status=%s", e.Status)
		}
		if a.Protocol != "" && e.Protocol == a.Protocol && e.Status == "decoded" {
			decoded = append(decoded, e)
		}
	}
	if a.Protocol != "" {
		require.NotEmpty(t, decoded)
	}
	if len(a.Fields) == 0 {
		return
	}
	var expectedFields []map[string]any
	if a.Fields[0] == '[' {
		require.NoError(t, json.Unmarshal(a.Fields, &expectedFields))
	} else {
		var fields map[string]any
		require.NoError(t, json.Unmarshal(a.Fields, &fields))
		expectedFields = []map[string]any{fields}
	}
	require.Len(t, decoded, len(expectedFields))
	for i, e := range decoded {
		fields, err := e.GetFields()
		require.NoError(t, err)
		// The native session model is the authoritative profile semantic output.
		for key, value := range e.Session {
			fields[key] = value
		}
		assertMVPJSONFields(t, expectedFields[i], fields)
	}
}

// assertDNSPointerMVPEvents checks the independent RFC1035 name-compression
// cases shared by the sealed capture matrix. DNSSEC RDATA remains opaque.
func assertDNSPointerMVPEvents(t testing.TB, expected json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Protocol string `json:"expected_protocol"`
		Count    int    `json:"message_count"`
		Name     string `json:"response_name"`
		Type     int    `json:"response_rr_type"`
		Section  string `json:"response_section"`
		RData    string `json:"response_rdata_hex"`
		TTL      int    `json:"response_ttl"`
	}
	require.NoError(t, json.Unmarshal(expected, &a))
	var decoded []*ProtocolEvent
	for _, e := range events {
		if e.Protocol != "dns" {
			continue
		}
		if e.Status == "decoded" || e.Status == "deferred" {
			_, err := e.GetFields()
			require.NoError(t, err)
			decoded = append(decoded, e)
		}
	}
	require.Len(t, decoded, a.Count)
	if a.Protocol == "" {
		return
	}
	response := decoded[1]
	require.NotZero(t, response.ResponseTo)
	require.Equal(t, decoded[0].ID, response.ResponseTo)
	require.Equal(t, "matched", response.Session["Association Status"])
	facts, ok := response.Session["DNS"].(map[string]any)
	require.True(t, ok)
	rows, ok := facts[a.Section].([]map[string]any)
	require.True(t, ok)
	require.Len(t, rows, 1)
	require.Equal(t, a.Name, rows[0]["Name"])
	require.EqualValues(t, a.Type, rows[0]["Type"])
	require.EqualValues(t, a.TTL, rows[0]["TTL"])
	raw, ok := rows[0]["RData"].([]byte)
	require.True(t, ok)
	require.Equal(t, a.RData, hex.EncodeToString(raw))
	if a.Type == 1 {
		require.Equal(t, "203.0.113.7", rows[0]["Address"])
	} else {
		require.Equal(t, "opaque", rows[0]["RData Completeness"])
	}
}
