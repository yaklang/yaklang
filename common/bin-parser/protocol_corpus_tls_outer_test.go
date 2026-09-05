package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type tlsOuterCaptureCase struct {
	captureID          string
	roadmapName        string
	frame              int
	recordVersion      uint16
	recordLength       uint16
	handshakeLength    uint32
	clientHelloVersion uint16
	wantSNI            string
	wantALPN           []string
}

type tlsOuterRecordReader struct {
	*bytes.Reader
	inputBitLength uint64
}

func newTLSOuterRecordReader(input []byte) *tlsOuterRecordReader {
	return &tlsOuterRecordReader{
		Reader:         bytes.NewReader(input),
		inputBitLength: uint64(len(input)) * 8,
	}
}

func (r *tlsOuterRecordReader) InputBitLength() uint64 {
	return r.inputBitLength
}

func TestProtocolCorpusTLSOuterFields(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	tests := []tlsOuterCaptureCase{
		{
			captureID:          "ndpi-anydesk",
			roadmapName:        "AnyDesk",
			frame:              124,
			recordVersion:      0x0301,
			recordLength:       284,
			handshakeLength:    280,
			clientHelloVersion: 0x0303,
			wantALPN:           []string{"anydesk/6.2.0/linux"},
		},
		{
			captureID:          "ndpi-dingtalk",
			roadmapName:        "DingTalk",
			frame:              9,
			recordVersion:      0x0301,
			recordLength:       512,
			handshakeLength:    508,
			clientHelloVersion: 0x0303,
			wantSNI:            "static.dingtalk.com",
			wantALPN:           []string{"h2", "http/1.1"},
		},
		{
			captureID:          "ndpi-doh",
			roadmapName:        "DoH",
			frame:              4,
			recordVersion:      0x0301,
			recordLength:       512,
			handshakeLength:    508,
			clientHelloVersion: 0x0303,
			wantSNI:            "mozilla.cloudflare-dns.com",
			wantALPN:           []string{"h2", "http/1.1"},
		},
		{
			captureID:          "ndpi-dot",
			roadmapName:        "DoT",
			frame:              4,
			recordVersion:      0x0303,
			recordLength:       193,
			handshakeLength:    189,
			clientHelloVersion: 0x0303,
		},
		{
			captureID:          "ndpi-imaps",
			roadmapName:        "IMAPS",
			frame:              4,
			recordVersion:      0x0301,
			recordLength:       222,
			handshakeLength:    218,
			clientHelloVersion: 0x0303,
			wantSNI:            "mail.ntop.org",
		},
		{
			captureID:          "ndpi-smtps",
			roadmapName:        "SMTPS",
			frame:              3,
			recordVersion:      0x0301,
			recordLength:       512,
			handshakeLength:    508,
			clientHelloVersion: 0x0303,
		},
		{
			captureID:          "ndpi-wechat",
			roadmapName:        "WeChat/MicroMsg",
			frame:              94,
			recordVersion:      0x0301,
			recordLength:       233,
			handshakeLength:    229,
			clientHelloVersion: 0x0303,
			wantSNI:            "web.wechat.com",
			wantALPN:           []string{"h2", "http/1.1"},
		},
	}

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	wantedRoadmapNames := make(map[string]struct{}, len(tests))
	wantedCaptures := make(map[string]string, len(tests))
	for _, test := range tests {
		_, duplicate := wantedRoadmapNames[test.roadmapName]
		require.False(t, duplicate, "duplicate TLS outer roadmap case %q", test.roadmapName)
		wantedRoadmapNames[test.roadmapName] = struct{}{}
		wantedCaptures[test.captureID] = test.roadmapName
	}

	manifestCaptures := make(map[string]string, len(tests))
	capturesByID := make(map[string]protocolCorpusCapture, len(tests))
	for _, capture := range manifest.Captures {
		if capture.RoadmapName == nil {
			continue
		}
		if _, wanted := wantedRoadmapNames[*capture.RoadmapName]; !wanted {
			continue
		}
		manifestCaptures[capture.ID] = *capture.RoadmapName
		capturesByID[capture.ID] = capture
	}
	require.Equal(t, wantedCaptures, manifestCaptures, "TLS outer capture enumeration changed")

	seen := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		test := test
		t.Run(test.roadmapName, func(t *testing.T) {
			capture, ok := capturesByID[test.captureID]
			require.True(t, ok, "TLS outer case points to a missing manifest capture")
			require.Equal(t, "upstream-positive", capture.EvidenceKind)
			require.NotNil(t, capture.RoadmapName)
			require.Equal(t, test.roadmapName, *capture.RoadmapName)
			if test.captureID == "ndpi-anydesk" {
				require.Equal(t, 124, test.frame, "AnyDesk should use its stronger ALPN-bearing ClientHello")
			} else {
				require.NotNil(t, capture.RepresentativeFrame)
				require.Equal(t, capture.RepresentativeFrame.Number, test.frame, "case no longer uses the manifest representative frame")
			}

			record := tlsOuterRecordFromCapture(t, corpusDir, capture, test.frame, 1)
			require.Len(t, record, 5+int(test.recordLength))
			require.Equal(t, byte(22), record[0], "TLS record content type")
			require.Equal(t, test.recordVersion, binary.BigEndian.Uint16(record[1:3]), "TLS record legacy version")
			require.Equal(t, test.recordLength, binary.BigEndian.Uint16(record[3:5]), "TLS record length")
			require.Equal(t, byte(1), record[5], "TLS handshake type")
			require.Equal(t, test.handshakeLength, tlsOuterUint24(record[6:9]), "TLS handshake length")

			reader := newTLSOuterRecordReader(record)
			node, err := parser.ParseBinary(reader, "application-layer.tls", "Transport Layer Security")
			require.NoError(t, err)
			require.Zero(t, reader.Len(), "TLS rule left part of the bounded record unread")
			terminals, firstBit, lastBit := protocolCorpusConsumedRange(node, uint64(len(record))*8)
			require.Positive(t, terminals)
			require.Zero(t, firstBit, "TLS rule left an unparsed record prefix")
			require.Equal(t, uint64(len(record))*8, lastBit, "TLS rule left an unparsed record suffix")
			coveredTerminals, coverageErr := protocolCorpusTerminalCoverage(node, record)
			require.NoError(t, coverageErr, "TLS terminal fields do not cover the complete record")
			require.Positive(t, coveredTerminals)

			result, err := node.Result()
			require.NoError(t, err)
			recordLayer := mustChild(t, result, "Record Layer")
			require.Equal(t, uint64(22), uintVal(t, recordLayer.Child("ContentType")))
			require.Equal(t, uint64(test.recordVersion), uintVal(t, recordLayer.Child("Version")))
			require.Equal(t, uint64(test.recordLength), uintVal(t, recordLayer.Child("Length")))

			hello := mustChild(t, recordLayer, "TLSClientHello")
			require.Equal(t, uint64(1), uintVal(t, hello.Child("Handshake Type")))
			require.Equal(t, uint64(test.handshakeLength), uintVal(t, hello.Child("Length")))
			clientHello := mustChild(t, hello, "ClientHello")
			require.Equal(t, uint64(test.clientHelloVersion), uintVal(t, clientHello.Child("Legacy Version")))

			extensions := mustChild(t, clientHello, "Extensions").Children()
			sni := tlsOuterFindExtension(t, extensions, 0)
			if test.wantSNI == "" {
				require.Nil(t, sni, "capture unexpectedly acquired an SNI extension")
			} else {
				require.NotNil(t, sni, "capture lost its SNI extension")
				require.Equal(t, test.wantSNI, strVal(t, mustChild(t, sni, "SNI", "Host Name")))
			}

			alpn := tlsOuterFindExtension(t, extensions, 16)
			if test.wantALPN == nil {
				require.Nil(t, alpn, "capture unexpectedly acquired an ALPN extension")
			} else {
				require.NotNil(t, alpn, "capture lost its ALPN extension")
				require.Equal(t, test.wantALPN, tlsOuterDecodeALPN(t, bytesVal(t, alpn.Child("Octets"))))
			}

			seen[test.captureID] = struct{}{}
			t.Logf("capture=%s frame=%d record=%d handshake=ClientHello sni=%q alpn=%v", test.captureID, test.frame, len(record), test.wantSNI, test.wantALPN)
		})
	}
	require.Len(t, seen, len(tests), "a TLS outer capture was silently skipped")
}

