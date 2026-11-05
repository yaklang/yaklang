package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// The model is server acceptance plus exact authenticated request octets;
// neither a header prefix nor key availability can authorize application data.
func FuzzTLSEarlyRecordAcceptanceAndIsolation(f *testing.F) {
	for _, flags := range []uint8{0, 1, 2, 3, 4, 5, 8, 9, 16, 17, 33, 65} {
		f.Add(flags, uint16(17), []byte{1, 3, 5})
	}
	f.Fuzz(func(t *testing.T, flags uint8, split uint16, noise []byte) {
		suite := []uint16{0x1301, 0x1302, 0x1303}[int(flags>>5)%3]
		client := int(flags >> 4 & 1)
		flow, events, keys, feed, seal := tlsEarlyRecordTestFlow(t, suite, client, flags&8 != 0)
		flow.domain = CaptureDomain{Interface: int(flags), Encapsulation: "vlan:fuzz-early"}
		ch, sh := n13AuditHelloPair(suite, true, true, 0)
		request := []byte("GET /early-fuzz HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
		cut := 1 + int(split)%(len(request)-1)
		first := seal("early", client, 0, 23, request[:cut])
		second := seal("early", client, 1, 23, request[cut:])
		bad := flags&2 != 0
		if bad {
			position := len(first) - 1
			if len(noise) > 0 {
				position = 5 + int(noise[0])%(len(first)-5)
			}
			first[position] ^= 1
		}
		eoed := seal("early", client, 2, 22, reliabilityHS(5, nil))
		missing := flags&4 != 0
		if missing {
			delete(keys.values, "CLIENT_EARLY_TRAFFIC_SECRET")
		}
		feed(client, carrierTestRecord(22, reliabilityHS(1, ch)))
		feed(client, first)
		feed(client, second)
		complete, _ := closeNotifyTestHTTP(*events)
		require.Empty(t, complete)
		feed(1-client, carrierTestRecord(22, reliabilityHS(2, sh)))
		accepted := flags&1 != 0
		extensions := eeScopeBody()
		if accepted {
			extensions = eeScopeBody(tlsResumptionExt{42, nil})
		}
		feed(1-client, seal("handshake", 1-client, 0, 22, reliabilityHS(8, extensions)))
		complete, _ = closeNotifyTestHTTP(*events)
		if accepted && !bad && !missing {
			require.Len(t, complete, 1)
			require.Equal(t, request, complete[0].Raw)
		} else {
			require.Empty(t, complete)
		}
		if accepted {
			feed(client, eoed)
		}
		n := 32
		if suite == 0x1302 {
			n = 48
		}
		finished := seal("handshake", client, 0, 22, reliabilityHS(20, bytes.Repeat([]byte{0x37}, n)))
		feed(client, finished)
		for _, e := range *events {
			require.Equal(t, uint64(17), e.TCPFlowID, "all observations keep this physical TCP generation")
			if e.Protocol == "http" && (e.Status == "decoded" || e.Status == "deferred") {
				require.True(t, accepted && !bad && !missing, "no request can be reconstructed across unaccepted or missing early bytes")
				require.Equal(t, request, e.Raw)
				require.Equal(t, flow.domain, e.Domain)
				require.Equal(t, "decrypted", e.SourceBytes.Kind)
			}
			if e.Protocol == "tls" && bytes.Equal(e.Raw, finished) {
				require.Empty(t, e.Error, "early traffic cannot consume a valid handshake sequence zero")
			}
		}
		flow.closeSession()
		require.Zero(t, flow.a.stats().BufferedBytes)
	})
}
