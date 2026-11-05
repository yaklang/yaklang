package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func eeRecordFlow(t *testing.T, suite uint16, server int, observedOffer bool) (*binFlow, *binTLS) {
	t.Helper()
	f, s := boundaryTLSFlow(t, suite)
	s.app = [2]bool{}
	s.client = 1 - server
	if observedOffer {
		s.client, s.version = -1, 0
		ext := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}, {16, eeScopeALPN("h2", "http/1.1")}}, tlsResumptionDHExtensions(true)...)
		ch := tlsResumptionHello(true, nil, suite, ext...)
		_, err := s.handshakeMessage(1-server, reliabilityHS(1, ch), false)
		require.NoError(t, err)
		_, err = s.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, suite, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
		require.NoError(t, err)
		require.Nil(t, s.firstHello)
	}
	return f, s
}

func TestTLSEncryptedExtensionsAuthenticatedRecords(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, profile := range []string{"offered", "unoffered", "version-extension", "client-sender", "application-epoch", "fragmented-offered", "fragmented-unoffered", "partial-offer-context"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, profile), func(t *testing.T) {
					f, s := eeRecordFlow(t, suite, server, profile != "partial-offer-context")
					selected := "h2"
					refuse := profile == "unoffered" || profile == "fragmented-unoffered" || profile == "version-extension" || profile == "client-sender" || profile == "application-epoch"
					if profile == "unoffered" || profile == "fragmented-unoffered" {
						selected = "h3"
					}
					extensions := []tlsResumptionExt{{16, eeScopeALPN(selected)}}
					if profile == "version-extension" {
						extensions = append(extensions, tlsResumptionExt{43, []byte{3, 3}})
					}
					dir := server
					if profile == "client-sender" {
						dir = 1 - server
					}
					if profile == "application-epoch" {
						s.app[dir] = true
					}
					reverseKey, reverseSequence := s.keys[1-dir], s.seq[1-dir]
					encoded := reliabilityHS(8, eeScopeBody(extensions...))
					pieces := [][]byte{encoded}
					if profile == "fragmented-offered" || profile == "fragmented-unoffered" {
						pieces = [][]byte{encoded[:7], encoded[7:]}
					}
					for i, piece := range pieces {
						e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, dir, s.seq[dir], piece, 22, 3)}
						e.Length = len(e.Raw)
						sequence := s.seq[dir]
						_, err := f.consumeTLS(dir, e)
						require.Equal(t, true, e.Session["Authentication Verified"])
						require.Equal(t, sequence+1, s.seq[dir], "authentication consumes the record even when message admission refuses it")
						if i < len(pieces)-1 {
							require.NoError(t, err)
							require.Empty(t, s.alpn)
							require.Equal(t, true, e.Session["Handshake Incomplete"])
							continue
						}
						if refuse {
							var typed *ProtocolError
							require.ErrorAs(t, err, &typed)
							require.Equal(t, ErrMalformedMessage, typed.Kind)
							require.Equal(t, uint16(0x304), s.version)
							require.Empty(t, s.alpn)
							require.NotContains(t, e.Session, "Handshake Messages")
						} else {
							require.NoError(t, err)
							require.Equal(t, selected, s.alpn)
							require.Len(t, e.Session["Handshake Messages"], 1)
						}
						require.Nil(t, s.child, "handshake observation does not invent an application message")
					}
					require.Same(t, reverseKey, s.keys[1-dir])
					require.Equal(t, reverseSequence, s.seq[1-dir])
				})
			}
		}
	}
}

func TestTLSEncryptedExtensionsOfferOwnershipAndNewFlow(t *testing.T) {
	state := newBinTLS()
	ext := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}, {16, eeScopeALPN("h2")}}, tlsResumptionDHExtensions(true)...)
	ch := tlsResumptionHello(true, nil, 0x1301, ext...)
	_, err := state.handshakeMessage(0, reliabilityHS(1, ch), false)
	require.NoError(t, err)
	require.True(t, state.alpnOfferKnown)
	copyWithoutOffer := *state
	copyWithoutOffer.alpnOffer = nil
	require.Equal(t, int64(cap(state.alpnOffer)), state.storage()-copyWithoutOffer.storage(), "all retained offer bytes are charged")
	for i := range ch {
		ch[i] ^= 0xff
	}
	require.Equal(t, eeScopeALPN("h2"), state.alpnOffer)
	_, err = state.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, 0x1301, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
	require.NoError(t, err)
	require.Nil(t, state.firstHello)
	before := bytes.Clone(state.alpnOffer)
	_, err = state.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{16, eeScopeALPN("h3")})), false)
	require.Error(t, err)
	require.Equal(t, before, state.alpnOffer)
	other := newBinTLS()
	require.False(t, other.alpnOfferKnown)
	require.Empty(t, other.alpnOffer)
}