func TestProtocolCorpusTLSOuterRejectsBrokenRecordBounds(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	var imaps protocolCorpusCapture
	for _, capture := range manifest.Captures {
		if capture.ID == "ndpi-imaps" {
			imaps = capture
			break
		}
	}
	require.Equal(t, "ndpi-imaps", imaps.ID, "negative fixture capture is missing")

	// Frame 6 contains a complete ServerHello record. The current TLS rule
	// intentionally exposes non-ClientHello handshakes as a bounded Payload,
	// which makes outer record-length violations directly observable.
	record := tlsOuterRecordFromCapture(t, corpusDir, imaps, 6, 2)
	require.Equal(t, byte(2), record[5], "negative fixture handshake type")

	t.Run("truncated record", func(t *testing.T) {
		tlsOuterRequireParseError(t, record[:len(record)-1])
	})

	t.Run("inflated record length", func(t *testing.T) {
		malformed := append([]byte(nil), record...)
		declared := binary.BigEndian.Uint16(malformed[3:5])
		require.Less(t, declared, ^uint16(0))
		binary.BigEndian.PutUint16(malformed[3:5], declared+1)
		tlsOuterRequireParseError(t, malformed)
	})
}

func tlsOuterRecordFromCapture(t *testing.T, corpusDir string, capture protocolCorpusCapture, frame, handshakeType int) []byte {
	t.Helper()
	captureData := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
	stream := protocolCorpusTCPStreamThroughFrame(t, captureData, capture.LinkType, frame)
	record, offset := tlsOuterFindHandshakeRecord(stream, byte(handshakeType))
	require.NotNil(t, record, "%s frame %d stream has no complete TLS handshake type %d", capture.ID, frame, handshakeType)
	require.Zero(t, offset, "%s frame %d direction has bytes before the selected TLS record", capture.ID, frame)
	return record
}

