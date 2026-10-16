package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Payloads from the public Wireshark ISO 8583-1 samples
// (iso8583_ascii_sample.pcapng and iso8583_bin_sample.pcapng, originally
// attached to Wireshark issue 12244). Each constant keeps the two-byte length
// prefix used on the TCP carrier; the ASCII samples use a little-endian prefix.
const (
	iso8583ASCII0800Hex = "910030383030383233383030303030326331303030613034303030303030303030303030303030393039323231333032303030303033313931333032303930393030333032333632323930303030303030303030303130343030333530312e30322e30364e4730312e303061303030303035323231353632333456353135303030383630303030303030303033595931303034"
	iso8583ASCII0200Hex = "3301303230306232333830343031323063313930306132303030303030303030303030353030303033303030303030303030303030343434303930393232333130353030303031333139333130353039303930323130363633363233343337363336323334313030303030333031303d3135303735303030303030303030303030303030333032333632323930303030303030303030303130343030333530312e30322e30364e4730312e303061303030303035323231353632333456353135303037363943354436464330314435463838434530303836303030303030303030335959303031303536324332333330353230383331303033303030303332323230363635333635374431393239353330393039303037303030303332323230313930313638454537383943373231453833424235"
	iso8583BCD1200Hex   = "48001200d020000002c000000000000000000008161234567890123456000000005699000234000431323334352020203637383930313233342020202020200009424c414820424c4148"
	iso8583ASCII1200Hex = "6e003132303064303230303030303032633030303030303030303030303030303030303030383136313233343536373839303132333435363030303030303030353639393030303233343030343132333435202020363738393031323334202020202020303039424c414820424c4148"
	iso8583BCD0200Hex   = "de000200b238040120c1900a20000000000005000030000000000001110909221310000004191310090900210663623437363336323334313030303030333031303d31353037353030303030303030303030303030303330323336323239303030303030303030303031303430003530312e30322e30364e4730312e30306130303030303532323135363233345635313530303736925365200000000000083630303030303030000359593001003131433233333035323038333130303330303030333232323036363533363537001638454537383943373231453833424235"
)

func iso8583Sample(t *testing.T, text string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(text)
	require.NoError(t, err)
	return raw
}

func iso8583Element(t *testing.T, fields map[string]any, field int) map[string]any {
	t.Helper()
	elements, ok := fields["Data Elements"].(map[string]any)
	require.True(t, ok, "data element projection missing")
	element, ok := elements[strconv.Itoa(field)].(map[string]any)
	require.Truef(t, ok, "data element %d missing", field)
	return element
}

