package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type nativeDNSQuestion struct {
	Name        string `json:"name"`
	Type, Class uint16
}
type nativeDNSAnswer struct {
	Name        string `json:"name"`
	Type, Class uint16
	TTL         uint32
	Data        []string `json:"data"`
}
type nativeDNSEDNSOption struct {
	Code  uint16
	Value []byte
}
type nativeDNSCanonical struct {
	ID          uint16
	QR          bool
	RCode       uint16 `json:"rcode"`
	Question    []nativeDNSQuestion
	Answer      []nativeDNSAnswer
	Authority   []string
	Additional  []nativeDNSAnswer
	EDNS        int
	UDPPayload  uint16                `json:"udp_payload"`
	EDNSFlags   uint16                `json:"edns_flags"`
	EDNSOptions []nativeDNSEDNSOption `json:"edns_options"`
}
type nativeDNSExchange struct {
	Transport, Method string
	HTTPStatus        int `json:"http_status"`
	Request, Response nativeDNSCanonical
}

func nativeDNSName(s string) string { return strings.ToLower(strings.TrimSuffix(s, ".")) + "." }
func nativeDNSRecords(t testing.TB, records []map[string]any) []nativeDNSAnswer {
	t.Helper()
	out := []nativeDNSAnswer{}
	positions := map[string]int{}
	for _, rr := range records {
		a := nativeDNSAnswer{Name: nativeDNSName(rr["Name"].(string)), Type: rr["Type"].(uint16), Class: rr["Class"].(uint16), TTL: rr["TTL"].(uint32)}
		switch a.Type {
		case 1, 28:
			a.Data = []string{rr["Address"].(string)}
		case 2, 5, 12:
			a.Data = []string{nativeDNSName(rr["Target"].(string))}
		case 15:
			a.Data = []string{fmt.Sprintf("%d %s", rr["Priority"], nativeDNSName(rr["Target"].(string)))}
		case 16:
			var txt []string
			for _, s := range rr["Text"].([]string) {
				txt = append(txt, strconv.Quote(s))
			}
			a.Data = []string{strings.Join(txt, " ")}
		case 6:
			a.Data = []string{fmt.Sprintf("%s %s %d %d %d %d %d", nativeDNSName(rr["MNAME"].(string)), nativeDNSName(rr["RNAME"].(string)), rr["Serial"], rr["Refresh"], rr["Retry"], rr["Expire"], rr["Minimum"])}
		default:
			t.Fatalf("native oracle requires canonicalization for RR type %d", a.Type)
		}
		// dnspython exposes RRsets, whereas the decoder exposes individual RRs.
		key := fmt.Sprintf("%s/%d/%d/%d", a.Name, a.Type, a.Class, a.TTL)
		if at, ok := positions[key]; ok {
			out[at].Data = append(out[at].Data, a.Data...)
		} else {
			positions[key] = len(out)
			out = append(out, a)
		}
	}
	for i := range out {
		sort.Strings(out[i].Data)
	}
	return out
}
func nativeDNSCanonicalize(t testing.TB, dns map[string]any) nativeDNSCanonical {
	t.Helper()
	c := nativeDNSCanonical{ID: dns["ID"].(uint16), QR: dns["Response"].(bool), RCode: dns["RCODE"].(uint16), Question: []nativeDNSQuestion{}, Authority: []string{}, EDNS: -1, EDNSOptions: []nativeDNSEDNSOption{}}
	for _, q := range dns["Questions"].([]map[string]any) {
		c.Question = append(c.Question, nativeDNSQuestion{nativeDNSName(q["Name"].(string)), q["Type"].(uint16), q["Class"].(uint16)})
	}
	c.Answer = nativeDNSRecords(t, dns["Answers"].([]map[string]any))
	for _, rr := range nativeDNSRecords(t, dns["Authority"].([]map[string]any)) {
		class := map[uint16]string{1: "IN", 3: "CH", 4: "HS"}[rr.Class]
		typ := map[uint16]string{1: "A", 2: "NS", 5: "CNAME", 6: "SOA", 12: "PTR", 16: "TXT", 28: "AAAA"}[rr.Type]
		require.NotEmpty(t, class, "unsupported oracle authority class %d", rr.Class)
		require.NotEmpty(t, typ, "unsupported oracle authority type %d", rr.Type)
		lines := make([]string, 0, len(rr.Data))
		for _, data := range rr.Data {
			lines = append(lines, fmt.Sprintf("%s %d %s %s %s", rr.Name, rr.TTL, class, typ, data))
		}
		c.Authority = append(c.Authority, strings.Join(lines, "\n"))
	}
	additional := []map[string]any{}
	for _, rr := range dns["Additional"].([]map[string]any) {
		if rr["Type"].(uint16) != 41 {
			additional = append(additional, rr)
			continue
		}
		require.Equal(t, -1, c.EDNS, "multiple OPT records")
		c.EDNS = int(rr["EDNS Version"].(uint8))
		c.UDPPayload = rr["UDP Size"].(uint16)
		ttl := rr["TTL"].(uint32)
		c.EDNSFlags = uint16(ttl)
		require.Equal(t, dns["Flags"].(uint16)&15|uint16(ttl>>24)<<4, c.RCode, "shared codec must expose the complete RCODE without oracle repair")
		for _, option := range rr["Options"].([]map[string]any) {
			c.EDNSOptions = append(c.EDNSOptions, nativeDNSEDNSOption{Code: option["Code"].(uint16), Value: option["Value"].([]byte)})
		}
	}
	c.Additional = nativeDNSRecords(t, additional)
	return c
}
func nativeDNSKey(c nativeDNSCanonical, transport string) string {
	q := c.Question[0]
	return fmt.Sprintf("%s/%d/%v/%s/%d/%d", transport, c.ID, c.QR, q.Name, q.Type, q.Class)
}
func nativeDNSLoad(t testing.TB, root, mode string) ([]byte, []nativeDNSExchange, []CaptureOption) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, mode+"-native.pcapng"))
	require.NoError(t, err)
	oracle, err := os.ReadFile(filepath.Join(root, mode+"-client-oracle.json"))
	require.NoError(t, err)
	var rows []nativeDNSExchange
	require.NoError(t, json.Unmarshal(oracle, &rows))
	var schema []struct{ Request, Response map[string]json.RawMessage }
	require.NoError(t, json.Unmarshal(oracle, &schema))
	for _, row := range schema {
		for _, message := range []map[string]json.RawMessage{row.Request, row.Response} {
			for _, field := range []string{"authority", "additional", "edns", "udp_payload", "edns_flags", "edns_options"} {
				require.Contains(t, message, field, "native oracle must retain extended DNS evidence")
			}
		}
	}
	options := []CaptureOption{WithProtocolDecodeAs("udp", 19553, "dns")}
	if mode != "dns" {
		keys, err := os.ReadFile(filepath.Join(root, mode+".keys"))
		require.NoError(t, err)
		secrets, err := ParseTLSKeyLog(string(keys))
		require.NoError(t, err)
		options = append(options, WithTLSSecrets(secrets))
	}
	return raw, rows, options
}

