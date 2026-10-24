package bin_parser

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"strings"
	"testing"
)

func ssdpFixtures() []string {
	common := "HOST: 239.255.255.250:1900\r\nNT: urn:schemas-upnp-org:device:MediaServer:1\r\nUSN: uuid:device-1::urn:schemas-upnp-org:device:MediaServer:1\r\n"
	alive := "CACHE-CONTROL: max-age=66\r\nLOCATION: http://192.0.2.10:49152/description.xml\r\nSERVER: Windows/10 UPnP/1.1 Audit/1.0\r\n"
	return []string{
		"NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:alive\r\n" + alive + "\r\n",
		"NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:byebye\r\n\r\n",
		"NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:update\r\nLOCATION: http://192.0.2.10/desc.xml\r\nBOOTID.UPNP.ORG: 7\r\nNEXTBOOTID.UPNP.ORG: 8\r\nCONFIGID.UPNP.ORG: 23\r\n\r\n",
		"M-SEARCH * HTTP/1.1\r\nHOST: [ff02::c]:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: ssdp:all\r\n\r\n",
		"M-SEARCH * HTTP/1.1\r\nHOST: 192.0.2.10:1900\r\nMAN: \"ssdp:discover\"\r\nST: uuid:device-1\r\n\r\n",
		"HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nUSN: uuid:device-1::upnp:rootdevice\r\nEXT:\r\n" + alive + "\r\n",
	}
}

func TestSSDPExactDiscoveryRuleAndNativeParity(t *testing.T) {
	for _, wire := range ssdpFixtures() {
		for _, w := range []string{wire, strings.ReplaceAll(strings.ReplaceAll(wire, "HOST:", "hOsT:"), "NTS:", "nts:")} {
			n := protocolCorpusRequireBoundedRuleParse(t, []byte(w), "application-layer.ssdp", "SSDP")
			require.NotNil(t, n)
			got, err := ParseStructured([]byte(w), "application-layer.ssdp", "SSDP")
			require.NoError(t, err)
			require.NotEmpty(t, got["fields"])
			info, err := stream_parser.DecodeSSDPMessage([]byte(w))
			require.NoError(t, err)
			require.Equal(t, false, info["Identity Verified"])
			if strings.Contains(w, "ssdp:byebye") {
				require.NotContains(t, info, "Location")
			}
			for at := 0; at < len(w); at++ {
				_, err = stream_parser.DecodeSSDPMessage([]byte(w[:at]))
				require.Error(t, err, "cut %d", at)
			}
		}
	}
	// Extension repetition is legal and retained in wire order.
	w := strings.Replace(ssdpFixtures()[0], "\r\n\r\n", "\r\nX-Vendor: a\r\nX-Vendor: b\r\n\r\n", 1)
	f, err := stream_parser.DecodeSSDPMessage([]byte(w))
	require.NoError(t, err)
	hs := f["Headers"].([]map[string]any)
	require.Equal(t, "a", hs[len(hs)-2]["Value"])
	require.Equal(t, "b", hs[len(hs)-1]["Value"])
}

func TestSSDPDoesNotAcceptPartialMagicOrConflictingIdentity(t *testing.T) {
	w := ssdpFixtures()[0]
	for _, bad := range []string{
		"NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n",
		strings.Replace(w, "NTS: ssdp:alive", "NTS: ssdp:bogus", 1),
		strings.Replace(w, "USN: uuid:device-1::urn:", "USN: uuid:device-1::wrong:", 1),
		strings.Replace(w, "\r\n\r\n", "\r\nUSN: uuid:device-2\r\n\r\n", 1),
		strings.Replace(w, "max-age=66", "max-age=66, max-age=77", 1),
		strings.Replace(w, "max-age=66", "max-age=\"broken", 1),
		strings.Replace(w, "\r\n\r\n", "\r\nTransfer-Encoding: chunked\r\n\r\n", 1),
		strings.Replace(w, "\r\n\r\n", "\r\nContent-Length: 1\r\n\r\n", 1),
		strings.Replace(w, "SERVER:", "BAD NAME:", 1),
		strings.Replace(w, "max-age=66", "max-age=6\x006", 1), w + "body", w + w,
		strings.ReplaceAll(w, "\r\n", "\n"), string(bytes.Repeat([]byte{'A'}, 65508)),
	} {
		_, err := stream_parser.DecodeSSDPMessage([]byte(bad))
		require.Error(t, err, "%q", bad[:min(60, len(bad))])
	}
}

func TestSSDPObservedAndroidAllTargetIsIdentifiedWithConformanceWarning(t *testing.T) {
	w := strings.ReplaceAll(ssdpFixtures()[0], "urn:schemas-upnp-org:device:MediaServer:1", "ssdp:all")
	f, err := stream_parser.DecodeSSDPMessage([]byte(w))
	require.NoError(t, err)
	require.NotEmpty(t, f["Conformance Warnings"])
	require.Equal(t, "ssdp:all", f["Discovery Target"])
	require.Equal(t, false, f["Full UDA Conformance Validated"])
}