func TestISO8583ASCIIFieldsFromPublicCapture(t *testing.T) {
	payload := iso8583Sample(t, iso8583ASCII0800Hex)
	fields, err := decodeISO8583Message(payload[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "0800", fields["MTI"])
	require.Equal(t, "ISO 8583:1987", fields["MTI Version"])
	require.Equal(t, "Network management", fields["MTI Class"])
	require.Equal(t, "Request", fields["MTI Function"])
	require.Equal(t, "Acquirer", fields["MTI Origin"])
	require.Equal(t, "ascii", fields["Encoding"])
	require.Equal(t, true, fields["Secondary Bitmap"])
	require.Equal(t, "0909221302", iso8583Element(t, fields, 7)["Value"])
	require.Equal(t, 10, iso8583Element(t, fields, 7)["Length"])
	require.Equal(t, "000003", iso8583Element(t, fields, 11)["Value"])
	require.Equal(t, "00", iso8583Element(t, fields, 39)["Value"])
	require.Equal(t, "30236229", iso8583Element(t, fields, 41)["Value"])
	require.Equal(t, "000000000001040", iso8583Element(t, fields, 42)["Value"])
	require.Equal(t, "01.02.06NG01.00a00000522156234V5150", iso8583Element(t, fields, 48)["Value"])
	require.Equal(t, "LLLVAR", iso8583Element(t, fields, 48)["Length Type"])
	require.Equal(t, "004", iso8583Element(t, fields, 70)["Value"])

	auth := iso8583Sample(t, iso8583ASCII0200Hex)
	fields, err = decodeISO8583Message(auth[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "0200", fields["MTI"])
	require.Equal(t, "Financial", fields["MTI Class"])
	require.Equal(t, "000000000444", iso8583Element(t, fields, 4)["Value"])
	require.Equal(t, "000013", iso8583Element(t, fields, 11)["Value"])
	require.Equal(t, "076", iso8583Element(t, fields, 49)["Value"])
	require.Equal(t, []byte{0x9c, 0x5d, 0x6f, 0xc0, 0x1d, 0x5f, 0x88, 0xce}, iso8583Element(t, fields, 52)["Value"])
	require.Equal(t, "binary", iso8583Element(t, fields, 52)["Type"])
	require.Equal(t, true, iso8583Element(t, fields, 52)["Sensitive"])
	require.Equal(t, "8EE789C721E83BB5", iso8583Element(t, fields, 120)["Value"])
}

func TestISO8583BCDFieldsFromPublicCapture(t *testing.T) {
	payload := iso8583Sample(t, iso8583BCD1200Hex)
	fields, err := decodeISO8583Message(payload[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "1200", fields["MTI"])
	require.Equal(t, "ISO 8583:1993", fields["MTI Version"])
	require.Equal(t, "Request", fields["MTI Function"])
	require.Equal(t, "Acquirer", fields["MTI Origin"])
	require.Equal(t, "bcd", fields["Encoding"])
	require.Equal(t, "1234567890123456", iso8583Element(t, fields, 2)["Value"])
	require.Equal(t, true, iso8583Element(t, fields, 2)["Sensitive"])
	require.Equal(t, "000000005699", iso8583Element(t, fields, 4)["Value"])
	require.Equal(t, "000234", iso8583Element(t, fields, 11)["Value"])
	require.Equal(t, "12345   ", iso8583Element(t, fields, 41)["Value"])
	require.Equal(t, "678901234      ", iso8583Element(t, fields, 42)["Value"])
	// The 1993 revision widened the response code to three digits; this advice
	// is the only message in either capture that uses it, and both encodings
	// of the same exchange agree on the three-digit value.
	require.Equal(t, "004", iso8583Element(t, fields, 39)["Value"])
	require.Equal(t, 3, iso8583Element(t, fields, 39)["Length"])
	require.Equal(t, "BLAH BLAH", iso8583Element(t, fields, 125)["Value"])

	auth := iso8583Sample(t, iso8583BCD0200Hex)
	fields, err = decodeISO8583Message(auth[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "0200", fields["MTI"])
	require.Equal(t, "000000000111", iso8583Element(t, fields, 4)["Value"])
	require.Equal(t, "000004", iso8583Element(t, fields, 11)["Value"])
	require.Equal(t, "021", iso8583Element(t, fields, 22)["Value"])
	require.Equal(t, "6362341000003010=15075000000000000000", iso8583Element(t, fields, 35)["Value"])
	require.Equal(t, []byte{0x92, 0x53, 0x65, 0x20, 0x00, 0x00, 0x00, 0x00}, iso8583Element(t, fields, 52)["Value"])
}

func TestISO8583ProbeAndFramingFromPublicCapture(t *testing.T) {
	for _, sample := range []string{iso8583ASCII0800Hex, iso8583ASCII0200Hex, iso8583BCD1200Hex, iso8583BCD0200Hex} {
		payload := iso8583Sample(t, sample)
		result, bigEndian := probeISO8583(payload, 1<<20)
		require.Equalf(t, ProbeAccept, result.Verdict, "probe must admit the captured message")
		require.False(t, bigEndian, "the captured carrier uses a little-endian length prefix")

		flow := &binFlow{
			iso8583: &iso8583Session{},
			a:       &binParser{config: BinParserConfig{MaxMessageBytes: 1 << 20, MaxBufferedBytes: 1 << 20}},
		}
		n, spec, err := flow.frameISO8583(payload)
		require.NoError(t, err)
		require.NotNil(t, spec)
		require.Equal(t, len(payload), n)

		concatenated := append(append([]byte(nil), payload...), payload...)
		n, _, err = flow.frameISO8583(concatenated)
		require.NoError(t, err)
		require.Equal(t, len(payload), n)

		n, spec, err = flow.frameISO8583(payload[:len(payload)-1])
		require.NoError(t, err)
		require.Nil(t, spec)
		require.Zero(t, n)

		result, _ = probeISO8583(payload[:len(payload)-4], 1<<20)
		require.Equalf(t, ProbeNeedMore, result.Verdict, "a partial carrier must hold for more bytes")
	}
}

func TestISO8583VersionDependentResponseCode(t *testing.T) {
	// The public captures pin both revisions of element 39: ISO 8583:1987
	// carries two digits, ISO 8583:1993 carries three. The same exchange is
	// captured in ASCII and binary and both carriers agree.
	advice := iso8583Sample(t, iso8583ASCII1200Hex)
	fields, err := decodeISO8583Message(advice[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "1200", fields["MTI"])
	require.Equal(t, "ISO 8583:1993", fields["MTI Version"])
	require.Equal(t, "004", iso8583Element(t, fields, 39)["Value"])
	require.Equal(t, 3, iso8583Element(t, fields, 39)["Length"])

	binaryAdvice := iso8583Sample(t, iso8583BCD1200Hex)
	fields, err = decodeISO8583Message(binaryAdvice[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "004", iso8583Element(t, fields, 39)["Value"])
	require.Equal(t, "1234567890123456", iso8583Element(t, fields, 2)["Value"])
	require.Equal(t, "000000005699", iso8583Element(t, fields, 4)["Value"])

	management := iso8583Sample(t, iso8583ASCII0800Hex)
	fields, err = decodeISO8583Message(management[2:], DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "ISO 8583:1987", fields["MTI Version"])
	require.Equal(t, "00", iso8583Element(t, fields, 39)["Value"])
	require.Equal(t, 2, iso8583Element(t, fields, 39)["Length"])
}

func TestISO8583RejectsMalformedEnvelopes(t *testing.T) {
	_, err := decodeISO8583Message([]byte("12X0"+strings.Repeat("0", 16)), DefaultParserBudget())
	require.Error(t, err)

	payload := iso8583Sample(t, iso8583ASCII0800Hex)
	body := append([]byte(nil), payload[2:]...)
	_, err = decodeISO8583Message(append(body, 'Z'), DefaultParserBudget())
	require.Error(t, err, "bytes outside the declared data elements must not be invented")

	_, err = decodeISO8583Message(body[:len(body)-2], DefaultParserBudget())
	require.Error(t, err, "a truncated final element must not decode")

	random := append([]byte{0x20, 0x00}, bytes.Repeat([]byte{'A'}, 32)...)
	result, _ := probeISO8583(random, 1<<20)
	require.Equal(t, ProbeReject, result.Verdict, "length prefix and ASCII bytes alone are not ISO 8583")
}

// iso8583SealedAnswer mirrors the independent verifier's answer document in
// the validation batch. It is produced by tools/verify.py, never by the parser.
type iso8583SealedAnswer struct {
	Capture         string `json:"capture"`
	SHA256          string `json:"sha256"`
	Bytes           int    `json:"bytes"`
	Flows           int    `json:"flows"`
	DecodedMessages int    `json:"decoded_messages"`
	RefusedMessages int    `json:"refused_messages"`
	Events          [][]struct {
		Direction       string         `json:"direction"`
		Offset          int            `json:"offset"`
		Status          string         `json:"status"`
		Length          int            `json:"length"`
		MTI             string         `json:"mti"`
		Version         string         `json:"version"`
		Encoding        string         `json:"encoding"`
		SecondaryBitmap bool           `json:"secondary_bitmap"`
		ElementOrder    []int          `json:"element_order"`
		Elements        map[string]any `json:"elements"`
		Reason          string         `json:"reason"`
	} `json:"events"`
}

func iso8583Sealed(t *testing.T) ([]string, map[string]iso8583SealedAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("iso8583-public-samples/controls.json")
	require.NoError(t, err)
	var controls struct {
		Schema string   `json:"schema"`
		Inputs []string `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal(raw, &controls))
	require.Equal(t, "pr5013-iso8583-public-samples/v1", controls.Schema)
	require.Len(t, controls.Inputs, 2)
	answers := map[string]iso8583SealedAnswer{}
	for _, control := range controls.Inputs {
		data, err := trafficfixture.ReadFile("iso8583-public-samples/answers/" + control + ".json")
		require.NoError(t, err)
		var answer iso8583SealedAnswer
		require.NoError(t, json.Unmarshal(data, &answer))
		require.Equal(t, control, strings.TrimSuffix(answer.Capture, ".pcapng"))
		answers[control] = answer
	}
	return controls.Inputs, answers
}

// TestISO8583SealedReplay drives both public captures through the shared
// reader, reassembly and protocol engine, then compares every admitted message
// with the independently produced answer document.
func TestISO8583SealedReplay(t *testing.T) {
	inputs, answers := iso8583Sealed(t)
	for _, control := range inputs {
		t.Run(control, func(t *testing.T) {
			answer := answers[control]
			raw, err := trafficfixture.ReadFile("iso8583-public-samples/captures/" + control + ".pcapng")
			require.NoError(t, err)
			require.Len(t, raw, answer.Bytes)
			require.Equal(t, answer.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))

			var events []*ProtocolEvent
			var stats ProtocolStats
			err = ReplayPcap(bytes.NewReader(raw),
				WithTCPReassemblyWorkers(1),
				WithBinParserConfig(BinParserConfig{MaxBufferedBytes: 32 << 20, MaxMessageBytes: 1 << 20}),
				WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
				WithOnProtocolStats(func(s ProtocolStats) { stats = s }),
			)
			require.NoError(t, err)
			require.Zero(t, stats.BufferedBytes, "parser did not release retained bytes")
			require.Zero(t, stats.CallbackPanics)

			// One event per carried message. The trailing short message of each
			// capture stays a diagnostic event: its MTI is outside the published
			// dimensions, so it is refused rather than trimmed into a decode.
			// Callback order across flows is not guaranteed, so match on the
			// sealed carrier identity instead of arrival order.
			require.Len(t, events, answer.DecodedMessages+answer.RefusedMessages)
			byLength := map[int][]*ProtocolEvent{}
			for _, event := range events {
				byLength[event.Length] = append(byLength[event.Length], event)
			}
			matched := 0
			for _, flow := range answer.Events {
				for _, expected := range flow {
					// The sealed answer measures the message body; the event spans
					// the two-byte little-endian carrier prefix as well.
					length := expected.Length + 2
					candidates := byLength[length]
					require.NotEmptyf(t, candidates, "no event for the sealed message at offset %d", expected.Offset)
					var event *ProtocolEvent
					for i, candidate := range candidates {
						if expected.Status == "decoded" && candidate.Fields["MTI"] == expected.MTI {
							event = candidate
							byLength[length] = append(candidates[:i], candidates[i+1:]...)
							break
						}
						if expected.Status != "decoded" && candidate.Status == "malformed" {
							event = candidate
							byLength[length] = append(candidates[:i], candidates[i+1:]...)
							break
						}
					}
					require.NotNilf(t, event, "no event matched the sealed message at offset %d", expected.Offset)
					matched++
					if expected.Status != "decoded" {
						// The sealed answer records why the carrier refused the
						// message; the event keeps the same typed classification.
						require.NotEmpty(t, expected.Reason)
						require.Equal(t, "iso8583", event.Protocol)
						require.Equal(t, "malformed", event.Status)
						require.Contains(t, expected.Reason, "MTI")
						require.Contains(t, event.Error, "MTI")
						continue
					}
					require.Equal(t, "iso8583", event.Protocol)
					require.Equal(t, "decoded", event.Status)
					require.Equal(t, iso8583Profile, event.Profile)
					require.Equal(t, expected.MTI, event.Fields["MTI"])
					require.Equal(t, expected.Version, event.Fields["MTI Version"])
					require.Equal(t, expected.Encoding, event.Fields["Encoding"])
					require.Equal(t, expected.SecondaryBitmap, event.Fields["Secondary Bitmap"])
					for field, raw := range expected.Elements {
						want, ok := raw.(map[string]any)
						require.Truef(t, ok, "sealed element %s is not an object", field)
						got := iso8583Element(t, event.Fields, atoiField(t, field))
						require.Equal(t, iso8583WireValue(want["Value"]), iso8583WireValue(got["Value"]), "element %s of %s", field, expected.MTI)
						// JSON numbers decode as float64 while the field tree keeps
						// integer lengths; compare the numeric value only.
						sealedLength, err := strconv.ParseFloat(fmt.Sprint(want["Length"]), 64)
						require.NoError(t, err)
						require.Equal(t, sealedLength, float64(got["Length"].(int)), "element %s length of %s", field, expected.MTI)
					}
				}
			}
			require.Equal(t, len(events), matched, "every event matched exactly one sealed message")
		})
	}
}

// iso8583WireValue normalizes a sealed or decoded element value for
// comparison: binary elements are bytes and are shown as hexadecimal digits on
// the wire, while text and numeric elements compare as captured.
func iso8583WireValue(value any) string {
	if raw, ok := value.([]byte); ok {
		return hex.EncodeToString(raw)
	}
	return fmt.Sprint(value)
}

func atoiField(t *testing.T, field string) int {
	t.Helper()
	value, err := strconv.Atoi(field)
	require.NoError(t, err)
	return value
}