func tlsOuterFindHandshakeRecord(stream []byte, handshakeType byte) ([]byte, int) {
	for offset := 0; offset+9 <= len(stream); offset++ {
		if stream[offset] != 22 || stream[offset+1] != 3 {
			continue
		}
		recordLength := int(binary.BigEndian.Uint16(stream[offset+3 : offset+5]))
		end := offset + 5 + recordLength
		if recordLength < 4 || end > len(stream) || stream[offset+5] != handshakeType {
			continue
		}
		handshakeLength := int(tlsOuterUint24(stream[offset+6 : offset+9]))
		if handshakeLength+4 > recordLength {
			continue
		}
		return append([]byte(nil), stream[offset:end]...), offset
	}
	return nil, -1
}

func tlsOuterFindExtension(t *testing.T, extensions []*base.NodeValue, extensionType uint64) *base.NodeValue {
	t.Helper()
	var found *base.NodeValue
	for _, extension := range extensions {
		if uintVal(t, extension.Child("Type")) != extensionType {
			continue
		}
		require.Nil(t, found, "duplicate TLS extension type %d", extensionType)
		found = extension
	}
	return found
}

func tlsOuterDecodeALPN(t *testing.T, data []byte) []string {
	t.Helper()
	require.GreaterOrEqual(t, len(data), 2, "ALPN extension is shorter than its list length")
	listLength := int(binary.BigEndian.Uint16(data[:2]))
	require.Equal(t, len(data)-2, listLength, "ALPN protocol-name list length")

	var protocols []string
	for offset := 2; offset < len(data); {
		nameLength := int(data[offset])
		offset++
		require.Positive(t, nameLength, "ALPN protocol name is empty")
		require.LessOrEqual(t, offset+nameLength, len(data), "ALPN protocol name exceeds extension data")
		protocols = append(protocols, string(data[offset:offset+nameLength]))
		offset += nameLength
	}
	require.NotEmpty(t, protocols)
	return protocols
}

func tlsOuterUint24(data []byte) uint32 {
	if len(data) < 3 {
		panic(fmt.Sprintf("uint24 needs 3 bytes, got %d", len(data)))
	}
	return uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
}

func tlsOuterRequireParseError(t *testing.T, input []byte) {
	t.Helper()
	reader := newTLSOuterRecordReader(input)
	_, err := parser.ParseBinary(reader, "application-layer.tls", "Transport Layer Security")
	require.Error(t, err, "TLS rule accepted a record whose declared bound is unavailable")
}