// These pinned captures contain TLS records and HTTP/2 transport/header frames
// in addition to complete DNS messages. They cannot hide unrelated application
// events just because those events lack a DNS field.
func nativeDNSRequireEnvelope(t testing.TB, mode string, e *ProtocolEvent) {
	t.Helper()
	require.Contains(t, []string{"h1", "h2"}, mode, "plain DNS capture has no non-DNS envelopes")
	if e.Protocol == "tls" {
		return
	}
	require.Equal(t, "h2", mode, "HTTP/1 capture only permits TLS carrier envelopes")
	require.Contains(t, []string{"http2", "doh"}, e.Protocol, "unexpected application event without DNS")
	wire := bytes.TrimPrefix(e.Raw, []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	require.GreaterOrEqual(t, len(wire), 9)
	frameType := wire[3]
	stream := binary.BigEndian.Uint32(wire[5:9]) & 0x7fffffff
	require.EqualValues(t, frameType, e.Session["Frame Type"])
	require.EqualValues(t, stream, e.Session["Stream ID"])
	require.Equal(t, "decrypted", e.SourceBytes.Kind)
	require.NotEmpty(t, e.SourceBytes.PacketRefs)
	require.NotEmpty(t, e.SourceBytes.ParentPDUs)
	if e.Protocol == "http2" {
		require.Zero(t, stream)
		require.Contains(t, []byte{4, 8}, frameType) // SETTINGS / connection WINDOW_UPDATE
		return
	}
	require.NotZero(t, stream)
	require.Equal(t, true, e.Session["DoH"])
	require.Equal(t, true, e.Session["HTTP Semantics Validated"])
	require.Equal(t, "2", e.Session["HTTP Version"])
	require.Contains(t, []byte{0, 1, 8}, frameType) // unfinished DATA / HEADERS / stream WINDOW_UPDATE
	if frameType != 8 {
		require.Zero(t, wire[4]&1, "a completed DoH body must produce DNS")
		require.Equal(t, false, e.Session["End Stream"])
		require.Equal(t, false, e.Session["Exchange Complete"])
	}
}

// The codec and expected messages come from pinned CoreDNS/dnspython/httpx,
// independently of Yaklang. Capture, socket, server and tshark evidence are
// retained separately. TLS keys authorize decryption, never certificate trust.
func TestNativeDNSDoHCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "bin-parser", "testdata", "protocol-native", "dns-doh")
	manifestRaw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Representation string
		Files          []struct {
			File, SHA256 string
			Bytes        int
		}
	}
	require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
	require.Equal(t, "native_capture", manifest.Representation)
	require.Greater(t, len(manifest.Files), 15)
	for _, f := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(root, f.File))
		require.NoError(t, err)
		require.Equal(t, f.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
		require.Len(t, raw, f.Bytes)
		require.NotContains(t, f.File, "private")
		require.NotEqual(t, "server-key.pem", f.File)
	}
	server, err := os.ReadFile(filepath.Join(root, "server-oracle.log"))
	require.NoError(t, err)
	require.Equal(t, 24, strings.Count(string(server), "[INFO]"))
	for _, mode := range []string{"dns", "h1", "h2"} {
		raw, rows, options := nativeDNSLoad(t, root, mode)
		expected := map[string]nativeDNSCanonical{}
		for _, row := range rows {
			transport := row.Transport
			if mode != "dns" {
				transport = "doh"
				require.Equal(t, 200, row.HTTPStatus)
			}
			expected[nativeDNSKey(row.Request, transport)] = row.Request
			expected[nativeDNSKey(row.Response, transport)] = row.Response
		}
		require.Len(t, expected, 2*len(rows))
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/workers%d/deferred%v", mode, workers, deferred), func(t *testing.T) {
					events, stats, err := binReplay(t, raw, workers, append(append([]CaptureOption{}, options...), WithBinParserDeferred(deferred))...)
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					require.Zero(t, stats.Malformed)
					require.Zero(t, stats.LimitedBytes)
					observed := map[string]nativeDNSCanonical{}
					envelopes := map[string]int{}
					requests := map[uint64]*ProtocolEvent{}
					responses := 0
					for _, e := range events {
						require.Empty(t, e.Error, "%s %s", e.Protocol, e.Summary)
						dns, ok := e.Session["DNS"].(map[string]any)
						if !ok {
							nativeDNSRequireEnvelope(t, mode, e)
							envelopes[e.Protocol]++
							continue
						}
						c := nativeDNSCanonicalize(t, dns)
						transport := e.Transport
						if mode != "dns" {
							transport = "doh"
							require.Equal(t, "doh", e.Protocol)
							require.Equal(t, "decrypted", e.SourceBytes.Kind)
							require.NotEmpty(t, e.SourceBytes.ParentPDUs)
							require.NotEmpty(t, e.SourceBytes.PacketRefs)
							if _, present := e.Session["Payload Decrypted"]; present {
								require.Equal(t, true, e.Session["Payload Decrypted"])
							}
						}
						key := nativeDNSKey(c, transport)
						want, ok := expected[key]
						require.True(t, ok, "unexpected DNS PDU %s", key)
						require.Equal(t, want, c, key)
						require.NotContains(t, observed, key, "duplicate PDU")
						observed[key] = c
						if !c.QR {
							requests[e.ID] = e
						} else {
							responses++
							if mode == "dns" {
								require.NotZero(t, e.ResponseTo)
								request := requests[e.ResponseTo]
								require.NotNil(t, request)
								require.Equal(t, dnsQuestionKey(request.Session["DNS"].(map[string]any)), dnsQuestionKey(dns))
							} else {
								require.Equal(t, "matched", e.Session["Association Status"])
								require.Equal(t, strings.TrimSuffix(c.Question[0].Name, "."), e.Session["Matched Request"])
							}
						}
						decoded, err := e.Decode()
						require.NoError(t, err)
						require.NotEmpty(t, decoded)
					}
					require.Equal(t, expected, observed)
					require.Equal(t, len(rows), responses)
					require.Equal(t, map[string]map[string]int{
						"dns": {}, "h1": {"tls": 27}, "h2": {"tls": 39, "http2": 6, "doh": 18},
					}[mode], envelopes, "pinned capture envelope counts exclude extra protocol events")
				})
			}
		}
		t.Run(mode+"/tshark", func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, mode+"-tshark.tsv"))
			require.NoError(t, err)
			responseKeys := map[string]int{}
			methods := map[string]int{}
			statuses := 0
			for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
				fields := strings.Split(line, "\t")
				require.Len(t, fields, 17)
				// -T fields emits every occurrence. HTTP/2 can coalesce multiple
				// DNS PDUs and status headers in one captured TCP packet.
				if fields[4] != "" {
					flags := strings.Split(fields[4], ",")
					names, types := strings.Split(fields[6], ","), strings.Split(fields[7], ",")
					codes, response := strings.Split(fields[5], ","), 0
					require.Len(t, names, len(flags))
					require.Len(t, types, len(flags))
					for i, flag := range flags {
						require.Contains(t, []string{"False", "True"}, flag)
						if flag != "True" {
							continue
						}
						require.NotEmpty(t, names[i])
						key := nativeDNSName(names[i]) + "/" + types[i]
						responseKeys[key]++
						wantCode := "0"
						if strings.HasPrefix(names[i], "missing.") {
							wantCode = "3"
						}
						require.Less(t, response, len(codes))
						require.Equal(t, wantCode, codes[response])
						response++
					}
					if response > 0 {
						require.Len(t, codes, response)
					}
				}
				method, status := fields[13], fields[14]
				if mode == "h1" {
					method, status = fields[15], fields[16]
				}
				if method != "" {
					for _, value := range strings.Split(method, ",") {
						methods[value]++
					}
				}
				if status != "" {
					for _, value := range strings.Split(status, ",") {
						require.Equal(t, "200", value)
						statuses++
					}
				}
			}
			require.Len(t, responseKeys, 6)
			for _, row := range rows {
				q := row.Request.Question[0]
				key := q.Name + "/" + fmt.Sprint(q.Type)
				require.Equal(t, map[bool]int{true: 2, false: 1}[mode == "dns"], responseKeys[key])
			}
			if mode != "dns" {
				require.Equal(t, map[string]int{"POST": 3, "GET": 3}, methods)
				require.Equal(t, 6, statuses)
			}
		})
		if mode != "dns" {
			t.Run(mode+"/no-key", func(t *testing.T) {
				events, stats, err := binReplay(t, raw, 2)
				require.NoError(t, err)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.Malformed)
				require.Zero(t, stats.CallbackPanics)
				require.Zero(t, stats.LimitedBytes)
				opaque := 0
				for _, e := range events {
					require.Empty(t, e.Error)
					require.Equal(t, "tls", e.Protocol)
					require.Nil(t, e.Session["DNS"])
					if e.Session["Content Visibility"] == "encrypted" {
						opaque++
					}
					require.NotEqual(t, true, e.Session["Authentication Verified"])
				}
				require.Positive(t, opaque)
			})
		}
	}
}

// Keep ordering independent of map iteration in callers that digest canonical
// output for scheduler and performance equivalence tests.
func nativeDNSCanonicalKeys(rows map[string]nativeDNSCanonical) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
